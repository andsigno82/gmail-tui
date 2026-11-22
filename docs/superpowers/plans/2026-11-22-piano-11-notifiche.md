# gmail-tui — Piano 11: notifiche desktop — Implementation Plan

> **For agentic workers:** branch `feat/piano-11` (da `main` 8f852c3). Leggere `AGENTS.md`. TDD vero (test rosso verificato prima del fix, RED riportato), commit per task, checkbox `- [ ]`. Le decisioni D11-1 e D11-2 sono di Andrea e non si riaprono. Le scelte tecniche sono proposte qui e motivate.

**Goal:** alla posta nuova in INBOX la TUI esegue un comando configurato dall'utente (`notify_command` in `config.toml`), con mittente, oggetto e conteggio passati SOLO come variabili d'ambiente. Zero dipendenze: l'utente sceglie `notify-send`, `terminal-notifier`, `osascript`, o uno script wrapper.

**Stack:** nessuna dipendenza nuova. Solo `os/exec`, `context`, `strings`, `unicode` della standard library. Nessun CGO.

## Decisioni

| ID | Decisione | Scelta |
|---|---|---|
| D11-1 | Meccanismo | Comando configurabile eseguito a ogni `ChangeNewMail` (decisione di Andrea, modello: hook `mail-received` di aerc, `aerc-config(5)`). |
| D11-2 | Ambito | Solo INBOX: il Watch osserva già solo INBOX (`internal/mail/imap/watch.go`). Nessun IDLE su altre cartelle. |
| D11-3 (proposta) | Chiave config | `notify_command`, lista di argomenti TOML (`notify_command = ["osascript", "-e", "..."]`). Primo elemento = eseguibile, resto = argomenti. **Mai una stringa interpretata da una shell.** Vuota o assente = notifiche disabilitate (default, nessun cambiamento per chi non la configura). |
| D11-4 (proposta) | Dati del messaggio | Solo variabili d'ambiente: `GMAIL_TUI_FROM` (mittente del messaggio più recente), `GMAIL_TUI_SUBJECT` (oggetto del più recente), `GMAIL_TUI_COUNT` (numero di messaggi nell'evento), `GMAIL_TUI_FOLDER` (`INBOX`). Il contenuto delle email non entra mai in argv né in una shell. Sanitizzazione: caratteri di controllo, ANSI ed elementi di formato Unicode (bidi) rimossi, spazi compattati, lunghezza limitata (FROM 100 rune, SUBJECT 200 rune, con "…"). |
| D11-5 (proposta) | Esecuzione | `exec.CommandContext` senza shell, in un goroutine del `tea.Cmd` (fuori dal loop UI), timeout 5 s (il processo viene ucciso allo scadere), stdin chiuso, stdout scartato, stderr troncato per il messaggio d'errore. Un processo per evento. |
| D11-6 (proposta) | Coalescing | Un evento `ChangeNewMail` = una sola esecuzione, con `COUNT` = numero di messaggi dell'evento. Il Watch già raggruppa i nuovi UID di una ri-verifica in un solo evento (`recheckNewMail`): N messaggi arrivati insieme producono una notifica riassuntiva. Nessun timer aggiuntivo: aggiungerlo sarebbe una seconda politica di ritardo, da decidere solo se serve. |
| D11-7 (proposta) | Dati del mittente/oggetto | Il Watch, nella stessa ri-verifica, legge ENVELOPE + FLAGS + INTERNALDATE dei nuovi UID (`fetchRefs`, nessun `\Seen` né `BODY`). Il contratto `mail.Change` riceve il campo `Refs` (additivo). Se la lettura fallisce, l'evento parte comunque con i soli ID e `FROM`/`SUBJECT` vuoti: la notifica non dipende dalla riuscita del fetch intestazioni. |
| D11-8 (proposta) | Primo caricamento | Il comando non parte per gli eventi arrivati prima del primo `msgsLoaded` riuscito della INBOX (`notifyArmed`). Il Watch parte con baseline sull'UID più alto, quindi i messaggi già presenti all'avvio non generano eventi comunque: il gate copre la finestra tra avvio del feed e primo caricamento. |
| D11-9 (proposta) | Errori | Errore del comando (avvio fallito, exit diverso da 0, timeout) → toast d'errore UNA sola volta per sessione (`notifyErrShown`). Gli errori successivi non producono toast. |
| D11-10 (proposta) | Menu `,` | Una voce in sola visualizzazione in fondo al menu: "Notifiche desktop" con "comando configurato" o "non configurate (vedi README)". Nessuna modifica dal menu: il comando è un array di argomenti, scriverlo in un campo di testo sarebbe la stringa-shell che D11-3 esclude. Attivazione = modifica di `config.toml`. |
| D11-11 (proposta) | Demo | La demo non ha posta in arrivo (nessun `Emit` fuori dai test): con `notify_command` configurato la demo non notifica da sola. Il percorso si prova con i test (`Mock.Emit` + runner iniettato) e con l'integrazione su binario reale. Una simulazione di posta in demo sarebbe una funzione nuova: fuori da questo piano, da chiedere ad Andrea se serve. |

