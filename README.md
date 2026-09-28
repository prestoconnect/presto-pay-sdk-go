# presto-pay-sdk-go

[![CI](https://github.com/prestoconnect/presto-pay-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/prestoconnect/presto-pay-sdk-go/actions/workflows/ci.yml)

Standalone, framework-agnostic Go library for the **Presto Connect** payment gateway. **Pre-1.0 — the API can
still move.**

- **Go 1.24+** (1.24, 1.25, 1.26 in CI) — no build tags, no cgo
- **Zero dependencies** — stdlib only
- **Thread-safe** `Client` — build once, share across goroutines, like `*sql.DB`

## Status

Client construction and key validation, the error types, key loading, canonicalization, timestamp formatting,
the four payment operations (`Init`, `Query`, `Reverse`, `Refund`) with validation, response mapping, and the
`Raw` escape hatch, webhook verification, and `ConfigFromEnv` are all implemented and tested today. See
[CHANGELOG.md](CHANGELOG.md) for details. Remaining work — vendoring the shared wire-contract test vectors, a
staging smoke test, and tagging a first release — is still open.

## Contents

- [Install](#install)
- [Quick start](#quick-start)
- [Merchant identity](#merchant-identity)
- [Configuration from environment](#configuration-from-environment)
- [Retries and idempotency](#retries-and-idempotency)
- [Webhooks](#webhooks)
- [Errors](#errors)
- [Samples](#samples)
- [Staging smoke test](#staging-smoke-test)

## Install

```bash
go get github.com/prestoconnect/presto-pay-sdk-go
```

Pre-1.0, so pin a specific tag or commit rather than tracking the module unpinned.

## Quick start

```go
client, err := prestopay.New(prestopay.Config{
    Environment:      prestopay.Staging,
    MerchantID:       os.Getenv("PRESTOPAY_MID"),
    PrivateKeyPEM:    []byte(os.Getenv("PRESTOPAY_PRIVATE_KEY")),
    PrestoPublicKeys: [][]byte{[]byte(os.Getenv("PRESTOPAY_PUBLIC_KEY"))},
})

res, err := client.Payments.Init(ctx, prestopay.InitRequest{
    PrestoMRN:    "YOUR_PRESTO_MRN",
    TxnType:      prestopay.TxnTypeWebPay,
    TxnRefNum:    "order-123",
    DisplayDesc:  "Order 123",
    Amount:       10_000,
    CurrencyCode: "MYR",
    NotifyURL:    "https://your-app.example/presto/notify",
    RedirectURL:  "https://your-app.example/presto/return/order-123",
})
if err != nil {
    // see Errors below
}
res.PaymentURL
```

`prestopay.New(Config)` builds and validates a `*Client` — merchant ID, private key, and Presto public keys are
checked eagerly, so a bad key is a `*ConfigError` at construction rather than a mysterious failure on the first
real call.

## Merchant identity

`MerchantID` (`mid`) is set once on `Config` and sent on every request; `PrestoMRN` is set per request, so one
client can use several `prestoMrn`s under its `mid`. To serve several merchants, build one client per `mid`
(they can share the same keys) and route each request to the matching client.

## Configuration from environment

```go
cfg, err := prestopay.ConfigFromEnv(os.Getenv)
client, err := prestopay.New(cfg)
```

`ConfigFromEnv` builds a `Config` from these variables:

| Variable | Description |
|----------|-------------|
| `PRESTOPAY_ENV` or `PRESTOPAY_BASE_URL` | `staging` / `production`, or an explicit URL |
| `PRESTOPAY_MID` | Merchant ID |
| `PRESTOPAY_PRIVATE_KEY` or `PRESTOPAY_PRIVATE_KEY_FILE` | PKCS#8 PEM text, or a path to one |
| `PRESTOPAY_PUBLIC_KEY` or `PRESTOPAY_PUBLIC_KEY_FILE` | Presto certificate (PEM or DER), or SPKI PEM |

Onboarding delivers the merchant key as a PKCS#12 keystore; this SDK reads no PKCS#12 (no maintained Go parser
exists), so convert it once with `openssl`:

```bash
openssl pkcs12 -in partner.p12 -nocerts -nodes -out partner-key.pem
```

**This command can exit non-zero on some keystores, but still work.** Some `.p12` files encrypt their
certificate bag with an older algorithm (e.g. RC2-40-CBC) that OpenSSL 3 refuses by default, so it prints
something like `digital envelope routines:inner_evp_generic_fetch:unsupported` and returns exit code 1 — but
only *after* writing `partner-key.pem`, since the private key itself commonly uses an algorithm OpenSSL 3 does
support. Check the output file: if it contains a `BEGIN PRIVATE KEY` block, the conversion succeeded and the
error can be ignored. For a clean exit code, prepend `-legacy` (`openssl pkcs12 -legacy -in ...`); this needs the
legacy provider available in your OpenSSL build (`openssl list -provider legacy -providers`), which stock builds
sometimes omit.

## Retries and idempotency

`Init`, `Reverse`, and `Refund` are **not** safely retried after the request may have reached Presto — they are
retried automatically only when an `httptrace`-proven `RequestNotSent` shows nothing reached the gateway. A
duplicate `TxnRefNum` on `Init` returns error `1203`; reconcile with `Query` rather than re-calling `Init`:

```go
res, err := client.Payments.Query(ctx, prestopay.QueryRequest{
    PrestoMRN: "YOUR_PRESTO_MRN",
    TxnRefNum: "order-123",
})
```

`Query` is read-only and safe to retry; `Config.RetryReads` governs how many times and how it backs off for
`Query`, and (for the `RequestNotSent` case only) for `Init`/`Reverse`/`Refund` too.

If you supply a custom `Config.HTTPClient` or `Transport`, never set an `Idempotency-Key` or
`X-Idempotency-Key` header: `net/http` treats a POST carrying either as replayable and may silently resend it on
a broken idle connection, which is exactly the transparent retry this SDK's idempotency guarantees depend on
not happening.

## Webhooks

```go
v, err := prestopay.NewWebhookVerifier(prestopay.WebhookConfig{
    MerchantIDs:      []string{os.Getenv("PRESTOPAY_MID")},
    PrestoPublicKeys: [][]byte{[]byte(os.Getenv("PRESTOPAY_PUBLIC_KEY"))},
})

http.HandleFunc("/presto/notify", func(w http.ResponseWriter, r *http.Request) {
    event, err := v.VerifyRequest(r)
    if err != nil {
        prestopay.WriteAck(w, prestopay.AckForError(err))
        return
    }
    if err := fulfilOnce(r.Context(), event.EventRefNum, event); err != nil {
        prestopay.WriteAck(w, prestopay.AckResend)
        return
    }
    prestopay.WriteAck(w, prestopay.AckOK)
})
```

A verifier needs no private key, so a webhook-only service configures nothing else and never holds signing
material. `MerchantIDs` is a set — Presto signs webhooks for every merchant with the same key, so the `mid`
check is mandatory, not defensive.

## Errors

Implemented today. Every error type returned by this package satisfies:

```go
type Error interface {
    error
    Operation() Operation
    MayHaveTakenEffect() bool
    ReconcileBy() (ReconcileKey, bool)
}
```

| Type | When | Extra fields |
|------|------|---------------|
| `*ConfigError` | Invalid config or request input, including invalid UTF-8 | `Field` |
| `*TransportError` | Network failure, timeout, or context cancellation | `RequestNotSent` |
| `*APIError` | Non-200 status (`Kind: KindHTTP`) or `success: false` (`Kind: KindBusiness`) | `Kind`, `HTTPStatus`, `ErrorCode`, `ErrorMessage`, `RawBody`; `Canonical` on codes `1006`/`1007`, observed clock offset on `1005` |
| `*SignatureError` | Missing or invalid signature, wrong webhook `mid`, stale webhook `ts` | `Source`, `Canonical` |
| `*ResponseError` | Malformed body, missing required field, bad `ts`, echo mismatch | `Source`, `RawBody` |

Every type implements `Unwrap`, so `errors.Is` works through them, and inspection is `errors.As`.
`MayHaveTakenEffect` is decided where the error is constructed — a 500 on `Init` is indeterminate, a 500 on
`Query` means nothing happened — and `ReconcileBy` carries the lookup key for a follow-up `Query` once that
operation exists.

`RawBody` and `Canonical` can carry PII (`cardBin`, `cardSummary`, `receiptEmail`, `receiptName`); `Error()`
redacts them unless `Config.ShowErrorBodies` is `true`. The fields themselves are always populated regardless.

## Samples

- [examples/net-http/](examples/net-http/) — a runnable demo against Presto's real staging gateway using only
  `net/http` and `html/template`: a checkout page with a hosted-vs-self-selected payment method toggle, a return
  page, JSON endpoints for query/reverse/refund, and webhook handling that dedupes on `EventRefNum`.
- [examples/chi/](examples/chi/) — the same demo routed with [chi](https://github.com/go-chi/chi) instead of the
  stdlib mux, showing the SDK is router-agnostic.
- [examples/lambda/](examples/lambda/) — a webhook-only receiver deployed as an AWS Lambda function behind API
  Gateway, holding no private key since verification needs none.

`examples/chi` and `examples/lambda` have their own `go.mod` with real third-party dependencies and a `replace`
directive pointing at the local SDK source — the SDK module itself stays dependency-free.

## Staging smoke test

`prestopay/staging_test.go` calls Presto's real staging gateway: `Init`, an immediate `Query` on the same
transaction, and a duplicate `Init` to confirm error `1203`. It is excluded from the normal build entirely (a
`staging` build tag) and, even when built with that tag, still requires an explicit opt-in, since it creates a
real payment record on every run:

```bash
PRESTOPAY_STAGING_SMOKE=1 go test -tags staging ./prestopay/... -run TestStagingSmoke -v
```

Configure real staging credentials the same way as [Configuration from environment](#configuration-from-environment),
plus `PRESTO_MRN`. Without `PRESTOPAY_STAGING_SMOKE=1` the test skips even under `-tags staging`, and without the
tag at all `go test ./...` never compiles it.
