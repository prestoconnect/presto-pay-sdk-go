# Changelog

All notable changes to this project are documented in this file.

## Unreleased

### Changed

- The README is now a getting-started guide: creating your key pair and sending Presto the `.der` public key,
  how a payment flows, a four-step quick start and a payment status table. Reference material moved to
  `docs/` (payments and errors, webhooks, production).
- The examples answer HTTP 401 to a webhook that fails with a `*SignatureError`, instead of acknowledging it.
- `LoadPrivateKey`'s hint for a non-PEM input no longer assumes the key came from an onboarding keystore.

## 0.3.0 - 2026-09-30

### Changed

- **Breaking:** `NotifyEvent.PaymentStatus` is removed. A webhook reports what happened (`EventCode`, `Success`),
  not the payment's resulting status, and deriving one was guesswork: a `Refunded` or `Reversed` event with
  `Success: false` is a refund or reversal that failed, leaving the payment in its previous status, which the
  event does not carry. Call `Payments.Query` for the current status. This follows the shared wire contract.
- The `net-http` and `chi` examples query the payment in their webhook handler and list the returned status.
  They mark `EventRefNum` as seen only after the query succeeds, so a failed query's redelivery is not dropped as
  a duplicate.

### Fixed

- `AckForError` answered `AckOK` for any `*SignatureError` or `*ResponseError`, including ones from a `Query`
  made inside the webhook handler, which told Presto to stop redelivering an event the handler never processed.
  It now answers `AckOK` only for errors whose `Source` is `"webhook"`, and `AckResend` for everything else.

## 0.2.0 - 2026-09-28

### Changed

- The staging smoke test now also exercises `Reverse` and `Refund` on a `PendingAuthorise` payment. This found
  another wrong assumption: `Reverse` on a payment that was never paid **succeeds** and cancels it
  (`PaymentStatus` becomes `Cancelled`) rather than failing — there is nothing to reverse financially if nothing
  was ever paid. `Refund` in the same state is correctly rejected with error `1227`.
- README: a `PendingAuthorise` payment expires (`PaymentStatus` becomes `Expired`) 15 minutes after `Init` if not
  finalised by then — a confirmed business rule. Once `Expired`, `Reverse` fails with error `1219` rather than
  cancelling it. (An earlier version of this entry guessed error `1221`; that guess was wrong.)
- `ErrorCodeGuestAccountRefundNotSupported` (`1242`), captured when reversing a guest-checkout `AffinBank`
  payment. Refunds can be requested for any payment method; their outcome depends on the method, and some
  require manual or offline processing.

## 0.1.0 - 2026-09-28

First tagged release.

### Added

- Wire-core primitives: canonicalization, gateway timestamps (fixed UTC+08:00, three fractional digits), PKCS#8
  private-key and X.509/SPKI public-key loading, `RSASSA-PKCS1-v1_5`/SHA-256 sign and verify.
- The send path: whole-call deadline, jittered `RetryReads` backoff, and `httptrace`-proven `RequestNotSent`
  classification, proven against real sockets (closed port, unresolvable host, reset-after-body-sent, blackholed
  connect, deadline abort).
- The four payment operations (`Init`, `Query`, `Reverse`, `Refund`): request validation, wire-body construction
  (including the stringified-array fields and UTF-8 rejection on outgoing strings), response mapping with the
  echo check, and `MayHaveTakenEffect`/`ReconcileBy` stamped per operation and per error type.
- The `Raw` escape hatch (`Post`/`Sign`/`VerifyBody`) for gateway endpoints not yet wrapped by the SDK.
- Webhook verification (`NewWebhookVerifier`, `Verify`, `VerifyRequest`), following the gateway's confirmed
  order (signature before `mid`, `mid` before freshness), and `Ack`/`AckOK`/`AckResend`/`WriteAck`/`AckForError`
  for the notify reply contract.
- `ConfigFromEnv` for building a `Config` from `PRESTOPAY_*` environment variables, including the `_FILE`
  variants for keys.
- `1005` business errors report the observed clock offset between the request and response timestamps
  (`APIError.ClockOffset`); `1006`/`1007` attach the canonical string that failed to verify (`APIError.Canonical`).
- `examples/net-http/`: a runnable demo against Presto's real staging gateway using only `net/http` and
  `html/template`.
- `examples/chi/`: the same demo routed with `chi` instead of the stdlib mux.
- `examples/lambda/`: a webhook-only receiver deployed as an AWS Lambda function behind API Gateway.
- A staging smoke test (`prestopay/staging_test.go`, `-tags staging`, gated behind `PRESTOPAY_STAGING_SMOKE=1`)
  that calls the real gateway: `Init`, an immediate `Query`, and a duplicate `Init` confirming Presto's own
  idempotent-by-`TxnRefNum` behavior (returns the existing payment's current status rather than creating a
  second record).
- `LICENSE` (Apache 2.0, matching the Java and JS SDKs), `CONTRIBUTING.md`, and `SECURITY.md`.

### Known limitations

- No PKCS#12 support — the onboarding keystore must be converted once with `openssl` (documented in the README
  and in the key-loading error messages).
- The shared wire-contract test vectors are not yet vendored into `spec/`; contract tests are hand-written against
  captured request/response examples instead.