## Fonti

- `aerc-config(5)`, sezione hooks (`mail-received`): modello del comando configurabile alla posta nuova (D11-1).
- `os/exec` (standard library): `exec.CommandContext` senza shell, `cmd.Env`, `WaitDelay`. Il processo è `argv[0]` diretto: nessuna espansione di variabili né di metacaratteri.
- Repo: `internal/mail/imap/watch.go` (`recheckNewMail`, `sendChange`), `internal/mail/store.go` (`Change`), `internal/app/watch.go` (`handleChange`), `internal/app/settings.go` (menu `,`), `internal/config/config.go` (`Load`, `Save`, `Sanitized`, `Validate`).

## Global Constraints

- `mail.MailStore` resta l'unico contratto della UI; `internal/app` non importa `go-imap`. Il campo `Change.Refs` è additivo: chi non lo legge non cambia comportamento.
- Nessuna shell: il comando è `exec.Command(argv[0], argv[1:]...)`. Test che verifica che i dati non compaiono in argv.
- Nessuna credenziale reale, nessuna connessione a Gmail nei test. Il fetch intestazioni è testato con il memserver (`imapmemserver`).
- Colori: il toast usa gli slot esistenti. Nessun colore nuovo.
- Commenti e doc del codice in italiano con riferimento `Piano 11, Task N`; docs pubbliche (README, CHANGELOG) in inglese.
- Config: la modifica del menu `,` e del wizard passa da `load → set → Save`, quindi `notify_command` sopravvive (`internal/app/settings.go`, `internal/ui/wizard/wizard.go`).
- Il Piano 9 (composer, config, settings) gira in parallelo: le modifiche a `config.go` e `settings.go` restano piccole e isolate (un campo, una voce di menu).

## File Structure

Nuovi:
- `internal/app/notify.go`: sanitizzazione, costruzione env, `runNotify` (esecuzione con timeout), `notifyResult`.
- `internal/app/notify_test.go`: unit (sanitizzazione, env, timeout) e integrazione (`true`, processo helper).
- `internal/app/notify_flow_test.go`: flusso nel Model (arm, coalescing, errore una volta).
- `internal/mail/imap/watch_refs_test.go`: `Change.Refs` dal memserver, flag invariati.

