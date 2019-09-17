package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/aykevl/go-idx"
)

func backgroundUpdateIssuers(name string, destination *[]byte, client idx.Client) {
	for {
		log.Printf("updating %s issuer list", name)
		err := updateIssuers(client, destination)
		if err != nil {
			log.Printf("ERROR: failed to update %s issuer list: %s", name, err)
		}
		log.Printf("finished updating %s issuer list", name)

		time.Sleep(24 * time.Hour)
	}
}

func updateIssuers(client idx.Client, destination *[]byte) error {
	directory, err := client.DirectoryRequest()
	if err != nil {
		return err
	}

	// Encode banks list as JSON.
	data, err := json.MarshalIndent(directory.Issuers, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))

	// Atomically update the JSON data to be served.
	banksLock.Lock()
	*destination = data
	banksLock.Unlock()

	return nil
}
