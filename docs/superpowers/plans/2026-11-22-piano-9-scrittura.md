# gmail-tui — Piano 9: Scrittura (bozze, ripresa, contatti, annulla invio) — Implementation Plan

> **For agentic workers:** branch `feat/piano-9` (da `main` 8f852c3). Leggere `AGENTS.md`. TDD vero: per ogni task il test scritto prima va visto rosso (RED riportato nel report), poi verde. Commit per task, checkbox `- [ ]`. Gate dopo ogni task: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `golangci-lint run ./...` a 0 issues.

**Goal:** completare la scrittura del composer: bozze salvate in Gmail (cartella Bozze) ed effettivamente riprese; rubrica locale e comando esterno per l'autocompletamento di A/Cc; annullamento dell'invio entro 5 secondi. Il markdown → HTML è già consegnato nel Piano 7.2 ed è escluso.

**Stack:** nessuna dipendenza nuova. Nessun CGO. Nessun import di go-imap fuori da `internal/mail/imap`. Nessun comando esterno eseguito via shell.

## Decisioni

Decisioni di Andrea (non riaprirle):

| ID | Decisione | Scelta |
|---|---|---|
| D9-A | Salvataggio bozza | `esc` nel composer con bozza non vuota la salva con APPEND nella mailbox con ruolo `drafts` (flag `\Draft` + `\Seen`), toast "bozza salvata", ritorno alla vista di provenienza. Errore: il composer resta aperto con la bozza. |
| D9-B | Risalvataggio | Una bozza riaperta e risalvata sostituisce la precedente: APPEND nuova, poi rimozione della vecchia per UID **solo** nella mailbox Bozze (UID STORE `\Deleted` + UID EXPUNGE, mai EXPUNGE generico, mai fuori da Bozze). |
| D9-C | Scarto | `ctrl+d` apre la conferma "scartare la bozza? s/esc" (sostituisce la conferma di scarto su `esc`). `s` chiude il composer e, se la bozza è in Bozze, la rimuove da lì (stessa rimozione per UID). Bozza vuota senza copia salvata: chiusura diretta. |
| D9-D | Ripresa | Nella cartella Bozze (selettore `m`) `invio` su una riga riapre la bozza nel composer: destinatari, oggetto, corpo markdown (firma tolta dal corpo, verrà riaggiunta). Gli allegati non sono ricostruibili da file locali: la ripresa li elenca in un toast d'avviso ("allegati non ripresi: …"). |
| D9-E | Invio da bozza | All'invio riuscito di una bozza ripresa, la bozza viene tolta da Bozze (come Gmail). Un errore di rimozione non annulla l'invio: toast d'errore separato. |
| D9-F | Rubrica locale | Indirizzi dei messaggi caricati (mittenti dalle liste, destinatari dai messaggi aperti) e di quelli inviati, in `UserCacheDir/gmail-tui/contacts.json` (0600, scrittura atomica). Ordinati per frequenza, poi recenza. Mittenti da lista: aggiunti se nuovi, senza incremento (la lista si rilegge a ogni refresh). Destinatari da messaggio aperto e invii: incremento. |
| D9-G | Comando esterno | Opzionale, in config: `address_book_cmd = ["khard", "email", "--parsable", "%s"]` (array, niente shell). Il `%s` in ciascun argomento è sostituito dalla query; senza `%s` la query è l'ultimo argomento. Timeout 2 s. Output: una riga per contatto `indirizzo<TAB>nome` (nome opzionale). Errore o timeout: si mostrano solo i suggerimenti locali, con toast d'avviso. |
| D9-H | UI suggerimenti | Nei campi A e Cc, mentre si digita l'ultimo indirizzo (testo dopo l'ultima virgola), compare una lista di al più 5 suggerimenti sotto il campo. `ctrl+n`/`ctrl+p` scelgono la riga, `invio` inserisce `Nome <indirizzo>, ` (o solo l'indirizzo) al posto del frammento. `su`/`giù` restano la navigazione dei campi, `tab`/`shift+tab` invariati. Con lista vuota `invio` non fa nulla di nuovo. |
| D9-I | Annulla invio | `s` nella conferma non invia subito: il composer si chiude, toast "invio tra 5 s — u annulla", il timer di 5 s parte. `u` (fuori dal composer) riporta il composer com'era. Allo scadere parte l'invio. Se l'app si chiude prima, l'invio parte subito dentro l'uscita, con timeout `cmd.SendTimeout`. |
| D9-J | Sola lettura MCP | I nuovi metodi `MailStore` (`SaveDraft`, `DeleteDraft`) NON sono esposti al server MCP: il `Reader` di `internal/mcpserver` non li include, e la suite di sicurezza resta verde. |

Scelte del piano (da confermare solo se contestate):

| ID | Scelta | Motivazione |
|---|---|---|
| D9-1 | Il contratto `MailStore` riceve i byte RFC 5322 (`SaveDraft(ctx, raw []byte, replace []msg.MessageID)`), non `msg.Outgoing`. MIME costruito dall'app con `smtp.BuildMessage` (esportazione di `buildMIME`). | Il backend IMAP non dipende da SMTP né dal MIME; lo store resta un trasporto di messaggi. Stessa logica del MIME usato per l'invio. |
| D9-2 | Il "Ccn" dell'elenco dei gap non esiste nel composer (nessun campo Bcc, `internal/ui/composer/composer.go`). Non lo aggiungo: il suggerimento copre A e Cc. | Un campo nuovo cambia la navigazione con tab: decisione di prodotto separata. Da segnalare nel report. |
| D9-3 | "Bozza vuota" = nessun destinatario, oggetto, allegato e corpo (prima si guardava solo il corpo). | Una bozza con il solo A compilato va salvata, non persa. |
| D9-4 | Con il composer aperto su una bozza ripresa, `esc` risalva (sostituisce la precedente) anche senza modifiche. | Coerenza: `esc` salva sempre. |

## Global Constraints

- Tasti: `esc` salva/chiude; `ctrl+d` scarta; `ctrl+s` apre la conferma invio; `u` annulla l'invio (solo fuori dal composer); `ctrl+n`/`ctrl+p` scelgono un suggerimento; `invio` in Bozze riprende. Nessuno dei tasti esistenti cambia significato fuori dal composer (`u` è libero nella lista; nel thread `u` è scroll: il ramo di `u` vale solo quando c'è un invio in attesa).
- Ogni operazione IMAP sulle bozze passa dal gate (`idleGate`); la rimozione usa `deleteAndExpunge` con mailbox Bozze esplicita; gli ID vengono validati (prefisso = Bozze) PRIMA di toccare il server.
- Niente credenziali né connessioni reali nei test: IMAP su `imapmemserver`, SMTP su server in-process (già nel repo), comando esterno con il pattern `TestHelperProcess` (il binario di test rilancia sé stesso).
- Colori solo da slot del tema. Commenti e stringhe utente in italiano, con riferimento "Piano 9, Task N".
- Modifiche a `config` e `settings` piccole e isolate: il Piano 11 (notifiche) lavora in parallelo su quei file.
- Il file di rubrica non è mai scritto in demo (rubrica solo in memoria).

