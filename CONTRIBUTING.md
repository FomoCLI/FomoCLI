# Contributing

Thanks for helping make `fomocli` better!

## Setup

```sh
git clone https://github.com/FomoCLI/FomoCLI && cd FomoCLI
make build      # ./fomo
make test       # go test -race ./...
make lint       # go vet + gofmt check
./fomo --demo lb
```

No API key is needed for development: the test suite and `--demo` run against
an in-process mock (`internal/mock`) that serves payloads shaped like the
examples in the [FOMO API docs](https://fomoapi.io/docs).

## Layout

| Path | What lives there |
|------|------------------|
| `cmd/fomocli` | `main` - just calls `cli.Execute()` |
| `internal/cli` | Cobra commands and their renderers |
| `internal/api` | HTTP + WebSocket client, credit tracking, cache modes |
| `internal/cache` | On-disk response store behind `--offline` |
| `internal/config` | `config.json` and environment variables |
| `internal/ui` | Palette, number formatting, tables, banner, spinner |
| `internal/jx` | Nil-safe accessor for decoded JSON |
| `internal/mock` | Mock API + fixtures for tests and `--demo` |

## Guidelines

- Read payload fields through `jx` so a renamed or null field degrades to `—`
  instead of crashing.
- Every command must support `--json` (raw API payload on stdout) and keep
  stdout clean - footers, spinners and warnings go to stderr.
- Add a fixture and a case in `internal/cli/cli_test.go` for new endpoints.
- Use [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:` ...).

## Releasing

Tag `vX.Y.Z` and push the tag; the release workflow runs GoReleaser and
publishes binaries for Linux, macOS and Windows (amd64 + arm64).
