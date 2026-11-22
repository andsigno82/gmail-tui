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
- [ ] Wrong password at the test step: connection test fails within its 30 s budget, a clear error is shown and the wizard returns to the password step with the credentials retained — retry works.
- [ ] Correct password: wizard completes, `~/.config/gmail-tui/config.toml` is written **without the password** (grep it), the password is in the keychain, the real inbox loads and the status bar shows "configurazione completata — connesso come …".
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
- [ ] `Tab`/`Shift+Tab` cycle fields (From skipped); `Ctrl+E` shows/hides Cc — a hidden Cc is never sent.
- [ ] With a signature configured, it is appended on send as `--` + signature below the body.
- [ ] `Ctrl+S` opens the confirmation bar: "Invio a … — <subject> — <n> allegati — s conferma · esc annulla". `Esc` returns to the composer with the draft intact; `s` sends.
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
- [ ] `gmail-tui login` on an existing config: starts the wizard even if a config exists; completing it **rewrites the config** (address kept, `name`/`signature` reset — re-set them afterwards).
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
- [ ] Gmail announces MOVE, UIDPLUS, SPECIAL-USE and LIST-EXTENDED: `openssl s_client -quiet -connect imap.gmail.com:993`, then `a CAPABILITY` lists all four.

## Doctor

- [ ] `gmail-tui doctor` prints one line per check: config path + address (or "nessuna, uso defaults…"), theme ok, password found (or missing), writable cache dir; exit code 0 when healthy, 1 on blocking problems (broken config/theme/cache).
- [ ] Run `gmail-tui doctor` with `GMAIL_APP_PASSWORD` unset → the password line says missing and suggests "gmail-tui login", without failing the other checks.
