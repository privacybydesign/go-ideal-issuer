package main

// This file contains the same functions as
// src/main/java/org/irmacard/ideal/web/IdinResource.java in irma_ideal_server,
// reimplemented in Go.

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"log"

	"golang.org/x/crypto/pbkdf2"
)

// Anonymize the BIC and IBAN numbers by hashing them using PBKDF2.
// The goal is to treat the IBAN/BIC like a regular low-entropy password so
// that trying all possible IBAN numbers is really difficult. Think of banks
// that issue very predictable (low entropy) IBAN numbers for which it is
// feasible to compute lower numbers.
// Sadly we can't really use a salt as we need to index the token in a
// database and we don't have the equivalent of a username.
func makeToken(bic, iban string) []byte {
	input := bic + "-" + iban
	salt := config.TokenStaticSalt
	if len(salt) == 0 {
		// Make sure we have a salt configured - just in case.
		log.Fatal("no static salt configured")
	}
	return pbkdf2.Key([]byte(input), []byte(salt), 10000, 32, sha512.New)
}

// Hash a token to be stored in the database.
// The reason a hash is applied first is to avoid timing attacks while
// retrieving a token. When looking up a token the database compares the
// user-supplied token to tokens in the database in a way that's certainly
// not constant-time. Hashing it first makes timing attacks impossible.
func hashToken(token []byte) string {
	digest := sha512.Sum512(token)
	return hex.EncodeToString(digest[:32])
}

// Sign a token for the happy flow from iDeal to iDIN, without pause.
// Because it is signed we can be sure that it came from us.
//
// Returns the HMAC signature.
func signToken(token []byte) []byte {
	key := config.TokenHMACKey
	if len(key) == 0 {
		// Make sure we have a salt configured - just in case.
		log.Fatal("no HMAC key configured")
	}

	mac := hmac.New(sha512.New, []byte(key))
	mac.Write(token)
	signature := mac.Sum(nil)
	// 32 bytes (256 bits) is long enough.
	return signature[:32]
}
