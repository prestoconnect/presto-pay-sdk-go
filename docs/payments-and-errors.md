# Payments and errors

This guide covers the four payment operations and how to handle their failures. It assumes you've set up a
client as in the [quick start](../README.md#quick-start).

- [Query a payment](#query-a-payment)
- [Reverse a payment](#reverse-a-payment)
- [Refund a payment](#refund-a-payment)
- [Errors](#errors)
- [When you don't know whether it worked](#when-you-dont-know-whether-it-worked)
- [Retries and deadlines](#retries-and-deadlines)

Every request needs your `prestoMrn`, passed as `PrestoMRN`. A missing or invalid field returns a
`*ConfigError` before anything is sent.

## Query a payment

`Query` returns a payment's current status and details. Look it up by your `TxnRefNum` or by Presto's
`PaymentRefNum`:

```go
payment, err := client.Payments.Query(ctx, prestopay.QueryRequest{
    PrestoMRN: "YOUR_PRESTO_MRN",
    TxnRefNum: "order-123", // or PaymentRefNum
})

payment.PaymentStatus  // see the status table in the README
payment.ReversalStatus // Reversing, Failed or Success, once you've requested a reversal
payment.RefundStatus   // Refunding, Failed or Success, once you've requested a refund
payment.RefundDetails  // one entry per refund
```

`Query` only reads, so it is always safe to call again.

## Reverse a payment

`Reverse` undoes a whole payment:

```go
reversal, err := client.Payments.Reverse(ctx, prestopay.ReverseRequest{
    PrestoMRN:      "YOUR_PRESTO_MRN",
    PaymentRefNum:  paymentRefNum,   // or TxnRefNum
    ReversalRefNum: "rev-order-123", // your reference for this reversal, at most 50 characters
    Remark:         "Customer cancelled",                     // optional
    NotifyURL:      "https://your-app.example/presto/notify", // optional: get a Reversed webhook
})
```

What happens depends on the payment's status:

- **`PendingAuthorise`** (not paid yet): the payment is cancelled and its status becomes `Cancelled`.
- **`Expired`**: fails with error `1219` (`prestopay.ErrorCodeInvalidStatusForReversal`). There's
  nothing left to undo.
- **Paid**: the gateway decides. It can refuse once the payment has settled (`1220`) or its reversal window
  has passed (`1221`). Use `Refund` instead in that case.

## Refund a payment

`Refund` returns all or part of a paid payment's amount:

```go
refund, err := client.Payments.Refund(ctx, prestopay.RefundRequest{
    PrestoMRN:     "YOUR_PRESTO_MRN",
    PaymentRefNum: paymentRefNum,
    RefundRefNum:  "ref-order-123",     // your reference for this refund, at most 50 characters
    Remark:        "Item out of stock", // required, at most 200 characters
    Amount:        2_500,               // optional: leave it out to refund the full amount
    NotifyURL:     "https://your-app.example/presto/notify", // optional: get a Refunded webhook
})
```

You can request a refund for any payment method, but whether it succeeds depends on the method; some need
manual or offline processing by Presto. A successful `Refund` call means Presto accepted the request, not
that the money has moved. Check `RefundStatus` with `Query`, or wait for the `Refunded` webhook, before
treating the refund as complete. A refund on an unpaid (`PendingAuthorise`) payment fails with `1227`; use
`Reverse` to cancel it instead.

## Errors

Every error the package returns satisfies `prestopay.Error`:

```go
type Error interface {
    error
    Operation() Operation
    MayHaveTakenEffect() bool
    ReconcileBy() (ReconcileKey, bool)
}
```

Inspect it with `errors.As`:

| Type | When | Useful fields |
|------|------|---------------|
| `*ConfigError` | Invalid config or request input, including invalid UTF-8 | `Field` |
| `*TransportError` | Network failure, timeout or cancelled context | `RequestNotSent` |
| `*APIError` | Presto rejected the request: a non-200 status (`Kind == KindHTTP`) or `success: false` (`Kind == KindBusiness`) | `HTTPStatus`, `ErrorCode`, `ErrorMessage`, `RawBody`; `Canonical` on `1006`/`1007`; `ClockOffset` on `1005` |
| `*SignatureError` | A response or webhook signature is missing or invalid, a webhook is for another `mid`, or a webhook is too old | `Source`, `Canonical` |
| `*ResponseError` | A response or webhook body is malformed or missing a field | `Source`, `RawBody` |

```go
var apiErr *prestopay.APIError
if errors.As(err, &apiErr) && apiErr.ErrorCode == prestopay.ErrorCodePaymentNotFound {
    // ...
}
```

For codes that point at your setup (`1005`, `1006`, `1007`), see
[Troubleshooting](production.md#troubleshooting).

`RawBody` and `Canonical` can contain card and customer details, so `Error()` leaves them out unless
`Config.ShowErrorBodies` is true. The fields themselves are always set.

## When you don't know whether it worked

A timeout, a server error (HTTP 5xx) or a garbled response leaves you not knowing whether Presto acted on your
request. Every error tells you this directly: `MayHaveTakenEffect()` is true when the request may have
reached Presto and been acted on.

**`Init`: call it again with the same `TxnRefNum`.** That's safe. If the first call reached Presto, you get
the existing payment and its current status back rather than a second payment:

```go
payment, err := client.Payments.Init(ctx, req)
var pe prestopay.Error
if errors.As(err, &pe) && pe.MayHaveTakenEffect() {
    payment, err = client.Payments.Init(ctx, req) // same TxnRefNum: returns the payment if it was created
}
```

If Presto answers `1203` (`prestopay.ErrorCodeDuplicateTxnRefNum`), a payment with that `TxnRefNum` exists;
`Query` it to find its state.

**`Reverse` and `Refund`: check before trying again.** Sending one of these twice could reverse or refund
twice, so `Query` first, and look at `ReversalStatus` or `RefundStatus`. Try again only if the first request
didn't take effect. `ReconcileBy()` gives you the key to query with.

## Retries and deadlines

By default the client doesn't retry. Set `Config.RetryReads` to turn retries on:

```go
client, err := prestopay.New(prestopay.Config{
    // ...
    RetryReads: prestopay.RetryReads{MaxRetries: 2, InitialBackoff: 200 * time.Millisecond, MaxBackoff: 2 * time.Second},
    Deadline:   20 * time.Second,
})
```

Even then, it retries only when that's safe:

- **`Query`**: on network errors and HTTP 5xx responses, honouring `Retry-After`.
- **`Init`, `Reverse`, `Refund`**: only when the request certainly never left your machine, for example when
  the connection was refused or the host name didn't resolve (`RequestNotSent` is true).

Retries back off exponentially with jitter. `Deadline` (30 seconds by default) is the budget for a whole call,
retries included; your `ctx` can shorten it further.
