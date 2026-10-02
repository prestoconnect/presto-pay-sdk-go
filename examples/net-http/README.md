# Presto Pay SDK (Go) — net/http demo

A runnable demo of the Go Presto Pay SDK against **Presto's real staging gateway**, using only `net/http`,
`html/template`, and the SDK itself — no third-party Go dependencies (the checkout page loads Tailwind CSS and
Font Awesome from a CDN for styling, which is a browser asset, not a module import). Staging credentials are not
bundled; provide your own key files and merchant identifiers through `.env`.

The checkout page models a merchant deciding **where the shopper picks a payment method**: a toggle switches
between letting Presto's hosted payment page collect it (`AllowedPaymentMethods` omitted) and collecting it on
this site first (`AllowedPaymentMethods` sent). Either way, the final authorization always happens on Presto's
hosted page — the toggle only changes who asks which method to use.

## Run it

```bash
cp .env.example .env   # then edit .env with your staging credentials
go run .
```

Open `http://localhost:8080/` (`$PORT` to change the port) and click through. Every submitted payment starts a
real call to the configured Presto staging gateway. No public URL is required to run the payment flow itself; it
uses localhost for the customer redirect and webhook URL. The server prints a warning because Presto cannot
deliver webhooks to localhost — see below to fix that.

## What it demonstrates

| Route | SDK feature |
|-------|-------------|
| `GET /` | The checkout page. The "Show payment methods on checkout" toggle switches the form between the hosted flow and a self-hosted method picker (client-side, inline in `templates/index.html`) |
| `POST /checkout` | `client.Payments.Init(...)`, with `AllowedPaymentMethods` set only when the toggle is on. Always JSON in and out — the checkout page submits via `fetch` and redirects the browser to `paymentUrl` itself. Returns the `InitResponse`, or a 400/502 error body with `mayHaveTakenEffect`/`reconcileBy` |
| `GET /return/{txnRefNum}` | `client.Payments.Query(...)` after the customer comes back from the hosted payment page |
| `GET /payments/{paymentRefNum}` | `client.Payments.Query(...)` by `paymentRefNum`, for `curl` |
| `POST /payments/{paymentRefNum}/reverse` | `client.Payments.Reverse(...)` |
| `POST /payments/{paymentRefNum}/refund` | `client.Payments.Refund(...)` |
| `POST /presto/notify` | `prestopay.NewWebhookVerifier(...).VerifyRequest(...)`, `WriteAck`/`AckForError`, HTTP 401 for a `*SignatureError`, `Query`, and the same guarded order update as the return page, so a redelivery never fulfils twice — shown in the "Recent webhooks" table on the checkout page |

Selecting a payment method on this site is not a replacement payment processor: it demonstrates the merchant UI
and SDK request that restricts which method Presto displays. The final authorization still happens on the
hosted payment page returned by `res.PaymentURL`.

Try the JSON API directly with `curl`:

```bash
curl -X POST localhost:8080/checkout -H 'Content-Type: application/json' -d '{"amount":2500,"currencyCode":"MYR"}'
curl localhost:8080/payments/PP-from-the-response-above
curl -X POST localhost:8080/payments/PP-from-the-response-above/reverse
```

## Seeing webhooks actually fire

Presto can only deliver a webhook to a **publicly reachable** URL — it cannot reach your `localhost`. The
payment routes (`checkout`/query/reverse/refund) work over plain `localhost` regardless, but to see
`/presto/notify` actually get called:

1. Tunnel this port, e.g. `ngrok http 8080`.
2. Set `PUBLIC_URL` in `.env` to the tunnel's `https://` URL before starting the server.
3. Start a checkout from `/` and complete it on the hosted payment page — Presto will POST to
   `<PUBLIC_URL>/presto/notify`, logged to stdout and shown in the "Recent webhooks" table on `/`.

## Configuration

See `.env.example`. `PRESTOPAY_ENV`, `PRESTOPAY_MID`, `PRESTO_MRN`, `PRESTOPAY_PRIVATE_KEY_FILE`, and
`PRESTOPAY_PUBLIC_KEY_FILE` are required; `PORT` and `PUBLIC_URL` are optional.
