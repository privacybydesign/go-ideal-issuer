package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/aykevl/go-idx"
)

var (
	iDealIssuersJSON []byte
	banksLock        sync.Mutex
)

func sendErrorResponse(w http.ResponseWriter, httpCode int, errorCode string) {
	w.WriteHeader(httpCode)
	w.Write([]byte("error:" + errorCode))
}

func cmdServe() {
	if config.StaticDir != "" {
		log.Println("serving static files from:", config.StaticDir)
		static := http.FileServer(http.Dir(config.StaticDir))
		http.Handle("/", static)
	} else {
		log.Println("not serving static files, set -static flag or configure static_dir to enable")
	}

	cert, err := readCertificate(filepath.Join(configDir, config.IDealMerchantCert))
	if err != nil {
		log.Fatal(err)
	}
	sk, err := readPrivateKey(filepath.Join(configDir, config.IDealMerchantSk))
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
				ReturnURL:  config.ServerURL + config.IDealPathPrefix + "redirect",
				Certificate: tls.Certificate{
					Certificate: [][]byte{cert.Raw},
					PrivateKey:  sk,
				},
				AcquirerCert: iDealAcquirerCert,
			},
		}

		// iDeal routes
		http.HandleFunc(config.IDealPathPrefix+"banks", apiIDealIssuers)
		http.HandleFunc(config.IDealPathPrefix+"start", func(w http.ResponseWriter, r *http.Request) {
			apiIDealStart(w, r, ideal, false)
		})
		http.HandleFunc(config.IDealPathPrefix+"start-donation", func(w http.ResponseWriter, r *http.Request) {
			apiIDealStart(w, r, ideal, true)
		})
		http.HandleFunc(config.IDealPathPrefix+"return", func(w http.ResponseWriter, r *http.Request) {
			apiIDealReturn(w, r, ideal)
		})
		http.HandleFunc(config.IDealPathPrefix+"redirect", apiIdealRedirect)
		http.HandleFunc(config.IDealPathPrefix+"delete", apiIdealDelete)

		// Route to retrieve allowed payment amounts
		http.HandleFunc(config.IDealPathPrefix+"amounts", apiPaymentAmounts)

		// Start updating the banks list in the background.
		go backgroundUpdateIssuers("iDeal", &iDealIssuersJSON, ideal)

		// Start auto-closing transactions in the background.
		go idealAutoCloseTransactions(ideal)
	}

	log.Println("serving from", config.ServerAddress)

	if config.EnableTLS {
		certFilePath := filepath.Join(configDir, config.TLSCertificate)
		keyFilePath := filepath.Join(configDir, config.TLSPrivateKey)
		err = http.ListenAndServeTLS(config.ServerAddress, certFilePath, keyFilePath, nil)
	} else {
		err = http.ListenAndServe(config.ServerAddress, nil)
	}
	if err != nil {
		log.Fatal("could not start server: ", err)
	}
}
