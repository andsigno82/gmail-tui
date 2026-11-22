## What

<!-- One or two sentences: what does this PR change? -->

## Why

<!-- Link the issue: Fixes #123 — or explain the motivation. -->

## Checklist

- [ ] Tests written first (TDD) and `go test -race ./...` is green
- [ ] `golangci-lint run ./...` reports 0 issues
- [ ] `go build ./... && go vet ./...` are clean
- [ ] UI changes use theme slots only (no hard-coded colors) and touch IMAP only through `mail.MailStore`
- [ ] Docs updated if user-visible: README (keymap/CLI tables), CONTRIBUTING, `docs/smoke-checklist.md`
- [ ] Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`, …)
