package main

import (
	"log"
	"net/http"

	"github.com/aykevl/go-idx"
	"github.com/privacybydesign/irmago"
)

const IDINAttributes = idx.IDINServiceIDAddress | idx.IDINServiceIDDateOfBirth | idx.IDINServiceIDGender | idx.IDINServiceIDName

func apiIDINIssuers(w http.ResponseWriter, r *http.Request) {
	// Atomically get the JSON.
	banksLock.Lock()
	data := iDINIssuersJSON
	banksLock.Unlock()

	if len(data) == 0 {
		sendErrorResponse(w, 404, "no-issuers-loaded")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(data)
}

func apiIDINStart(w http.ResponseWriter, r *http.Request, idin *idx.IDINClient) {
	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}
	bank := r.PostFormValue("bank")
	transaction := idin.NewTransaction(bank, "idin", "fakeid", IDINAttributes)
	err := transaction.Start()
	if err != nil {
		log.Println("failed to create transaction:", err)
		sendErrorResponse(w, 500, "transaction")
		return
	}
	w.Write([]byte(transaction.IssuerAuthenticationURL()))
}

func apiIDINReturn(w http.ResponseWriter, r *http.Request, idin *idx.IDINClient) {
	if err := r.ParseForm(); err != nil {
		sendErrorResponse(w, 400, "no-params")
		return
	}
	trxid := r.PostFormValue("trxid")
	response, err := idin.TransactionStatus(trxid)
	if err != nil {
		log.Println("failed to request transaction status:", err)
		sendErrorResponse(w, 500, "transaction")
		return
	}

	attributes := map[string]string{
		"initials":    response.Attributes["urn:nl:bvn:bankid:1.0:consumer.initials"],
		"familyname":  response.Attributes["urn:nl:bvn:bankid:1.0:consumer.legallastname"],
		"prefix":      response.Attributes["urn:nl:bvn:bankid:1.0:consumer.legallastnameprefix"],
		"dateofbirth": samlMapDateOfBirth(response.Attributes["urn:nl:bvn:bankid:1.0:consumer.dateofbirth"]),
		"gender":      samlMapGender(response.Attributes["urn:nl:bvn:bankid:1.0:consumer.gender"]),
		"address":     response.Attributes["urn:nl:bvn:bankid:1.0:consumer.street"] + " " + response.Attributes["urn:nl:bvn:bankid:1.0:consumer.houseno"],
		"zipcode":     response.Attributes["urn:nl:bvn:bankid:1.0:consumer.postalcode"],
		"city":        response.Attributes["urn:nl:bvn:bankid:1.0:consumer.city"],
		"country":     response.Attributes["urn:nl:bvn:bankid:1.0:consumer.country"],
	}

	disjunction := irma.AttributeDisjunctionList{
		&irma.AttributeDisjunction{
			Values: map[irma.AttributeTypeIdentifier]*string{},
		},
	}

	for key, value := range attributes {
		value := value // this is necessary
		id := irma.NewAttributeTypeIdentifier(config.IDINCredentialID + "." + key)
		disjunction[0].Attributes = append(disjunction[0].Attributes, irma.NewAttributeTypeIdentifier(config.IDINCredentialID+"."+key))
		disjunction[0].Values[id] = &value
	}

	// TODO: cache, or load on startup
	sk, err := readPrivateKey(configDir + "/sk.der")
	if err != nil {
		log.Println("cannot open private key:", err)
		sendErrorResponse(w, 500, "signing")
		return
	}

	jwt := irma.NewSignatureRequestorJwt("Privacy by Design Foundation", &irma.SignatureRequest{
		Message: "Bank attributes from iDIN",
		DisclosureRequest: irma.DisclosureRequest{
			Content: disjunction,
		},
	})
	text, err := jwt.Sign(config.IDINServerName, sk)
	if err != nil {
		log.Println("cannot sign signature request:", err)
		sendErrorResponse(w, 500, "signing")
		return
	}

	w.Write([]byte(text))
}

func samlMapDateOfBirth(dateofbirth string) string {
	return dateofbirth[6:8] + "-" + dateofbirth[4:6] + "-" + dateofbirth[:4]
}

func samlMapGender(isocode string) string {
	switch isocode {
	case "0":
		return "unknown"
	case "1":
		return "male"
	case "2":
		return "female"
	case "9":
		return "not applicable"
	default:
		return ""
	}
}
