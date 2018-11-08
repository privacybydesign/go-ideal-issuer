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
	iDINIssuersJSON  []byte
	banksLock        sync.Mutex
)

func sendErrorResponse(w http.ResponseWriter, httpCode int, errorCode string) {
	w.WriteHeader(httpCode)
	w.Write([]byte("error:" + errorCode))
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

		// iDeal routes
		http.HandleFunc("/irma_ideal_server/api/v1/ideal/banks", apiIDealIssuers)
		http.HandleFunc("/irma_ideal_server/api/v1/ideal/start", func(w http.ResponseWriter, r *http.Request) {
			apiIDealStart(w, r, ideal)
		})
		http.HandleFunc("/irma_ideal_server/api/v1/ideal/return", func(w http.ResponseWriter, r *http.Request) {
			apiIDealReturn(w, r, ideal)
		})

		// Start updating the banks list in the background.
		go backgroundUpdateIssuers("iDeal", &iDealIssuersJSON, ideal)
	}

	if config.EnableIDIN {
		log.Println("enabling iDIN")

		iDINAcquirerCert, err := readCertificate(filepath.Join(configDir + config.IDINAcquirerCert))
		if err != nil {
			log.Fatal(err)
		}

		idin := &idx.IDINClient{
			CommonClient: idx.CommonClient{
				BaseURL:    config.IDINBaseURL,
				MerchantID: config.IDINMerchantID,
				SubID:      config.IDINSubID,
				ReturnURL:  config.IDINReturnURL,
				Certificate: tls.Certificate{
					Certificate: [][]byte{cert},
					PrivateKey:  sk,
				},
				AcquirerCert: iDINAcquirerCert,
			},
		}

		// iDIN routes
		http.HandleFunc("/irma_ideal_server/api/v1/idin/banks", apiIDINIssuers)
		http.HandleFunc("/irma_ideal_server/api/v1/idin/start", func(w http.ResponseWriter, r *http.Request) {
			apiIDINStart(w, r, idin)
		})
		http.HandleFunc("/irma_ideal_server/api/v1/idin/return", func(w http.ResponseWriter, r *http.Request) {
			apiIDINReturn(w, r, idin)
		})

		// Start updating the banks list in the background.
		go backgroundUpdateIssuers("iDIN", &iDINIssuersJSON, idin)
	}

	log.Println("serving from", addr)
	http.ListenAndServe(addr, nil)
}
