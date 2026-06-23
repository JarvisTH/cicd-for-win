# Contributing

## Project structure

- `cmd/ci/main.go` — Entry point
- `internal/runner/` — Core CI engine (check/build/test/push/deploy/cache)
- `internal/serve/` — Web UI backend + frontend
- `internal/config/` — Configuration loading
- `internal/security/` — Encryption utilities
- `internal/sshutil/` — SSH/SFTP utilities
- `internal/cmd/` — CLI command definitions
- `internal/output/` — CLI output formatting
- `internal/util/` — Shared utilities

## Adding a new project type

1. `internal/runner/detect.go` — Add constant + detection case
2. `internal/runner/check.go` — Add check case
3. `internal/runner/build.go` — Add build case
4. `internal/runner/test.go` — Add test case
5. `internal/runner/deploy.go` — Add deploy commands

## Running tests

```bash
go test ./... -count=1
```

## Building

```bash
go build -o ci.exe ./cmd/ci/
```
