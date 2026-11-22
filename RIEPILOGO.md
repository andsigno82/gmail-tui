# gmail-tui — Riepilogo completo del lavoro

> Report generato il 2026-11-22. Il progetto è un **client Gmail completo per il
> terminale** scritto in Go con l'ecosistema Charm, open-source (MIT), pensato
> per la community.

## Cosa c'è ora nel repo (main)

App TUI **funzionante e completa per tutte le feature v1 della spec**
(`docs/superpowers/specs/2026-11-20-gmail-tui-design.md`) + tutto il pacchetto
community per la pubblicazione su GitHub.

- **16 package Go · 375+ test · `go test -race` tutto verde · `golangci-lint`
  0 issues · `go vet` pulito · build CGO_ENABLED=0**
- Verificato end-to-end con smoke in pty reale (10/10: wizard → demo →
  composer → invio → r/f → `:theme` → `:save` → quit)
- CI GitHub Actions: lint + test race + build matrix macOS/Linux/Windows
- 58 commit su `main`, uno per task, workflow subagent-driven con review per
  task e review finale per piano (6 piani, tutti mergiati)

## Come provarlo

```bash
go run ./cmd/gmail-tui          # wizard → premi "s" per la demo (6 msg esempio)
export GMAIL_APP_PASSWORD="…"   # App Password da myaccount.google.com/apppasswords
go run ./cmd/gmail-tui          # sessione reale (IDLE, inbox live)
go run ./cmd/gmail-tui login    # wizard guidato, salva nel keychain
go build -ldflags "-X main.version=0.1.0-dev" -o gmail-tui ./cmd/gmail-tui
```

## I 6 piani di implementazione (tutti eseguiti e mergiati)

### Piano 1 — Fondamenta (aa3305c)
- Module Go (`gmail-tui` locale), licenza MIT, stub CLI con versione
- `internal/config`: TOML in `~/.config/gmail-tui/`, default, validazione
- `internal/theme`: **12 temi classici embedded** (default, catppuccin×4,
  dracula, one-dark/light, nord, gruvbox×2, tokyo-night) con slot semantici
  (fg/bg/primary/unread/starred/…) — aggiungerne uno = un file TOML, il test
  tabellare li valida da solo
- `internal/msg`: tipi normalizzati (MessageRef/Message/Outgoing/Query) +
  parsing MIME con enmime (`DisableTextConversion` per far rispettare la
  regola "text/plain vince") e fallback HTML→testo con k3a/html2text
- `internal/mail`: interfaccia **MailStore** (il contratto centrale: la UI non
  conosce IMAP) + Mock in-memory con feed di cambiamenti
- `internal/mail/cache`: cache bbolt con invalidazione per uidvalidity
- CLI `doctor`/`themes`, CI, 370 test complessivi oggi

### Piano 2 — Backend Gmail IMAP+SMTP (ab794ef)
- `internal/mail/imap`: Store completo (go-imap v2): ListMessages
  newest-first con paginazione, FetchMessage cache-first con PEEK,
  SaveAttachment (con filtro parti degeneri coerente col parser),
  MarkRead/Star/Archive/ApplyLabel con semantica Gmail via UID STORE/EXPUNGE,
  Search server-side con post-filtro allegati, **Watch via IDLE** con
  riconnessione a backoff e **idleGate** (ops-vs-IDLE e op-vs-op: le
  operazioni interrompono l'IDLE in sicurezza invece di uccidere la
  connessione)
- `internal/mail/smtp`: builder MIME (Q-encoding, base64, Bcc mai negli
  header) + Sender con STARTTLS (testato con server TLS self-signed)
- CLI `fetch`/`send` + `msg.ParseQuery` (from:/is:unread/label:…)
- Test di protocollo **in-process** (imapmemserver + server go-smtp): zero
  Docker, zero rete nei test
- Deviazioni verificate dal reviewer contro i sorgenti delle librerie pinnate
  (go-imap v2.0.0-beta.9: DialInsecure, UIDExpunge, sentinel bbolt in
  `go.etcd.io/bbolt/errors`)

### Piano 3 — TUI core (a59bee5)
- `internal/ui/styles`: stili lipgloss derivati SOLO dai temi
  (compat.AdaptiveColor dark/light — v2 ha rimosso AdaptiveColor)
- `internal/app`: root model (routing viste, status bar, spinner, pending),
  inbox con bubbles/list + **handler mouse custom** (la libreria v2 non lo
  ha: click, doppio-click, rotella con clock iniettabile), thread view
  (viewport, quote attenuate, allegati, `o` apre nel browser), ricerca `/`
  con ParseQuery, mutazioni a/s/v/l con refresh che preserva il cursore,
  Watch integrato (auto-refresh, indicatore `⟳ riconnessione…`), keymap
  centrale + help overlay + schermata min-size + demo mode
