# Webhooks

Presto POSTs a signed JSON webhook to the `NotifyURL` you pass to `Init`, `Reverse` or `Refund` when something
happens to that payment. The [quick start](../README.md#4-handle-the-webhook) has a complete handler; this
guide explains each part.

- [What a webhook tells you](#what-a-webhook-tells-you)
- [Verifying it](#verifying-it)
- [Replying](#replying)
- [Handling redeliveries](#handling-redeliveries)
- [The freshness window](#the-freshness-window)
- [Several merchants, and webhook-only services](#several-merchants-and-webhook-only-services)

## What a webhook tells you

`verifier.VerifyRequest(r)` returns a `prestopay.NotifyEvent`:

| Field | Value |
|-------|-------|
| `EventCode` | What happened: `Authorised`, `Cancelled`, `Reversed`, `Refunded` or `Expired` (compare with the `prestopay.EventCode*` constants). Presto may add codes |
| `Success` | Whether it worked. A `Refunded` event with `Success` false is a refund that failed |
| `TxnRefNum`, `PaymentRefNum`, `PrestoMRN`, `MID` | Which payment it's about |
| `EventRefNum` | Identifies this event; the same on every redelivery |
| `Amount`, `CurrencyCode`, `PaymentDetails` | The payment's amount and how it was paid |

A webhook reports an event, not the payment's resulting status. A failed refund, for example, leaves the
payment in whatever status it had before, which the event doesn't carry. To act on a webhook, `Query` the
payment and use the status it returns.

## Verifying it

The verifier checks that:

- the body is signed by Presto,
- the required fields are present,
- the event is for one of your `MerchantIDs`, and
- its timestamp is within 15 minutes of your clock.

The `mid` check matters: Presto signs webhooks for every merchant with the same key, so a genuine webhook for
someone else's account would otherwise pass.

`VerifyRequest` reads the raw body from the request for you, with a size limit. If the body has already been
read, for example by a queue consumer or a framework, pass the exact bytes to `verifier.Verify(body)`. Never
pass JSON you've decoded and encoded again: the signature won't match.

A failure returns `*SignatureError` (bad signature, another merchant's `mid`, or a stale timestamp) or
`*ResponseError` (malformed body). Answer a `*SignatureError` with HTTP 401. For a malformed body,
`prestopay.WriteAck(w, prestopay.AckForError(err))` replies `{"resend":false}`, since a redelivery would fail
the same way.

## Replying

Reply HTTP 200 with a JSON body, using `prestopay.WriteAck`:

| Ack | Body | When to send it |
|-----|------|-----------------|
| `prestopay.AckOK` | `{"resend":false}` | You've recorded the event, or had already recorded it earlier |
| `prestopay.AckResend` | `{"resend":true}` | Your own processing failed, for example the `Query` or your database |

Presto retries 1, 2, 5 and 10 minutes after the first attempt, so an event is delivered at most five times over
about 18 minutes. Only ask for a resend when trying again could succeed.

`prestopay.AckForError(err)` picks the ack for an error: `AckOK` for a webhook that failed verification, and
`AckResend` for anything else, including a failed `Query` inside your handler.

Reply quickly. Record the event and reply, and do slow work such as emails or fulfilment afterwards.

## Handling redeliveries

The same event can arrive more than once, for example after you ask for a resend. Every delivery of an event
has the same `EventRefNum`, so:

- record `EventRefNum` once you've handled the event, under a unique constraint in your database;
- skip events you've already recorded, and still reply `AckOK`;
- record it only after the `Query` succeeds, so a failed attempt isn't mistaken for a handled one on
  redelivery.

Keep recorded `EventRefNum`s for at least as long as the redelivery schedule (about 18 minutes).

## The freshness window

The verifier rejects a webhook whose timestamp is more than 15 minutes from your clock, so a captured webhook
can't be replayed later. Each redelivery carries a fresh timestamp, so redeliveries pass. Keep your server's
clock in sync with NTP.

To change the window, set `WebhookConfig.MaxTimestampAge`. Widen it only if you deduplicate on
`EventRefNum`, since that becomes your protection against replays.

## Several merchants, and webhook-only services

`MerchantIDs` is a set, so one verifier and one endpoint can serve several merchants. Use `event.MID` to pick
the matching client before you `Query`.

A verifier holds no private key. A service that only receives webhooks, such as
[`examples/lambda`](../examples/lambda/), needs only Presto's certificate and your `mid`s.
