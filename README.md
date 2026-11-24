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

Config file: `~/.config/gmail-tui/config.toml` on Linux (or `$XDG_CONFIG_HOME/gmail-tui/`), `~/Library/Application Support/gmail-tui/config.toml` on macOS, `%AppData%\gmail-tui\config.toml` on Windows.

## Keymap

| Key | Action |
|---|---|
| `j`/`k`, `↑`/`↓` | move cursor (list) / previous–next message (conversation in the thread or preview) / scroll (one message) |
| `Enter` | open selected conversation (full screen; with the preview focused, the previewed conversation) — in a conversation, expand or collapse the selected message |
| `Tab` | two-column layout: move focus between list and preview |
| `Esc` / `q` | back / quit (two-column layout with the preview focused: back to the list) |
| `Ctrl+C` | quit from anywhere |
| `/` | search — on Gmail accounts the query goes verbatim to Gmail (`older_than:7d`, `in:`, `-label:`, `from:x is:unread label:Work has:attachment`); without Gmail, plain words match text |
| `r` / `f` / `c` | reply / forward / compose new |
| `m` then `Enter` in Drafts | resume a draft in the composer (recipients with names, subject, body, attachments) |
| `x` | mark/unmark the row under the cursor for a multi-selection (☑); `X` selects all loaded rows (again: none); `Esc` clears the selection |
| `a` | archive the conversation in the current folder (Inbox only); with a selection, all selected conversations |
| `#` | move the conversation in the current folder to trash — asks for confirmation (`s` confirms, `Esc` cancels); with a selection, all selected; from a thread it returns to the list |
| `u` | undo the last archive or trash (toast for 10 s; a new archive or trash replaces the undo; an unsent message is still sent) |
| `m` | folders and labels picker, as a tree with nested labels and unread counts (`Enter` opens, `Esc` closes); choosing a folder clears the active search |
| `s` | star on/off (with a selection: on for all if any is unstarred, otherwise off for all) |
| `v` | mark read/unread (with a selection: read for all if any is unread, otherwise unread for all) |
| `l` | label panel on Gmail: with a selection, check labels on all selected conversations (☑ on all, ◪ on some); `tab` toggles the row under the cursor, `space` too when the filter is empty; type to filter, or a new name and `Enter` to create it; `Enter` applies additions and removals together. Without Gmail (`X-GM-EXT-1`): the one-line prompt, as before; with a selection it adds the label to all selected conversations |
| `o` | open the selected message's HTML body in browser (thread view) |
| mouse click on a link | open the link in the browser (thread view, or the preview: it does not move the focus); only `http`, `https` and `mailto` links are clickable |
| `s` / `Esc` | when a link or button whose text shows another domain waits for confirmation in the status bar: `s` opens the real address, `Esc` cancels; hosts with look-alike letters are shown in punycode (`xn--…`) |
| `:` | commands (thread view): `:save <n> [path]`, `:open <n>`, `:theme <name>` |
| `pgup`/`pgdn`/`space`/`d`/`u` | page scroll (thread view, or the preview when focused) |
| `,` | settings menu: theme, layout, mark-as-read, name, signature |
| `?` | full help overlay |
| mouse | wheel = scroll, click = select, double-click = open; two-column layout: click on the preview focuses it, double-click opens it, wheel scrolls it |

**Conversations.** On Gmail accounts the list shows one row per conversation: participants, message count in parentheses, unread state, and user labels as colored chips. The thread view shows all messages of the conversation in date order, your replies included: read messages are collapsed to one line, the latest and unread ones are expanded. `j`/`k` choose a message; `r`, `f`, `o` and `:save` act on the selected message (the latest by default).

Composer: `Tab`/`Shift+Tab` between fields · `Ctrl+E` show/hide Cc and Bcc (Ccn) · `Ctrl+S` send (confirmation first; the message is saved to Drafts at once, then a 5-second window in which `u` in the inbox or thread brings the composer back on that draft) · `Esc` save the draft to Drafts and close · `Ctrl+D` discard (asks for confirmation) · while typing an address in To/Cc/Ccn, `Ctrl+N`/`Ctrl+P` choose a suggestion and `Enter` inserts it. The status line always shows a context-sensitive help bar; `?` opens the full overlay in the inbox and thread views (it closes with `Esc`/`?`).

Notifications: the outcome of an action ("archived", "sent", errors…) appears as a toast in the top-right corner. Info and success toasts disappear after 3 seconds; error toasts stay until you press a key. Up to 3 toasts stack; when a fourth arrives the oldest non-error toast goes first. The status bar is split into segments: mode, folder, status (spinner, selection count, messages such as reconnecting or setup warnings) and a short help line, which is cut first when the window is narrow. It turns colored only for confirmations that wait for an answer (send, discard draft, trash, link).

