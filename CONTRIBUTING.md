# Contributing to gmail-tui

Thanks for your interest in contributing! This document covers the development
setup, the quality gates every change must pass, and a short guide to the most
contributor-friendly area of the codebase: **themes**.

For usage and installation see the [README](README.md); for planned work see
[ROADMAP.md](ROADMAP.md).

## Development setup

Requirements:

- **Go 1.26+** (the version pinned in `go.mod` and CI)
- **golangci-lint v2.15.0** — the version CI runs; e.g. `brew install golangci-lint`

```sh
git clone <your fork URL> && cd gmail-tui
go build ./... && go vet ./...
go test -race ./...
golangci-lint run ./...
```

There is no Makefile — the four commands above are the whole toolchain. The
binary is built with `go build ./cmd/gmail-tui` (output: `./gmail-tui`).
Configuration lives at `~/.config/gmail-tui/config.toml`; see `gmail-tui doctor`
to sanity-check your local setup.

## Quality gates (non-negotiable)

CI runs the same checks on every PR:

1. **Tests**: `go test -race ./...` must be green. Development is
   **test-driven**: write the failing test first, then make it pass (the whole
   codebase was built this way, so there are tests to imitate in every package).
2. **Lint**: `golangci-lint run ./...` with **0 issues**. Enabled linters are
   `errcheck`, `govet`, `staticcheck`, `unused` (see `.golangci.yml`).
3. **Cross-platform build**: `go build ./...` with `CGO_ENABLED=0` must compile
   on Linux, macOS and Windows (see `.github/workflows/ci.yml`).

## Architecture rules

- **`MailStore` is the backend contract** (`internal/mail/store.go`). The UI
  (`internal/ui`, `internal/app`) talks **only** to the `mail.MailStore`
  interface — never to IMAP directly. IMAP/SMTP code lives in
  `internal/mail/imap` and nowhere else. A quick check:

  ```sh
  grep -rn "go-imap" internal --include="*.go" -l | grep -v internal/mail/imap
  # must print nothing
  ```

- **The UI uses theme slots only, never hard-coded colors** — see the package
  doc in `internal/theme/theme.go`. If you are adding UI code, pick a slot
  (`primary`, `error`, `unread`, …); if nothing fits, discuss it in the PR.

## Adding a theme (easy first contribution)

Themes are plain TOML files, embedded at build time. Adding one needs **no Go
code at all**:

1. Create `internal/theme/themes/<name>.toml`. The theme name is the file name
   (letters, digits and hyphens only, max 64 chars — e.g. `solarized-dark`).
2. Fill the 14 semantic slots. Each slot has a `dark` and/or a `light` value
   (a missing side falls back to the other); colors are hex `#rgb` or
   `#rrggbb`. Slugs: `fg`, `bg`, `muted`, `border`, `primary`, `accent`,
   `error`, `success`, `warning`, `unread`, `starred`, `quoted`, `help`,
   `selection`. Example (from `dracula.toml`):

   ```toml
   [fg]
   dark = "#f8f8f2"
   [bg]
   dark = "#282a36"
   [primary]
   dark = "#bd93f9"
   [error]
   dark = "#ff5555"
   # …remaining slots: see internal/theme/themes/dracula.toml
   ```

3. Validation is automatic: the table test `TestEveryEmbeddedThemeValid`
   (`internal/theme/theme_test.go`) loads **every** embedded theme and checks
   all slots, so dropping the file in is enough:

   ```sh
   go test ./internal/theme/ -run TestEveryEmbeddedThemeValid -v
   ```

4. See it live: `gmail-tui themes` lists it; `gmail-tui --theme <name>` (or
   `:theme <name>` at runtime) previews it; `gmail-tui doctor` re-validates the
   configured theme.

Dark-only is fine (most built-in themes are); light terminals then reuse the
dark values. Browse `internal/theme/themes/` for complete examples.

## Commit conventions

We follow [Conventional Commits](https://www.conventionalcommits.org): the
subject line starts with a type — `feat`, `fix`, `docs`, `test`, `refactor`,
`ci`, `chore` — optionally scoped, e.g.:

```
feat(wizard): first-run setup, login subcommand, demo seed, status polish
fix(app): prevent duplicate send while one is in flight
docs: README, smoke checklist, roadmap pointers
```

## Pull requests

1. Fork / branch, keep the change small and focused.
2. Make sure all three quality gates above pass locally.
3. Open the PR using the template checklist (tests, lint, docs, commit style).
4. CI must be green (lint + race tests + cross-OS build); a maintainer reviews
   and merges.

If the change alters user-visible behavior (keys, CLI flags, messages), update
the README keymap/CLI tables and, where relevant, `docs/smoke-checklist.md`.

## Reporting bugs and requesting features

Use the issue templates: **Bug report** (please include `gmail-tui version`,
your OS/terminal, reproduction steps and `gmail-tui doctor` output) and
**Feature request** (start from the problem/use case). Never paste your app
password in an issue; the doctor output contains no secrets, but redact your
email address if you prefer.

## Code of conduct

Be courteous and constructive. This project follows the
[Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/)
in spirit: assume good faith, keep criticism technical.

## License

By contributing you agree that your contributions are licensed under the MIT
License that covers this project (see [LICENSE](LICENSE)).