Modificati:
- `internal/config/config.go` (+ test): campo `NotifyCommand`, validazione, `Sanitized`.
- `internal/mail/store.go`: campo `Refs` in `Change`.
- `internal/mail/imap/watch.go`: `recheckNewMail` legge le intestazioni dei nuovi UID.
- `internal/app/model.go`, `internal/app/watch.go`: campi, arm sul primo load, emissione del comando, `notifyResult`.
- `internal/app/settings.go` (+ test): voce di sola lettura in fondo.
- `cmd/gmail-tui/main.go`: `m.SetNotify(cfg.NotifyCommand)`.
- `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, `docs/superpowers/plans/2026-11-22-piano-sviluppo-gap.md` (stato del Piano 11).

## Waves

| Wave | Contenuto | Task | Dipende da |
|---|---|---|---|
| 1 | Config (`notify_command`) e contratto (`Change.Refs`) | 1-2 | — |
| 2 | Esecuzione sicura (sanitizzazione, env, timeout) | 3 | — |
| 3 | Flusso nel Model: arm, emissione, errore una volta, menu, wiring | 4-5 | 1, 2 |
| 4 | Documentazione, smoke, verifica | 6 | 3 |

### Task 1: chiave `notify_command` in config

**Files:** `internal/config/config.go`, `internal/config/config_test.go`.

- Campo `NotifyCommand []string` con tag `toml:"notify_command"`. Default: nil (disabilitato).
- `Validate`: se la lista non è vuota e il primo elemento è vuoto o solo spazi, `ErrInvalid` (`notify_command: primo elemento vuoto`).
- `Sanitized`: un comando non valido torna nil; gli altri campi restano.
- `Save` non scrive la chiave quando la lista è nil (verificato con il round-trip).

**Test (RED prima):**
- `Load` di un TOML con `notify_command = ["notify-send", "Posta", "nuova"]`: lista di 3 elementi, nessun errore.
- Round-trip `Save` → `Load` con la lista, e con nil.
- Primo elemento vuoto: `errors.Is(err, ErrInvalid)`; `Sanitized()` lo azzera senza toccare `Theme`/`Address`.

- [ ] RED → impl → GREEN → Commit `feat(config): notify_command as argv list for desktop notifications`

### Task 2: `Change.Refs` con le intestazioni dei nuovi messaggi

**Files:** `internal/mail/store.go`, `internal/mail/imap/watch.go`, `internal/mail/imap/watch_refs_test.go` (nuovo).

- `Change` acquista `Refs []msg.MessageRef`: per `ChangeNewMail`, le intestazioni dei nuovi messaggi dal più recente, lette con ENVELOPE/FLAGS/INTERNALDATE (nessun `\Seen`, nessun BODY). Vuoto per gli altri tipi e in caso di fetch fallito (best-effort, D11-7).
- `recheckNewMail`: dopo `UIDSearch` e il filtro `uid > lastUID`, chiama `s.fetchRefs("INBOX", newUIDs)` nella stessa connessione (siamo già dentro il round/gate). Errore di fetch: `Refs` nil, l'evento parte lo stesso; NON deve diventare errore di riconnessione.
- Mock: nessuna modifica (i test passano `Refs` direttamente a `Emit`).

**Test (RED prima), memserver:** appendere un messaggio con `From`/`Subject` noti mentre il Watch è armato; il `ChangeNewMail` ha `Refs[0].From`/`Subject` corretti e il messaggio resta non letto (FLAGS senza `\Seen` dopo l'evento). Caso con due messaggi: `len(Refs) == 2`, dal più recente.

- [ ] RED → impl → GREEN → Commit `feat(imap): Change.Refs carries headers of new INBOX messages (no \Seen)`

### Task 3: esecuzione sicura del comando

**Files:** `internal/app/notify.go` (nuovo), `internal/app/notify_test.go` (nuovo).

- `notifyEnv(refs []msg.MessageRef, count int) []string`: costruisce `GMAIL_TUI_FROM`, `GMAIL_TUI_SUBJECT` (dal primo ref, il più recente), `GMAIL_TUI_COUNT`, `GMAIL_TUI_FOLDER=INBOX`, più `os.Environ()`.
- `sanitizeNotifyText(s string, max int) string`: rimuove sequenze ANSI (`ESC [ … lettera`, `ESC ] … BEL|ESC \`, `ESC` singolo), converte `\t\r\n` in spazio, scarta `unicode.IsControl` e `unicode.Is(unicode.Cf, r)`, compatta gli spazi, tronca a `max` rune con "…".
- `runNotify(ctx context.Context, argv, env []string) error`: `context.WithTimeout(5 s)`, `exec.CommandContext(ctx, argv[0], argv[1:]...)`, `Env = env`, `Stdin = nil`, stdout scartato, stderr in un buffer limitato a 512 byte. Errore di avvio, exit diverso da 0 o timeout: errore con `argv[0]` e stderr sanitizzato. `WaitDelay` 1 s per non restare appesi a figli che tengono la pipe.
- `type notifyRunner func(ctx context.Context, argv, env []string) error` (seam: `runNotify` in produzione, fake nei test del Model).

**Test (RED prima):**
- Sanitizzazione: `"\x1b[31mRossi\x1b[0m\n‮euro"` → `"Rossi euro"`; troncamento con "…" alla lunghezza esatta; stringa vuota.
- Env: contiene `GMAIL_TUI_SUBJECT=…` sanitizzato; l'argv costruito dal Model è uguale alla config, byte per byte (il mittente e l'oggetto non vi compaiono mai).
- Timeout: `runNotify` con `sleep` e un timeout ridotto (parametro interno) → errore in meno di 2 s (gated `runtime.GOOS != "windows"`).
- Integrazione: `runNotify` con `true` → nil; con binario inesistente → errore. Il test delle variabili usa il pattern del processo helper (il test binary rieseguito con una variabile sentinella che scrive `GMAIL_TUI_SUBJECT` su un file temporaneo): nessuno script esterno, nessuna shell.

- [ ] RED → impl → GREEN → Commit `feat(app): run notify command without shell with sanitized env and timeout`

### Task 4: flusso nel Model

**Files:** `internal/app/model.go`, `internal/app/watch.go`, `internal/app/notify.go`, `internal/app/notify_flow_test.go` (nuovo).

- Campi Model: `notifyArgv []string`, `notifyRun notifyRunner` (default `runNotify`), `notifyArmed bool`, `notifyErrShown bool`.
- `SetNotify(argv []string)`: seam dal main, come `SetPrefs`.
- Arm: al primo `msgsLoaded` riuscito e corrente (dopo i check di stantìo e di errore), `notifyArmed = true`.
- `handleChange` per `ChangeNewMail`: se `notifyArgv` non è vuota e `notifyArmed`, il cmd esegue `notifyRun` con argv ed env e ritorna `notifyResult{err}`. Il toast "nuova posta in arrivo" resta invariato.
- `notifyResult` in `update`: errore e `!notifyErrShown` → `notifyErrShown = true` + toast d'errore `notifica desktop: …`; altrimenti nessun effetto.
- Il comando NON parte da `ChangeFlags` né da `ChangeDisconnected`.

**Test (RED prima), con `notifyRun` fake che registra argv/env:**
- Nessun comando prima del primo load (evento in finestra di avvio → nessuna chiamata).
- Dopo il load: `ChangeNewMail` con 2 ID e 2 Refs → una sola chiamata, `COUNT=2`, `FROM`/`SUBJECT` del più recente.
- `ChangeFlags` e `ChangeDisconnected` → nessuna chiamata.
- Comando non configurato → nessuna chiamata, nessun cambio di comportamento.
- Fake che fallisce: primo evento → un toast d'errore; secondo evento che fallisce → nessun nuovo toast.

- [ ] RED → impl → GREEN → Commit `feat(app): notify on new INBOX mail once the list is loaded`

### Task 5: voce di sola lettura nel menu `,` e wiring

**Files:** `internal/app/settings.go`, `internal/app/settings_test.go`, `cmd/gmail-tui/main.go`.

- `rowNotify` in fondo all'enum (dopo `rowSignature`), etichetta "Notifiche desktop". Valore: "comando configurato" o "non configurate (vedi README)". Invio e ←→ non cambiano nulla: la voce legge `m.notifyArgv` e non ha bozze.
- `settingsValueText`: caso esplicito prima del default.
- `cmd/gmail-tui/main.go`: `m.SetNotify(cfg.NotifyCommand)` accanto a `SetPrefs`.

**Test (RED prima):** `settingsView` contiene "Notifiche desktop" e "non configurate" senza comando, "comando configurato" con comando; la navigazione `↓` fino all'ultima voce; invio sulla voce non modifica `m.markRead`/`m.name`.

- [ ] RED → impl → GREEN → Commit `feat(app): read-only notifications row in the settings menu`

### Task 6: documentazione e verifica

**Files:** `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, `docs/superpowers/plans/2026-11-22-piano-sviluppo-gap.md`.

