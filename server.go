package main

import (
	"crypto/tls"
	"github.com/privacybydesign/irmago/server"
	"github.com/privacybydesign/irmago/server/irmaserver"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/aykevl/go-idx"
)

var (
	iDealIssuersJSON []byte
	iDINIssuersJSON  []byte
	banksLock        sync.Mutex
)

func sendErrorResponse(w http.ResponseWriter, httpCode int, errorCode string) {
	w.WriteHeader(httpCode)
	w.Write([]byte("error:" + errorCode))
}

func startIrmaServer() {
	configuration := &server.Configuration{
		// Replace with address that IRMA apps can reach
		URL:                   "http://localhost:4242/irma",
		IssuerPrivateKeysPath: "config",
		Verbose:               2,
	}

	err := irmaserver.Initialize(configuration)
	if err != nil {
		log.Println("IRMA server could not be started:", err)
	}
	http.Handle("/irma/", irmaserver.HandlerFunc())
}

func cmdServe(addr string) {
	if config.StaticDir != "" {
		log.Println("serving static files from:", config.StaticDir)
		static := http.FileServer(http.Dir(config.StaticDir))
		http.Handle("/", static)
	} else {
		log.Println("not serving static files, set -static flag or configure static_dir to enable")
	}

	cert, err := readFile(configDir + "/ideal-cert.der")
	if err != nil {
		log.Fatal(err)
	}
	sk, err := readPrivateKey(configDir + "/ideal-sk.der")
	if err != nil {
		log.Fatal(err)
	}

	if config.EnableIDeal {
		log.Println("enabling iDeal")

		iDealAcquirerCert, err := readCertificate(filepath.Join(configDir, config.IDealAcquirerCert))
		if err != nil {
			log.Fatal(err)
		}

		ideal := &idx.IDealClient{
			CommonClient: idx.CommonClient{
				BaseURL:    config.IDealBaseURL,
				MerchantID: config.IDealMerchantID,
				SubID:      config.IDealSubID,
				ReturnURL:  config.IDealReturnURL,
				Certificate: tls.Certificate{
					Certificate: [][]byte{cert},
					PrivateKey:  sk,
				},
				AcquirerCert: iDealAcquirerCert,
			},
		}

		// Channels to send transactions over that must be closed in 24 hour, or
		// removed from this list before this time.
		trxidAddChan := make(chan string)
		trxidRemoveChan := make(chan string)

		// Start IRMA server
		startIrmaServer()

		// iDeal routes
		http.HandleFunc(config.IDealPathPrefix+"banks", apiIDealIssuers)
		http.HandleFunc(config.IDealPathPrefix+"start", func(w http.ResponseWriter, r *http.Request) {
			apiIDealStart(w, r, ideal, trxidAddChan)
		})
		http.HandleFunc(config.IDealPathPrefix+"return", func(w http.ResponseWriter, r *http.Request) {
			apiIDealReturn(w, r, ideal, trxidRemoveChan)
		})

		// Start updating the banks list in the background.
		go backgroundUpdateIssuers("iDeal", &iDealIssuersJSON, ideal)

		// Start auto-closing transactions in the background.
		go idealAutoCloseTransactions(ideal, trxidAddChan, trxidRemoveChan)
	}

	log.Println("serving from", addr)
	err = http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("could not start server: ", err)
	}
}
