# gmail-tui — Piano 4: Composer, allegati, wizard, temi runtime — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Le funzioni di scrittura e configurazione che mancano alla v1: composer (da pop, con risposta/inoltro), gestione allegati (`:save`/`:open`), first-run wizard con keychain, comando `:theme` a runtime con persistenza, firma, modalità demo con dati di esempio.

**Architecture:** Il composer è un package `internal/ui/composer` (model Bubble Tea con campi stile pop: From/To/Cc/Subject/Body/Attachments, navigazione con tab, filepicker, conferma pre-invio) consumato dal root via un'interfaccia `Sender` (`Send(ctx, msg.Outgoing) error` — già soddisfatta da `smtp.Sender` e `mail.Mock`). La modalità comando `:` nel thread view serve `:save`, `:open`, `:theme`. Il wizard è una vista dedicata del root mostrata quando manca la config; la password va nel keychain **via CLI di sistema** (`security` su macOS, `secret-tool` su Linux) — deviazione dalla spec (go-keyring richiede CGO, incompatibile col build CGO_ENABLED=0), emendamento approvato dal controller 2026-11-21.

**Tech Stack:** bubbletea/bubbles v2 (textarea, textinput, filepicker) · keychain CLI di sistema · Piano 1-3 (config, theme, msg, mail, smtp, app)

**Nota esecuzione:** subagent su GLM-5.3-Flash / glm-5-turbo quando disponibile; altrimenti general-purpose.

## Global Constraints (cumulative)

