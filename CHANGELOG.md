# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.1.0]: https://github.com/andsigno82/gmail-tui/releases/tag/v0.1.0
