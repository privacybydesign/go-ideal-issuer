package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/aykevl/go-idx"
	"github.com/privacybydesign/irmago"
)

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

func apiIDealStart(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient, trxidAddChan chan string) {
	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}
	bank := r.PostFormValue("bank")
	transaction := ideal.NewTransaction(bank, "1", config.PaymentAmount, config.PaymentMessage, "ideal")
	err := transaction.Start()
	if err != nil {
		log.Println("failed to create transaction:", err)
		sendErrorResponse(w, 500, "transaction")
		return
	}
	trxidAddChan <- transaction.TransactionID() // auto-close transaction
	w.Write([]byte(transaction.IssuerAuthenticationURL()))
}

func apiIDealReturn(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient, trxidRemoveChan chan string) {
	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}
	trxid := r.PostFormValue("trxid")
	response, err := ideal.TransactionStatus(trxid)
	if err != nil {
		sendErrorResponse(w, 500, "transaction")
		log.Println("failed to request transaction status:", err)
		return
	}

	// Remove this transaction from the list of transactions to auto-close.
	if response.Status != idx.Open {
		// Transaction was closed.
		// Remove this transaction from the list of transactions to auto-close.
		trxidRemoveChan <- trxid
	}
	if response.Status != idx.Success {
		// Expected a success response here.
		sendErrorResponse(w, 500, "transaction")
		log.Println("transaction %s has status %s on return", trxid, response.Status)
		return
	}

	attributes := map[string]string{
		"fullname": response.ConsumerName,
		"iban":     response.ConsumerIBAN,
		"bic":      response.ConsumerBIC,
	}

	validity := irma.Timestamp(irma.FloorToEpochBoundary(time.Now().AddDate(1, 0, 0)))
	credid := irma.NewCredentialTypeIdentifier(config.IDealCredentialID)
	credentials := []*irma.CredentialRequest{
		&irma.CredentialRequest{
			Validity:         &validity,
			CredentialTypeID: &credid,
			Attributes:       attributes,
		},
	}

	// TODO: cache, or load on startup
	sk, err := readPrivateKey(configDir + "/sk.der")
	if err != nil {
		log.Println("cannot open private key:", err)
		sendErrorResponse(w, 500, "signing")
		return
	}

	jwt := irma.NewIdentityProviderJwt("Privacy by Design Foundation", &irma.IssuanceRequest{
		Credentials: credentials,
	})
	text, err := jwt.Sign(config.IDealServerName, sk)
	if err != nil {
		log.Println("cannot sign signature request:", err)
		sendErrorResponse(w, 500, "signing")
		return
	}

	rawToken := makeToken(response.ConsumerBIC, response.ConsumerIBAN)
	token := base64.URLEncoding.EncodeToString(rawToken)

	// Save the token (hashed) to the database, to prevent timing attacks on the
	// database on retrieval.
	_, err = tokenDB.Exec("INSERT INTO idin_tokens (hashedToken) VALUES (?)", hashToken(rawToken))
	if err != nil {
		log.Fatal("failed to insert token into database:", err)
		// unreachable
		sendErrorResponse(w, 500, "signing")
		return
	}

	rawSignature := signToken(rawToken)
	signature := base64.URLEncoding.EncodeToString(rawSignature)
	signedToken := token + ":" + signature

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	err = encoder.Encode(struct {
		JWT   string `json:"jwt"`
		Token string `json:"token"`
	}{
		JWT:   text,
		Token: signedToken,
	})
	if err != nil {
		log.Println("ideal: cannot encode JSON and send response:", err)
	}
}

func idealAutoCloseTransactions(ideal *idx.IDealClient, trxidAddChan, trxidRemoveChan chan string) {
	const closeAfter = 24 * time.Hour
	const tickInterval = time.Hour

	ticker := time.Tick(tickInterval)
	transactions := make(map[string]time.Time)
	for {
		select {
		case trxid := <-trxidAddChan:
			transactions[trxid] = time.Now()
		case trxid := <-trxidRemoveChan:
			if _, ok := transactions[trxid]; ok {
				delete(transactions, trxid)
			} else {
				log.Println("trying to close an already-closed transaction:", trxid)
			}
		case <-ticker:
			now := time.Now()
			for trxid, created := range transactions {
				if created.Before(now.Add(-closeAfter)) {
					delete(transactions, trxid)

					// If this transaction is still not closed, re-add it here.
					status, err := ideal.TransactionStatus(trxid)
					if err != nil {
						log.Printf("transaction %s status could not be requested, retrying in %s: %s", trxid, closeAfter, err)
						transactions[trxid] = time.Now()
					} else if status.Status == idx.Open {
						log.Printf("transaction %s is still not closed, retrying in %s", trxid, closeAfter)
						transactions[trxid] = time.Now()
					} else {
						log.Printf("transaction %s was closed with status %s", trxid, status.Status)
					}
				}
			}
		}
	}
}
