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
| `a` | archive the conversation in the current folder (Inbox only) |
| `#` | move the conversation in the current folder to trash — asks for confirmation (`s` confirms, `Esc` cancels); from a thread it returns to the list |
| `m` | folders and labels picker (`Enter` opens, `Esc` closes); choosing a folder clears the active search |
| `s` | star on/off |
| `v` | mark the conversation in the current folder read/unread |
| `l` | apply/remove label (refused for the current folder) |
| `o` | open the selected message's HTML body in browser (thread view) |
| `:` | commands (thread view): `:save <n> [path]`, `:open <n>`, `:theme <name>` |
| `pgup`/`pgdn`/`space`/`d`/`u` | page scroll (thread view, or the preview when focused) |
| `,` | settings menu: theme, layout, mark-as-read, name, signature |
| `?` | full help overlay |
| mouse | wheel = scroll, click = select, double-click = open; two-column layout: click on the preview focuses it, double-click opens it, wheel scrolls it |

**Conversations.** On Gmail accounts the list shows one row per conversation: participants, message count in parentheses, unread state, and user labels as colored chips. The thread view shows all messages of the conversation in date order, your replies included: read messages are collapsed to one line, the latest and unread ones are expanded. `j`/`k` choose a message; `r`, `f`, `o` and `:save` act on the selected message (the latest by default).

Composer: `Tab`/`Shift+Tab` between fields · `Ctrl+E` show/hide Cc · `Ctrl+S` send (confirmation first) · `Esc` cancel. The status line always shows a context-sensitive help bar; `?` opens the full overlay in the inbox and thread views (it closes with `Esc`/`?`).

Notifications: the outcome of an action ("archived", "sent", errors…) appears as a toast in the top-right corner. Info and success toasts disappear after 3 seconds; error toasts stay until you press a key. Up to 3 toasts stack; when a fourth arrives the oldest non-error toast goes first. The status bar stays neutral and shows only lasting state (folder, active search, reconnecting, setup warnings); it turns colored only for confirmations that wait for an answer (send, discard draft, trash).

## Themes

Twelve embedded themes: `default`, `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `dracula`, `one-dark`, `one-light`, `nord`, `gruvbox-dark`, `gruvbox-light`, `tokyo-night`.

Select with `theme = "…"` in the config, the `--theme` flag, or `:theme <name>` at runtime (immediate preview, persisted to config); `gmail-tui themes` lists them all. Adding a theme is one TOML file — no test changes needed (the table-driven test validates it automatically) — see [CONTRIBUTING.md](CONTRIBUTING.md).

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

## Notes & limitations (v1)

- **Gmail conversations** need the server to announce `X-GM-EXT-1` (Gmail does). Without it the list shows one row per message and `/` uses the plain matcher. `gmail-tui doctor` reports the capability.
- **Conversations and folders**: spam and Trash messages stay out of a conversation, except when you open it from those folders. `a`, `#` and `v` on a row act only on the conversation's messages in the current folder; All Mail is never touched by archive or trash.
- **Conversation paging**: the list pages count conversations, and each page reads at most 500 messages. A page can be shorter than 50, and a very old conversation deep in the list may not appear.
- **Lazy bodies**: opening a conversation downloads the bodies of the expanded messages only; collapsed rows show sender and date.
- **Cache upgrade**: the message cache moved to schema v3. On the first start after upgrading, cached message entries are discarded and downloaded again.
- **Pagination**: the list loads 50 messages at a time; moving the cursor to the last row loads the next 50. Search results are not paginated.
- **IDLE push watches INBOX only**: new mail in other folders is not pushed; the refresh after an operation still applies.
- **Archive only in Inbox**: `a` is refused with an info toast when the selected message is not in INBOX (also for search results from other folders).
- **Recipient validation**: the composer does not open the send confirmation without a recipient or with a malformed address; `gmail-tui send` exits 1 on the same errors. The domain must contain a dot; internationalized domains are sent in punycode; non-ASCII local parts (SMTPUTF8) are not supported. Display names are allowed (`"Rossi, Mario" <m@x.it>`).
- **`gmail-tui login` updates the config**: the address is replaced, while `name`, `signature`, `theme`, `layout` and `preview_mark_read` are kept. The wizard also starts when no password is available and the config has no address.
- **IMAP/SMTP hosts are fixed to the Gmail preset** (`imap.gmail.com:993`, `smtp.gmail.com:587` STARTTLS) and not configurable in v1.
- **Markdown**: composed messages go out as `multipart/alternative`. The `text/plain` part is the Markdown source; the `text/html` part is rendered with GitHub-flavored Markdown (tables, strikethrough, task lists). Raw HTML typed in a message is shown as escaped text. `gmail-tui send --markdown` does the same for stdin. Messages you receive are shown as plain text: Markdown in incoming mail is not interpreted.
- The composer is adapted from [charmbracelet/pop](https://github.com/charmbracelet/pop) (MIT).
- An external `SIGINT` exits without cancelling the IMAP IDLE watcher (harmless — the process is exiting anyway); use `q`/`Ctrl+C` inside the app for a clean exit.

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
