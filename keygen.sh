#!/bin/bash

dir=$(cd -P -- "$(dirname -- "$0")" && pwd -P)
mkdir -p "$dir/config"

SK=$dir/config/sk
PK=$dir/config/pk

if [ ! -e "$SK.der" ]; then
    # Generate a private key in PEM format
    openssl genrsa -out ${SK}.pem 2048
    # Calculate corresponding public key, saved in DER format
    openssl rsa -in ${SK}.pem -pubout -outform DER -out ${PK}.der
    openssl rsa -in ${SK}.pem -outform DER -out ${SK}.der
    rm -f ${SK}.pem
fi
