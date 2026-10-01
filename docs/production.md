# Production

- [Configuration from environment](#configuration-from-environment)
- [Keys](#keys)
- [Several merchants](#several-merchants)
- [Custom HTTP client](#custom-http-client)
- [Going live checklist](#going-live-checklist)
- [Troubleshooting](#troubleshooting)

## Configuration from environment

`prestopay.ConfigFromEnv` builds a `Config` from environment variables:

```go
cfg, err := prestopay.ConfigFromEnv(os.Getenv)
if err != nil {
    log.Fatal(err)
}
client, err := prestopay.New(cfg)
```

| Variable | Value |
|----------|-------|
| `PRESTOPAY_ENV` or `PRESTOPAY_BASE_URL` | `staging` or `production`, or a gateway base URL (which wins if both are set) |
| `PRESTOPAY_MID` | Your `mid` |
| `PRESTOPAY_PRIVATE_KEY` or `PRESTOPAY_PRIVATE_KEY_FILE` | Your PKCS#8 PEM private key, or a path to it |
| `PRESTOPAY_PUBLIC_KEY` or `PRESTOPAY_PUBLIC_KEY_FILE` | Presto's certificate, or a path to its `.der` file |

`ConfigFromEnv` takes a lookup function, so you can pass something other than `os.Getenv`, such as a function
that reads your secret store. Set other `Config` fields, like `RetryReads`, on the returned value before calling
`New`.

## Keys

`Config.PrivateKeyPEM` takes a PKCS#8 PEM private key (`-----BEGIN PRIVATE KEY-----`), as created in
[Before you start](../README.md#1-create-your-key-pair). Load it from your secret store rather than a file
where you can.

`Config.PrestoPublicKeys` takes Presto's certificate as DER or PEM. It's a list so that, when Presto announces a
new certificate, you can accept both the old and the new one until the change-over is done.

**Using an existing `.p12` keystore.** The SDK doesn't read PKCS#12. If you already have your key in a `.p12`
file, convert it once:

```bash
openssl pkcs12 -in merchant.p12 -nocerts -nodes -out merchant-key.pem
```

Some keystores use an old encryption algorithm that OpenSSL 3 refuses: it prints an `unsupported` error and
exits with status 1, but only after writing the key. If `merchant-key.pem` contains a `BEGIN PRIVATE KEY` block,
the conversion worked. For a clean exit, add `-legacy`, which needs OpenSSL's legacy provider.

## Several merchants

A client belongs to one `mid`: it sends that `mid` on every request. One `mid` can have several `prestoMrn`s,
which you choose per request, so one client covers all of them.

To serve several merchants, build one client per `mid` (they can share the same keys) and route each request to
the matching client, for example with a `map[string]*prestopay.Client` keyed by `mid`. A single webhook verifier
can accept all of them; see [Webhooks](webhooks.md#several-merchants-and-webhook-only-services).

## Custom HTTP client

Set `Config.HTTPClient` to use your own `*http.Client`, for proxies, connection pooling or tracing. The SDK
turns off redirects on it regardless.

Never set an `Idempotency-Key` or `X-Idempotency-Key` header in a custom transport. `net/http` treats a POST
carrying either header as safe to replay, and may resend it on a broken idle connection, which could send an
`Init`, `Reverse` or `Refund` twice.

## Going live checklist

- [ ] Generate a separate key pair for production and register its public key with Presto.
- [ ] Use `prestopay.Production` with your production `mid`, `prestoMrn` and Presto certificate. Never mix
      staging and production values.
- [ ] Load the private key from a secret store, not from source control or the image.
- [ ] Make `NotifyURL` a public HTTPS URL that Presto can reach.
- [ ] Have your return page `Query` the payment instead of trusting the redirect.
- [ ] Have your webhook handler `Query` the payment, deduplicate on `EventRefNum` under a unique constraint,
      return 401 for a `*SignatureError`, and reply `AckResend` when your own processing fails.
- [ ] After a timeout or server error, call `Init` again with the same `TxnRefNum`, and query before retrying
      `Reverse` or `Refund`, as in
      [Payments and errors](payments-and-errors.md#when-you-dont-know-whether-it-worked).
- [ ] Set `RetryReads` if you want `Query` retried; the default is no retries.
- [ ] Keep the server clock in sync with NTP.
- [ ] Log `ErrorCode` and `ErrorMessage` from `*APIError`, so you can quote them to Presto support.
- [ ] Leave `Config.Strict` off in production. In staging, turn it on to catch responses that break the
      gateway's documented rules.

## Troubleshooting

**`1005` (`ErrorCodeClockSkew`).** Your request's timestamp is too far from Presto's clock. Sync the server
clock with NTP; `APIError.ClockOffset` shows how far off it was. The SDK converts to the gateway's time zone
itself, so the host's time zone doesn't matter.

**`1006` or `1007` (`ErrorCodeInvalidSignature`, `ErrorCodeSignatureVerificationFailed`).** Presto couldn't
verify your signature. Usually the private key doesn't match the public key you registered for this
environment, or you're using a staging key in production or the other way round. `APIError.Canonical` holds the
exact string the SDK signed; `prestopay.Canonicalize(fields)` rebuilds it from a request's fields.

**`*SignatureError` from a payment call.** Presto's response didn't verify with the certificate you configured.
Check that it's the certificate for this environment, and whether Presto has announced a new one.

**`1102` or `1106` (`ErrorCodeInvalidMID`, `ErrorCodeInvalidMerchantReference`).** The `mid` or `prestoMrn`
isn't valid for this environment.

**Webhooks never arrive.** `NotifyURL` must be reachable from the internet. `localhost` and private addresses
won't work; during development, use a tunnel such as ngrok and pass its URL as `NotifyURL`.

**Webhooks fail with `*SignatureError`.** Either the event is for a `mid` that isn't in `MerchantIDs`, its
timestamp is more than 15 minutes from your clock (sync with NTP), or the Presto certificate is for the wrong
environment.

**An endpoint the SDK doesn't cover.** `client.Raw.Post(ctx, path, body)` signs, sends and verifies a request
to any gateway path. `client.Raw.Sign` and `client.Raw.VerifyBody` expose the two halves.
