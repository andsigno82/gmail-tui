# Roadmap

What shipped in v0.1.0, what is planned next, and what is under consideration.
Feature requests are welcome — open an issue and check this file first (see
[CONTRIBUTING.md](CONTRIBUTING.md) for how to contribute).

## v0.1.0 — shipped

The first release is a complete single-account Gmail client over IMAP/SMTP
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
  bodies (plain text in v0.1.0; the HTML part came later).
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

## Unreleased — delivered

Everything below is on `main` and described in the [CHANGELOG](CHANGELOG.md),
but is not in a release yet.

- **Inbox paging** — the list loads 50 conversations at a time; reaching the
  last row loads the next 50 (not during search).
- **Folders and labels** — `m` opens a picker with the Gmail folders (by
  SPECIAL-USE role, so localized names work) and labels, as a tree with
  nested labels and unread counts.
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
- **Toast feedback** — the outcome of an action is a toast in the top-right
  corner: info and success disappear after 3 seconds, errors stay until a key
  is pressed.
- **Gmail conversations** — threads by `X-GM-THRID`, Gmail search syntax with
  `X-GM-RAW`, per-message labels with `X-GM-LABELS`, via an in-tree patch of
  go-imap (see `third_party/go-imap/PATCHES.md`). The thread view shows the
  whole conversation, your replies included. Servers without `X-GM-EXT-1`
  keep one row per message.
- **Label panel** — `l` lists all labels with a check on those present on
  every target and a mixed mark on those present on some; additions and
  removals are applied together, from any folder.
- **Drafts and contacts** — `Esc` in the composer saves the message to the
  Drafts folder, `Ctrl+D` discards it after confirmation, and opening a draft
  resumes it in the composer. Address completion from a local address book
  (addresses already seen, ordered by frequency and recency) plus the optional
  `address_book_cmd` command. `Ctrl+E` shows Cc and Bcc.
- **Undo send** — the message is saved to Drafts at once and sent after 5
  seconds; `u` in the inbox or thread brings the composer back on that draft.
- **Bulk actions and undo** — `x` marks the row under the cursor, `X` all
  loaded rows, `Esc` clears; `a`, `#`, `s`, `v` and `l` act on the whole
  selection. `u` undoes the last archive or trash (toast for 10 seconds).
- **Desktop notifications** — when new mail arrives in INBOX, the
  `notify_command` from the config runs once per batch; sender, subject and
  count reach it as environment variables, never through a shell.
- **MCP server, read-only** — `gmail-tui mcp` serves `list_folders`,
  `list_recent`, `search_messages`, `read_message` and `read_thread` over
  stdio for AI agents. No tool sends, drafts or modifies anything, and every
  result wraps the email content in untrusted-content markers.
- **Rich rendering and layout** — the HTML body is drawn as blocks: buttons,
  tables, quotes, lists and attachment chips. A link whose text shows another
  domain gets a warning and asks before opening. Floating windows with a
  shadow, a segmented status bar and a splash at startup.
- **Local cache and snapshots** — the last 200 opened messages are kept in a
  local cache (the least recently used is dropped first), with snapshots of
  the first page of each folder and of each opened conversation. At startup
  the list appears at once and stays read-only until Gmail confirms it.
- **Timing trace** — `GMAIL_TUI_TRACE=1` writes a trace of timings (mail
  operations with the IMAP gate wait, `Update`/`View`, startup phases);
  `gmail-tui trace` summarizes it.
- **Message encoding** — the text parts of outgoing messages declare their
  `Content-Transfer-Encoding`: `7bit`, or `quoted-printable` for accents and long
  lines.
- **Important folder** — "Importanti" sits with the special folders, right after
  Inbox.
- **Trace steps** — each IMAP command of the list, thread and body read path is
  measured (`tcp`, `tls`, `login`, `list`, `select`, `search`, `fetch`), together
  with the steps of `Watch` and the LIST of the other operations, and summarized by
  `gmail-tui trace`; the STORE, MOVE and APPEND of mutations, the STATUS and SEARCH
  of the unread counts and `AllMailIDs` are not traced.
- **Connect deadline** — when the 90 s budget expires, the dial and the login are
  interrupted and the connection closed. A caller that sets no deadline gets the
  same budget.

## Next — planned

- **Send-only CLI completion** — allow `gmail-tui fetch`/`send` to work
  without a config file when the password is only in the keychain (resolve
  the address from the wizard instead of an empty user).
- **Incremental sync and performance (Piano 12b)** — designed on a real-account
  trace: CONDSTORE-based refresh, fewer IMAP round trips in the list, paged search.

## Later — under consideration

- **Gmail API backend** — an alternative store/sender implementation behind
  the existing `MailStore` abstraction, for accounts where App Passwords are
  unavailable (OAuth device flow). The interface split (`internal/mail`) was
  designed for this.
- **Multi-account** — multiple profiles in one binary (per-account config,
  keychain entries and cache).

Ideas and feedback: open a [feature request](
.github/ISSUE_TEMPLATE/feature_request.md).
