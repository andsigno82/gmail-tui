# AGENTS.md

Guidance for AI coding agents working on **gmail-tui**: a Gmail client for the
terminal, written in Go with the Charm v2 stack (bubbletea/bubbles/lipgloss),
talking IMAP/SMTP with an App Password (no OAuth, no Google Cloud project).

Human-facing docs: [README.md](README.md) (usage, keymap, CLI),
[CONTRIBUTING.md](CONTRIBUTING.md) (dev setup, rules), [ROADMAP.md](ROADMAP.md),
[SECURITY.md](SECURITY.md). Design spec and implementation plans:
`docs/superpowers/specs/` and `docs/superpowers/plans/`.

## Commands

Setup, quality gates, commit style and PR rules: see
[CONTRIBUTING.md](CONTRIBUTING.md). CI also runs `govulncheck ./...`, which
must stay clean. Run without credentials: `go run ./cmd/gmail-tui` → wizard →
`s` = demo mode.

Toolchain: `go 1.26.0` floor (required by `charm.land/*/v2`), `toolchain go1.27.3`
(pinned for a govulncheck-clean stdlib). CI uses Go 1.27.

## Layout

```
cmd/gmail-tui/        main: subcommand dispatch, wiring (config, keyring, store, TUI)
internal/app/         root bubbletea Model: view routing, status bar, keys, ops, search, watch, compose, wizard, demo
internal/ui/          view components: inbox, thread (+ ":" cmdmode), composer, wizard, styles
internal/mail/        MailStore interface (store.go) + in-memory Mock (mock.go)
internal/mail/imap/   the ONLY IMAP implementation (go-imap v2), IDLE watch, idleGate
internal/mail/smtp/   MIME builder + STARTTLS sender (go-smtp)
internal/mail/cache/  bbolt cache, invalidated by uidvalidity
internal/msg/         normalized types, MIME parsing (enmime), Gmail-style query parser
internal/cmd/         CLI subcommands: doctor, fetch, login, send
internal/config/      TOML config at ~/.config/gmail-tui/config.toml
internal/keyring/     keychain via system CLIs (security / secret-tool), env → keychain resolution
internal/theme/       embedded TOML themes → 14 semantic slots
site/                 GitHub Pages landing page (self-contained index.html)
docs/                 demo.tape (VHS) → demo.gif, smoke-checklist.md, specs/plans
```

## Architecture rules (non-negotiable)

1. **`mail.MailStore` is the backend contract** (`internal/mail/store.go`).
   `internal/app` and `internal/ui` talk only to the interface, never to IMAP.
   IMAP code lives in `internal/mail/imap` and nowhere else. Check:
   ```sh
   grep -rn "go-imap" internal --include="*.go" -l | grep -v internal/mail/imap   # must print nothing
   ```
2. **MailStore ops are serialized** (one at a time, single IMAP connection).
   The IMAP backend queues ops through its `idleGate` (interrupts IDLE safely).
   Read the contract comments in `store.go` before changing `Watch`, `Archive`,
   `ApplyLabel`, `Trash`, `ListMailboxes` or search semantics (e.g. `HasAttachment` is a client-side
   post-filter, `ThreadLen` is always 1).
3. **UI uses theme slots only, never hard-coded colors.** Styles derive from
   themes in `internal/ui/styles`. If no slot fits, raise it instead of adding a color.
   Exception (Piano 7.2, U1): the text color on colored bars is computed by `onColor`
   (`internal/ui/styles/styles.go`): the first of the theme's `fg`/`bg` that reaches
   WCAG 4.5:1 against the bar, otherwise black or white. Those two literals are the
   only ones allowed in that package. `TestContrastoStiliConSfondo` (`styles_test.go`)
   requires every style with a background to reach 4.5:1 on every theme, dark and light.
4. **No CGO.** Every target builds with `CGO_ENABLED=0`. This is why the keyring
   shells out to `security`/`secret-tool` instead of using go-keyring.
5. **Gmail hosts are fixed** (`imap.gmail.com:993`, `smtp.gmail.com:587`
   STARTTLS, constants in `cmd/gmail-tui/main.go`); not configurable in v1.
6. Password resolution order everywhere: `GMAIL_APP_PASSWORD` env → system
   keychain → absent (demo mode). Secrets must stay masked in errors and logs.
7. **Connect before any read.** Startup is `beginSession` → `connectCmd`
   (30s timeout) → `connResult`: on success list load + `Watch`, on failure an
   error status and `R` to retry. A retry never runs concurrent `Connect`s
   (`connectFailed` is cleared at dispatch).
8. **Bcc never appears in headers** (`internal/mail/smtp/mime.go`): it is an
   RCPT-only recipient.

