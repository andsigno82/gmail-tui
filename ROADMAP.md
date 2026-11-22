# Roadmap

What shipped in v1.0, what is planned next, and what is under consideration.
Feature requests are welcome — open an issue and check this file first (see
[CONTRIBUTING.md](CONTRIBUTING.md) for how to contribute).

## v1.0 — shipped

The current release is a complete single-account Gmail client over IMAP/SMTP
with an App Password (no Google Cloud project, no OAuth):

- **Inbox & threads** — message list with unread/starred markers and relative
  dates; thread view with quoted-line highlighting, page scrolling and
  mark-read-on-open.
- **Navigation & mouse** — `j`/`k` and arrow keys, page keys, mouse wheel
  scrolling, click to select, double-click to open.
- **Search** — `/` with Gmail-style operators (`from:`, `is:unread`,
  `label:`, `has:attachment`) and plain-text matching over sender, subject and
  body.
- **Labels** — apply/remove labels with the `l` mini-prompt.
- **Archive & star** — `a` archives, `s` toggles the star.
- **Composer** — compose new (`c`), reply (`r`) and forward (`f`) with
  pre-filled fields; Tab-navigated fields, Cc toggle (`Ctrl+E`), attachments,
  pre-send confirmation.
- **Sending** — SMTP via the Gmail preset (STARTTLS on port 587); markdown
  bodies (sent as `text/plain` for now).
- **Attachments** — save with `:save <n> [path]`, open in the OS viewer with
  `:open <n>`; `o` opens the HTML body in the browser.
- **Themes** — 12 embedded themes, selectable via config, `--theme` flag or
  `:theme <name>` at runtime (instant preview, persisted to config); adding a
  theme is one TOML file (see [CONTRIBUTING.md](CONTRIBUTING.md)).
- **First-run wizard & login** — `gmail-tui` on a fresh machine walks through
  the App Password setup with a real connection test; `gmail-tui login`
  re-runs it. Passwords are stored in the system keychain (with
  `GMAIL_APP_PASSWORD` as override).
- **Live updates** — IMAP IDLE push: new mail appears while the app is open,
  with automatic reconnect on network loss.
- **Cache** — on-disk message cache for fast restarts.
- **CLI** — `gmail-tui version`, `themes`, `doctor`, `fetch --limit/--query`,
  `send` (stdin body, `--to/--cc/--bcc/--subject/--attach`), `login`.
- **Demo mode** — without credentials the app runs with seeded mail, so the
  UI can be tried before configuring anything.

## v1.1 — planned

- **Markdown → HTML multipart sending** — the composer edits markdown but
  currently sends `text/plain` only; generate the `text/html` part and send
  multipart/alternative.
- **Send-only CLI completion** — allow `gmail-tui fetch`/`send` to work
  without a config file when the password is only in the keychain (resolve
  the address from the wizard instead of an empty user).
- **Recipient validation** — validate `To`/`Cc`/`Bcc` in the composer and the
  `send` CLI before the SMTP round-trip (today a malformed address only
  surfaces as a server error after sending is already under way).

## Later — under consideration

- **Gmail API backend** — an alternative store/sender implementation behind
  the existing `MailStore` abstraction, for accounts where App Passwords are
  unavailable (OAuth device flow). The interface split (`internal/mail`) was
  designed for this.
- **Multi-account** — multiple profiles in one binary (per-account config,
  keychain entries and cache).

Ideas and feedback: open a [feature request](
.github/ISSUE_TEMPLATE/feature_request.md).
