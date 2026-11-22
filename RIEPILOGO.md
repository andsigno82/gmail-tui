# gmail-tui — Riepilogo completo del lavoro

> Report generato il 2026-11-22, aggiornato a fine giornata con la
> **pubblicazione su GitHub**. Il progetto è un **client Gmail completo per il
> terminale** scritto in Go con l'ecosistema Charm, open-source (MIT), pensato
> per la community.

## 🚀 PUBBLICATO: github.com/andsigno82/gmail-tui

- **Release v0.1.0 LIVE** con 5 binari (darwin/linux amd64+arm64, windows zip)
  + checksums → https://github.com/andsigno82/gmail-tui/releases/tag/v0.1.0
- **Landing page LIVE** → https://andsigno82.github.io/gmail-tui/ (Pages,
  deploy workflow checkout@v7/upload@v5/deploy@v5 tutti verdi)
- **CI verde** su main (lint, test race, build matrix, **govulncheck: 0
  vulnerabilità code-reaching**)
- Repo metadata via gh: description, homepage (URL landing), 9 topics
- Module Go = `github.com/andsigno82/gmail-tui` (placeholder USER sostituiti
  ovunque; blocco rename obsoleto rimosso dal README)
- Force-push con sostituzione dell'Initial commit GPLv3 di GitHub con il MIT
  del progetto (copia di soccorso nel branch locale `gplv3-initial-rescue`)
- **Security pass**: aggiunto job govulncheck alla CI + dependabot
  (gomod+actions); trovate e corrette **12 vulnerabilità code-reaching**
  (x/net v0.23→v0.38+ via enmime/html.Parse su contenuto email non fidato;
  stdlib crypto/tls+x509 → toolchain go1.27.3); CI go-version 1.27; pins
  checkout/setup-go aggiornati (fix deprecation Node20)
- Contatti SECURITY/COC riscritti su canali GitHub-only (niente email placeholder)

## Cosa c'è ora nel repo (main)

- `cmd/gmail-tui/` — entrypoint: dispatch dei sottocomandi (`login`, `fetch`,
  `send`, `themes`, `doctor`, `version`) e avvio della TUI
- `internal/app/` — root model bubbletea (routing viste, status bar, keymap,
  operazioni, ricerca, Watch, composer, wizard, demo)
- `internal/ui/` — componenti: `inbox`, `thread` (+ command mode `:`),
  `composer`, `wizard`, `styles`
- `internal/mail/` — interfaccia `MailStore` + Mock; `imap/` (unico backend
  IMAP), `smtp/` (MIME + invio), `cache/` (bbolt)
- `internal/msg/`, `internal/config/`, `internal/keyring/`, `internal/theme/`
  (12 temi TOML embedded), `internal/cmd/` (CLI)
- `site/` landing page, `docs/` (GIF demo + tape VHS, smoke checklist,
  spec e piani), `.github/` (CI, release, Pages, dependabot, template)
- Documentazione: `README.md`, `AGENTS.md` (guida per agenti AI),
  `CONTRIBUTING.md`, `CHANGELOG.md`, `ROADMAP.md`, `SECURITY.md`,
  `CODE_OF_CONDUCT.md`, `LICENSE` (MIT)

## Come provarlo

```bash
# install (binari della release o da sorgente)
go install github.com/andsigno82/gmail-tui/cmd/gmail-tui@v0.1.0

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
- Raffinamento successivo (962011f): retry senza Connect concorrenti
  (`connectFailed` spento al dispatch, `pending` risale)

### Piano 7 — Uso quotidiano (branch `feat/piano-7`, non ancora mergiato)
- `MailStore` + `ListMailboxes` (ruoli SPECIAL-USE) e `Trash` (UID MOVE; rifiuta
  i server senza MOVE né UIDPLUS: mai EXPUNGE generico); Mock filtra per mailbox
- Fix Watch: `Select(INBOX)` prima di ogni IDLE (prima, dopo un'op su un'altra
  mailbox, il push di INBOX si fermava fino al re-IDLE)
- Inbox: carica 50 messaggi più vecchi quando il cursore arriva in fondo;
  refresh a profondità piena (tetto 500) solo dove cambia la lista
- `m` selettore cartelle/label, `#` cestino con conferma, `a` solo in INBOX
  (sulla mailbox della riga), `l` rifiuta la label della riga stessa
- Validazione destinatari (`net/mail`) in composer e `gmail-tui send`
- Demo con Inviati e Work; docs (README, site, ROADMAP, CHANGELOG [Unreleased],
  smoke, spec, AGENTS.md)
- 3 review indipendenti + `/code-review`: corretti stantii tra cartelle
  (`listGen`), follow-up perso del refresh, guard sulla mailbox sbagliata,
  ordine di valutazione nelle return
- Decisioni D1-D7 confermate da Andrea; conversazioni Gmail → Piano 8

## Processo

Ogni piano: spec → piano TDD task-per-task → esecuzione **subagent-driven**
(implementer + reviewer indipendenti per task, fix wave sulle review) →
review finale whole-branch → merge su main. Ledger di progresso in
`.superpowers/sdd/`. Trovi tutto in `docs/superpowers/{specs,plans}/`.

## Passi manuali rimasti (solo tu puoi)

1. ~~Crea il repo GitHub~~ ✅ (github.com/andsigno82/gmail-tui)
2. ~~Sostituisci `USER` e rinomina il module~~ ✅
3. ~~Push main + tag v0.1.0 → release~~ ✅ (5 binari pubblicati)
4. ~~Landing page live~~ ✅ (https://andsigno82.github.io/gmail-tui/)
5. ~~Metadata repo (description/homepage/topics)~~ ✅ (via gh)
6. **Social preview** — l'API non esiste (verificato): l'immagine 1280×640 è
   pronta in `.superpowers/sdd/social-preview.png` → Settings → General →
   Social preview → Upload an image
7. **PR #1 di dependabot** (golangci-lint-action 7→9): rebase richiesto —
   verifica i check e mergia
8. (Opzionale) tap Homebrew `homebrew-gmail-tui` + decommenta la sezione brew
   in `.goreleaser.yaml` (serve PAT `TAP_GITHUB_TOKEN`); FUNDING
9. Ripeti lo smoke con credenziali reali seguendo `docs/smoke-checklist.md`
   (il primo test reale ha già trovato e corretto il bug connect)

## Roadmap v1.1 (già documentata in ROADMAP.md)

- Invio multipart/alternative (md → HTML) — oggi il body va come text/plain
- Completamento CLI send-only, validazione destinatari
- Backend Gmail API dietro `MailStore` (l'astrazione esiste già)
