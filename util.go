package main

import (
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"io/ioutil"
	"os"
)

// Utility function to read the entire contents of a file.
func readFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return ioutil.ReadAll(file)
}

// Utility function to read a DER-encoded private key from a given path.
func readPrivateKey(path string) (*rsa.PrivateKey, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKCS8PrivateKey(data)
	if err != nil {
		return nil, err
	}
	return key.(*rsa.PrivateKey), nil
}

// Utility function to read a DER-encoded public key from a given path.
func readPublicKey(path string) (*rsa.PublicKey, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	key, err := x509.ParsePKIXPublicKey(data)
	if err != nil {
		return nil, err
	}
	if key, ok := key.(*rsa.PublicKey); ok {
		return key, nil
	} else {
		return nil, errors.New("cannot determine public key type")
	}
}

func readCertificate(path string) (*x509.Certificate, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(data)
}
