# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **Important folder** — `[Gmail]/Important` (`\Important`) is listed with the special folders, right after Inbox, as "Importanti". Before, it appeared nested under a "[Gmail]" branch among the labels.
- **Trace steps** — with `GMAIL_TUI_TRACE=1` each IMAP command of the list, thread and body read path is a `step` event (`tcp`, `tls`, `login`, `list`, `select`, `search`, `fetch`); the unread counts and `AllMailIDs` are not broken down. `gmail-tui trace` adds a "Passi IMAP" table (count, median, p95, max and failed runs per operation, step and folder role) and shows the steps of the first Connect in the startup timeline.
- **Local cache and snapshots (Piano 16)** — the list of the last folder appears at once at startup, from a local snapshot of its first page, while Gmail connects; the status bar shows `aggiornamento…` with the age of the data. Opening a conversation seen before shows it at once too. Snapshot data is read-only until the fresh list arrives: archive, trash, star, labels, selection, reply, forward and attachment actions are refused with a toast. If the connection fails, the snapshot stays on screen, read-only. The last 200 opened messages are kept (least recently used dropped first). The cache is not encrypted; its folder is mode 0700 and its file mode 0600. It is bound to the account address (stored as a hash) and emptied when the account changes. Demo mode and the MCP server use no cache. See Local cache in the README.
- **Timing trace** — `GMAIL_TUI_TRACE=1` writes a JSONL trace of timings (mail operations with IMAP gate wait, `Update`/`View` per message, startup phases, body rendering) to the user cache directory, mode 0600; `gmail-tui trace <file>` prints a summary. Only operation names, durations, counts and sizes are recorded: no addresses, subjects, bodies, label names, URLs or typed text. See Troubleshooting in the README.
- **Rich rendering and layout (Piano 15)** — the HTML body is drawn as blocks: links styled as buttons are bordered boxes with the destination host, tables fall back to text when too wide, quotes have coloured bars, lists are bulleted or numbered, and images show as `[img: alt]` placeholders (never downloaded). A link whose text shows another domain is marked `⚠` with a wavy red underline and asks for confirmation (`s` opens, `Esc` cancels). Attachments are clickable chips: PDF, PNG, JPEG, GIF and plain text open from a private temporary folder removed on exit; other types stay on `:save`. The folder picker shows a tree with nested labels and unread counts for Inbox and labels. The settings, label and folder windows float over the view with a shadow. The status bar is split into segments, and the inbox shows a splash until the first list arrives. The demo mode has a rich newsletter, unread counts and a nested label.
- **Clickable links** — a URL in a message body (`http`, `https`, `mailto`) opens in the system browser with a left click, in the full-screen thread and in the two-column preview (the click does not move the focus there). Links are underlined with a dashed line in the link colour (Piano 15), wrap across lines as one link, and are wrapped in OSC 8 hyperlinks so `cmd+click` works in iTerm2, Ghostty, Kitty and WezTerm. Opening shows a toast with the destination; an invalid or other-scheme URL stays plain text. The `o` key's browser command also gets the filtered environment (no `GMAIL_APP_PASSWORD` or `GMAIL_TUI_*`).
- **Multi-selection and undo (Piano 10)** — `x` marks the row under the cursor, `X` all loaded rows, `Esc` clears. `a`, `#`, `s`, `v` and `l` act on the whole selection in the current folder; the selection clears after an action or a folder/search change. `u` undoes the last archive or trash for 10 s (the toast offers it); a new archive or trash replaces the undo. Undo archive needs Gmail (`X-GM-EXT-1`): without it the toast does not offer `u`.
- **Label panel on Gmail (Piano 10)** — `l` opens a list of all labels with a check on those present on every target and a mixed mark (◪) on those present on some. Additions and removals go together through `STORE ±X-GM-LABELS`, from any folder. Typing a name that matches no label and pressing `Enter` creates it. Without Gmail, `l` keeps the one-line prompt; with a selection it adds the label to all selected conversations.
- **Desktop notifications** — when new mail arrives in INBOX, gmail-tui runs the `notify_command` set in `config.toml`: a list of arguments, never a shell string. Sender, subject and count reach the command only as `GMAIL_TUI_FROM`, `GMAIL_TUI_SUBJECT`, `GMAIL_TUI_COUNT` and `GMAIL_TUI_FOLDER` environment variables, sanitized and shortened. One run per batch of unread new messages, at most one run at a time (the rest merge into a summary), 5 s timeout with process-group kill on Unix, one error toast per session. The command gets no `GMAIL_APP_PASSWORD`; the program must be an absolute path or a `PATH` name. Mail present at startup does not notify. The settings menu shows whether a command is configured.
- **Drafts** — `Esc` in the composer saves the message to the Drafts folder (kept by Gmail, marked as draft) instead of asking whether to discard; `Ctrl+D` discards it after confirmation. Opening a draft from Drafts resumes it in the composer; saving again replaces the previous copy. Sending a resumed draft removes it from Drafts. Attachments are downloaded when a draft is resumed and kept when it is saved again or sent; if a download fails the original draft is left in Drafts and the copy is saved as a new one.
- **Undo send** — after confirming, the message is saved to Drafts at once and sent after 5 seconds; `u` (inbox and thread only) brings the composer back on that draft. The copy leaves Drafts only after a successful send. Quitting within the window sends it before exiting; if that fails, the message stays in Drafts and the outcome is printed on stderr after the app closes.
- **Contacts** — the composer suggests addresses from a local address book (senders in the list, recipients of messages you open and of messages you send), ordered by frequency and recency. An optional external command (`address_book_cmd` in the config, e.g. `["khard", "email", "--parsable", "%s"]`) adds more; it runs without a shell, gets no `GMAIL_APP_PASSWORD` or `GMAIL_TUI_*` variables, has a 2-second timeout, a 150 ms debounce and a 64 KB output cap, and must be an absolute path or a command found in `PATH` (checked at startup; an invalid value falls back to none). The composer has a Bcc field (Ccn) shown with Cc.
- **MCP server, read-only** — `gmail-tui mcp` serves `list_folders`, `list_recent`, `search_messages` (Gmail query syntax, one row per conversation), `read_message` and `read_thread` over stdio for AI agents. Gmail IDs and user labels are returned; lists are paged with `has_more`/`next_offset`. No tool sends, drafts, archives, labels, deletes or marks messages; email content is wrapped in per-call nonce markers and flagged as untrusted; the password is never returned.
- **Gmail conversations** — on Gmail accounts (`X-GM-EXT-1`) the list shows one row per conversation: participants, message count, unread state and user labels as colored chips. The thread view shows the whole conversation in date order: read messages collapsed, the latest and unread ones expanded. `j`/`k` choose a message, `Enter` or a click expands or collapses it.
- **Gmail search** — `/` passes the query verbatim to Gmail (`X-GM-RAW`): `older_than:`, `in:`, `-label:` and the rest of the Gmail syntax work. Without Gmail the plain matcher is used.
- **Gmail labels** — per-message labels (`X-GM-LABELS`) shown as colored chips in the list and in the thread.
- **Doctor** — an informational line reports whether the server announces `X-GM-EXT-1`.
- **In-tree go-imap** — `third_party/go-imap` (v2.0.0-beta.9 with the Gmail patches listed in `PATCHES.md`).

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

