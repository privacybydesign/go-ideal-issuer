package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/privacybydesign/irmago/server"

	"github.com/aykevl/go-idx"
	irma "github.com/privacybydesign/irmago"
)

// Global variable to take account of the state with open and pending transactions
var transactionState sync.Map

// Constants that determine how often transactions are checked
const (
	firstCheckAfter          = 12 * time.Hour
	recheckAfter             = 24 * time.Hour
	saveSucceededTransaction = 48 * time.Hour
	saveReturnedTransaction  = time.Hour
	maxTransactionAge        = 7 * 24 * time.Hour
	tickInterval             = time.Second
	minimumCheckInterval     = 10 * time.Minute
)

// Struct with information that should be stored in the state
type IDealTransactionData struct {
	transactionID string
	entranceCode  string
	donationOnly  bool
	started       time.Time
	recheckAfter  time.Time
	statusChecked time.Time
	status        *idx.IDealTransactionStatus
}

func apiIDealIssuers(w http.ResponseWriter, r *http.Request) {
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

func apiPaymentAmounts(w http.ResponseWriter, r *http.Request) {
	b, err := json.Marshal(config.PaymentAmounts)
	if err != nil {
		sendErrorResponse(w, 500, "payment-amounts-cannot-marshal")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(b)
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

func amountAllowed(paymentAmount string) bool {
	for _, amount := range config.PaymentAmounts {
		if paymentAmount == amount {
			return true
		}
	}
	return false
}

func apiIDealStart(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient, donationOnly bool) {
	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}

	paymentAmount := r.FormValue("amount")
	if !amountAllowed(paymentAmount) {
		log.Println("requested payment amount is not allowed", paymentAmount)
		sendErrorResponse(w, 400, "invalid-amount")
		return
	}

	bank := r.FormValue("bank")
	ec, err := generateRandomAlphNumString(40)
	if err != nil {
		log.Println("failed to generate fresh entranceCode:", err)
		sendErrorResponse(w, 500, "no-ec")
		return
	}

	pid, err := generateRandomAlphNumString(10)
	if err != nil {
		log.Println("failed to generate fresh purchaseID:", err)
		sendErrorResponse(w, 500, "no-pid")
		return
	}

	paymentMessage := config.AuthenticationPaymentMessage
	if donationOnly {
		paymentMessage = config.DonationPaymentMessage
	}

	transaction := ideal.NewTransaction(bank, pid, paymentAmount, paymentMessage, ec)
	err = transaction.Start()
	if err != nil {
		log.Println("failed to create transaction:", err)
		sendErrorResponse(w, 500, "transaction")
		return
	}
	addTransactionToState(transaction.TransactionID(), ec, donationOnly)
	log.Printf("transaction %s started", transaction.TransactionID())
	w.Write([]byte(transaction.IssuerAuthenticationURL()))
}

func validateTransaction(w http.ResponseWriter, r *http.Request) *IDealTransactionData {
	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return nil
	}
	trxid := r.FormValue("trxid")
	ec := r.FormValue("ec")

	// Look up transaction in state
	v, ok := transactionState.Load(trxid)
	if !ok {
		sendErrorResponse(w, 404, "trxid-not-found")
		log.Println("trying to request api return result of an already-closed transaction:", trxid)
		return nil
	}
	transaction := v.(*IDealTransactionData)

	// Check ec
	if transaction.entranceCode != ec {
		sendErrorResponse(w, 403, "ec-mismatch")
		log.Printf("trying to retrieve result of transaction %s with ec %s, while actual ec is %s", trxid, ec, transaction.entranceCode)
		return nil
	}

	return transaction
}

func apiIDealReturn(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient) {
	transaction := validateTransaction(w, r)
	if transaction == nil {
		return
	}

	if !transaction.finished() {
		retryAt := transaction.statusChecked.Add(minimumCheckInterval)
		now := time.Now()
		if retryAt.After(now) {
			seconds := int(math.Ceil(retryAt.Sub(now).Seconds()))
			log.Printf("rate limiting transaction %s on return for %d seconds", transaction.transactionID, seconds)
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			sendErrorResponse(w, http.StatusTooManyRequests, "too-many-requests")
			return
		}

		response, err := ideal.TransactionStatus(transaction.transactionID)
		transaction.statusChecked = now
		if err != nil {
			sendErrorResponse(w, 500, "transaction")
			log.Println("failed to request transaction status:", err)
			return
		}
		transaction.status = response
	}

	log.Printf("transaction %s has status %s on return", transaction.transactionID, transaction.status.Status)
	switch transaction.status.Status {
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
	transaction.recheckAfter = time.Now().Add(saveReturnedTransaction)

	if transaction.donationOnly {
		w.WriteHeader(http.StatusNoContent)
		return
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

	sessionPkg, err := postIrmaRequest(request)
	if err != nil {
		log.Println("cannot start IRMA session:", err)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	err = encoder.Encode(&sessionPkg)
	if err != nil {
		log.Println("ideal: cannot encode JSON and send response:", err)
	}
}

func apiIdealRedirect(w http.ResponseWriter, r *http.Request) {
	trxid := r.URL.Query().Get("trxid")
	v, ok := transactionState.Load(trxid)
	if !ok {
		sendErrorResponse(w, 404, "trxid-not-found")
		log.Println("trying to request api redirect of an already-closed transaction:", trxid)
		return
	}
	transaction := v.(*IDealTransactionData)

	returnUrl := config.AuthenticationReturnURL
	if transaction.donationOnly {
		returnUrl = config.DonationReturnURL
	}
	url, err := url.Parse(returnUrl)
	if err != nil {
		sendErrorResponse(w, 500, "")
		log.Printf("invalid return url %s is set for redirect", returnUrl)
		return
	}
	url.RawQuery = r.URL.RawQuery
	log.Printf("redirect ideal return of transaction %s, donation only is %t", trxid, transaction.donationOnly)
	http.Redirect(w, r, url.String(), http.StatusFound)
}

func apiIdealDelete(w http.ResponseWriter, r *http.Request) {
	transaction := validateTransaction(w, r)
	if transaction == nil {
		return
	}

	if transaction.finished() {
		log.Printf("transaction %s deleted by user", transaction.transactionID)
		transactionState.Delete(transaction.transactionID)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	sendErrorResponse(w, 403, "transaction-not-finished")
	log.Printf("transaction %s is not fully handled by bank, so cannot be deleted yet", transaction.transactionID)
}

func postIrmaRequest(request irma.SessionRequest) (server.SessionPackage, error) {
	pkg := server.SessionPackage{}
	transport := irma.NewHTTPTransport(config.IrmaServerURL, false)
	transport.SetHeader("Authorization", config.IrmaServerToken)
	return pkg, transport.Post("session", &pkg, request)
}

func addTransactionToState(trxid string, ec string, donationOnly bool) {
	tdata := IDealTransactionData{
		transactionID: trxid,
		entranceCode:  ec,
		donationOnly:  donationOnly,
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
				transaction.statusChecked = now
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

func (t *IDealTransactionData) finished() bool {
	if t.status == nil {
		return false
	}
	finished := []idx.TransactionStatus{idx.Success, idx.Cancelled, idx.Expired}
	for _, s := range finished {
		if t.status.Status == s {
			return true
		}
	}
	return false
}
