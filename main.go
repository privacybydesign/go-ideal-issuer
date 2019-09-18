package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

// Flags parsed at program startup and never modified afterwards.
var configDir string

type Config struct {
	StaticDir      string `json:"static_dir"`
	EnableTLS      bool   `json:"enable_tls"`
	TLSCertificate string `json:"tls_certificate"`
	TLSPrivateKey  string `json:"tls_private_key"`

	EnableIDeal       bool   `json:"enable_ideal"`
	IDealPathPrefix   string `json:"ideal_path_prefix"`
	IDealAcquirerCert string `json:"ideal_acquirer_cert"`
	IDealCredentialID string `json:"ideal_credential_id"`
	IDealBaseURL      string `json:"ideal_base_url"`
	IDealMerchantID   string `json:"ideal_merchant_id"`
	IDealSubID        string `json:"ideal_sub_id"`
	IDealReturnURL    string `json:"ideal_return_url"`
	IrmaIdealIssuerSk string `json:"irma_ideal_issuer_sk"`
	PaymentAmount     string `json:"payment_amount"`
	PaymentMessage    string `json:"payment_message"`
}

var (
	config Config
)

func readConfig() error {
	data, err := readFile(configDir + "/config.json")
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &config)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s <command> [args...]\n", os.Args[0])
		fmt.Fprintln(flag.CommandLine.Output(), "Available commands: help, server")
		fmt.Fprintln(flag.CommandLine.Output(), "Flags:")
		flag.PrintDefaults()
	}

	flag.StringVar(&configDir, "config", "config", "Directory with configuration files")
	flag.StringVar(&config.StaticDir, "static", config.StaticDir, "Static files to serve")
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		return
	}
	switch flag.Arg(0) {
	case "help":
		flag.Usage()
	case "server":
		if flag.NArg() != 2 {
			fmt.Fprintln(flag.CommandLine.Output(), "Provide a host:port to bind to, for example:\n    ", os.Args[0], "server :8083")
			flag.Usage()
			return
		}
		err := readConfig()
		if err != nil {
			fmt.Fprintln(flag.CommandLine.Output(), "Could not read config file:", err)
			return
		}
		cmdServe(flag.Arg(1))
	default:
		fmt.Fprintln(flag.CommandLine.Output(), "Unknown command:", flag.Arg(0))
		flag.Usage()
	}
}