- **Connect honours its deadline** — when the 90 s budget expires, the dial and the login are interrupted and the connection is closed. Before, they kept running in the background: until the read timeouts, or forever when the server had sent its greeting and then stayed silent. The same applies to the connection test of the wizard and `gmail-tui doctor`.
- `a`, `#` and `v` on a list row act on the whole conversation in the current folder. Archive and Trash never touch All Mail.
- Opening a conversation downloads the bodies of the expanded messages only.
- The message cache moves to schema v3 (Gmail IDs and labels are now stored for every fetched message): cached message entries are downloaded again on the first start after the upgrade.
- The message cache moves to schema v4 (LRU limit of 200 messages and snapshots): the message bodies of schema v3 are kept, and the LRU index is built from them. The connection budget at startup is 90 s.
- `gmail-tui login` updates the address and keeps `name`, `signature`, `theme`, `layout` and `preview_mark_read`; before, it reset name and signature and set the theme to `default`.
- An invalid `layout` or `preview_mark_read` value is reset to its default for that field only (warning on stderr at startup); the other fields are kept, and saving from the settings menu or `:theme` repairs the same way.
- The first-run wizard also starts when no password is available and the config has no address.

- `a` (archive) works only on messages in INBOX; elsewhere it shows a status message. The check uses the selected message's mailbox, also for search results from other folders.
- `l` refuses a label equal to the selected message's mailbox (it would duplicate the message).
- Recipient domains must contain a dot (`a@b` is rejected, as Gmail does).
- `gmail-tui send` parses `--to/--cc/--bcc` as RFC 5322 address lists, so display names with commas work.
- IDLE push watches INBOX only; other folders are refreshed after operations.

### Fixed

- **Outgoing message encoding** — the text parts of sent messages, drafts and `gmail-tui send` declare `Content-Transfer-Encoding`: `7bit` for plain ASCII with lines up to 998 bytes, `quoted-printable` otherwise (accents, longer lines: most HTML parts). Before, no encoding was declared and those messages broke RFC 5322.
- **Ghost rows** — a message expunged between the IMAP search and the fetch no longer leaves an empty row in the list.
- The conversation message count `(N)` is never truncated in the list, even with long participant names.
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