## File Structure

| File | Ruolo |
|---|---|
| `internal/mail/store.go` | `SaveDraft`, `DeleteDraft` nel contratto (Task 2) |
| `internal/mail/mock.go` (+ `mock_draft.go` nuovo) | Mock: cartella Bozze di default, salvataggio e rimozione (Task 2) |
| `internal/mail/imap/draft.go` (nuovo) | APPEND con `\Draft`, rimozione per UID in Bozze (Task 3) |
| `internal/mail/smtp/mime.go` | `BuildMessage` esportato (Task 4) |
| `internal/contacts/book.go` (nuovo) | rubrica su file/memoria, `Observe`, `Suggest` (Task 8) |
| `internal/contacts/lookup.go` (nuovo) | comando esterno senza shell, timeout, parsing (Task 10) |
| `internal/config/config.go` | campo `AddressBookCmd` (Task 10) |
| `internal/app/draft.go` (nuovo) | salvataggio, scarto, ripresa, rimozione dopo invio (Task 5-7) |
| `internal/app/undo_send.go` (nuovo) | annulla invio: stato, timer, flush in uscita (Task 9) |
| `internal/app/suggest.go` (nuovo) | ricerca suggerimenti (locale + comando), risultati stanti (Task 12) |
| `internal/app/model.go`, `compose.go`, `messages.go`, `keys.go` | routing dei nuovi messaggi e tasti |
| `internal/ui/composer/composer.go` (+ `suggest.go` nuovo) | lista suggerimenti, `ctrl+n/p`, `invio` su suggerimento, hint (Task 11) |
| `internal/mail/demo.go` | bozza di esempio in Bozze, rubrica in memoria in demo (Task 13) |
| `cmd/gmail-tui/main.go` | apertura della rubrica nella cache, passaggio del comando (Task 10) |
| `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, piano dei gap | documentazione (Task 14) |

## Task 1: piano e commit

**Produces:** questo file, committato da solo.

- [ ] Scrivere il piano (questo documento).
- [ ] `git add docs/superpowers/plans/2026-11-22-piano-9-scrittura.md && git commit -m "docs: piano 9 scrittura (bozze, contatti, annulla invio)"`.

## Task 2: contratto MailStore e Mock per le bozze

**Produces:** `SaveDraft(ctx, raw []byte, replace []msg.MessageID) (msg.MessageID, error)` e `DeleteDraft(ctx, ids []msg.MessageID) error` nel contratto; Mock con cartella Bozze di default (`[Gmail]/Drafts`, ruolo `drafts`).

**Test (RED prima):**
- `TestMockSaveDraftAppearsInDrafts`: `SaveDraft` di un raw valido → ID `[Gmail]/Drafts:N`, visibile in `ListMessages` con `Label: "[Gmail]/Drafts"`.
- `TestMockSaveDraftReplacesOld`: con `replace` = [id vecchio] il vecchio scompare, il nuovo resta, nessun altro messaggio toccato.
- `TestMockDraftOpsRefuseOtherMailboxes`: `replace` o `DeleteDraft` con un ID dell'INBOX → errore, nessuna modifica.
- `TestMockSaveDraftWithoutDraftsBox`: Mailboxes senza ruolo drafts → errore.
- `TestMockListMailboxesDefault` aggiornato con la cartella Bozze.

- [x] Test RED riportati (errore di compilazione per simboli mancanti, poi fallimento comportamentale dopo lo stub).
- [x] Implementazione nel contratto (`store.go`, con commento Piano 9, Task 2) e nel Mock (`mock_draft.go`).
- [x] Gate verdi, commit `feat(mail): SaveDraft e DeleteDraft nel contratto e nel Mock (Piano 9, Task 2)`.

## Task 3: backend IMAP per le bozze

**Produces:** `internal/mail/imap/draft.go`.
- `SaveDraft`: sotto gate; rifiuta se non esiste la mailbox ruolo `drafts`; `APPEND` con flag `\Draft \Seen` e lettura del `AppendData.UID`; ID = `NewID(drafts, uid)`; poi, se `replace` non vuoto, validazione (tutti gli ID nella mailbox Bozze), `deleteAndExpunge(ctx, replace, drafts)`. Se la rimozione fallisce, errore esplicito "bozza salvata ma la precedente non rimossa" con l'ID nuovo.
- `DeleteDraft`: validazione Bozze, `deleteAndExpunge`. Senza UIDPLUS (né IMAP4rev2): rifiuto prima di toccare il server, come `Trash`.

**Test (RED prima), su `imapmemserver`:**
- `TestSaveDraftAppendsWithDraftFlag`: la bozza finisce in Bozze con `\Draft`.
- `TestSaveDraftReplaceRemovesOnlyOldUID`: due bozze, replace di una: resta solo la nuova; l'INBOX intatto.
- `TestDeleteDraftRefusesInboxIDs`: nessuna chiamata al server (stato invariato).
- `TestSaveDraftNoDraftsMailbox`: errore pulito.

- [x] RED riportato, implementazione, gate, commit `feat(imap): bozze in Bozze con APPEND \Draft e rimozione per UID (Piano 9, Task 3)`.

## Task 4: MIME esportato per le bozze

**Produces:** `smtp.BuildMessage(m msg.Outgoing, now time.Time) ([]byte, error)` (wrapper di `buildMIME`, doc con Piano 9, Task 4).

**Test (RED prima):** `TestBuildMessageRoundTripsWithMsgParse`: un `Outgoing` con `Markdown: true`, senza allegati, con To/Subject/corpo → `msg.Parse` restituisce To, Subject e BodyText (markdown sorgente). Nessun Bcc negli header (D9-1).

- [x] RED, implementazione, commit `refactor(smtp): BuildMessage esportato per le bozze (Piano 9, Task 4)`.

## Task 5: esc salva, ctrl+d scarta con conferma

**Produces:** `internal/app/draft.go`: `saveDraftCmd(store, out, replace)` (build MIME + SaveDraft dentro la cmd), messaggio `draftSaved{id, err}`; stato `Model.draftID`, `Model.draftBusy`; `handleComposerCancel` riscritto (D9-A, D9-3); `ctrl+d` in `keys.go` (`Discard`) apre `discardOpen`; `s` nel modale chiama `discardDraft`, che rimuove la bozza salvata se presente.

**Test (RED prima), con Mock:**
- `TestEscSavesDraftAndClosesComposer`: composer con A e corpo, `esc` → Mock ha la bozza in Bozze, toast "bozza salvata", view tornata.
- `TestEscEmptyDraftNoSave`: composer vuoto, `esc` → nessuna bozza, nessun toast.
- `TestEscKeepsComposerOnSaveError`: Mock con `Err` → composer aperto, toast d'errore.
- `TestCtrlDAsksDiscardConfirm` e `TestCtrlDDiscardDeletesSavedDraft`: il secondo parte da una bozza con `draftID` e verifica la rimozione.

- [x] RED, implementazione, gate, commit `feat(app): esc salva la bozza in Bozze, ctrl+d scarta con conferma (Piano 9, Task 5)`.

## Task 6: ripresa dalla cartella Bozze

**Produces:** in `draft.go`: `resumeDraft(ev)` (con `msg.Parse` del messaggio, firma tolta in coda al corpo, `draftID` impostato, allegati → toast d'avviso); in `folders.go`/`model.go`: `invio` su una riga quando `m.folder.Role == "drafts"` apre la ripresa invece del thread.

**Test (RED prima):**
- `TestEnterInDraftsResumesComposer`: Mock con una bozza in Bozze, cartella Bozze, `invio` → composer con A, oggetto, corpo; `draftID` = ID della bozza.
- `TestResumeStripsSignature`: bozza con firma → nel corpo del composer la firma non compare due volte.
- `TestResumeWarnsAboutAttachments`: bozza con allegato → toast "allegati non ripresi".
- `TestResavePreservesSingleCopy`: riprendere e `esc` → Bozze contiene una sola bozza (quella nuova).

- [x] RED, implementazione, commit `feat(app): riprende una bozza dalla cartella Bozze (Piano 9, Task 6)`.

## Task 7: invio da bozza ripresa

**Produces:** `sendCmd` riceve il `draftID`: dopo l'invio riuscito chiama `DeleteDraft` (D9-E); errore di rimozione → opDone con toast d'errore "bozza non rimossa da Bozze: …"; se la cartella corrente è Bozze, il refresh è richiesto.

**Test (RED prima):** `TestSendFromResumedDraftRemovesFromDrafts` (dopo l'invio la bozza non è più in Bozze, `Sent` contiene il messaggio); `TestSendFailureKeepsDraft` (Send che fallisce → la bozza resta).

- [x] RED, implementazione, commit `feat(app): invio di una bozza ripresa la toglie da Bozze (Piano 9, Task 7)`.

## Task 8: rubrica locale

**Produces:** `internal/contacts/book.go`: `Open(path string) (*Book, error)` (path "" = solo memoria), `Observe(now time.Time, bump bool, people ...string) error`, `Suggest(query string, limit int) []Suggestion`. Scrittura atomica 0600 (file temporaneo nella stessa directory + rename). Ordine: Count desc, Last desc, Addr asc. Matching: sottostringa case-insensitive su indirizzo o nome.

**Test (RED prima), in `internal/contacts`:** `TestObserveOrdersByFrequencyThenRecency`; `TestObserveWithoutBumpKeepsCount`; `TestSuggestMatchesNameAndAddress`; `TestFileIs0600AndRoundTrips`; `TestCorruptFileIsAnError`; `TestMemoryBookWritesNothing`.

- [x] RED, implementazione, commit `feat(contacts): rubrica locale da mittenti e destinatari (Piano 9, Task 8)`.

## Task 9: annulla invio

**Produces:** `internal/app/undo_send.go`: `undoSend *pendingSend{seq int, comp composer.Model, out msg.Outgoing, draftID}`. `s` nella conferma: chiude il composer (view = `composeBack`), toast "invio tra 5 s — u annulla", `tea.Tick(5s)` → `sendDueMsg{seq}`. `u` fuori dal composer con un invio in attesa: ripristina il composer (view = viewComposer, `composeBack` = vista corrente), annulla il timer (seq stantio). `quitCmd`: se c'è un invio in attesa, lo consegna prima di `tea.QuitMsg` con `cmd.SendTimeout`.

**Test (RED prima):** `TestConfirmSendWaitsFiveSeconds` (nessun invio dopo `s`; `sendDueMsg` con il seq giusto → `Sent` contiene il messaggio); `TestUndoRestoresComposer` (`s` → `u` → composer con lo stesso corpo; il timer successivo non invia); `TestUndoIgnoredInsideComposer` (`u` digitato nel composer resta testo); `TestQuitFlushesPendingSend` (`q` durante l'attesa → `Sent` contiene il messaggio prima del QuitMsg).

- [x] RED, implementazione, commit `feat(app): annulla invio con ritardo di 5 secondi (Piano 9, Task 9)`.

## Task 10: comando esterno dei contatti

**Produces:** `config.Config.AddressBookCmd []string` (`toml:"address_book_cmd"`); `internal/contacts/lookup.go`: `Lookup(ctx, argv []string, query string) ([]Suggestion, error)`: `exec.CommandContext` senza shell, `%s` sostituito, output parsato (righe senza indirizzo valido scartate, tetto 50 righe), exit non zero → errore con stderr. `cmd/gmail-tui/main.go`: apre `UserCacheDir/gmail-tui/contacts.json` (dir 0755 come la cache).

**Test (RED prima):** `TestLookupPassesQueryAsArgument` (helper process: l'argomento arriva intatto, anche con spazi e `;`); `TestLookupParsesTabSeparated`; `TestLookupTimeoutKillsProcess`; `TestLookupNonZeroExitIsError`.

- [ ] RED, implementazione, commit `feat(contacts): comando esterno address_book_cmd senza shell (Piano 9, Task 10)`.

## Task 11: suggerimenti nel composer (UI)

**Produces:** `internal/ui/composer/suggest.go`: stato `suggest` e `sel` per i campi To/Cc; ogni modifica del frammento corrente emette `SuggestQueryMsg{Field, Query}` (D9-H); `SetSuggestions(field, query, items)` accetta il risultato solo se il frammento corrente è ancora `query`; righe sotto il campo (massimo 5) nello stile `ListNormal`/`ListSelected`; `reservedLines` le conta; `ctrl+n`/`ctrl+p`/`invio` come da D9-H.

**Test (RED prima):** `TestTypingEmitsSuggestQuery`; `TestSetSuggestionsIgnoresStaleQuery`; `TestEnterInsertsSelectedSuggestion`; `TestEnterWithoutSuggestionsKeepsField`; `TestSuggestionRowsFitLayout`.

- [ ] RED, implementazione, commit `feat(composer): suggerimenti dei contatti in A e Cc (Piano 9, Task 11)`.

## Task 12: suggerimenti nell'app

**Produces:** `internal/app/suggest.go`: `SuggestQueryMsg` → se query vuota, lista vuota; altrimenti cmd che interroga rubrica locale + comando esterno (timeout 2 s; errore del comando → toast d'avviso una volta per sessione) → `suggestionsLoaded{field, query, items}`; risultati stanti scartati dal composer.

**Test (RED prima):** `TestSuggestionsFromLocalBook`; `TestSuggestionsMergeExternalAndDedupe`; `TestStaleSuggestionsDiscarded`.

- [ ] RED, implementazione, commit `feat(app): suggerimenti da rubrica locale e comando esterno (Piano 9, Task 12)`.

## Task 13: rubrica alimentata dai messaggi; demo

**Produces:** mittenti da `msgsLoaded` (`Observe(bump=false)`), destinatari da `msgLoaded`/`composeLoaded`/`previewLoaded` (`bump=true`), destinatari e mittente dell'invio riuscito dentro `sendCmd` (`bump=true`). Demo: rubrica solo in memoria (path ""), una bozza di esempio nella cartella Bozze del DemoStore.

**Test (RED prima):** `TestLoadedListFeedsBook`; `TestOpenedMessageBumpsRecipients`; `TestDemoDraftsFolderHasSample`; `TestDemoBookWritesNoFile`.

- [ ] RED, implementazione, commit `feat(app): rubrica alimentata da messaggi e invii; bozza d'esempio in demo (Piano 9, Task 13)`.

