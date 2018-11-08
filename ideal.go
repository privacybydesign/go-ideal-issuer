package main

import (
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

func apiIDealStart(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient) {
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
	w.Write([]byte(transaction.IssuerAuthenticationURL()))
}

func apiIDealReturn(w http.ResponseWriter, r *http.Request, ideal *idx.IDealClient) {
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
	text, err := jwt.Sign("ideal_server", sk)
	if err != nil {
		log.Println("cannot sign signature request:", err)
		sendErrorResponse(w, 500, "signing")
		return
	}

	w.Write([]byte(text))
}
