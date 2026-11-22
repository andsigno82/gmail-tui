# gmail-tui — Piano 7: uso quotidiano (paginazione, cartelle, cestino, validazione destinatari) — Implementation Plan

> **For agentic workers:** esecuzione subagent-driven (implementer + reviewer indipendente per task, come Piani 1-6). Steps con checkbox (`- [ ]`). Leggere prima `AGENTS.md` (regole di architettura, gotchas) e `internal/mail/store.go` (contratto).

**Goal:** colmare le lacune che impediscono l'uso quotidiano: (F1) vedere la posta oltre i primi 50 messaggi, (F2) navigare cartelle e label, (F3) spostare nel cestino, (F4) validare i destinatari prima dell'invio.

**Architecture:** due metodi nuovi nel contratto `mail.MailStore` (`ListMailboxes`, `Trash`), implementati in `internal/mail/imap` e nel `Mock`. Tutto il resto è UI in `internal/app`/`internal/ui` sopra il contratto. La paginazione NON cambia il contratto: `mail.Page.Offset` esiste già. La validazione è logica di dominio in `internal/msg`, usata sia dal composer sia dalla CLI `send`.

**Tech Stack:** go-imap v2.0.0-beta.9 (`Client.List` con `ListOptions.ReturnSpecialUse`, `Client.Move`, `imap.MailboxAttrTrash` — verificati nel sorgente pinnato) · stdlib `net/mail` · bubbles/bubbletea v2 già presenti.

## Decisioni (confermate da Andrea il 2026-11-22: tutti i default)

| ID | Decisione | Scelta |
|---|---|---|
| D1 | Conversazioni Gmail: patch in-tree di go-imap per `X-GM-THRID`/`X-GM-RAW`/`X-GM-LABELS`, raggruppamento lato client, o niente | **Fuori da questo piano**: Piano 8 dedicato se si sceglie la patch. Il raggruppamento lato client è sconsigliato: le risposte dell'utente stanno in Sent, non in INBOX, quindi le conversazioni risulterebbero monche |
| D2 | Markdown → HTML (nuova dipendenza, es. goldmark) | Fuori da questo piano (resta in ROADMAP v1.1) |
| D3 | Tasti | `#` = cestino (come Gmail web), `m` = selettore cartelle. `g`/`G` NON sono liberi: li usa bubbles/list (inizio/fine) |
| D4 | Conferma prima del cestino | Sì, modale come il discard della bozza (nessun undo in v1.1) |
| D5 | Profondità del refresh silenzioso dopo op/Watch | Rilista tutta la profondità caricata (tetto 500) solo per `ChangeNewMail`, `ChangeDisconnected` e dopo `Archive`/`Trash`; per `Star`/`MarkRead`/`ChangeFlags` basta la prima pagina, dato che quelle op non cambiano la composizione della lista |
| D6 | Cambio cartella con una ricerca attiva | La ricerca viene azzerata: la cartella nuova si apre senza filtri |
| D7 | Dopo `#` dalla thread view | Si torna alla lista (il messaggio non è più nella cartella). `Archive` e `Star` dal thread restano come oggi |

## Global Constraints

- Nessuna nuova dipendenza Go. `CGO_ENABLED=0` su linux/darwin/windows.
- `go test -race ./...` verde, `golangci-lint run ./...` 0 issues, `govulncheck ./...` pulito.
- Nessun import di go-imap fuori da `internal/mail/imap` (check in AGENTS.md).
- TDD: test rosso prima. Test IMAP solo su `imapmemserver` in-process; test app solo con `*mail.Mock`.
- Commenti e stringhe UI in italiano; riferimento `Piano 7, Task N` nei commenti delle decisioni non ovvie.
- Anti-regressione: nessuna firma esistente di `MailStore` cambia; i test esistenti restano verdi senza modifiche. Eccezioni previste e da motivare nel commit: i test che dipendono dal Mock che ignora `Label` (Task 1) e le firme interne `loadRefs`/`submitQuery`/`refreshRefs`, che ricevono la mailbox (Task 4).
- Nessun `EXPUNGE` non-UID: mai espungere messaggi non richiesti.

## File Structure