## Themes

Twelve embedded themes: `default`, `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `dracula`, `one-dark`, `one-light`, `nord`, `gruvbox-dark`, `gruvbox-light`, `tokyo-night`.

Select with `theme = "…"` in the config, the `--theme` flag, or `:theme <name>` at runtime (immediate preview, persisted to config); `gmail-tui themes` lists them all. Adding a theme is one TOML file — no test changes needed (the table-driven test validates it automatically) — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Rendering

Messages are drawn with their layout, not as plain text. The HTML body is converted to blocks (paragraphs, quotes, lists, tables, buttons, images) and drawn in the colours of the theme; a message without HTML goes through the same path. Nothing is downloaded: images are never fetched and no CSS is run.

- **Buttons**: a link styled as a button is drawn as a bordered box, with its destination host after it (`→ example.com`). A link becomes a button when its `style` attribute has a background and a padding or border radius, or when it has `role="button"` or a `btn`/`button` class, or when it is the only link in a coloured table cell. A click opens it like any other link.
- **Tables**: drawn as tables when the columns fit the window. When they do not, each row is shown as "Header: value" lines, so no word is broken. Tables above 200 rows or 20 columns are shown as text.
- **Quotes**: a coloured vertical bar per level; nested quotes cycle through four colours of the theme.
- **Lists**: bulleted (`-`) or numbered (the `start` attribute is kept); nested lists are indented under their item.
- **Images**: a placeholder `[img: alt text]`. The image itself is never downloaded.
- **Links**: underlined with a dashed line in the link colour. A link whose text shows a domain different from the real one (compared by registrable domain, IDN normalised) gets a wavy red underline and a `⚠` before the text. A click on it asks for confirmation in the status bar: `s` opens the real address, `Esc` cancels. Only `http`, `https` and `mailto` are clickable; other schemes stay plain text.
- **Attachments**: a chip per attachment, `▣ name · type · size`. A click opens the file only for PDF, PNG, JPEG, GIF and plain text, and only when both the content type and the file extension match. Any other type shows the toast "tipo non aperto: usa :save". Opened files go to a private folder (mode 0700, generated name) under the system temporary directory, removed on exit; folders left by sessions older than 24 hours are removed at start.
- **Limits**: HTML above 512 KiB is cut and the body says so; more than 5,000 elements are cut with a notice; quotes and lists nested deeper than 16 levels are flattened.
- **Folders**: the `m` picker shows folders as a tree, and labels with `/` in their name nest (`Work/Clients` under `Work`). Unread counts `(N)` are shown for Inbox and labels only (not Important, Drafts, Sent, Spam, Trash or All Mail); they refresh while the picker is open. The Important folder ("Importanti") sits with the special folders, right after Inbox.
- **Windows**: the settings menu (`,`), the label panel (`l`) and the folder picker (`m`) float over the view with a shadow: a band of `░` in the border colour, two columns to the right and one row down. `Esc` closes them.
- **Startup**: until the first list arrives, the inbox shows a splash with the `gmail-tui` title, a spinner with the phase ("connessione a imap.gmail.com…", then "caricamento posta…") and the version. There is no artificial delay. If the connection fails, the splash shows the error with secrets masked and "R per riprovare".
- **Empty states**: an empty folder reads "Nessun messaggio in <folder>"; with no message selected, the two-column preview reads "Seleziona un messaggio per leggerlo".

## Settings

Press `,` in the inbox or thread view to open the settings menu. `↑`/`↓` choose a row and `←`/`→` change its value; theme and layout preview at once. `Enter` on a choice saves to the config and closes; `Esc` cancels and restores the previous theme and layout. On **Name** and **Signature**, `Enter` starts editing and `Enter` (name) or `Esc` stops editing; `Ctrl+S` saves from any row. Name and signature apply to the next message you compose.

| Setting | Values | Config key | Default |
|---|---|---|---|
| Theme | the 12 themes | `theme` | `default` |
| Reading layout | one column, two columns | `layout = "single"` or `"split"` | one column |
| Mark as read in preview | immediately, after 2 seconds, never | `preview_mark_read = "immediate"`, `"delay"` or `"never"` | after 2 seconds |
| Name | shown in the From header | `name` | empty |
| Signature | appended to composed messages | `signature` | empty |

**Two-column layout** (`layout = "split"`) is active when the terminal is at least 120 columns wide; narrower terminals show one column. The list takes 35% of the width and the message preview the rest; the preview loads 150 ms after the cursor stops on a message. `Tab` moves focus to the preview: `j`/`k`, `pgup`/`pgdn`, `space`, `d`/`u` scroll it, and `Esc`, `q` or `Tab` return to the list. Actions (`r` `f` `a` `s` `v` `l` `#` `o` `:`) always apply to the previewed message. `Enter` or a double-click opens it full screen. With "after 2 seconds" (the default) a previewed unread message is marked read after two seconds on the same message; "immediately" marks it as soon as it loads, "never" leaves it unread.

