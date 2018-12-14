package main

import (
	"database/sql"
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
	StaticDir       string `json:"static_dir"`
	TokenStaticSalt string `json:"token_static_salt"`
	TokenHMACKey    string `json:"token_hmac_key"`
	DBDriverName    string `json:"db_driver"`
	DBDataSource    string `json:"db_datasource"`

	EnableIDeal       bool   `json:"enable_ideal"`
	IDealServerName   string `json:"ideal_server_name"`
	IDealPathPrefix   string `json:"ideal_path_prefix"`
	IDealAcquirerCert string `json:"ideal_acquirer_cert"`
	IDealCredentialID string `json:"ideal_credential_id"`
	IDealBaseURL      string `json:"ideal_base_url"`
	IDealMerchantID   string `json:"ideal_merchant_id"`
	IDealSubID        string `json:"ideal_sub_id"`
	IDealReturnURL    string `json:"ideal_return_url"`
	PaymentAmount     string `json:"payment_amount"`
	PaymentMessage    string `json:"payment_message"`
}

var (
	config  Config
	tokenDB *sql.DB
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
		fmt.Fprintln(flag.CommandLine.Output(), "Available commands: help, read, server")
		fmt.Fprintln(flag.CommandLine.Output(), "Flags:")
		flag.PrintDefaults()
	}

	flag.StringVar(&configDir, "config", "config", "Directory with configuration files")
	flag.StringVar(&config.StaticDir, "static", config.StaticDir, "Static files to serve")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Please provide a command")
		return
	}
	switch flag.Arg(0) {
	case "help", "usage":
		flag.Usage()
	case "server":
		if flag.NArg() != 2 {
			fmt.Fprintln(flag.CommandLine.Output(), "Provide a host:port to bind to for \"server\".")
			flag.Usage()
			return
		}
		err := readConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not read config file:", err)
			return
		}
		db, err := sql.Open(config.DBDriverName, config.DBDataSource)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Could not open sqlite3 database:", err)
			return
		}
		defer db.Close()
		tokenDB = db
		cmdServe(flag.Arg(1))
	default:
		fmt.Fprintln(flag.CommandLine.Output(), "Unknown command:", flag.Arg(0))
		flag.Usage()
	}
}
