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
  bodies (plain text in v1.0, HTML part since v1.1).
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

## v1.1 — delivered

- **Inbox paging** — the list loads 50 messages at a time; reaching the last
  row loads the next 50 (not during search).
- **Folders and labels** — `m` opens a picker with the Gmail folders (by
  SPECIAL-USE role, so localized names work) and labels.
- **Trash** — `#` moves a message to the Trash after confirmation; refused on
  servers without MOVE or UIDPLUS (no generic EXPUNGE).
- **Recipient validation** — `To`/`Cc`/`Bcc` are checked in the composer and by
  the `send` CLI before the SMTP round-trip.
- **Markdown sending** — composed messages go out as `multipart/alternative`
  (Markdown source as plain text, GFM-rendered HTML); `gmail-tui send --markdown`.
- **Settings menu and two-column layout** — `,` opens theme, layout,
  mark-as-read, name and signature; `layout = "split"` shows a message preview
  beside the list from 120 columns.
- **Readable colors** — text on colored bars is computed to reach WCAG AA
  contrast on every theme.

## Unreleased — delivered

- **Gmail conversations** — threads by `X-GM-THRID`, Gmail search syntax with
  `X-GM-RAW`, per-message labels with `X-GM-LABELS`, via an in-tree patch of
  go-imap (see `third_party/go-imap/PATCHES.md`). Servers without `X-GM-EXT-1`
  keep one row per message.

## v1.1 — planned

- **Send-only CLI completion** — allow `gmail-tui fetch`/`send` to work
  without a config file when the password is only in the keychain (resolve
  the address from the wizard instead of an empty user).

## Next — planned

- **Drafts and contacts** — save/resume drafts in the Gmail Drafts folder;
  address completion from previously seen addresses plus an optional external
  address-book command.
- **Bulk actions and undo** — mark several messages and archive/trash/label
  them at once; undo the last archive or trash.
- **Desktop notifications** — a configurable command run on new mail (sender
  and subject passed as environment variables).
- **Incremental sync** — CONDSTORE-based refresh and paged search.

## Later — under consideration

- **Gmail API backend** — an alternative store/sender implementation behind
  the existing `MailStore` abstraction, for accounts where App Passwords are
  unavailable (OAuth device flow). The interface split (`internal/mail`) was
  designed for this.
- **Multi-account** — multiple profiles in one binary (per-account config,
  keychain entries and cache).

Ideas and feedback: open a [feature request](
.github/ISSUE_TEMPLATE/feature_request.md).