A value the app does not accept (for example `layout = "wide"`) is reset to its default for that field only; the other fields are kept and a warning is printed to stderr at startup.

## Desktop notifications

When new mail arrives in INBOX, gmail-tui runs a command you choose. Set `notify_command` in `config.toml` as a list of arguments: the first item is the program, the rest are fixed arguments. **No shell is involved.** The program must be an absolute path (`/usr/local/bin/notify`) or a command name found in `PATH` (`notify-send`); a relative path such as `./notify.sh` is refused at startup. Without the key (the default) nothing runs.

```toml
notify_command = ["osascript", "-e", "display notification (system attribute \"GMAIL_TUI_SUBJECT\") with title (system attribute \"GMAIL_TUI_FROM\")"]
```

The command gets the message data only as environment variables:

| Variable | Value |
|---|---|
| `GMAIL_TUI_FROM` | sender of the newest new message (max 100 characters) |
| `GMAIL_TUI_SUBJECT` | its subject (max 200 characters) |
| `GMAIL_TUI_COUNT` | number of new messages in this batch |
| `GMAIL_TUI_FOLDER` | always `INBOX` |

Control and escape characters are removed and spaces are collapsed. Email content is untrusted: **do not put these variables in the argument list or in a `sh -c "..."` string**, because the shell would run whatever the subject contains. Tools that cannot read the environment (`terminal-notifier`, `notify-send` with literal arguments) need a small wrapper script that quotes the variables:

```sh
#!/bin/sh
# macOS, with terminal-notifier
terminal-notifier -title "$GMAIL_TUI_FROM" -message "$GMAIL_TUI_SUBJECT"
```

```sh
#!/bin/sh
# Linux, with notify-send
notify-send -- "$GMAIL_TUI_FROM" "$GMAIL_TUI_SUBJECT"
```

Set `notify_command = ["/path/to/wrapper"]` to use one. Behaviour: the command runs once per batch of new messages, counting only the new messages that are still unread (`GMAIL_TUI_COUNT`); mail already read elsewhere does not notify. Only the five newest messages of a batch are read for sender and subject. At most one command runs at a time: messages that arrive meanwhile are merged into one summary, sent when the running command ends. Mail present at startup or during the first load never notifies, and only INBOX is watched. The command is killed after 5 seconds, with its whole process group on Unix (on Windows only the direct process is killed). A failing command shows one error toast per session. `GMAIL_APP_PASSWORD` and any `GMAIL_TUI_*` variables from your own environment are never passed to the command. The settings menu (`,`) shows whether a command is configured; it is read-only, because the command is a list and cannot be edited as text. Demo mode generates no new mail, so the command runs there only when real mail arrives.

## CLI

