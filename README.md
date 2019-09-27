# iDeal issuer

This application provides an API backend to do payments using iDeal for the
[IRMA iDeal issuer](https://github.com/privacybydesign/irma_ideal_server).


Use it like this:

```
go-ideal-server server <host>:<port>
```

You will need to set up a configuration directory. This is `config/` by default
(relative to the current working directory) but you can set a different
directory using the `-config` flag. It needs to have the following entries:

  * A configuration file named `config.json`. Copy the contents of
    `config.example.json` and modify as appropriate:
      * `static_dir` is a directory you can serve (as the root directory).
        Useful for debugging.
      * `enable_tls` makes sure server is run over https
      * `tls_certificate` is the file path to the TLS certificate 
        when TLS is enabled)
      * `tls_private_key` is the file path to the private key of the TLS 
        certificate (when TLS is enabled)
      * `ideal_path_prefix` is the URL prefix that the HTTP server will listen
        on for the API.
      * `ideal_server_name` is the server name as configured in the IRMA API
        server.
      * `ideal_base_url`, `ideal_merchant_id`, `ideal_sub_id` must be provided
        by your bank.
      * `ideal_return_url` is the URL of the web front end that the bank will
        return to.
      * `ideal_acquirer_cert` is the path to the certificate of the bank you're
        trying to connect to, relative to the configuration directory. Your bank
        should provide this certificate. At the bank this certificate is sometimes
        branded as merchant certificate, because merchants have to use it. It is
        however the certificate of the bank. 
        
        The certificate must be a PEM-encoded X509 certificate. To test whether 
        the key has the right format, the following command can be used:
        `openssl x509 -in <filename> -inform pem`.
      * `ideal_merchant_cert` is the certificate (public key) of the key pair
        that is used to send messages to the bank as a merchant. This certificate
        also has to be uploaded at the bank. 
        
        The certificate must be a PEM-encoded X509 certificate. To test whether 
        the key has the right format, the following command can be used:
        `openssl x509 -in <filename> -inform pem`.
      * `ideal_merchant_sk` is the private key corresponding to `ideal_merchant_cert`.
        The key must be in non-encrypted, PEM-encoded PKCS1 format or in 
        non-encrypted, PEM-encoded PKCS8 format. 
        
        To test whether the key has the right format, one of the following commands
        must accept the key:
        * `openssl rsa -in <filename> -inform pem`
        * `openssl pkcs8 -in <filename> -inform pem -nocrypt`
        
     All paths must be relative to the configuration directory.
