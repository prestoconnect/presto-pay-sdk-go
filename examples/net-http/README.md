# Presto Pay SDK (Go) — net/http demo

A runnable demo of the Go Presto Pay SDK against **Presto's real staging gateway**, using only `net/http` and the
SDK itself — no third-party dependencies. Staging credentials are not bundled; provide your own key files and
merchant identifiers through `.env`.

## Run it

```bash
cp .env.example .env   # then edit .env with your staging credentials
go run .
```

The server listens on `http://localhost:8080` by default (`$PORT` to change it).

## What it demonstrates

| Route | SDK feature |
|-------|-------------|
| `POST /checkout` | `client.Payments.Init(...)`. Accepts an optional JSON body (`txnRefNum`, `amount`, `currencyCode`); fills in sane defaults otherwise. Returns the `InitResponse` as JSON, or a 400/502 error body with `mayHaveTakenEffect`/`reconcileBy` |
| `GET /payments/{paymentRefNum}` | `client.Payments.Query(...)`, for `curl` |
| `POST /payments/{paymentRefNum}/reverse` | `client.Payments.Reverse(...)` |
| `POST /payments/{paymentRefNum}/refund` | `client.Payments.Refund(...)` |
| `POST /presto/notify` | `prestopay.NewWebhookVerifier(...).VerifyRequest(...)`, `WriteAck`/`AckForError`, and deduping deliveries on `EventRefNum` before doing any fulfilment work |

Try it with `curl`:

```bash
curl -X POST localhost:8080/checkout -d '{"amount":2500,"currencyCode":"MYR"}'
curl localhost:8080/payments/PP-from-the-response-above
curl -X POST localhost:8080/payments/PP-from-the-response-above/reverse
```

## Seeing webhooks actually fire

Presto can only deliver a webhook to a **publicly reachable** URL — it cannot reach your `localhost`. The
payment routes (`checkout`/query/reverse/refund) work over plain `localhost` regardless, but to see
`/presto/notify` actually get called:

1. Tunnel this port, e.g. `ngrok http 8080`.
2. Set `PUBLIC_URL` in `.env` to the tunnel's `https://` URL before starting the server.
3. Start a checkout and complete it on the hosted payment page — Presto will POST to
   `<PUBLIC_URL>/presto/notify`, logged to stdout along with the derived `PaymentStatus`.

## Configuration

See `.env.example`. `PRESTOPAY_ENV`, `PRESTOPAY_MID`, `PRESTO_MRN`, `PRESTOPAY_PRIVATE_KEY_FILE`, and
`PRESTOPAY_PUBLIC_KEY_FILE` are required; `PORT` and `PUBLIC_URL` are optional.
