# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Toast notifications** — action outcomes appear as toasts in the top-right corner, drawn with the lipgloss v2 compositor: info and success disappear after 3 s, errors stay until a key is pressed, up to 3 stacked (errors are never evicted by newer toasts). The status bar is neutral and keeps only lasting state; it is colored only for pending confirmations.
- **Settings menu** — `,` opens a menu in the inbox and thread views: theme (live preview), reading layout, mark-as-read in preview, name and signature. `Enter` on a choice saves, `Esc` cancels and restores the previous theme and layout; `Ctrl+S` saves from any row.
- **Two-column layout** — `layout = "split"` shows the list and a message preview side by side from 120 columns (list 35%, preview the rest). `Tab` moves focus to the preview; `preview_mark_read` (`immediate`, `delay`, `never`) sets when a previewed unread message is marked read. Click on the preview focuses it; double-click or `Enter` opens the message full screen.
- **Markdown sending** — composed messages are sent as `multipart/alternative`: the plain-text part is the Markdown source, the HTML part is rendered with GitHub-flavored Markdown (tables, strikethrough, task lists). Raw HTML typed in a message is shown as escaped text. `gmail-tui send --markdown` does the same for stdin.
- **Sender name** — the `name` setting is shown in the From header as `Name <address>` (RFC 2047 when not ASCII).
- **Config keys** — `layout` and `preview_mark_read`, with the defaults `single` and `delay`.

- **Inbox paging** — the list loads 50 messages at a time; moving the cursor to the last row loads the next 50. Search results are not paginated.
- **Folders and labels** — `m` opens a picker with the Gmail folders (by IMAP SPECIAL-USE role, so localized names work) and labels; choosing a folder clears the active search.
- **Trash** — `#` moves the message to the Trash after confirmation (`s`); from a thread it returns to the list. Refused on servers without MOVE or UIDPLUS.
- **Recipient validation** — the composer blocks the send confirmation for a missing or malformed recipient; `gmail-tui send` exits 1 on the same errors.
- **Demo** — seeded Sent folder and a `Work` label.
- **`doctor` IMAP check** — with an address and a password (env or keychain), `doctor` connects and checks that the server announces MOVE, UIDPLUS, SPECIAL-USE and LIST-EXTENDED.
- **Internationalized domains** — recipient domains are converted to punycode before sending.

### Changed

- `gmail-tui login` updates the address and keeps `name`, `signature`, `theme`, `layout` and `preview_mark_read`; before, it reset name and signature and set the theme to `default`.
- An invalid `layout` or `preview_mark_read` value is reset to its default for that field only (warning on stderr at startup); the other fields are kept, and saving from the settings menu or `:theme` repairs the same way.
- The first-run wizard also starts when no password is available and the config has no address.

- `a` (archive) works only on messages in INBOX; elsewhere it shows a status message. The check uses the selected message's mailbox, also for search results from other folders.
- `l` refuses a label equal to the selected message's mailbox (it would duplicate the message).
- Recipient domains must contain a dot (`a@b` is rejected, as Gmail does).
- `gmail-tui send` parses `--to/--cc/--bcc` as RFC 5322 address lists, so display names with commas work.
- IDLE push watches INBOX only; other folders are refreshed after operations.

### Fixed

- Text on colored bars (status bar, OK and error messages, selected row, demo banner) had 1.0–3.5:1 contrast in some themes. The text color is now computed per theme and variant to reach WCAG AA (4.5:1), falling back to black or white.

- IDLE reselects INBOX before each round, so push keeps working after operations on other folders.
- Recipients with a display name (`Mario <m@x.it>`) are sent to SMTP `RCPT TO` as the bare address; before, the server rejected them.
- `doctor` no longer reports the password as missing when it is in the keychain.

## [0.1.0] - 2026-11-22

Initial release: a complete terminal Gmail client built on the Charm stack — IMAP IDLE push, composer with reply/forward, 12 themes, and a keychain-only credential flow. This entry summarizes the ~54 conventional commits of the first development cycle; infra and docs commits are folded into the entries below.

### Added

- **Terminal UI** — inbox list and thread reading view (Bubble Tea), status bar with a context-sensitive help bar, full help overlay (`?`), and mouse support (wheel scroll, click select, double-click open).
- **IMAP backend** — IDLE watch with automatic reconnect and a change feed driving live refresh; mutual-exclusion gating so store operations break IDLE safely instead of killing the connection.
- **Message cache** — bbolt cache with uidvalidity invalidation; cache-first message and attachment fetch, with stale-result guarding in the UI.
- **Search** — Gmail-style server-side search: `from:`, `is:unread`, `label:`, `has:attachment`, plus plain-word text matching.
- **Mail actions** — read/unread toggle, star, archive, and label apply/remove, all with status-line feedback.
- **Composer** — markdown textarea with reply/forward builders, Cc toggle, and a send confirmation; duplicate sends are prevented while one is in flight (single-flight). Adapted from [charmbracelet/pop](https://github.com/charmbracelet/pop) (MIT).
- **Attachments and HTML** — `:save <n> [path]` and `:open <n>` in the thread view; `--attach` on `gmail-tui send`; `o` opens any HTML part in the browser.
- **Command mode** — `:` commands in the thread view: `:save`, `:open`, `:theme <name>` (immediate theme preview, persisted to the config).
- **Themes** — 12 embedded TOML themes (default, 4× catppuccin, dracula, one-dark/light, nord, 2× gruvbox, tokyo-night) with semantic slots and validation; adding a theme is one TOML file, no test changes needed.
- **Setup** — first-run wizard and `gmail-tui login`, storing the App Password in the **system keychain** via the OS keychain CLI (resolution order: `GMAIL_APP_PASSWORD` env → keychain → demo mode).
- **Demo mode** — with a config but no credentials, the app starts offline with seeded mail and a banner.
- **CLI** — `gmail-tui fetch|send|themes|doctor|version` (`doctor` diagnoses config, theme, password and cache).
- **Project infrastructure** — CI (golangci-lint, race tests, Linux/macOS/Windows build matrix), GoReleaser release pipeline with Homebrew tap placeholder, README, CONTRIBUTING guide, issue/PR templates.

### Changed

- Nothing — this is the first release; every entry above is new.

### Security

- **App Password only** — Gmail 2-Step Verification and an App Password are required; regular passwords do not work with IMAP/SMTP, and **no OAuth tokens** are requested, stored, or supported in v1.
- **Credentials never in config or logs** — the App Password lives only in the system keychain or the `GMAIL_APP_PASSWORD` env var; it is never written to `config.toml` and never logged, and keychain-related errors are masked so the secret cannot leak through them.
- **Encrypted transport only** — IMAP over implicit TLS (`imap.gmail.com:993`) and SMTP over STARTTLS (`smtp.gmail.com:587`), the fixed Gmail preset.

[Unreleased]: https://github.com/andsigno82/gmail-tui/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/andsigno82/gmail-tui/releases/tag/v0.1.0