```
internal/mail/store.go, mock.go (+mock_test.go)          T1: contratto ListMailboxes/Trash
internal/mail/imap/mailboxes.go (+mailboxes_test.go)      T1: LIST + ruoli SPECIAL-USE
internal/mail/imap/mutate.go (+mutate_test.go)            T1: Trash via UID MOVE
internal/msg/validate.go (+validate_test.go)              T2: ValidateRecipients
internal/cmd/send.go (+send_test.go)                      T2: validazione CLI
internal/ui/inbox/inbox.go (+inbox_test.go)               T3: AppendRefs, AtEnd
internal/app/model.go, ops.go, messages.go, keys.go       T3-T6
internal/app/folders.go (+folders_test.go)                T4: selettore cartelle
internal/app/*_test.go                                     T3-T6
README.md, ROADMAP.md, CHANGELOG.md, AGENTS.md,           T7
docs/smoke-checklist.md, site/index.html,
docs/superpowers/specs/2026-11-20-gmail-tui-design.md
```

## Waves

```
Wave 1 (parallelo, file disgiunti)   T1 backend contract + IMAP     T2 validazione msg + CLI
            │                                   │
Wave 2 (sequenziale, un solo implementer su internal/app: i task toccano tutti model.go/keys.go)
            T3 paginazione → T4 cartelle (dip. T1) → T5 cestino UI (dip. T1) → T6 validazione composer (dip. T2)
            │
Wave 3      T7 documentazione   ∥   T8 verifica finale (Livello B), dopo T7
```

Subagent: Wave 1 = 2 `general-purpose` in parallelo; Wave 2 = 1 implementer + 1 reviewer indipendente per task; Wave 3 = 1 agente docs, poi review finale.

---

### Task 1: contratto `ListMailboxes` + `Trash` (store, Mock, IMAP)

**Files:** `internal/mail/store.go`, `internal/mail/mock.go` (+test), create `internal/mail/imap/mailboxes.go` (+test), modify `internal/mail/imap/mutate.go` (+test).

**Produces:**
```go
// internal/mail/store.go
type Mailbox struct {
	Name string // nome IMAP (es. "[Gmail]/Cestino"): è il valore per Query.Label
	Role string // "" | "inbox" | "sent" | "drafts" | "trash" | "junk" | "all" | "flagged"
}
// nel MailStore:
ListMailboxes(ctx context.Context) ([]Mailbox, error)
Trash(ctx context.Context, ids []msg.MessageID) error
```

