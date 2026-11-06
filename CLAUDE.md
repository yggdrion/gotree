# CLAUDE.md

Go TUI (Bubble Tea). `main.go` in root; code in `internal/repo` (git logic) and `internal/ui` (model/view/menu).

## Before every commit/push

Run all of these locally; CI runs the same and fails the PR otherwise:

```sh
gofmt -l .
go vet ./...
go test ./...
golangci-lint run ./...   # config: .golangci.yml; CI uses latest v2
```

If `golangci-lint` is not on PATH, run it without installing:

```sh
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest run ./...
```

Never push with lint findings (e.g. `unused` on test helpers copied between packages).
