# Presto Pay SDK (Go) — Lambda webhook receiver

An AWS Lambda function that verifies Presto webhook deliveries behind API Gateway. Unlike the
[net-http](../net-http/) and [chi](../chi/) demos, this one only verifies webhooks — a verifier needs no private
key, so the function holds no signing material at all, only Presto's public key and the accepted merchant IDs.

This example has its own `go.mod` with a real third-party dependency (`aws-lambda-go`) and a newer Go
requirement than the SDK itself (`aws-lambda-go` currently requires Go 1.26); the SDK module stays
dependency-free, and a `replace` directive here points at the local SDK source rather than a published version.

## What it does

`handleRequest` reads the raw request body from the API Gateway event and calls `verifier.Verify(body)` — the
same verifier used by the net/http and chi demos, just fed a byte slice instead of an `*http.Request`, since
`Verify` exists exactly for consumers (queues, Lambda events) that have already buffered the body. It logs the
verified event and acknowledges with `AckOK`/`AckForError`, reusing `WriteAck` by recording it into an
`httptest.ResponseRecorder` and copying the status/body into the Lambda response — the notify-ack contract stays
defined in one place regardless of which example calls it.

A real deployment would look up `event.EventRefNum` in a table with a uniqueness constraint before doing any
fulfilment work, since Presto redelivers an unacknowledged webhook up to five times; this demo only logs.

## Build and package

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o bootstrap .
zip function.zip bootstrap
```

Deploy `function.zip` as a Lambda function with the `provided.al2023` runtime (or any runtime supporting a
custom `bootstrap` executable), architecture `arm64`, and an API Gateway HTTP API trigger with proxy integration
pointed at whatever path you configure as the webhook's `notifyUrl` (e.g. `/presto/notify`).

## Configuration

Set these as the function's environment variables (via the AWS console, CLI, SAM, CDK, or Terraform — however
you manage the rest of the stack):

| Variable | Description |
|----------|--------------|
| `PRESTOPAY_MID` | Merchant ID(s) this function accepts webhooks for |
| `PRESTOPAY_PUBLIC_KEY` or `PRESTOPAY_PUBLIC_KEY_FILE` | Presto's public certificate (PEM or DER), or SPKI PEM. `_FILE` must point at a path bundled into the deployment package (e.g. `/var/task/presto-public-key.der`), since there is no persistent filesystem to read from otherwise |

## Testing locally

`lambda.Start` expects the Lambda Runtime API, so `go run .` on its own won't serve anything useful. Either:

- Call `handleRequest` directly from a Go test with a hand-built `events.APIGatewayProxyRequest`, or
- Use `sam local invoke` (AWS SAM CLI) with a sample event file containing a signed webhook body.
