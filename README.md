# Presto Pay SDK for Go

[![Go Reference](https://pkg.go.dev/badge/github.com/prestoconnect/presto-pay-sdk-go/prestopay.svg)](https://pkg.go.dev/github.com/prestoconnect/presto-pay-sdk-go/prestopay)
[![CI](https://github.com/prestoconnect/presto-pay-sdk-go/actions/workflows/ci.yml/badge.svg)](https://github.com/prestoconnect/presto-pay-sdk-go/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Accept payments through the **Presto Connect** payment gateway from any Go application. The SDK signs every
request, verifies every response and webhook, and gives you typed requests and results, so you don't have to
handle the gateway's signature scheme yourself.

- **Go 1.24+**, no cgo, works with any router
- **Zero dependencies**: standard library only
- **Safe for concurrent use**: build one `*Client` and share it, like `*sql.DB`

## Contents

- [Install](#install)
- [Before you start](#before-you-start)
- [How a payment works](#how-a-payment-works)
- [Quick start](#quick-start)
- [Payment statuses](#payment-statuses)
- [Next steps](#next-steps)

## Install

```bash
go get github.com/prestoconnect/presto-pay-sdk-go@v0.3.1
```

The package is `github.com/prestoconnect/presto-pay-sdk-go/prestopay`.

## Before you start

### 1. Create your key pair

You sign every request with your own RSA private key, and Presto verifies it with the matching public key.
Generate the pair yourself with `openssl`; the private key never leaves your systems:

```bash
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out merchant-key.pem
openssl req -new -x509 -key merchant-key.pem -days 3650 -subj "/CN=Your Company" -outform DER -out merchant.der
```

`merchant-key.pem` is your private key in the PKCS#8 PEM format the SDK reads; keep it secret and out of
source control. Send `merchant.der` (your public key, in the DER format Presto requires) to Presto.

### 2. Get your details from Presto

| From Presto | What it is | Where it goes |
|-------------|------------|---------------|
| Merchant ID (`mid`) | Identifies your merchant account | `Config.MerchantID` |
| Presto merchant reference (`prestoMrn`) | Identifies the shop or outlet; one `mid` can have several | Every request: `PrestoMRN` |
| Presto certificate (`.der`) | Verifies Presto's responses and webhooks; the SDK reads it as is | `Config.PrestoPublicKeys` |

Staging and production are separate: each has its own `mid`, `prestoMrn` and Presto certificate, and you
register your public key for each. Never mix them.

## How a payment works

```
 Your server                      Presto                     Shopper's browser
     |---- 1. Init ------------------>|                              |
     |<--- PaymentURL ----------------|                              |
     |---- 2. redirect to PaymentURL ------------------------------->|
     |                                |<---- 3. shopper pays --------|
     |                                |---- 4a. redirect to your RedirectURL -->|
     |<--- 4b. webhook to your NotifyURL                             |
     |---- 5. Query ----------------->|                              |
```

1. Your server calls `Init` with your order's reference and amount. Presto returns a `PaymentURL`.
2. You redirect the shopper to `PaymentURL`.
3. The shopper chooses a payment method and pays on Presto's page.
4. Presto sends the shopper's browser back to your `RedirectURL` **and** POSTs a signed webhook to your
   `NotifyURL`. These happen independently and can arrive in either order.
5. On both, you call `Query` to get the payment's status from Presto, and update the order.

The identifiers you'll see:

| Name | Who creates it | What it's for |
|------|----------------|---------------|
| `TxnRefNum` | You | Your reference for the payment, such as an order ID. Unique per payment, at most 50 characters |
| `PaymentRefNum` | Presto | Presto's reference for the payment, returned by `Init` |
| `EventRefNum` | Presto | Identifies one webhook event; stays the same when Presto redelivers it |
| `ReversalRefNum`, `RefundRefNum` | You | Your reference for a reversal or a refund |

## Quick start

### 1. Create the client

Build it once at startup and reuse it. Bad keys fail here, not on the first payment. The webhook verifier
needs only Presto's certificate and your `mid`.

```go
import "github.com/prestoconnect/presto-pay-sdk-go/prestopay"

privateKey, err := os.ReadFile("merchant-key.pem")
if err != nil {
    log.Fatal(err)
}
prestoCert, err := os.ReadFile("presto.der")
if err != nil {
    log.Fatal(err)
}

client, err := prestopay.New(prestopay.Config{
    Environment:      prestopay.Staging,
    MerchantID:       "YOUR_MID",
    PrivateKeyPEM:    privateKey,
    PrestoPublicKeys: [][]byte{prestoCert},
})
if err != nil {
    log.Fatal(err)
}

verifier, err := prestopay.NewWebhookVerifier(prestopay.WebhookConfig{
    MerchantIDs:      []string{"YOUR_MID"},
    PrestoPublicKeys: [][]byte{prestoCert},
})
if err != nil {
    log.Fatal(err)
}
```

To configure the client from environment variables instead, see
[Configuration](docs/production.md#configuration-from-environment).

### 2. Start a payment

```go
payment, err := client.Payments.Init(r.Context(), prestopay.InitRequest{
    PrestoMRN:             "YOUR_PRESTO_MRN",
    TxnType:               prestopay.TxnTypeWebPay,
    TxnRefNum:             orderID,
    DisplayDesc:           "Order " + orderID,
    Amount:                10_000, // minor units: MYR 100.00
    CurrencyCode:          "MYR",
    NotifyURL:             "https://your-app.example/presto/notify",
    RedirectURL:           "https://your-app.example/presto/return/" + orderID,
    AllowedPaymentMethods: []string{prestopay.PaymentMethodCard}, // Skip this unless you build your own payment selection page
})
if err != nil {
    // See "When you don't know whether it worked" in docs/payments-and-errors.md.
    return err
}

// Save payment.PaymentRefNum with the order, then send the shopper to Presto.
http.Redirect(w, r, payment.PaymentURL, http.StatusFound)
```

`NotifyURL` must be reachable from the internet; on your own machine, use a tunnel such as ngrok.

### 3. Show the result on your return page

The redirect only tells you the shopper came back, not whether they paid. Ask Presto:

```go
result, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
    PrestoMRN: "YOUR_PRESTO_MRN",
    TxnRefNum: orderID,
})
if err != nil {
    return err
}

switch result.PaymentStatus {
case prestopay.PaymentStatusAuthorised:
    // Paid: show the confirmation.
case prestopay.PaymentStatusPendingAuthorise:
    // Not finished yet: show "processing" and check again shortly.
default:
    // Not paid (Failed, Cancelled, Expired, ...): let the shopper try again.
}
```

### 4. Handle the webhook

A webhook tells you something happened to a payment (`EventCode`, and `Success` for whether it worked), not the
payment's resulting status, so query for that here too.

```go
http.HandleFunc("POST /presto/notify", func(w http.ResponseWriter, r *http.Request) {
    event, err := verifier.VerifyRequest(r)
    if err != nil {
        var sigErr *prestopay.SignatureError
        if errors.As(err, &sigErr) {
            w.WriteHeader(http.StatusUnauthorized) // forged, for another mid, or too old
            return
        }
        prestopay.WriteAck(w, prestopay.AckForError(err)) // malformed body
        return
    }

    if !orders.IsEventHandled(event.EventRefNum) {
        payment, err := client.Payments.Query(r.Context(), prestopay.QueryRequest{
            PrestoMRN:     event.PrestoMRN,
            PaymentRefNum: event.PaymentRefNum,
        })
        if err != nil {
            prestopay.WriteAck(w, prestopay.AckResend)
            return
        }
        orders.UpdateStatus(event.TxnRefNum, payment.PaymentStatus, event.EventRefNum)
    }
    prestopay.WriteAck(w, prestopay.AckOK)
})
```

`AckOK` tells Presto the event is handled. `AckResend` asks Presto to deliver it again (after 1, 2, 5 and 10
minutes), which you want when your own processing failed. Presto redelivers an event with the same
`EventRefNum`, so record it once handled and skip it on later deliveries. See [Webhooks](docs/webhooks.md)
for the details.

Update the order the same way from your return page and your webhook: whichever arrives first records the
status, and the other finds it already done.

## Payment statuses

`PaymentStatus` is one of these strings; compare it with the `prestopay.PaymentStatus*` constants.

| Status | Meaning | What to do |
|--------|---------|------------|
| `PendingAuthorise` | Created; the shopper hasn't finished paying | Wait. It becomes `Expired` if not paid within 15 minutes of `Init` |
| `Authorised` | Paid | Fulfil the order |
| `Failed` | The payment attempt failed | Don't fulfil; let the shopper try again with a new `TxnRefNum` |
| `Cancelled` | Cancelled before it was paid, for example by `Reverse` | Don't fulfil |
| `Expired` | Not paid within 15 minutes | Don't fulfil; start a new payment if the shopper returns |
| `PendingReverse` | A reversal is in progress | Query again later |
| `Reversed` | The payment was reversed | Treat the order as cancelled |
| `PendingRefund` | A refund is in progress | Query again later |
| `PartialRefunded` | Part of the amount was refunded | Update the order's refunded amount |
| `Refunded` | The full amount was refunded | Treat the order as refunded |

The gateway can add statuses, so handle an unknown value without failing.

## Next steps

- [Payments and errors](docs/payments-and-errors.md): look up, reverse and refund payments; handle errors and
  timeouts safely.
- [Webhooks](docs/webhooks.md): replies, redelivery, deduplication and the freshness window.
- [Production](docs/production.md): configuration, several merchants, custom HTTP clients, the go-live
  checklist and troubleshooting.
- Examples, runnable against Presto staging:
  - [`examples/net-http`](examples/net-http/): a checkout with a return page and webhook handling, using only
    the standard library.
  - [`examples/chi`](examples/chi/): the same checkout routed with [chi](https://github.com/go-chi/chi).
  - [`examples/lambda`](examples/lambda/): a webhook-only receiver on AWS Lambda, holding no private key.

## Contributing

Building, testing, code style and the release process are in [CONTRIBUTING.md](CONTRIBUTING.md). Report
security issues as described in [SECURITY.md](SECURITY.md), not in a public issue.

## License

Apache License 2.0. See [LICENSE](LICENSE).
