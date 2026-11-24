# gmail-tui — manual smoke checklist

Release smoke test against a real Gmail account (and demo mode where noted).
Every item must pass: tick `[ ]` → `[x]`. Expected results describe what the
app actually shows; status messages are quoted verbatim (UI strings are Italian).

## Setup

- [ ] 2-Step Verification is active on the Gmail account and an App Password exists (https://myaccount.google.com/apppasswords) — regular passwords do not work.
- [ ] Choose a credential path: `export GMAIL_APP_PASSWORD=…` or the wizard (`gmail-tui login`), which stores it in the system keychain (macOS `security`, Linux `secret-tool`).

## First launch (wizard + demo)

- [ ] With no config and no `GMAIL_APP_PASSWORD`, `gmail-tui` starts the **wizard**: welcome step shows the 2FA instructions and the app-passwords URL; `o` opens that URL in the browser; `Enter` continues; `s` skips.
- [ ] Address step: empty + `Enter` → error ("inserisci l'indirizzo…"); no `@` → error ("manca @"); valid address → next step.
- [ ] Password step: input is masked (`*`); `Esc` goes back to the address step keeping the typed value.
- [ ] Wrong password at the test step: connection test fails within its 90 s budget, a clear error is shown and the wizard returns to the password step with the credentials retained — retry works.
- [ ] Correct password: wizard completes, the config file is written (`~/.config/gmail-tui/config.toml` on Linux, `~/Library/Application Support/gmail-tui/config.toml` on macOS) **without the password** (grep it), the password is in the keychain, the real inbox loads and the status bar shows "configurazione completata — connesso come …".
- [ ] With a config present but no password: **demo mode** — full-width yellow banner "demo mode — set GMAIL_APP_PASSWORD", inbox seeded with 6 realistic messages (3 unread ●, 2 starred ★).
- [ ] Wizard skip (`s`): lands in the same seeded demo mode with the banner.
- [ ] Terminal smaller than 80×24 (e.g. 40×10): dedicated "too small" screen naming the current and required size; `q` still exits.
- [ ] `q` in the inbox exits cleanly (exit code 0); `Ctrl+C` exits cleanly from any state, including the composer and the help overlay.

## Reading

- [ ] Inbox rows show unread marker ●/○, star ★, truncated sender, subject and relative date (today → time, yesterday → "ieri", older → date).
- [ ] Mouse: wheel scrolls the list, click selects a row, double-click opens it.
- [ ] `Enter` opens the message: header block Da/A/Oggetto/Data, body in a scrollable viewport, quoted `>` lines dimmed/italic, attachments listed at the bottom with 📎 and human-readable sizes.
- [ ] Opening an unread message marks it read automatically (marker flips on the next list refresh, no error).
- [ ] Thread scrolling: `j`/`k` line-by-line; `pgup`/`pgdn`/`space`/`d`/`u` page/half-page scroll.
- [ ] `o` on a message with an HTML part: browser opens a `.html` temp file containing the original HTML. On a plain-text-only message: nothing happens (no error).
- [ ] Click a URL in the body (full-screen thread): the browser opens it and the toast reads "aperto nel browser: <host>". Clicking plain text does nothing.
- [ ] In the two-column preview, a click on a link opens it and does not move the focus; a click on the text next to it focuses the preview as before.
- [ ] A URL longer than the line wraps onto two lines and both parts open the same link.
- [ ] Links are underlined; a `javascript:` or `file:` text is not underlined and does not open.
- [ ] iTerm2, Ghostty, Kitty or WezTerm: `cmd+click` on a link opens it natively (OSC 8), without going through the click handler.
- [ ] Demo mode: the "Go Weekly" newsletter contains `https://example.com/newsletter/47`; clicking it opens the browser.
- [ ] `Esc`/`q` in the thread returns to the inbox with the list and cursor position preserved; a slow fetch interrupted by `Esc` does not open a stale message later.

## Search

- [ ] `/` opens the search input (prompt "/ ", syntax placeholder, blinking cursor). `q` typed here is query text, not quit; `Esc` is the only way out and cancels without searching (draft preserved if reopened).
- [ ] `from:<you> is:unread` + `Enter` → inbox shows only matching messages and the status bar shows `N risultati per "…"`.
- [ ] Free text (e.g. a subject word) + `Enter` → matching messages listed (tokens: `from:`, `to:`, `label:`, `is:unread`, `is:starred`, `has:attachment`; anything else is free text).
- [ ] Empty query + `Enter` → full list reload, search badge cleared.
- [ ] After a search, archive/star refresh the **same** search result, not the whole inbox.

## Compose & send

- [ ] `c` opens the composer: From prefilled from config (read-only, never focused), focus starts on To.
- [ ] `r` on an inbox row (fetch-then-compose) or in an open thread: To = sender, subject `Re:` (idempotent), body pre-filled with the quote header "Il <date>, <from> ha scritto:" and every line prefixed `> `; focus starts on the body.
- [ ] `f`: subject `Fwd:`, no recipients, quote + a note that original attachments are not re-attached in v1.
- [ ] `Tab`/`Shift+Tab` cycle fields (From skipped); `Ctrl+E` shows/hides Cc and Bcc — a hidden Cc or Ccn that is filled in is still sent (the confirmation bar lists it).
- [ ] With a signature configured, it is appended on send as `--` + signature below the body.
- [ ] `Ctrl+S` opens the confirmation bar: "Invio a … — <subject> — <n> allegati — s conferma · esc annulla". `Esc` returns to the composer with the draft intact; `s` sends.
- [ ] After `s`, the message is in Drafts at once; within 5 s `u` (inbox or thread) reopens the composer on that draft, and the copy is gone from Drafts only after the send succeeds.
- [ ] `u` typed in the search bar or in a prompt is text, not undo.
- [ ] Send success: back to the inbox, status "inviato", list refreshes (message visible in "Sent" on the web).
- [ ] Send failure (e.g. bad network): error in the status bar, composer stays open with the draft; `Ctrl+S` re-opens the confirmation to retry.
- [ ] `Esc` on a composer with a non-empty raw body opens the discard confirm "scartare la bozza? s conferma · esc annulla"; `s` discards, `Esc` resumes editing. An untouched empty composer exits directly.
- [ ] Pressing `Ctrl+S`/`s` repeatedly while a send is in flight produces exactly **one** email (no duplicates).
- [ ] In demo mode the same flow succeeds against the mock (send recorded, no network error).

## Attachments

- [ ] On the composer's attachments field, `Enter` opens the file picker; `Enter` on a file adds its absolute path and the picker stays open for multiple adds; `d` removes the selected entry; `Esc` closes.
- [ ] The send confirmation lists the attachment count ("1 allegato" / "N allegati"); the received mail shows the attachments on the web.
- [ ] In a thread, `:save <n> [path]` saves the n-th attachment (1-based): with a path the file is written there; without a path a picker opens (`Enter` on a directory saves inside it with the original name). Status: "salvato in …". Content matches the original file.
- [ ] `:open <n>` saves the attachment to the temp dir with its original name and opens it with the system application.
- [ ] Demo mode: `:save 1` on the seeded message with `report-spese-q4.pdf` writes a real file.

## Labels & flags

- [ ] `a` archives: status "archiviata", the message leaves the list after the silent refresh, cursor stays on a sensible row.
- [ ] `s` toggles the star: status "stella on"/"stella off", row shows ★ accordingly.
- [ ] `v` toggles read/unread: status "letta"/"segna non letta".
- [ ] `l` opens the label prompt "etichetta: " in the status bar; `Enter` applies ("etichetta Work applicata"), `Esc` cancels, `Enter` on an empty name cancels without action. Typing `l` again while the prompt is open inserts the letter `l` — no accidental action.
- [ ] Labels applied are visible on the web (Gmail label); removing a label with `l` on a labelled message shows "etichetta X rimossa".

## Themes

- [ ] `gmail-tui --theme dracula` starts with the Dracula palette; the whole UI (list, thread, status bar, composer, picker) uses theme colors only.
- [ ] Unknown `--theme name` falls back to the default theme with a startup warning ("tema \"name\" non disponibile — uso default").
- [ ] `:theme tokyo-night` in a thread recolors the UI immediately (no restart) and persists `theme = "tokyo-night"` to the config (survives a relaunch).
- [ ] `:theme` with no argument lists the 12 available themes; `:theme nonsense` shows an error with the theme list and keeps the current theme.
- [ ] `gmail-tui themes` prints exactly: default, catppuccin-frappe, catppuccin-latte, catppuccin-macchiato, catppuccin-mocha, dracula, gruvbox-dark, gruvbox-light, nord, one-dark, one-light, tokyo-night.

## Push & reconnect

- [ ] With a real session open, send yourself an email from the Gmail web UI: it appears in the list within seconds (IMAP IDLE) and the status bar shows "nuova posta in arrivo".
- [ ] Star/read a message from the web: the list updates automatically without a status message.
- [ ] Turn the network off (Wi-Fi toggle): the status bar shows the reconnect indicator "⟳ riconnessione…"; turn it back on: the indicator clears after the next successful refresh and the list is current again.
- [ ] `q`/`Ctrl+C` after using IDLE exits promptly with no hang.

## CLI commands

- [ ] `gmail-tui version` → `gmail-tui dev` (or the build version injected with `-ldflags "-X main.version=…"`).
- [ ] `gmail-tui fetch --limit 5` → the 5 newest messages of the real inbox (password from env or keychain; without one, a clear "password non trovata" error, exit 1).
- [ ] `gmail-tui fetch --query "from:<x> is:unread"` → only matching messages.
- [ ] `echo body | gmail-tui send --to a@b --subject "smoke" --attach ./f.pdf` → prints "sent to 1 recipients", the mail arrives with subject, body and attachment; missing `--to` → error on stderr, exit 1.
- [ ] `gmail-tui login` on an existing config: starts the wizard even if a config exists; completing it updates the address and keeps `name`, `signature`, `theme`, `layout` and `preview_mark_read`.
- [ ] Unknown subcommand / stray positional argument → usage printed, exit 1.

## Daily use (v1.1)

- [ ] Scroll past 50: in the inbox move the cursor to the last row (`G` or repeated `j`); the next 50 messages load, with no duplicated rows.
- [ ] Folder switch: `m` opens "cartelle e label"; `Enter` on Inviati shows sent mail and the status bar tag reads Inviati; an active search is cleared; `m` back to "Posta in arrivo" restores the inbox.
- [ ] Trash on a real account: `#` opens the confirmation; `Esc` cancels; `s` moves the message out of the list. Verify in Gmail web that the message is in the Cestino (not deleted). From a thread, `s` returns to the list.
- [ ] `#` inside the Cestino folder shows "già nel cestino" and does nothing else.
- [ ] `a` outside Posta in arrivo (Inviati or a label) shows "archivia è disponibile solo in Posta in arrivo"; the message is untouched.
- [ ] `l` with the current folder name (`INBOX` in the inbox) shows "etichetta INBOX non applicata: è la cartella corrente"; no label is applied.
- [ ] Malformed address: in the composer, To = `anna@@example` and `Ctrl+S` shows "A: indirizzo non valido" and no confirmation; empty To shows "nessun destinatario".
- [ ] `echo body | gmail-tui send --to not-an-address --subject smoke` prints "send: A: indirizzo non valido …" on stderr, exit 1.
- [ ] Gmail announces MOVE, UIDPLUS, SPECIAL-USE and LIST-EXTENDED: `gmail-tui doctor` prints `imap: ok (MOVE UIDPLUS SPECIAL-USE LIST-EXTENDED)`.
- [ ] Display name and IDN: send to `"Rossi, Mario" <your-address>` from the composer and from `gmail-tui send`; the message arrives and the To header shows the name.

## Readability, settings & layout (Piano 7.2)

- [ ] Themes `catppuccin-latte` (light) and `one-dark` (dark), `gmail-tui --theme <name>` (demo mode is fine): the status bar, the OK and error status messages, the selected inbox row and the demo banner are all readable; no bar has text that blends into its background.
- [ ] `,` in the inbox opens the settings menu, centred; `↑`/`↓` move through Tema, Layout lettura, Segna come letto in anteprima, Nome, Firma; `esc` closes it.
- [ ] On Tema, `←`/`→` recolors the whole UI at once; `esc` restores the previous theme; `invio` on Tema saves and the config file holds the new `theme`.
- [ ] `,` also works in a thread view. While the menu is open, list keys such as `a`, `s`, `#` do nothing and mouse clicks are ignored.
- [ ] On Layout lettura, `←`/`→` selects "due colonne" (applied at once); `invio` saves `layout = "split"`.
- [ ] With `layout = "split"` and a terminal of at least 120 columns: list on the left (about 35%), preview on the right after a short pause, separated by a vertical line. Narrowing below 120 columns returns to one column; widening again restores two.
- [ ] `Tab` moves focus to the preview (the separator changes color); `j`/`k` scroll the preview and the list does not move; `esc` or `q` return to the list and do not quit the app; double-click on the preview or `invio` opens the message full screen.
- [ ] Mark as read in preview: with "dopo 2 secondi" an unread message is still unread after a moment and becomes read after about 2 s; with "mai" it stays unread; with "subito" it is marked read as soon as the preview loads.
- [ ] Name: set Nome in the menu (`invio` edits, `invio` ends editing, then `ctrl+s` saves). A new message shows From as `Nome <address>`, and the received mail shows the name as sender in Gmail web.
- [ ] Markdown: a composed message with `**bold**`, a GFM table and a `> quote` arrives in Gmail web formatted (bold text, a table, a quote block; no raw asterisks or pipes). Literal `<b>x</b>` typed in the body arrives as text.
- [ ] `echo "**ciao**" | gmail-tui send --markdown --to <you> --subject smoke` arrives formatted; without `--markdown` the same text arrives as plain text.
- [ ] A received message with Markdown (`**x**`) is shown as raw text in the app: expected, incoming Markdown is not interpreted.
- [ ] Invalid config: set `layout = "wide"` in the config file and start the app: a warning naming the invalid field appears on stderr, the app runs in one column, and `name` and `signature` are still set.

## Gmail conversations (Piano 8) — real account only

- [ ] `gmail-tui doctor` prints the `X-GM-EXT-1` line as present on Gmail.
- [ ] A conversation with a reply you sent (Sent folder) is one row in the list, with the participants, the message count in parentheses, and the row bold when any message is unread.
- [ ] Opening it shows all messages in date order, your reply included: read ones collapsed, the latest and unread ones expanded. `j`/`k` choose a message, `Enter` expands it and its body loads.
- [ ] `/older_than:30d` returns the same messages, count and subjects, as the Gmail web search for the same query.
- [ ] `/in:sent` and `/-label:<something>` behave like Gmail web.
- [ ] A user label appears as a colored chip and is not duplicated after `a` on the row.
- [ ] `a` on a conversation in the Inbox removes its Inbox messages only: in Gmail web they remain in All Mail with their labels.
- [ ] `#` on a conversation in the Inbox moves only its Inbox messages to Trash; messages in Sent stay where they are.
- [ ] `r` on a selected message that is not the latest replies to that message; `o` and `:save` act on the selected message.
- [ ] Spam and Trash messages are absent from a conversation opened from the Inbox, and present when it is opened from Trash.
- [ ] Do not paste real addresses, subjects or bodies into the repo: redact any transcript before using it as a fixture (plan Task 13, point 5).

## MCP server (Piano 14) — real account only

- [ ] `claude mcp add gmail-tui -- /path/to/gmail-tui mcp`, then in Claude Code `list_folders` shows the Gmail folders and labels with their roles.
- [ ] `search_messages` with `older_than:30d` returns the same conversations as Gmail web for the same query: one row each, with `thread_id` and `count`. When `has_more` is true, `next_offset` pages through the results without gaps or repeats.
- [ ] `read_thread` with a `thread_id` from a search result returns the messages in date order with their bodies, and they stay unread in Gmail web.
- [ ] `read_message` shows `gmail_id`, `thread_id` and the user labels as Gmail shows them.
- [ ] A query with a newline or an over-long text is refused by the tool; no request reaches Gmail.
- [ ] The App Password does not appear in the client logs nor in any tool result.
- [ ] Do not paste real addresses, subjects or bodies into the repo: redact any transcript first.

## Notifications (Piano 7.3)

- [ ] `v` on a message: a toast "letta"/"non letta" appears top right and disappears after about 3 s; the status bar keeps its neutral color.
- [ ] Three quick actions (`v`, `s`, `v` on different rows): three stacked toasts, each disappearing on its own.
- [ ] Composer with an empty To and `Ctrl+S`: an error toast "nessun destinatario" stays after 3 s and disappears at the next key press.
- [ ] An error toast plus three quick success toasts: the error is still on screen.
- [ ] Send confirmation, discard confirmation and trash confirmation still use the colored status bar.
- [ ] Active search badge, `⟳ riconnessione…` and the folder tag stay in the status bar (not toasts).

## Desktop notifications (Piano 11) — real account only

Use a wrapper script that appends `$GMAIL_TUI_FROM`, `$GMAIL_TUI_SUBJECT` and `$GMAIL_TUI_COUNT` to a temporary file, and set it as `notify_command` (see README). Do not paste real senders or subjects into the repo.

- [ ] Start gmail-tui with the wrapper configured: the file gets no line for mail already in the INBOX.
- [ ] Send one message to the account from another address: one line in the file with the right sender and subject. The message stays unread in Gmail web.
- [ ] Send three messages in quick succession: the lines in the file add up to three, and a batch shows a `COUNT` greater than 1. Record how the batches were split.
- [ ] A subject with ANSI escape codes or over 200 characters: the file shows no escape codes and the subject is cut with `…`.
- [ ] Make the wrapper exit 1 (or point `notify_command` to `false`): one error toast `notifica desktop: …`; the next new message does not show it again.
- [ ] Mark a new message as read from Gmail web before the notification window: no line is written for it (only unread messages count).
- [ ] Set `notify_command = ["./notify.sh"]`: gmail-tui refuses the config with a clear error (relative path); the notifications stay off.
- [ ] Make the wrapper sleep 30 s, then send two messages: one command runs at a time; the second message arrives as one summary line once the first run ends.
- [ ] Settings `,`: the row "Notifiche desktop" reads "comando configurato"; without the key it reads "non configurate (vedi README)".
- [ ] macOS: the `osascript` example from the README shows a banner, which confirms that `system attribute` works on this macOS version.
- [ ] Linux, if available: the `notify-send -- ...` wrapper shows a notification. This path is not verified in the repo.

## Doctor

- [ ] `gmail-tui doctor` prints one line per check: config path + address (or "nessuna, uso defaults…"), theme ok, password found (or missing), writable cache dir; exit code 0 when healthy, 1 on blocking problems (broken config/theme/cache).
- [ ] Run `gmail-tui doctor` with `GMAIL_APP_PASSWORD` unset and no keychain entry → the password line says missing and suggests "gmail-tui login", the IMAP check is skipped, the other checks do not fail.
- [ ] With the password only in the keychain → `password: keychain ok` and the `imap:` line is printed.

## Piano 10 — smistamento (real account, run by hand)

Run on a real Gmail account; nothing here is covered by tests.

- [ ] `x` on two rows, then `a`: both leave the Inbox in Gmail web; the selection is empty afterwards.
- [ ] `a` then `u` within 10 s: the conversation is back in the Inbox in Gmail web, and no "\Inbox" label appears as a user label on it (the Sent messages of the thread do not get \Inbox).
- [ ] `#` on a selection, then `u`: the messages are back in their folder with their user labels (known limit: labels lost with the trash are not restored, see the plan, D10-T3).
- [ ] `u` after a new `a`: only the last action is undone; the earlier one stays.
- [ ] Label panel on a mixed selection (one conversation with Work, one without): Work shows ◪; `tab` on it and `Enter` removes it from the conversation that had it, `Lavoro` created by name is added to both, in Gmail web from another folder.
- [ ] Label panel on a conversation in a different folder: removing a label from Gmail web's label list is reflected after the refresh.

## Piano 9 — scrittura (real account, run by hand)

Run on a real Gmail account; nothing here is covered by tests.

- [ ] `Esc` in a new message with a recipient and text: the message appears in Gmail web under Drafts, with the same recipient, subject and body.
- [ ] `Enter` on that draft in gmail-tui's Drafts folder: recipients, subject, Cc/Bcc and body are back; the signature is not doubled.
- [ ] Save again after an edit: Drafts keeps one copy (the new one).
- [ ] Bcc of a draft: the recipient is still there after resuming; the sent message shows no Bcc header in Gmail web (Sent).
- [ ] Send a resumed draft: it leaves Drafts; the message is in Sent once.
- [ ] `Ctrl+S` then `s`: the message goes out after about 5 seconds; `u` within 5 seconds brings the composer back with the text.
- [ ] Quit within the 5-second window: the message is sent before the program exits.
- [ ] Address suggestions: type the first letters of a known sender in To, `Ctrl+N`/`Ctrl+P` select, `Enter` inserts `Name <address>, `.
- [ ] With `address_book_cmd` set (e.g. khard): suggestions include its results; with the command missing or failing, one warning toast and local suggestions still work.

## Piano 15 — rendering, folders, windows, splash (Ghostty) — real account only

Run on Andrea's Ghostty terminal. Do not paste real senders, subjects, addresses or bodies into the repo: note only the kind of message.

- [ ] Links are underlined with a dashed line in the link colour; a `javascript:` or `file:` text is not underlined and does not open.
- [ ] A link whose text shows another domain has a wavy red underline and the `⚠ ` marker in the text; both are visible (no colour-only signal).
- [ ] `cmd+click` on a link opens it natively (OSC 8) in Ghostty.
- [ ] A link or button whose text only mentions a domain inside a sentence (`Vai a paypal.com`) pointing elsewhere is also marked `⚠`; a host with look-alike letters is shown in punycode (`xn--...`) in the confirmation and the toast.
- [ ] Clicking a `⚠` link shows "il link porta a <real host>, non a <shown host>: apri? s/esc" in the status bar; `Esc` cancels with no browser; `s` opens the real address.
- [ ] A button of a real newsletter: a bordered box with its label and `→ host` next to it; clicking opens the destination.
- [ ] A table of a real order confirmation: it fits the window, or falls back to "Header: value" lines when too narrow; no broken words.
- [ ] Clicking the chip of a PDF attachment opens it in the system viewer; the file sits in an `s-*` folder under `$XDG_RUNTIME_DIR/gmail-tui/open` (or the user cache directory, `gmail-tui/open`), mode 0700, with a generated name; nothing is written to the shared `/tmp`.
- [ ] Clicking a `.zip` attachment shows "tipo non aperto: usa :save" and writes nothing to the temporary folder.
- [ ] After quitting, the session folder `s-*` is gone from the `gmail-tui/open` folder.
- [ ] Unread counts in `m` match Gmail web for Inbox and each label; `(N)` appears only when N > 0; Drafts, Sent, Spam, Trash and All Mail show no count.
- [ ] A nested label (a name with `/`) appears as a branch under its parent in `m`, and selecting it opens that label.
- [ ] The `,`, `l` and `m` windows show a shadow: a `░` band to the right and below the window, in the border colour, visible in Ghostty.
- [ ] `Esc` closes each window; while a window is open, list keys do nothing and mouse clicks are ignored.
- [ ] The status bar shows mode, folder, status and help as separate segments; the help is cut first when the window is narrow.
- [ ] Startup: the inbox shows the splash (title, spinner phase "connessione a imap.gmail.com…" then "caricamento posta…", version), then the list, with no artificial delay.
- [ ] Network off at startup: the splash shows the sanitised error and "R per riprovare"; `R` retries.
- [ ] An empty folder reads "Nessun messaggio in <folder>"; with no message selected, the two-column preview reads "Seleziona un messaggio per leggerlo".

## Timing trace (Piano 16) — real account, Ghostty

Run on Andrea's Ghostty terminal to measure where the time goes. Do not paste the trace file contents anywhere other than a chat you trust; it holds no personal data, but still check it.

- [ ] Without `GMAIL_TUI_TRACE`: no `trace-*.jsonl` appears in the `gmail-tui` user cache directory after a session.
- [ ] `GMAIL_TUI_TRACE=1 gmail-tui`: on quit, stderr prints `traccia dei tempi: <path>`; the file exists, mode 0600 (`ls -l`).
- [ ] Use the app: start, open 3 messages (one with a large HTML body), move 20 times with `j`/`k`, switch folder with `m`, search with `/`, quit.
- [ ] `gmail-tui trace <path>` prints the summary: startup timeline, `MailStore` operations with the gate wait columns, messages with `Update`/`View`, the slowest frames, bytes per `View` and the body rendering rows.
- [ ] Privacy: `grep -ciE "@|http|<your label names>|<a subject>" <path>` prints 0; typed letters appear only as `"key":"char"`.
- [ ] Paste the summary in the chat to compare startup, key and message-opening times with the demo mode.

## Local cache and snapshots (Piano 16) — real account only

Run the steps in order: the second start needs the cache filled by the first. Close gmail-tui between steps unless a step says otherwise.

- [ ] Demo mode (no password, `s` in the wizard): no `cache.db` appears in the `gmail-tui` user cache directory.
- [ ] First real start with an empty cache: the splash stays until the first list arrives. Quit.
- [ ] Second real start: the list appears before Connect finishes; the status bar shows `aggiornamento… (dati di N fa)`, where N matches the time since the last session. When the fresh list arrives, that text disappears.
- [ ] While `aggiornamento…` shows, press `a` on a row: toast `aggiornamento in corso: azione disponibile tra poco`. The message stays in INBOX (check on the Gmail web). The same toast for `s`, `#` and `r`.
- [ ] After the text disappears, `a` on the same row works, and the message that moves is the one on that row (check on the Gmail web).
- [ ] Open a conversation seen before: the thread shows at once. The fresh version replaces it without moving the cursor or collapsing expanded messages.
- [ ] Connect failure with a snapshot: turn Wi-Fi off and start. The list stays; the status bar reads `connessione fallita — lista non aggiornata, premi R per riprovare`; the toast `connessione fallita: …` appears; `a` shows the provisional toast. Turn Wi-Fi on and press `R`: the status clears when the fresh list arrives, and `a` works.
- [ ] Account change: set another `address` in the config and start. No mail of the previous account appears before Connect. `cache.db` is still there (its account hash changed).
- [ ] Permissions: `ls -ld` on the `gmail-tui` cache folder shows `drwx------`; `ls -l` on `cache.db` shows `-rw-------`.
- [ ] Trace: with `GMAIL_TUI_TRACE=1` on the second start, the file contains `"k":"cache"` events with `"op":"snap_list"` and `"mode":"hit"`; `gmail-tui trace <path>` runs without errors.
- [ ] Delete the `gmail-tui` cache folder with gmail-tui closed. The next start behaves like the first one (splash until Connect).

## Piano 12a — fixes (real account, run by hand)

Run on a real Gmail account; nothing here is covered by tests.

- [ ] `m`: "Importanti" is the second entry, right after Inbox, with the other special folders and no `[Gmail]` branch above it. Opening it lists the messages Gmail marks as important. It shows no unread count.
- [ ] Compose a message with accented letters and a line longer than 1000 characters (paste a long URL), send it to yourself. In Gmail web, "Show original": the text part has `Content-Transfer-Encoding: quoted-printable`, no line above 998 characters, and the message reads correctly in Gmail and in another client.
- [ ] Send a plain ASCII message: "Show original" shows `Content-Transfer-Encoding: 7bit` and the text as typed.
- [ ] Markdown message with accents: both parts of the `multipart/alternative` open correctly. Save it as a draft with `Esc`, reopen it from Drafts: the text is the same.
- [ ] Delete a message from Gmail web while the list is open: after the refresh there is no empty row.
- [ ] Network off at startup: the splash shows the error within the 90 s budget. Network on and `R`: the app connects (the timed-out attempt left no connection behind).

## Timing trace, steps (Piano 12a) — real account

This is the measurement the Piano 12b design is built on. Do it first and paste the summary in the chat.

- [ ] `GMAIL_TUI_TRACE=1 gmail-tui`: start, wait for the list, open 2 conversations (one with several messages), press `v` on a row, switch to another folder with `m`, quit.
- [ ] `gmail-tui trace <path>`: the summary has the "Passi IMAP" table, with rows for `Connect tcp`, `Connect tls` and `Connect login`, and `list`, `select`, `search` and `fetch` for `ListConversations`, `ThreadMessages` and `FetchMessage`.
- [ ] Startup timeline: it lists the three Connect steps. Note the time of `Connect login` (the 28.9 s of the 23 November trace were all inside Connect).
- [ ] First `ListConversations`: note the slowest step, and whether it is in `inbox` or in `all`.
- [ ] Privacy: `grep -ciE "inbox|all mail|\[Gmail\]" <path>` prints 0.
- [ ] Paste the "Passi IMAP" table and the startup timeline in the chat: they decide the order of Piano 12b.
