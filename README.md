# gmail-tui

[![CI](https://github.com/andsigno82/gmail-tui/actions/workflows/ci.yml/badge.svg)](https://github.com/andsigno82/gmail-tui/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

🌐 **[Landing page](https://andsigno82.github.io/gmail-tui/)** — screenshots, themes showcase *(after the first push + Pages setup)*

Docs: [CHANGELOG](CHANGELOG.md) · [SECURITY](SECURITY.md) · [CONTRIBUTING](CONTRIBUTING.md)

**A Gmail client for your terminal, built with Charm.**

Inbox, threads, search, compose/reply/forward with attachments, labels, themes and IMAP IDLE push — no Google Cloud project, no OAuth: just an App Password.

<!-- Demo GIF — regenerate with: vhs docs/demo.tape -->
![demo](docs/demo.gif)

## Requirements

- A Gmail account with **2-Step Verification** and an **App Password** — create one at <https://myaccount.google.com/apppasswords> (regular passwords do not work with IMAP/SMTP).
- A terminal of at least **80×24** columns (truecolor recommended).
- Go 1.26+ to build from source.

## Quickstart

```sh
# 1. Install (any one of)
brew install andsigno82/gmail-tui/gmail-tui                       # placeholder tap — created after the first release
go install github.com/andsigno82/gmail-tui/cmd/gmail-tui@latest
git clone https://github.com/andsigno82/gmail-tui && cd gmail-tui && go build ./cmd/gmail-tui

# 2. Configure — first-run wizard (stores the password in the system keychain)
gmail-tui login
#    or set the password yourself: export GMAIL_APP_PASSWORD=xxxx

# 3. Run
gmail-tui
```

Without a config file the app launches the **first-run wizard**; with a config but no password it starts in **demo mode** with seeded mail and a banner.

## Keymap

| Key | Action |
|---|---|
| `j`/`k`, `↑`/`↓` | move cursor (list) / scroll (thread) |
| `Enter` | open selected message |
| `Esc` / `q` | back / quit |
| `Ctrl+C` | quit from anywhere |
| `/` | search — Gmail-style: `from:x is:unread label:Work has:attachment`, plain words match text |
| `r` / `f` / `c` | reply / forward / compose new |
| `a` | archive (Inbox only) |
| `#` | move to trash — asks for confirmation (`s` confirms, `Esc` cancels); from a thread it returns to the list |
| `m` | folders and labels picker (`Enter` opens, `Esc` closes); choosing a folder clears the active search |
| `s` | star on/off |
| `v` | mark read/unread |
| `l` | apply/remove label (refused for the current folder) |
| `o` | open HTML body in browser (thread view) |
| `:` | commands (thread view): `:save <n> [path]`, `:open <n>`, `:theme <name>` |
| `pgup`/`pgdn`/`space`/`d`/`u` | page scroll (thread view) |
| `?` | full help overlay |
| mouse | wheel = scroll, click = select, double-click = open |

Composer: `Tab`/`Shift+Tab` between fields · `Ctrl+E` show/hide Cc · `Ctrl+S` send (confirmation first) · `Esc` cancel. The status line always shows a context-sensitive help bar; `?` opens the full overlay in the inbox and thread views (it closes with `Esc`/`?`).

## Themes

Twelve embedded themes: `default`, `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `dracula`, `one-dark`, `one-light`, `nord`, `gruvbox-dark`, `gruvbox-light`, `tokyo-night`.

Select with `theme = "…"` in the config, the `--theme` flag, or `:theme <name>` at runtime (immediate preview, persisted to config); `gmail-tui themes` lists them all. Adding a theme is one TOML file — no test changes needed (the table-driven test validates it automatically) — see [CONTRIBUTING.md](CONTRIBUTING.md).

## CLI

| Command | Description |
|---|---|
| `gmail-tui` | launch the TUI (wizard on first run, demo mode without credentials) |
| `gmail-tui login` | re-run the setup wizard (rewrites the config — see Notes) |
| `gmail-tui fetch [--limit N] [--query "…"]` | list matching messages (default limit 20) |
| `gmail-tui send --to a@b.com [--cc …] [--bcc …] --subject "…" [--attach f]... < body` | send an email read from stdin |
| `gmail-tui themes` | list available themes |
| `gmail-tui doctor` | diagnose config, theme, password and cache; with credentials, also checks that the IMAP server announces MOVE, UIDPLUS, SPECIAL-USE and LIST-EXTENDED |
| `gmail-tui version` | print the version |

Password resolution (TUI and CLI alike): `GMAIL_APP_PASSWORD` env → system keychain → absent (demo mode).

## Notes & limitations (v1)

- **Pagination**: the list loads 50 messages at a time; moving the cursor to the last row loads the next 50. Search results are not paginated.
- **IDLE push watches INBOX only**: new mail in other folders is not pushed; the refresh after an operation still applies.
- **Archive only in Inbox**: `a` is refused with a status message when the selected message is not in INBOX (also for search results from other folders).
- **Recipient validation**: the composer does not open the send confirmation without a recipient or with a malformed address; `gmail-tui send` exits 1 on the same errors. The domain must contain a dot; internationalized domains are sent in punycode; non-ASCII local parts (SMTPUTF8) are not supported. Display names are allowed (`"Rossi, Mario" <m@x.it>`).
- **`gmail-tui login` rewrites the config**: your address is kept, but `name` and `signature` are reset to empty (and theme to `default`). Back up `~/.config/gmail-tui/config.toml` before re-running it.
- **IMAP/SMTP hosts are fixed to the Gmail preset** (`imap.gmail.com:993`, `smtp.gmail.com:587` STARTTLS) and not configurable in v1.
- **Markdown bodies are sent as `text/plain`** — the composer is a markdown textarea but generates no HTML part yet (v1.1: markdown → HTML multipart).
- The composer is adapted from [charmbracelet/pop](https://github.com/charmbracelet/pop) (MIT).
- An external `SIGINT` exits without cancelling the IMAP IDLE watcher (harmless — the process is exiting anyway); use `q`/`Ctrl+C` inside the app for a clean exit.

## Roadmap

- **v1.1** — delivered: paging, folders and labels picker, trash, recipient validation. Planned: markdown → HTML multipart sending, CLI send-only completion
- **Later** — Gmail conversations (`X-GM-THRID`/`X-GM-RAW`, needs a go-imap patch), optional Gmail API backend behind the existing `MailStore` abstraction

See [ROADMAP.md](ROADMAP.md).

## Development

```sh
go build ./... && go vet ./...
go test -race ./...
golangci-lint run ./...
```

Contributions welcome — see [CONTRIBUTING.md](CONTRIBUTING.md). A manual smoke checklist for release testing lives in [docs/smoke-checklist.md](docs/smoke-checklist.md).

## License

MIT — see [LICENSE](LICENSE).
