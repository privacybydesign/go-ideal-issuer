# iDeal payment for iDIN attributes

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
      * `token_static_salt` is a "static salt" that is hashed together with the
        bank account numbers that are stored into the database. Changing it
        causes all tokens in the database to be invalidated.
      * `token_hmac_key` is a cryptograpic key that is used to sign tokens
        (using HMAC) sent to the client. Changing this key invalidates current
        sessions, but not the stored tokens.
      * `db_driver`, `db_datasource`: see
        [sql.Open](https://godoc.org/database/sql#Open) for details.
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
  * `sk.der`, which is the private key part of a keypair, used for signing JWTs
    to send to the API server. You can generate a key pair using the `keygen.sh`
    script.