- README (inglese): sezione "Desktop notifications" con la chiave, la lista di argomenti, le variabili d'ambiente, il timeout, la regola "no shell", esempi:
  - macOS, `osascript` (nessuno script): `notify_command = ["osascript", "-e", "display notification (system attribute \"GMAIL_TUI_SUBJECT\") with title (system attribute \"GMAIL_TUI_FROM\")"]`;
  - macOS, `terminal-notifier`: serve uno script wrapper, perché il binario non legge l'ambiente. Lo script passa `"$GMAIL_TUI_FROM"` e `"$GMAIL_TUI_SUBJECT"` tra virgolette;
  - Linux, `notify-send`: stesso schema, con uno script wrapper che chiama `notify-send -- "$GMAIL_TUI_FROM" "$GMAIL_TUI_SUBJECT"`;
  - avvertenza: non usare `sh -c "…$GMAIL_TUI_SUBJECT…"` nella config: il contenuto dell'email verrebbe interpretato dalla shell.
- CHANGELOG (inglese): voce "Added".
- `docs/smoke-checklist.md`: sezione "Piano 11, notifiche" (account reale di Andrea): posta di prova in INBOX con comando che scrive su file; notifica singola; più messaggi arrivati insieme = una notifica con COUNT; nessuna notifica all'avvio; comando in errore = un solo toast.
- Piano dei gap: stato "Piano 11: fatto" con hash.