## Testing conventions

- **TDD**: write the failing test first. Every package has tests to imitate.
- No network, no Docker in tests: IMAP tests run against an in-process
  `imapmemserver`, SMTP tests against an in-process go-smtp server (self-signed TLS).
- UI/app tests drive the model with `*mail.Mock` (`internal/mail/mock.go`);
  `Mock.Emit` simulates push events. Wrap the Mock when you need real-store
  behavior the Mock hides (see `internal/app/connect_test.go`: the no-op
  `Mock.Connect` once masked a real "store not connected" bug).
- Time-dependent UI (relative dates, double-click) uses injectable clocks
  (`clock func() time.Time`); never call `time.Now` directly in testable paths.
- Adding a theme needs no test changes: `TestEveryEmbeddedThemeValid` validates
  every file in `internal/theme/themes/`.

## Code style

- Follow the existing code exactly. Package docs and code comments are in
  **Italian**; user-visible TUI/CLI strings are Italian too (e.g.
  `"connessione fallita: … — premi R per riprovare"`). Public docs (README,
  CONTRIBUTING, CHANGELOG, site) are in **English**.
- Comments often reference the spec/plan task that motivated the code
  (e.g. `Piano 4, Task 5`); keep that convention for non-obvious decisions.
- Charm v2 APIs: `View() tea.View`, imports from `charm.land/...`; v2 removed
  `AdaptiveColor` (use the `compat` package as `internal/ui/styles` does) and
  bubbles/list has no mouse handling (custom handler in `internal/ui/inbox`).
- MIME parsing keeps `DisableTextConversion`: `text/plain` wins, HTML → text
  via k3a/html2text only as fallback.
- `fmt.Fprint*` is excluded from errcheck (CLI diagnostic output); every other
  error must be handled.
- The composer (`internal/ui/composer`) is adapted from charmbracelet/pop (MIT):
  keep the attribution in the code and in the README.

## Gotchas

- **Value receivers on bubbletea models**: `Model` methods use value receivers,
  so state mutated in a helper is lost unless the model is returned. Command
  builders such as `beginSession` stay pure; state changes happen at the call
  sites in `Update`. staticcheck SA4005 has already caught a real bug here.
- **Reserved keys**: `f` = forward, `b` is free, `,` = settings menu (inbox and thread), `Tab` = list/preview focus (two-column layout only). The thread viewport keymap is
  rebound (`internal/ui/thread/thread.go`) so `f`/`b` no longer page; paging
  is `pgup`/`pgdn`/`space` and `d`/`u`. Check the central keymap
  (`internal/app/keys.go`) before binding anything.
- Mouse input is gated while a modal is open; stale fetch results are
  discarded by guards in `internal/app`. Preserve both when touching routing.
- **`g`/`G` belong to bubbles/list** (go to start/end of the list): they are
  not in `internal/app/keys.go`; check it before binding them.
- **Trash never uses a generic EXPUNGE**: `Trash` rejects servers with neither
  MOVE nor UIDPLUS, because go-imap beta.9's `Client.Move` fallback expunges
  the whole mailbox.
- **List results are dropped on a stale generation**: every non-silent load
  (folder switch, search, connect) bumps `Model.listGen`; a result whose
  generation, mailbox or query differs from the current one is discarded.
- `BodyHTML` is populated for every HTML part (multipart/alternative too):
  the `o` key depends on it.
- **Markdown rendering (goldmark)**: `mdRenderer` in `internal/mail/smtp/mime.go` has
  no `html.WithUnsafe`, so raw HTML is escaped. goldmark's default raw-HTML renderer
  emits a `<!-- raw HTML omitted -->` placeholder; `rawHTMLEscaper` replaces it with
  the escaped text at priority 100. In goldmark v1.8.7 the renderers are sorted
  ascending and the lowest priority value wins (default 1000): do not raise it.
  `TestBuildMIMEMarkdownRawHTMLAsEscapedText` guards it.
- **Pinned library APIs**: go-imap is `v2.0.0-beta.9`. Verify APIs against the
  source of the pinned version, not against docs or memory (e.g. `DialInsecure`,
  `UIDExpunge`). bbolt sentinel errors live in `go.etcd.io/bbolt/errors`.

## Development workflow

Each feature followed: spec (`docs/superpowers/specs/`) → task-by-task TDD
plan (`docs/superpowers/plans/`) → implementation with an independent review
per task → whole-branch review → merge to `main`. For non-trivial work, write
or update a plan there first. `.superpowers/sdd/` holds the local progress
ledger and is gitignored.

Email content is untrusted input: after any dependency bump run
`govulncheck ./...` (x/net HTML parsing via enmime is code-reaching).
