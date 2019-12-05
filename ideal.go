package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"github.com/privacybydesign/irmago/server"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/aykevl/go-idx"
	"github.com/privacybydesign/irmago"
)

// Global variable to take account of the state with open and pending transactions
var transactionState sync.Map

// Constants that determine how often transactions are checked
const (
	firstCheckAfter          = 12 * time.Hour
	recheckAfter             = 24 * time.Hour
	saveSucceededTransaction = time.Hour
	maxTransactionAge        = 7 * 24 * time.Hour
	tickInterval             = time.Second
)

// Struct with information that should be stored in the state
type IDealTransactionData struct {
	transactionID string
	entranceCode  string
	started       time.Time
	recheckAfter  time.Time
	status        *idx.IDealTransactionStatus
}

func apiIDealIssuers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Atomically get the JSON.
	banksLock.Lock()
	data := iDealIssuersJSON
	banksLock.Unlock()

	if len(data) == 0 {
		sendErrorResponse(w, 404, "no-issuers-loaded")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func generateRandomAlphNumString(strLengthAim int) (string, error) {
	b := make([]byte, (strLengthAim*6)/8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	str := base64.StdEncoding.EncodeToString(b)

	r := strings.NewReplacer("+", "", "/", "", "=", "")
	return r.Replace(str), nil
}

func apiIDealStart(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}
	bank := r.FormValue("bank")
	ec, err := generateRandomAlphNumString(40)
	pid, err := generateRandomAlphNumString(10)
	if err != nil {
		log.Println("failed to generate fresh ec:", err)
		sendErrorResponse(w, 500, "no-ec")
		return
	}
	transaction := ideal.NewTransaction(bank, pid, config.PaymentAmount, config.PaymentMessage, ec)
	err = transaction.Start()
	if err != nil {
		log.Println("failed to create transaction:", err)
		sendErrorResponse(w, 500, "transaction")
		return
	}
	addTransactionToState(transaction.TransactionID(), ec)
	log.Printf("transaction %s started", transaction.TransactionID())
	w.Write([]byte(transaction.IssuerAuthenticationURL()))
}

func apiIDealReturn(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}
	trxid := r.FormValue("trxid")
	ec := r.FormValue("ec")

	// Look up transaction in state
	v, ok := transactionState.Load(trxid)
	if !ok {
		sendErrorResponse(w, 404, "trxid-not-found")
		log.Println("trying to request api return result of an already-closed transaction:", trxid)
		return
	}
	transaction := v.(*IDealTransactionData)

	// Check ec
	if transaction.entranceCode != ec {
		sendErrorResponse(w, 403, "ec-mismatch")
		log.Printf("trying to retrieve result of transaction %s with ec %s, while actual ec is %s", trxid, ec, transaction.entranceCode)
		return
	}

	if transaction.status == nil || transaction.status.Status != idx.Success {
		response, err := ideal.TransactionStatus(trxid)
		if err != nil {
			sendErrorResponse(w, 500, "transaction")
			log.Println("failed to request transaction status:", err)
			return
		}
		transaction.status = response

		log.Printf("transaction %s has status %s on return", trxid, response.Status)
		switch response.Status {
		case idx.Success:
			break
		case idx.Open:
			transaction.recheckAfter = time.Now().Add(recheckAfter)
			sendErrorResponse(w, 500, "transaction-open")
			return
		case idx.Cancelled:
			sendErrorResponse(w, 500, "transaction-cancelled")
			return
		case idx.Expired:
			sendErrorResponse(w, 500, "transaction-expired")
			return
		default:
			transaction.recheckAfter = time.Now().Add(recheckAfter)
			sendErrorResponse(w, 500, "transaction")
			return
		}
		// Save transaction for some time in case IRMA session fails and user wants to start it again without having to pay again
		transaction.recheckAfter = time.Now().Add(saveSucceededTransaction)
	}

	attributes := map[string]string{
		"fullname": transaction.status.ConsumerName,
		"iban":     transaction.status.ConsumerIBAN,
		"bic":      transaction.status.ConsumerBIC,
	}

	// Start IRMA session to issue iDeal credential
	validity := irma.Timestamp(irma.FloorToEpochBoundary(time.Now().AddDate(1, 0, 0)))
	credid := irma.NewCredentialTypeIdentifier(config.IDealCredentialID)

	credentials := []*irma.CredentialRequest{
		{
			Validity:         &validity,
			CredentialTypeID: credid,
			Attributes:       attributes,
		},
	}
	request := irma.NewIssuanceRequest(credentials)

	sessionPointer, token, err := postIrmaRequest(request)
	if err != nil {
		log.Println("cannot start IRMA session:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	err = encoder.Encode(struct {
		SessionPointer *irma.Qr `json:"sessionPointer"`
		Token          string   `json:"token"`
	}{
		SessionPointer: sessionPointer,
		Token:          token,
	})
	if err != nil {
		log.Println("ideal: cannot encode JSON and send response:", err)
	}
}

func postIrmaRequest(request irma.SessionRequest) (qr *irma.Qr, token string, err error) {
	pkg := &server.SessionPackage{}

	transport := irma.NewHTTPTransport(config.IrmaServerURL)
	transport.SetHeader("Authorization", config.IrmaServerToken)
	err = transport.Post("session", pkg, request)

	return pkg.SessionPtr, pkg.Token, err
}

func addTransactionToState(trxid string, ec string) {
	tdata := IDealTransactionData{
		transactionID: trxid,
		entranceCode:  ec,
		started:       time.Now(),
		recheckAfter:  time.Now().Add(firstCheckAfter),
	}
	transactionState.Store(trxid, &tdata)
}

func idealAutoCloseTransactions(ideal *idx.IDealClient) {
	ticker := time.Tick(tickInterval)
	for range ticker {
		now := time.Now()
		transactionState.Range(func(key interface{}, value interface{}) bool {
			transaction := value.(*IDealTransactionData)

			if transaction.status != nil {
				if transaction.status.Status != idx.Open {
					if time.Now().After(transaction.recheckAfter) {
						log.Printf("succeeded transaction %s was closed", transaction.transactionID)
						transactionState.Delete(key)
					}
					return true
				} else if transaction.started.Add(maxTransactionAge).Before(time.Now()) {
					log.Printf("transaction %s reached its maximum age without any status change, closing", transaction.transactionID)
					transactionState.Delete(key)
					return true
				}
			}

			if transaction.recheckAfter.Before(now) {
				status, err := ideal.TransactionStatus(transaction.transactionID)
				transaction.status = status
				if err != nil {
					log.Printf("transaction %s status could not be requested, retrying in %s: %s", transaction.transactionID, recheckAfter, err)
					transaction.recheckAfter = time.Now().Add(recheckAfter)
				} else if status.Status == idx.Open {
					log.Printf("transaction %s is still not closed, retrying in %s", transaction.transactionID, recheckAfter)
					transaction.recheckAfter = time.Now().Add(recheckAfter)
				} else if status.Status == idx.Success {
					transaction.recheckAfter = time.Now().Add(saveSucceededTransaction)
					log.Printf("transaction %s succeeded but user has not started IRMA issuance yet, transaction data stored at max until %s", transaction.transactionID, transaction.recheckAfter)
				} else {
					log.Printf("transaction %s was closed with status %s", transaction.transactionID, status.Status)
					transactionState.Delete(key)
				}
			}
			return true
		})
	}
}