- Fix importanti dalle review: BodyHTML popolato per OGNI html part (il tasto
  `o` ora funziona anche sulle multipart/alternative), guard fetch stanti,
  mouse gated nei modali, rebind viewport f/b per liberare `f` = inoltra
- Floor Go 1.26 (richiesto da charm.land v2); `View() tea.View`

### Piano 4 — Composer, wizard, temi runtime (3c0643b)
- `internal/keyring`: keychain via **CLI di sistema** (`security`/`secret-tool`
  — go-keyring richiede CGO, incompatibile col build), risoluzione
  env→keychain, segreti mascherati negli errori
- Command mode `:` nel thread: `:save <n> [path]` (filepicker), `:open <n>`,
  `:theme <nome>` (re-theme immediato + persistenza config)
- `internal/ui/composer`: composer adattato da pop (MIT, attribuzione): campi
  From/To/Cc/Subject/Body/Attachments, firma, reply/forward precompilati,
  conferma pre-invio, **guard anti doppio invio** (`sending` flag)
- First-run wizard (istruzioni 2FA, input mascherato, **test connessione
  reale 30s**, config + keychain, skip → demo) + `gmail-tui login`
- Demo seed (6 messaggi realistici), StatusOK, smoke pty 10/10

### Piano 5 — Community & release (b5fe6ee)
- `README.md` inglese (quickstart, keymap, temi, CLI, note, attribuzione pop),
  `docs/smoke-checklist.md` (checklist manuale della spec),
  `CONTRIBUTING.md`, issue/PR templates, `ROADMAP.md`
- `.goreleaser.yaml` + release workflow (validati con goreleaser check)
- **GIF demo reale** generata con VHS (31.3s, verificata frame-by-frame)

### Piano 6 — presenza GitHub (c7c89e3)
- `site/index.html`: **landing page** self-contained (hero + GIF + 6 card
  feature + showcase 12 temi con swatch hex verificati + quickstart +
  keymap) — verificata con screenshot Playwright, subpath-safe
- `.github/workflows/pages.yml`: deploy Pages automatico (azioni ai major
  correnti: checkout@v7, upload@v5, deploy@v5)
- `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md` [0.1.0], `.github/FUNDING.yml`

### Bugfix post-release test dell'utente (oggi)
- **"imap store non connesso"**: `beginSession` lanciava load/Watch senza mai
  chiamare `store.Connect` — il Mock (Connect no-op) mascherava il bug; con
  credenziali reali l'avvio falliva
- Fix: `beginSession` = `connectCmd` (timeout 30s) → `connResult`: ok ⇒
  load + Watch; errore ⇒ status "connessione fallita: …" con **`R` per
  riprovare** (binding `Connect`, short-help condizionale)
- Test nuovi: catena connect→load con store che replica il contratto reale,
  fallimento+retry; TestInit/Watch/WizardSkip adattati; bug reale trovato dal
  lint (SA4005: mutazioni su receiver valore) e corretto

## Processo

Ogni piano: spec → piano TDD task-per-task → esecuzione **subagent-driven**
(implementer + reviewer indipendenti per task, fix wave sulle review) →
review finale whole-branch → merge su main. Ledger di progresso in
`.superpowers/sdd/`. Trovi tutto in `docs/superpowers/{specs,plans}/`.

## Passi manuali rimasti (solo tu puoi)

1. Crea il repo GitHub (`github.com/TUOUTENTE/gmail-tui`)
2. Sostituisci `USER` ovunque (`git grep -l USER`) e rinomina il module:
   `go mod edit -module github.com/TUOUTENTE/gmail-tui` + sostituzione import
   + `go mod tidy && go build`
3. `git push` + `git tag v0.1.0 && git push origin v0.1.0` → la release
   workflow pubblica i binari
4. Settings → Pages → Source: **GitHub Actions** → la landing page va live
5. (Opzionale) tap Homebrew `homebrew-gmail-tui` + decommenta la sezione brew
   in `.goreleaser.yaml`; FUNDING; description/topics/social preview del repo
6. Smoke con credenziali reali seguendo `docs/smoke-checklist.md`

## Roadmap v1.1 (già documentata in ROADMAP.md)

- Invio multipart/alternative (md → HTML) — oggi il body va come text/plain
- Completamento CLI send-only, validazione destinatari
- Backend Gmail API dietro `MailStore` (l'astrazione esiste già)
