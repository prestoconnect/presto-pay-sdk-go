# Contributing

Thanks for looking at `presto-pay-sdk-go`. This covers building, testing, and releasing the library itself —
for how to *use* the SDK in your own project, see [README.md](README.md).

## Building and testing

```bash
go build ./...
go vet ./...
go test -race ./...
```

CI (`.github/workflows/ci.yml`) runs this on Go 1.24, 1.25, and 1.26, plus separate `lint` (`golangci-lint`),
`vulncheck` (`govulncheck`), and `apidiff` (`gorelease`, catches accidental breaking changes to the public API)
jobs.

Run the linter locally before pushing:

```bash
go run github.com/golangci/golangci-lint/cmd/golangci-lint@latest run
```

Optional live staging smoke test (requires real staging credentials — see `prestopay/staging_test.go`'s own doc
comment for what it checks):

```bash
PRESTOPAY_STAGING_SMOKE=1 go test -tags staging ./prestopay/... -run TestStagingSmoke -v
```

## Code style

- Go 1.24+ language level, no build tags or cgo in the SDK module itself.
- Zero new runtime dependencies in the SDK module (`go.mod` at the repo root) unless explicitly agreed — the
  product goal is zero-deps. `examples/chi` and `examples/lambda` are separate modules and may take real
  dependencies.
- Match existing naming and error-handling patterns in `prestopay/`.
- `golangci-lint run` must be clean; fix findings rather than suppressing them where reasonable.

## Releasing

1. Set `Version` in `prestopay/version.go` to the release version (no `-dev` suffix) and move the CHANGELOG
   `Unreleased` entries under that version.
2. Commit, then push a matching tag, e.g. `git tag v0.3.0 && git push origin v0.3.0`.
3. pkg.go.dev indexes the new version automatically the first time it (or the module proxy) is fetched —
   no separate publish step, unlike Maven Central or npm.
4. Bump `Version` back to the next `-dev` suffix, e.g. `0.3.0-dev` after releasing `v0.2.0`.
