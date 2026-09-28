# Presto Pay SDK (Go) — chi demo

The same checkout demo as [../net-http/](../net-http/), routed with [chi](https://github.com/go-chi/chi)
instead of the stdlib `http.ServeMux`. The SDK itself has no opinion about which router calls it — every
`client.Payments.*`/`prestopay.NewWebhookVerifier` call here is identical to the net/http version; only the
routing and URL-parameter extraction (`chi.URLParam`) differ, plus `chi`'s `Logger`/`Recoverer` middleware.

Unlike the main SDK module, this example has its own `go.mod` with real third-party dependencies (`chi`) — the
SDK module itself stays dependency-free. It depends on the tagged `v0.2.0` release, the same way any other
consumer would.

## Run it

```bash
cp .env.example .env   # then edit .env with your staging credentials
go run .
```

Open `http://localhost:8081/` (`$PORT` to change the port; defaults differ from the net/http demo so both can
run side by side).

## What it demonstrates

Identical routes and behavior to [../net-http/](../net-http/) — see that README for the full route table,
`curl` examples, and how to see webhooks actually fire via a tunnel.

## Configuration

See `.env.example`. `PRESTOPAY_ENV`, `PRESTOPAY_MID`, `PRESTO_MRN`, `PRESTOPAY_PRIVATE_KEY_FILE`, and
`PRESTOPAY_PUBLIC_KEY_FILE` are required; `PORT` and `PUBLIC_URL` are optional.