- [ ] Gate finale: build, vet, `go test -race -count=1 ./...`, `golangci-lint run ./...` 0 issues; build `CGO_ENABLED=0` su linux/darwin/windows; `govulncheck ./...`.
- [ ] Commit `docs: Piano 11 notifications in README, CHANGELOG, smoke checklist and gap plan`

## Rischi e mitigazioni

- **Comando lento o bloccato:** timeout 5 s con kill; il toast d'errore è una volta sola; il comando gira in un goroutine del `tea.Cmd`, mai nel loop.
- **Email ostile:** i dati entrano solo in env e vengono sanitizzati; il comando non passa mai per una shell, quindi nessuna iniezione di metacaratteri.
- **Fetch intestazioni in più sul Watch:** un ENVELOPE per evento, sui soli nuovi UID, senza `\Seen`.
- **Notifiche all'avvio:** esclusi dalla baseline del Watch e dal gate del primo load.
- **Demo:** nessuna posta simulata, quindi la demo non notifica da sola (D11-11).
- **Linux non verificato nel repo:** l'esempio `notify-send` si prova solo a mano (smoke di Andrea su Linux, se disponibile): dichiarato nel README e nel report.

## Revisione indipendente (review del coordinatore)

Decisioni prese dopo la review indipendente di `feat/piano-11` (punti numerati come nel report di review):

| ID | Decisione | Motivazione |
|---|---|---|
| D11-12 (review 3) | Conteggio e notifica sui soli NON letti | `Change.Unread` = messaggi nuovi senza `\Seen` (`UID SEARCH NOT SEEN`). Dopo una riconnessione con posta arrivata offline parte UNA notifica riassuntiva; se i nuovi sono tutti letti (letti da un altro client) non parte nulla. Lo stesso vale per il Watch in regime. |
| D11-13 (review 7) | Un processo di notifica alla volta | Se un comando è in corso, gli eventi successivi accumulano non letti e mittente dell'ultimo evento; il riepilogo parte alla fine del processo precedente. Evita una raffica di processi con un comando lento. |
| D11-14 (review 2) | Intestazioni solo degli ultimi 5 UID | `fetchRefs` su tutti i nuovi UID teneva il gate per molto tempo dopo posta offline. Si leggono le intestazioni dei 5 UID più alti; il conteggio resta reale. |
| D11-15 (review 1) | Ambiente filtrato | Il comando non riceve `GMAIL_APP_PASSWORD` né le `GMAIL_TUI_*` preesistenti. Le sole variabili GMAIL_TUI_* del comando sono quelle del messaggio. |
| D11-16 (review 4) | Timeout sull'intero gruppo di processi | Su unix il comando gira in un proprio gruppo (`Setpgid`) e il timeout uccide il gruppo: nessun figlio sopravvive. Su Windows resta il solo processo diretto (limite documentato). |
| D11-17 (review 5) | Eseguibile: percorso assoluto o nome nel PATH | `./x` e `scripts/x` sono rifiutati in `Validate` con errore chiaro: il comando dipenderebbe dalla cartella di lavoro. |
| D11-18 (review 6) | Armamento solo dal load della INBOX | Il primo load riuscito di una cartella diversa (es. Lavoro) non arma le notifiche: il Watch osserva INBOX (D11-2). |
| D11-19 (review 9) | CSI C1 U+009B come ESC [ | Il corpo della sequenza viene rimosso insieme all'introduttore, come per ESC [. |

## Alternative scartate

- **Stringa comando interpretata da `sh -c`:** scartata da D11-3. Il contenuto dell'email finirebbe nella shell.
- **Dipendenza di notifica (`beeep`, `go-notify`):** scartata da D11-1 (zero dipendenze, l'utente sceglie).
- **Campo di testo nel menu `,` per il comando:** scartato da D11-10: l'utente scriverebbe una stringa-shell.
- **Dati da `ListMessages` dopo il refresh:** scartato. La lista mostra solo la cartella corrente, e il refresh non distingue i nuovi dai vecchi senza uno stato in più.