## Task 14: documentazione e gate finali

**Produces:** README (tasti: `ctrl+d`, `u`, `ctrl+n/p`, `invio` in Bozze; config `address_book_cmd`), CHANGELOG (sezione Piano 9), `docs/smoke-checklist.md` (sezione "Piano 9 — scrittura", da eseguire da Andrea su account reale: bozza salvata e visibile in Gmail web, ripresa, rimozione dopo invio, suggerimenti, annulla invio), piano dei gap (Piano 9 consegnato, con la nota su Ccn di D9-2).

- [ ] Gate finali: build, vet, `go test -race -count=1 ./...`, `golangci-lint run ./...` 0 issues; build `CGO_ENABLED=0` per linux/darwin/windows; `govulncheck ./...` pulito; `grep -rn "go-imap" internal --include="*.go" -l | grep -v internal/mail/imap` vuoto.
- [ ] Prova in tmux con HOME temporanea (demo): bozza salvata, ripresa, suggerimenti, annulla invio.
- [ ] Commit `docs: Piano 9 (README, CHANGELOG, smoke, piano dei gap)`.

## Rischi

| Rischio | Mitigazione |
|---|---|
| Rimozione dalla Bozze elimina la bozza sbagliata | ID validati per prefisso Bozze prima di ogni comando; test che l'INBOX non cambia. |
| APPEND riuscito, rimozione della vecchia fallita: due bozze | Errore esplicito con il nuovo ID; nessun retry automatico. |
| Doppio invio (timer + flush in uscita) | Timer e flush consumano lo stesso `undoSend`, che si azzera prima della consegna; test su entrambi. |
| Suggerimenti stantii o lenti | Risultati legati alla query; comando con timeout 2 s, fuori dal loop UI. |
| Comando esterno che legge dati sensibili | Nessuna password passa al comando; niente shell; output trattato come testo, non eseguito. |
| Conflitti con il Piano 11 su config/settings | Solo un campo in `config.Config`; nessuna modifica a `settings.go`. |
| Ccn assente (D9-2) | Segnalato; decisione separata. |
| Smoke reale non eseguibile da me | Non eseguito: nessuna connessione a Gmail; la sezione è nello smoke checklist per Andrea. |