- `ListMailboxes` (IMAP): `c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true})`; scarta le mailbox con `imap.MailboxAttrNoSelect` (es. `[Gmail]`); mappa gli attributi `\Sent \Drafts \Trash \Junk \All \Flagged` nel `Role`; `INBOX` ha role `inbox`. Ordine: INBOX, poi le speciali in ordine fisso (Speciali, Inviati, Bozze, Tutti, Spam, Cestino), poi le label in ordine alfabetico. La logica sta in un helper interno SENZA gate (`listMailboxes(c)`); il metodo pubblico fa `defer s.gate.acquire()()` e chiama l'helper. Il gate NON è rientrante (`imap.go`, commento su acquire): Trash non deve chiamare il metodo pubblico.
- `Trash` (IMAP), sotto gate:
  1. trova la mailbox con role `trash` tramite l'helper `listMailboxes(c)` (nessuna → errore `"cestino non trovato"` senza toccare messaggi);
  2. rifiuta ids già nella mailbox cestino (errore, nessuna azione sugli altri);
  3. rifiuta se il server non ha né `imap.CapMove` né `imap.CapUIDPlus`: il fallback di `Client.Move` in beta.9 userebbe `EXPUNGE` generico (verificato in `imapclient/move.go`);
  4. per ogni mailbox d'origine (`groupByMailbox`): `Select` + `c.Move(imap.UIDSetNum(uids...), trash).Wait()`.
  - Cache: nessuna invalidazione (chiave mailbox+uid; la entry d'origine resta orfana e innocua). Documentarlo nel commento.
- **Watch e selezione (bug preesistente, diventa bloccante con le cartelle):** `idleRound` entra in IDLE senza `Select` (`imap/watch.go`, `c.Idle()`), e INBOX viene riselezionata solo in `recheckNewMail`. Dopo un `ListMessages` su un'altra cartella, l'IDLE osserverebbe quella cartella e la posta nuova in INBOX non arriverebbe fino al timer di re-IDLE. Correzione: `Select(st.mailbox)` all'inizio di ogni `idleRound`, prima di `Idle()`. Test memserver: list su un'altra cartella, poi mail nuova in INBOX → arriva `ChangeNewMail`. Lo stesso caso esiste già oggi dopo `ApplyLabel` off.
- `Mock`: `Mailboxes []Mailbox` configurabile (default: INBOX + `[Gmail]/Trash` role trash). **Il Mock deve filtrare per mailbox**: oggi `matches` ignora `Label` (`mock.go`, commento "q.Label: non supportato dal mock"). `ListMessages`/`Search` filtrano per prefisso mailbox dell'ID (`MessageID.Split`), con `Label == ""` uguale a `INBOX`. `Trash` sposta i messaggi cambiando il prefisso dell'ID nella mailbox cestino. `Err` iniettabile come le altre op. Senza questo, demo mode e test app di T4/T5 non dimostrano nulla. Verificare che `app.DemoStore` metta i messaggi seed in `INBOX:`.
- Impatto sui chiamanti (qui, non in T8, altrimenti T1 non compila): `cmd/gmail-tui/main.go` (`selectStore`, `backendFactory`), `app.DemoStore`, i wrapper del Mock nei test (`internal/app/connect_test.go`, recorder in `search_test.go`).
- Contract comments in `store.go` coerenti con lo stile esistente (semantica Gmail, gate, errori).

**Test (imapmemserver):** Watch dopo list su un'altra cartella (vedi sopra); mailbox cestino creata con `user.Create("[Gmail]/Trash", &imap.CreateOptions{SpecialUse: []imap.MailboxAttr{imap.MailboxAttrTrash}})` (memserver la restituisce con `ReturnSpecialUse`, verificato in `imapmemserver/mailbox.go`). Casi: ruoli mappati e `\Noselect` scartata; Trash sposta (sparisce da INBOX, compare nel cestino); cestino assente → errore e INBOX intatta; id già nel cestino → errore; ids misti da due mailbox.

- [ ] RED → impl → GREEN → vet/lint/race → Commit `feat(mail): ListMailboxes and Trash in MailStore (IMAP SPECIAL-USE, UID MOVE)`

### Task 2: validazione destinatari (dominio + CLI)

**Files:** create `internal/msg/validate.go` (+test); modify `internal/cmd/send.go` (+test).

**Produces:**
```go
// ValidateRecipients controlla To/Cc/Bcc con net/mail.ParseAddress per ogni
// elemento. Errore se nessun destinatario o se un indirizzo è malformato
// (il messaggio cita l'indirizzo e il campo).
func ValidateRecipients(o Outgoing) error
```
- Validi: `a@b.it`, `Mario Rossi <m@x.it>`. Non validi: `abc`, `a@`, stringa con soli spazi.
- `cmd.Send`: chiama `msg.ValidateRecipients` dopo `splitAddrs`/`dedupAddrs` e PRIMA di `sender.Send`; errore → stderr ed exit code 1, coerente con gli altri errori di `send.go` e con `send_test.go`.

- [ ] RED → impl → GREEN → Commit `feat(msg): recipient validation before SMTP (CLI send)`

### Task 3: paginazione dell'inbox

**Files:** `internal/ui/inbox/inbox.go` (+test), `internal/app/model.go`, `ops.go`, `messages.go` (+test).

**Produces:**
- `inbox.Model.AppendRefs(refs []msg.MessageRef) tea.Cmd` (dedup per ID, mantiene il cursore) e `inbox.Model.AtEnd() bool` (cursore sull'ultimo item).
- Stato app: `exhausted bool` (ultima pagina ricevuta < 50 o vuota) e `loadingMore bool` (una pagina in flight: mai due insieme).
- Trigger: dopo ogni tasto/mouse in inbox, se `AtEnd() && !exhausted && !loadingMore && queryRaw == ""` → `ListMessages(Query{Label: m.mailbox}, Page{Offset: len(m.refs), Limit: 50})` → nuovo msg `moreLoaded{refs, err}`.
- Refresh silenzioso (`refreshRefs`), secondo D5: `Limit = min(max(50, len(m.refs)), 500)` per `ChangeNewMail`/`ChangeDisconnected`/`Archive`/`Trash`; `Limit: 50` per le altre op. Il tetto 500 va annotato con un commento in italiano (`Piano 7, Task 3`: limite noto; per superarlo, paginare anche il refresh). Ogni refresh ricalcola `exhausted`.
- `msgsLoaded`/`moreLoaded` portano la `mailbox` (Task 4) e la query di origine: un risultato che non corrisponde allo stato corrente viene scartato (stesso pattern anti-stantio di `fetchMsg`).
- La ricerca (`Search`) resta non paginata (contratto invariato).
- Il recorder dei test (`search_test.go`) va esteso per registrare anche `pg`.

**Test:** Mock con 120 messaggi: load iniziale 50; cursore in fondo → 100; poi 120 + `exhausted`; nessuna richiesta doppia con `loadingMore`; dedup quando arriva posta nuova tra due pagine; messaggio archiviato tra due pagine (caso "salto": il refresh a profondità piena lo recupera); refresh dopo archive mantiene 100 ref; refresh dopo star usa `Limit: 50`.

- [ ] RED → impl → GREEN → Commit `feat(app): load older mail on scroll (inbox pagination)`

### Task 4: selettore cartelle e label (dip. T1)

**Files:** create `internal/app/folders.go` (+test); modify `model.go`, `keys.go`, `ops.go`.

**Produces:**
- Stato app `mailbox string` ("" = INBOX), SEPARATO da `query`/`queryRaw` per non alterare badge e refresh della ricerca.
- Tasto `m` (D3) in inbox: `ListMailboxes` → modale con bubbles/list (stile del prompt label, solo slot tema); invio = `m.mailbox = Name`, reset di paginazione (`exhausted`/`loadingMore`), re-list; esc chiude. Etichette leggibili per i ruoli ("Inviati", "Cestino"…), nome IMAP per le label.
- `loadRefs`/`submitQuery`/`refreshRefs`/pagina successiva ricevono la mailbox e usano `Query{Label: m.mailbox}` (cambio di firma interno, da citare nel commit); una ricerca senza `label:` esplicito eredita `m.mailbox`.
- Cambio cartella: azzera la ricerca (D6), azzera la paginazione, è un load NON silenzioso (cursore in cima, status con il nome della cartella).
- Risultati stantii: `msgsLoaded`/`moreLoaded` con `mailbox` diversa da `m.mailbox` vengono scartati (un refresh o un Watch partito nella cartella A non deve sovrascrivere la lista della cartella B).
- Status bar mostra la cartella corrente quando non è INBOX.
- Mouse disattivato con la modale aperta: aggiungere la modale al gate mouse di `model.go` e a `shortHelpFor`; aggiungere `m` (e in T5 `#`) a `routedKeys` in `keys_test.go`, altrimenti `TestRoutedKeysAreDocumented` non li copre.

**Test:** apertura/chiusura modale; selezione → ListMessages con Label giusta (recorder); ricerca dentro cartella; cambio cartella con ricerca attiva la azzera; refresh dopo op resta nella cartella; risultato stantio della cartella precedente scartato; con la modale aperta i tasti globali (`a`, `s`…) e il mouse non agiscono.

- [ ] RED → impl → GREEN → Commit `feat(app): folder and label picker (m)`

### Task 5: cestino nella UI (dip. T1)

**Files:** `model.go`, `keys.go`, `ops.go` (+test).

- Tasto `#` (D3) in inbox e thread: apre la conferma (D4), stesso pattern di `discardOpen`: `"spostare nel cestino? s conferma · esc annulla"` con stile `StatusErr`.
- Conferma → `Trash` fuori dal loop UI → `opDone` → refresh silenzioso (come `Archive`); dalla thread view si torna alla lista (D7).
- Nella vista Cestino (`m.mailbox` con role trash) `#` non apre la conferma: status "già nel cestino".
- Errore (es. cestino non trovato) → status di errore, lista invariata.

**Test:** conferma/annulla; Trash chiamato con l'ID giusto; errore mostrato; dalla thread view torna alla lista.

- [ ] RED → impl → GREEN → Commit `feat(app): move to trash (#) with confirmation`

### Task 6: validazione nel composer (dip. T2)

**Files:** `model.go` (gestione di `composer.SendRequestedMsg`, oggi intorno a model.go:486) (+test).

- Prima di aprire la conferma d'invio, DOPO i guard esistenti (stantio, `sending`, `discardOpen`, oggi subito dopo il case `composer.SendRequestedMsg`): `msg.ValidateRecipients(draft)`; errore → status di errore, il composer resta aperto con la bozza intatta, nessuna conferma. L'ordine conta: `compose_test.go` invia un `Outgoing` vuoto come richiesta stantia e deve continuare a essere scartato in silenzio.

**Test:** indirizzo malformato → niente conferma, bozza intatta; indirizzo valido → flusso invariato (test di invio esistenti verdi).

- [ ] RED → impl → GREEN → Commit `feat(app): validate recipients before send confirmation`

### Task 7: documentazione

- `README.md`: keymap (`#`, `m`, caricamento automatico in fondo alla lista); Notes: "IDLE osserva solo INBOX"; validazione dei destinatari.
- `site/index.html`: stessa keymap.
- `ROADMAP.md`: v1.1 aggiunge paginazione, cartelle, cestino; Later aggiunge "Conversazioni Gmail (X-GM-THRID/X-GM-RAW) — richiede patch go-imap" (D1).
- `CHANGELOG.md`: sezione `[Unreleased]`.
- `docs/smoke-checklist.md`: voci per scorrimento oltre 50, cambio cartella, cestino su account reale (verificare in Gmail web che il messaggio sia nel Cestino), indirizzo malformato.
- `docs/superpowers/specs/2026-11-20-gmail-tui-design.md`: sezione MailStore con i due metodi nuovi.
- `AGENTS.md`: gotcha "`g`/`G` occupati da bubbles/list"; regola "Trash mai con EXPUNGE generico".

- [ ] Commit `docs: v1.1 daily-use features (pagination, folders, trash, validation)`

### Task 8: verifica finale (Livello B, obbligatoria)

- [ ] `go build ./... && go vet ./...`, `go test -race ./...`, `golangci-lint run ./...`, `govulncheck ./...`, `CGO_ENABLED=0 GOOS={linux,darwin,windows} go build ./...`
- [ ] Check go-imap fuori da `internal/mail/imap` → nessun output
- [ ] Skill `criticalthink --iterative` su tutto il diff del branch + `/code-review`
- [ ] Analisi impatti: ricontrollo dei chiamanti di `MailStore` già adattati in T1 e del Watch dopo cambio cartella
- [ ] Anti-regressione: suite esistente verde senza modifiche non motivate; demo mode (`go run ./cmd/gmail-tui` → `s`) con prova manuale di `m`, `#` e scorrimento
- [ ] Smoke su account reale da `docs/smoke-checklist.md` (lo esegue Andrea: credenziali reali)

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Offset per conteggio instabile: posta nuova tra due pagine (duplicati) o archiviata (un messaggio saltato) | Dedup per ID; dopo `Archive`/`Trash` il refresh rilista l'intera profondità e recupera il salto |
| Il refresh di molti ref diventa lento | D5: profondità piena solo dove cambia la composizione della lista; tetto 500 commentato |
| Watch osserva la cartella sbagliata dopo un cambio cartella | `Select(INBOX)` prima di ogni IDLE (T1) con test dedicato |
| Risultato di un refresh della cartella precedente sovrascrive la lista | `mailbox` nei messaggi di risultato, scarto se non corrisponde (T3/T4) |
| Deadlock: Trash sotto gate che richiama ListMailboxes | Helper interno senza gate (T1) |
| Demo e test app falsamente verdi perché il Mock ignora `Label` | Mock filtra per mailbox (T1) |
| Il fallback di `Move` usa `EXPUNGE` generico (data loss) | Trash rifiuta i server senza MOVE né UIDPLUS (Gmail li ha entrambi) |
| Nome del cestino localizzato (`[Gmail]/Cestino`) | Ruolo da SPECIAL-USE, mai il nome |
| Push assente per le cartelle diverse da INBOX | Limite documentato; il refresh dopo le op resta attivo |
| Regressione di ricerca/badge separando cartella e query | `mailbox` separato da `query`; i test esistenti di search/refresh restano invariati |
| Conflitti di merge su `internal/app` | Wave 2 sequenziale, un solo implementer |

## Alternative scartate

- Cestino via `ApplyLabel("[Gmail]/Trash")`: COPY non toglie il messaggio da INBOX.
- Tasti a sequenza stile Gmail (`g i`): `g` è già occupato e una sequenza richiede una macchina a stati; una modale con un tasto basta.
- Conversazioni lato client: risultato mediocre su Gmail (risposte in Sent); rimandate a D1.