- Module `gmail-tui`, Go 1.26, MIT; golangci-lint v2.15.0 → 0 issues; `go test -race` verde
- NESSUNA nuova dipendenza Go in questo piano (keychain = exec CLI; tutto il resto c'è)
- Composer attribuito a pop (MIT): commento di origine in composer.go + riga attribuzione già presente nel README (Piano 5 la completa)
- BodyText inviato come text/plain (markdown sorgente invariato; conversione md→HTML = roadmap v1.1, documentata)
- La firma da config va appended come pop: corpo + "\n\n--\n" + signature (solo se signature != "")
- Mai password in chiaro in config.toml; keychain via CLI; fallback: solo env var
- Chiavi r/f libere in inbox/thread: `r` risposta, `f` inoltra (composer precompilato), `c` nuovo (già stub — diventa reale)
- Polish ereditato da P3 (inclusi nei task indicati): StatusOK use-or-drop (Task 5), checkbox piano P3 e "Verifica finale" (Task 5), viewport scroll keys nel help thread (Task 2), demo seed (Task 5)

## File Structure

```
internal/keyring/keyring.go (+keyring_test.go)      (Task 1: CLI keychain store)
internal/ui/thread/cmdmode.go (+cmdmode_test.go)    (Task 2: modalità comando `:`)
internal/app/model.go / messages.go / ops.go        (Task 2-4: integrazioni)
internal/ui/composer/composer.go (+composer_test.go)(Task 3: composer model)
internal/app/compose.go (+compose_test.go)          (Task 3-4: reply/forward/new + invio)
internal/ui/wizard/wizard.go (+wizard_test.go)      (Task 5: first-run wizard)
internal/cmd/login.go (+login_test.go)              (Task 5: gmail-tui login)
cmd/gmail-tui/main.go                               (Task 5: wiring login)
docs/superpowers/plans/2026-11-21-piano-3-tui.md    (Task 5: checkbox — bookkeeping)
```

---

### Task 1: keyring via CLI di sistema

**Files:** Create `internal/keyring/keyring.go` + `keyring_test.go`.

**Produces:**
```go
type Store interface { Get(service, user string) (string, error); Set(service, user, secret string) error; Delete(service, user string) error }
func New() (Store, error) // sceglie il backend disponibile
```
- Implementazioni: darwin → `security add-generic-password|find-generic-password -w|delete-generic-password -s gmail-tui -a <user>`; linux → `secret-tool store/lookup/clear --service=<svc> <user>` (attributi label/usuario). Altri OS → errore "keychain non supportata; usa GMAIL_APP_PASSWORD"
- Errori: `ErrNotFound` per get senza risultato; exec errori wrapped con il comando; Set sovrascrive (delete-then-add per security)
- Get/Set/Delete testati su darwin contro il keychain REALE con service/utente di test dedicati e t.Cleanup (skip su non-darwin o se `security` assente); versione "finta" (runner di comandi iniettabile `func(name string, args ...string) ([]byte, error)`) per testare su qualsiasi OS la scelta backend, la costruzione argomenti e i path d'errore
- Ordine di risoluzione password (documentato su New): env GMAIL_APP_PASSWORD → keychain → assente (wizard/env instruction). Funzione helper `keyring.Resolve(getenv func(string) string, kr Store, user string) (string, source string, error)` con source ∈ {"env","keychain",""}

- [ ] Steps: RED (backend finto) → impl → GREEN (+ test reali darwin) → vet/lint/race → Commit `feat(keyring): system-CLI keychain store with env-first resolution`

### Task 2: modalità comando `:` nel thread (:save, :open, :theme)

**Files:** Create `internal/ui/thread/cmdmode.go` + test; modify `internal/app/model.go`/`ops.go` per consumo.

**Produces:**
- `:` in thread view apre la command line (textinput inline sotto il corpo, stile SearchInput); esc annulla; enter esegue
- Comandi (parse semplice split-spaces, documentati in help thread):
  - `:save <n> [percorso]` — salva l'allegato n (1-based, dall'elenco a fondo messaggio) tramite `store.SaveAttachment(id, partIndex, path)`; senza percorso → filepicker bubbles (directory corrente, conferma con enter); esito in statusMsg ("salvato in …" con StatusOK se adottato in Task 5, altrimenti StatusBar)
  - `:open <n>` — salva in tmpfile con nome originale in os.TempDir e apre con open/xdg-open (stesso meccanismo `o`)
  - `:theme <nome>` — theme.Load + styles.New + propagate al root (msg `themeChanged{th theme.Theme, st styles.Styles}`) → la UI cambia subito; persistenza: root salva la config (load → set Theme → config.Save sul path di config.Path(), gestendo l'errore in statusMsg senza perdere il tema runtime). `:theme` senza argomenti → status con l'elenco `theme.Names()`
  - comando sconosciuto → statusMsg errore con elenco comandi
- Routing: `:` catturato dal root quando view==viewThread e delegato al thread; i tasti j/k ecc. vanno al textinput del comando mentre è aperto
- Mappatura n → PartIndex: usa `m.Attachments[n-1].PartIndex` (il PartIndex è l'indice filtrato — coerenza garantita dal fix Piano 2)
- Test: parse comandi (validi/invalidi/argomenti mancanti), save con percorso esplicito (Mock.Raw), open produce il tmpfile giusto, theme change propaga stili nuovi (assert su un colore che cambia), persistenza chiamata (config salvata con theme aggiornato, altri campi preservati), routing esc/enter, help thread aggiornato (inclusi i viewport keys j/k/pgup/pgdn/space/f/b/d/u — polish P3)

- [ ] Steps: RED → impl → GREEN → vet/lint/race → Commit `feat(thread): command mode :save/:open/:theme with persistence`

### Task 3: composer — model + reply/forward/new

**Files:** Create `internal/ui/composer/composer.go` + test, `internal/app/compose.go` + test; modify root per apertura composer.

**Produces (composer):**
```go
type Field int // from,to,cc,subject,body,attachments
func New(cfg ComposerConfig, st styles.Styles) Model
type ComposerConfig struct {
    From, Signature string
    To, Cc, Subject, Body string   // precompilati per reply/forward
    QuotedBody string              // citazione iniziale nel textarea
    Attachments []string
}
func (m Model) Update(tea.Msg) (Model, tea.Cmd)
func (m Model) View() string
func (m Model) Draft() msg.Outgoing // raccoglie i campi (con firma appended)
func (m Model) CancelRequested() bool
type SendRequestedMsg struct{ Out msg.Outgoing }
```
- Campi stile pop: From, To, Cc (toggle show), Subject, Body (textarea markdown), Attachments (filepicker multi: enter aggiunge, d rimuove, lista mostrata). Tab/shift-tab + frecce navigano; esc → CancelRequested (il root chiede conferma discard se body non vuoto)
- origine pop: header commento "Adattato da charmbracelet/pop (MIT)"
- Reply/Forward builder in app/compose.go: `replyFor(ref, full)` → To=From del messaggio, Subject="Re: "+subject (idempotente se già Re:), corpo citato `> ` per riga con header "Il <date>, <from> ha scritto:"; `forwardFor` → Subject="Fwd: "+subject, corpo citato senza destinatario + blocco allegati indicati (allegati NON ri-attachati in v1 — limitazione documentata); `newMessage(cfg)` → campi vuoti
- Root: `c` = composer vuoto; `r`/`f` in inbox (su ref selezionato: richiede il full msg — se non in cache il fetch già avvenuto all'apertura NO: r/f in inbox non hanno il full → fetch-then-compose cmd; in thread r/f usano il msg già caricato) → composer view
- Test: navigazione campi, Draft corretto (firma appended, campi mappati), reply/forward builders (idempotenza Re:, citazione, header citazione), filepicker add/remove, cancel con body vuoto/non vuoto

- [ ] Steps: RED → impl → GREEN → vet/lint/race → Commit `feat(composer): pop-style composer with reply/forward builders`

### Task 4: invio — conferma, Sender integration, feedback

**Files:** Modify `internal/app/model.go`/`compose.go`/`messages.go`; test.

**Produces:**
- app.New signature estesa: `New(store mail.MailStore, sender Sender, th theme.Theme)` con `type Sender interface { Send(ctx context.Context, m msg.Outgoing) error }` (soddisfatta da smtp.Sender e Mock); main.go passa smtp.Sender in modalità reale, Mock in demo (già MailStore: il Mock è anche Sender — un solo oggetto)
- SendRequestedMsg → conferma modale ("Invio a <to> — s conferma, esc annulla" con oggetto e n. allegati) → sendCmd (sender.Send con ctx 60s — riusare const sendTimeout) → opDone{"inviato", err} + ritorno inbox; errore → StatusErr con dettaglio
- Demo mode: Mock.Send registra (Sent) — il wizard/test può verificarlo; banner resta
- msg.Outgoing.From = cfg.Address (il campo From del composer è readonly-vista ma precompilato)
- Test: conferma→send chiamato con l'Outgoing giusto (recorder Sender), annulla→non chiamato, errore sender → StatusErr, ritorno inbox dopo invio, signature presente nell'invio

- [ ] Steps: RED → impl → GREEN → vet/lint/race → Commit `feat(app): send flow with confirmation and sender integration`

### Task 5: first-run wizard + login + demo seed + polish

**Files:** Create `internal/ui/wizard/wizard.go` + test, `internal/cmd/login.go` + test; modify `main.go`, root; checkbox docs P3; demo seed.

**Produces:**
- Wizard (vista del root quando `config` assente E nessuna password risolvibile — mostra anche quando config c'è ma password no? NO: solo config assente; password mancante con config presente → banner esistente):
  1. schermata benvenuto: istruzioni 2FA + URL https://myaccount.google.com/apppasswords (apribile con `o`), `invio` per continuare
  2. input indirizzo gmail + input password app (masked)
  3. test connessione REALE (imap NewStore+Connect+ListMessages limit 1, con spinner; timeout 30s; errore → messaggio chiaro e retry)
  4. successo: config.Save (address/name/theme default, signature vuota) + keyring.Set (fallimento keyring → statusMsg che indica l'env var; wizard completato comunque) → root con store reale
  - "salta" (s) → demo mode con seed (Task sotto)
- `gmail-tui login` (subcommand): stesso wizard in pty (basta lanciare la TUI in modalità wizard forzata); test: flag parse + factory
- keyring resolution nel wiring main: `keyring.Resolve(getenv, kr, cfg.Address)` al posto del solo getenv (Task 1)
- Demo seed: mail.Mock con 5-6 messaggi realistici (unread/starred/allegati fittizi) via un `app.DemoStore()` factory (così il cloner vede l'app viva) — solo quando si entra in demo dal wizard o senza password
- Polish P3: StatusOK usato per feedback positivi (inviato/salvato/etichetta — i messaggi successo passano a StatusOK); checkbox Task 1-2 e "Verifica finale" del piano P3 tickate; (viewport keys già fatto in Task 2)
- Test: wizard flow (step machine: welcome→input→testing→done/skip), config scritta correttamente, keyring.Set chiamato (fake), test-connection fallito → retry state, login flag parse, DemoStore seed assertions, StatusOK assertions su feedback positivi

- [ ] Steps: RED → impl → GREEN → vet/lint/race → Commit `feat(wizard): first-run setup, login subcommand, demo seed, status polish`

## Verifica finale del piano

- [ ] `go vet ./... && go test -race ./... && golangci-lint run ./...` verdi
- [ ] `go run ./cmd/gmail-tui` demo mode: c→composer→invio (Mock.Sent), r/f precompilati, `:theme dracula` cambia tema e persiste, `:save 1` su messaggio con allegato
- [ ] `go run ./cmd/gmail-tui login` parte il wizard; con credenziali vere: connessione reale e config scritta (smoke opzionale documentato)
- [ ] Conformance: composer usa SOLO msg.Outgoing + Sender interface (nessun import smtp in app/ui)

## Prossimo piano

Piano 5 — Community: README+GIF (VHS), CONTRIBUTING, templates issue/PR, goreleaser + Homebrew tap, tag v0.1.0 con rename module (`go mod edit -module github.com/<utente>/gmail-tui` — decisione nome/org da chiedere all'utente), release CI.