| Command | Description |
|---|---|
| `gmail-tui` | launch the TUI (wizard on first run, demo mode without credentials) |
| `gmail-tui login` | re-run the setup wizard (updates the address, keeps the other settings — see Notes) |
| `gmail-tui fetch [--limit N] [--query "…"]` | list matching messages (default limit 20) |
| `gmail-tui send --to a@b.com [--cc …] [--bcc …] --subject "…" [--attach f]... [--markdown] < body` | send an email read from stdin; `--markdown` sends the body as Markdown (plain text + HTML part) |
| `gmail-tui themes` | list available themes |
| `gmail-tui doctor` | diagnose config, theme, password and cache; with credentials, also checks that the IMAP server announces MOVE, UIDPLUS, SPECIAL-USE and LIST-EXTENDED |
| `gmail-tui mcp` | run a read-only [MCP](#mcp-server-read-only) server on stdio for AI agents |
| `gmail-tui trace <file>` | summarize a timing trace (see [Troubleshooting](#troubleshooting)) |
| `gmail-tui version` | print the version |

Password resolution (TUI and CLI alike): `GMAIL_APP_PASSWORD` env → system keychain → absent (demo mode).

## MCP server (read-only)

`gmail-tui mcp` starts a [Model Context Protocol](https://modelcontextprotocol.io) server on stdio, so an AI agent can find and read your mail. It is **read-only**: no tool sends, drafts, archives, labels, deletes or marks messages. Reading does not mark a message as read (the body is fetched with `BODY.PEEK`).

Tools:

- `list_folders`: folders and labels, with their role;
- `list_recent`: newest messages of a folder (default INBOX), without body;
- `search_messages`: the Gmail query syntax passed verbatim (`older_than:`, `in:`, `-label:`, …), one row per conversation with `thread_id` and `count`, without body. Paged with `has_more` and `next_offset`. On a server without Gmail extensions it searches locally, one row per message;
- `read_message`: headers, Gmail IDs (`gmail_id`, `thread_id`), user labels, plain-text body and attachment metadata (name, type, size). Attachment contents are never returned;
- `read_thread`: a Gmail conversation by `thread_id` (from the search or list rows), in date order, with the plain-text bodies of the latest 20 messages; `messages_omitted` counts the older ones. Gmail accounts only.

Email content is untrusted. Every result wraps it between `BEGIN_UNTRUSTED_EMAIL` and `END_UNTRUSTED_EMAIL` markers carrying a random nonce that changes on each call, and the JSON is marked `"untrusted": true`. Agents should treat that block as data, not as instructions. Limits: 100 rows per list, 64 KB per result, 8,000 characters of body by default (32,000 maximum).

Claude Code:

```sh
claude mcp add gmail-tui -- /absolute/path/to/gmail-tui mcp
```

Claude Desktop (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "gmail-tui": {
      "command": "/absolute/path/to/gmail-tui",
      "args": ["mcp"]
    }
  }
}
```

The password is resolved as everywhere else: system keychain, or `GMAIL_APP_PASSWORD` in the environment that starts the client. Do not put it in the `env` block of the client configuration, which is a plain file. The server never returns the password, and it opens no local cache, so it can run next to the TUI.

## Local cache

gmail-tui keeps a local cache in `cache.db`, in the user cache directory (`~/Library/Caches/gmail-tui/` on macOS, `~/.cache/gmail-tui/` on Linux). The folder is created with mode 0700 and the file with mode 0600. The cache is **not encrypted**: message bodies, senders and subjects are stored as plain text, protected only by these permissions. The password is never stored in it.

The cache holds:

- **Message bodies**: the 200 Gmail messages you opened most recently. The least recently used one is dropped first. The limit is a constant, not a setting.
- **Snapshots**: the first page of conversations of each folder you listed, and the message list of each conversation you opened.

At startup the list appears at once from the snapshot, before Gmail answers. The status bar shows `aggiornamento…` with the age of the data, for example `aggiornamento… (dati di 2 ore fa)`, until the fresh list arrives. A conversation you opened before also appears at once, and the fresh version replaces it when it arrives.

Data from a snapshot is **read-only** until Gmail confirms it. Until the fresh list of the folder has arrived, these actions are refused with the toast `aggiornamento in corso: azione disponibile tra poco`: archive, trash, star, mark read, labels, selection, reply, forward, saving and opening attachments, and loading the next page. Reading, `o`, composing (`c`), search (`/`), the folder picker (`m`), `R` and `u` keep working. When the fresh list arrives, its rows replace the snapshot and the actions work.

If the connection fails while a snapshot is shown, the list stays read-only and the status bar reads `connessione fallita — lista non aggiornata, premi R per riprovare`.

The cache is bound to the account address. The file stores a SHA-256 hash of the address, never the address itself. Starting with another account empties the cache.

Demo mode creates no cache file. The MCP server (`gmail-tui mcp`) does not use the cache.

To delete the cache, quit gmail-tui and remove the `gmail-tui` folder in the user cache directory. The cache is rebuilt on the next start.

## Notes & limitations (v1)

- **Gmail conversations** need the server to announce `X-GM-EXT-1` (Gmail does). Without it the list shows one row per message and `/` uses the plain matcher. `gmail-tui doctor` reports the capability.
- **Conversations and folders**: spam and Trash messages stay out of a conversation, except when you open it from those folders. `a`, `#` and `v` on a row act only on the conversation's messages in the current folder; All Mail is never touched by archive or trash.
- **Conversation paging**: the list pages count conversations, and each page reads at most 500 messages. A page can be shorter than 50, and a very old conversation deep in the list may not appear.
- **Links in the body**: the text of an HTML link is clickable, and so is a URL written in the text: a click opens it in the system browser. Terminals with native OSC 8 hyperlinks (iTerm2, Ghostty, Kitty, WezTerm) also get the link, so `cmd+click` works. Only `http`, `https` and `mailto` are opened; a link is underlined with a dashed line, and a link whose text shows another domain asks for confirmation first (see [Rendering](#rendering)). Links are not listed for keyboard selection.
- **Lazy bodies**: opening a conversation downloads the bodies of the expanded messages only; collapsed rows show sender and date.
- **Cache upgrade**: the message cache moved to schema v4 (200-message limit and snapshots). The upgrade keeps the message bodies already downloaded. Entries from before schema v3 are discarded and downloaded again.
- **Pagination**: the list loads 50 messages at a time; moving the cursor to the last row loads the next 50. Search results are not paginated.
- **IDLE push watches INBOX only**: new mail in other folders is not pushed; the refresh after an operation still applies.
- **Archive only in Inbox**: `a` is refused with an info toast when the selected message is not in INBOX (also for search results from other folders).
- **Recipient validation**: the composer does not open the send confirmation without a recipient or with a malformed address; `gmail-tui send` exits 1 on the same errors. The domain must contain a dot; internationalized domains are sent in punycode; non-ASCII local parts (SMTPUTF8) are not supported. Display names are allowed (`"Rossi, Mario" <m@x.it>`).
- **`gmail-tui login` updates the config**: the address is replaced, while `name`, `signature`, `theme`, `layout` and `preview_mark_read` are kept. The wizard also starts when no password is available and the config has no address.
- **IMAP/SMTP hosts are fixed to the Gmail preset** (`imap.gmail.com:993`, `smtp.gmail.com:587` STARTTLS) and not configurable in v1.
- **Markdown**: composed messages go out as `multipart/alternative`. The `text/plain` part is the Markdown source; the `text/html` part is rendered with GitHub-flavored Markdown (tables, strikethrough, task lists). Raw HTML typed in a message is shown as escaped text. `gmail-tui send --markdown` does the same for stdin. Both text parts declare their `Content-Transfer-Encoding` (`7bit`, or `quoted-printable` for accents and lines above 998 bytes). Messages you receive are shown as plain text: Markdown in incoming mail is not interpreted.
- The composer is adapted from [charmbracelet/pop](https://github.com/charmbracelet/pop) (MIT).
- An external `SIGINT` exits without cancelling the IMAP IDLE watcher (harmless — the process is exiting anyway); use `q`/`Ctrl+C` inside the app for a clean exit.

## Troubleshooting

### Slow startup, slow keys or slow message opening: timing trace

To see where the time goes on your real account, start the app with `GMAIL_TUI_TRACE=1`:

```sh
GMAIL_TUI_TRACE=1 gmail-tui
```

Without the variable nothing is measured and no file is written. With it, the app writes one JSON line per event to `trace-<date>-<time>.jsonl` in the user cache directory (`~/Library/Caches/gmail-tui/` on macOS, `~/.cache/gmail-tui/` on Linux; mode 0600) and prints the file path on stderr when you quit. Use the app for a minute (open messages, move with the keys, change folder), quit, then:

```sh
gmail-tui trace ~/Library/Caches/gmail-tui/trace-20261123-101500.jsonl
```

The summary, short enough to paste into an issue or a chat, has: the startup timeline (first screen, connect, first list); per mail operation count, median, p95 and max in milliseconds, with the **gate wait** (time queued behind another operation or an IDLE exit on the single IMAP connection) apart from the rest; per Bubble Tea message the `Update` and `View` times and the average bytes of the rendered screen; the 10 slowest frames; and the body rendering phases (`FromHTML`, `render`, sync or async). A **Passi IMAP** table follows: one row per operation, IMAP command (`tcp`, `tls`, `login`, `list`, `select`, `search`, `fetch`) and folder role, which tells which command is slow on a real account; the startup timeline also lists the Connect steps.

Event format (all times in microseconds, `t` = end of the event since start): `{"t":..,"k":"store","op":"ListConversations","dur_us":..,"wait_us":..,"n":..,"role":"inbox"}`; kinds are `meta`, `store`, `step`, `cache`, `msg`, `phase` and `body`.

**Privacy.** The trace holds only operation names, durations, counts and byte sizes. It never holds addresses, subjects, names, bodies, label or folder names (only the role: `inbox`, `important`, `sent`, `drafts`, `all`, `trash`, `junk`, `flagged` or `label`), URLs, passwords or error text. Typed text is never recorded: a key that produces a character is logged as `char`; only command keys (`enter`, `esc`, `ctrl+x`, arrows) keep their name.

## Roadmap

- **v1.1** — delivered: paging, folders and labels picker, trash, recipient validation, markdown → HTML multipart sending, settings menu, two-column layout, readable colors on colored bars. Planned: CLI send-only completion
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
