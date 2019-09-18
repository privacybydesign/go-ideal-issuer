# iDeal issuer

This application provides an API backend to do payments using iDeal for the
[iDIN IRMA issuer](https://github.com/privacybydesign/irma_idin_issuer).


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
        should provide this certificate.
  * `ideal-sk.der` and `ideal-cert.der` are a private key and certificate pair
    as used in the iDeal transactions. The certificate will need to be uploaded
    to your bank, the secret key is used to sign outgoing messages to your bank.
    Make sure the `ideal-sk.der` is in DER-encoded pkcs8 format.
