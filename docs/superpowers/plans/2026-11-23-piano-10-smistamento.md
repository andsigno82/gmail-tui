# gmail-tui — Piano 10: Smistamento (selezione multipla, annulla, label su selezione) — Implementation Plan

> **For agentic workers:** branch `feat/piano-10` (da `main` e1a66a2). Leggere `AGENTS.md`. TDD vero: per ogni task il test scritto prima va visto rosso (RED riportato nel report), poi verde. Commit per task, checkbox `- [ ]`. Gate dopo ogni task: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `~/go/bin/golangci-lint run ./...` a 0 issues.

**Goal:** smistare più conversazioni in una volta. Selezione con `x`/`X`, azioni `a`/`#`/`s`/`v`/`l` su tutta la selezione, annulla di archivia e cestino (toast 10 s, `u`), pannello label con stato misto (aggiunte e rimozioni insieme). Su Gmail (X-GM-EXT-1) le label si modificano da qualunque cartella con STORE ±X-GM-LABELS.

**Stack:** nessuna dipendenza nuova. Nessun CGO. Nessun import di go-imap fuori da `internal/mail/imap`.

## Decisioni

Decisioni di Andrea (non riaprirle):

| ID | Decisione | Scelta |
|---|---|---|
| D10-S1 | Selezione | `x` marca/smarca la riga sotto il cursore (casella ☑/☐ a inizio riga, slot tema); `X` seleziona tutte le righe caricate; `esc` annulla la selezione. |
| D10-S2 | Azioni su selezione | `a`/`#`/`l`/`s`/`v` agiscono su tutte le conversazioni selezionate, sempre nella cartella corrente (ConversationMembers e guard esistenti). |
| D10-S3 | Reset | La selezione si azzera dopo un'azione o cambiando cartella/ricerca. Con selezione attiva la barra di stato (neutra) mostra "N selezionate". |
| D10-U1 | Annulla | Dopo archivia o cestino (anche multipli): toast "archiviate N — u annulla" per 10 s; `u` riporta tutto dov'era. |
| D10-U2 | Precedenza di `u` | `u` è lo stesso tasto dell'annulla invio: vale per l'ULTIMA azione annullabile. Un nuovo archivia/cestino sostituisce l'annulla precedente; un invio in attesa resta inviato allo scadere (testato). `u` solo in viewInbox/viewThread. |
| D10-L1 | Label | `l` apre un elenco di tutte le label con ☑ su quelle presenti in TUTTE le selezionate, ☒/◪ (stato misto) se solo in alcune; spazio cambia lo stato; digitare filtra; invio applica aggiunte e rimozioni insieme; si può creare una label nuova digitandone il nome. |
| D10-L2 | Label su Gmail | Aggiunte/rimozioni via STORE ±X-GM-LABELS (`StoreGmailLabels` della patch, PATCHES.md) da qualunque cartella. Senza X-GM-EXT-1: solo aggiunta (COPY, come oggi); rimozione solo della label uguale alla cartella corrente. |
| D10-V1 | `v` e `s` su selezione | Come Gmail: `v` → se almeno una è non letta le segna tutte lette, altrimenti tutte non lette; `s` → se almeno una è senza stella la mette a tutte, altrimenti la toglie a tutte. |

Scelte tecniche del piano (da confermare solo se contestate):

| ID | Scelta | Motivazione |
|---|---|---|
| D10-T1 | Selezione indicizzata per ID della riga (`Conversation.Latest.ID`), non per indice. | Gli indici cambiano a ogni refresh silenzioso; l'ID della riga no. |
| D10-T2 | `Trash(ctx, ids) ([]msg.MessageID, error)` restituisce gli ID nel cestino (da `MoveData.DestUIDs`, UIDPLUS/MOVE). | Serve a `u` per riportare indietro con un MOVE preciso. Nessun EXPUNGE generico. |
| D10-T3 | Nuovo `Restore(ctx, ids, dest) ([]msg.MessageID, error)`: UID MOVE dal cestino a `dest`; guard: gli ID devono stare nella mailbox con ruolo `trash`. | Annulla cestino. Su Gmail il MOVE di ritorno ripristina la cartella d'origine; le altre label perse col cestino NON sono ripristinate (limite dichiarato, da verificare in smoke). |
| D10-T4 | Nuovo `AllMailIDs(ctx, ids) ([]msg.MessageID, error)` (solo Gmail): per ogni ID in INBOX legge `X-GM-MSGID` e cerca lo stesso messaggio in `[Gmail]/Tutti i messaggi` con `SEARCH X-GM-MSGID`. | Annulla archivia: il ritorno in INBOX va fatto sugli ID di All Mail dei SOLI messaggi archiviati, non su tutta la conversazione (i messaggi Inviati del thread non devono ricevere \Inbox). |
| D10-T5 | Nuovo `ModifyLabels(ctx, ids, add, remove []string) error`. Gmail: STORE ±X-GM-LABELS per mailbox di appartenenza (guard: ids nella cartella corrente). Senza Gmail: add → ApplyLabel on; remove → ApplyLabel off solo se l'ID sta nella mailbox-label, altrimenti errore prima di toccare il server. | Unica primitiva per il pannello label. `ApplyLabel` resta per il caso singolo (test esistenti). |
| D10-T6 | Annulla archivia = `ModifyLabels(add=["\\Inbox"])` sugli ID di All Mail da `AllMailIDs`. Senza X-GM-EXT-1 `AllMailIDs` fallisce e il toast NON offre `u`. | Spec del piano dei gap. La rimozione di \Inbox equivale ad archiviare su Gmail (documentazione Gmail IMAP): da verificare in smoke reale. |
| D10-T7 | Annulla e selezione operano sugli ID della conversazione nella cartella (`conversationIDs`), come `a`/`#` singoli. | Coerenza con Gmail web e con `guardBox`. |

Decisioni del coordinatore sui dubbi del piano (Piano 10, Task 4):

| ID | Decisione | Motivazione |
|---|---|---|
| D10-C1 | Annulla archivia su Gmail: ripristino di `\Inbox` con STORE +X-GM-LABELS sugli ID di All Mail dei soli messaggi archiviati (`AllMailIDs`, da X-GM-MSGID). Mai expunge. Senza X-GM-EXT-1 l'annulla dell'archivia non è disponibile (errore; la UI non offre `u`). | Non riportare in INBOX i messaggi Inviati del thread. |
| D10-C2 | Annulla cestino: MOVE di ritorno dal cestino alla cartella d'origine con gli UID di UIDPLUS (`MoveData`). Su Gmail le label utente si riapplicano con STORE +X-GM-LABELS: le passa il chiamante (`Restore` fa solo il MOVE, non conosce le label perse). Mai EXPUNGE generico; il MOVE di ritorno parte solo dalla mailbox con ruolo `trash`. | Il chiamante ha le label dalla riga; evita di far restituire a `Trash` dati non richiesti. |
| D10-C3 | `ModifyLabels(ids, add, remove)` via STORE ±X-GM-LABELS (`StoreGmailLabels` della patch go-imap; PATCHES.md aggiornato quando è usato, Task 5). Sicuro su ID di Tutti i messaggi (tocca solo le label). `\Inbox` in add/remove è ammesso (archivia/ripristina); gli altri attributi di sistema sono rifiutati e `\Inbox` non va esposto come label utente. | Unica primitiva per il pannello label e per l'annulla. |
| D10-C4 | I nuovi metodi (`Restore`, `AllMailIDs`, `ModifyLabels`) sono di SCRITTURA: il server MCP (`internal/mcpserver`) non li espone. Il Reader resta invariato; la sua suite di sicurezza è verde. | Sicurezza dati MCP (AGENTS.md). |
| D10-C5 | Cache: dopo `ModifyLabels` e `Restore` si aggiornano le copie per GmailID (stessa regola dei flag: solo le entry già in cache). | Coerenza con `storeFlags`/`updateCachedLabels`. |
| D10-C6 | Trash/Restore/ModifyLabels rispettano il gate (ops serializzate) come le altre op; nessun EXPUNGE generico. | AGENTS.md, regola 2. |

## Global Constraints

- Tasti: `x` marca, `X` tutte, `esc` azzera la selezione (solo viewInbox, senza modali); `u` annulla solo in viewInbox/viewThread e solo se c'è un'azione annullabile; `l` con selezione apre il pannello label.
- Ogni operazione IMAP passa dal gate (`idleGate`); nessun EXPUNGE generico; nessun expunge su `[Gmail]/Tutti i messaggi` (guard `refuseAllMail` e `guardBox` esistenti).
- `MailStore` resta l'unico contratto della UI. I nuovi metodi di scrittura (`Restore`, `AllMailIDs`, `ModifyLabels`) NON sono esposti al server MCP (`internal/mcpserver`): la sua suite di sicurezza resta verde.
- Risultati stantii scartati con `listGen`; cache per GmailID aggiornata (`UpdateGmailCopies`) dove possibile.
- Colori solo da slot del tema (checkbox su slot di `ListSelected`/`Accent`, contrasto WCAG 4.5:1). Commenti e stringhe utente in italiano con riferimento "Piano 10, Task N".
- Timer iniettabili (variabile di pacchetto come `undoWindow`): i test consegnano il messaggio di scadenza direttamente, senza `sleep`.
- Nessuna credenziale reale, nessuna connessione Gmail reale nei test. Verifica in Gmail web solo nello smoke (`docs/smoke-checklist.md`).

## File Structure

| File | Ruolo |
|---|---|
| `internal/app/select.go` (nuovo) | stato di selezione, `x`/`X`/`esc`, reset su cambio cartella/ricerca (Task 2) |
| `internal/ui/inbox/inbox.go` | casella ☑/☐ a inizio riga (Task 2) |
| `internal/app/bulk.go` (nuovo) | a/#/s/v su selezione, risoluzione ID e guard (Task 3) |
| `internal/mail/store.go`, `mock.go`, `mock_bulk.go` (nuovo) | `Trash` con ID, `Restore`, `AllMailIDs`, `ModifyLabels` (Task 4) |
| `internal/mail/imap/trash.go`, `restore.go` (nuovo), `gmail_labels.go` (nuovo) | backend IMAP/Gmail (Task 5) |
| `internal/app/undo.go` (nuovo) | ultima azione annullabile, toast 10 s, `u` (Task 6) |
| `internal/app/labels_picker.go` (nuovo), `internal/ui/labelpick/` (nuovo) | pannello label con stato misto (Task 7) |
| `internal/mcpserver/*` | nessuna esposizione dei nuovi metodi, test di sicurezza (Task 8) |
| `internal/mail/demo.go`, `internal/app/demo.go` | selezione, annulla e label in demo (Task 9) |
| `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, piano dei gap, `AGENTS.md` | documentazione (Task 9) |

## Task 1: piano e commit

**Produces:** questo file, committato da solo.

- [x] Scrivere il piano (questo documento).
- [ ] `git add docs/superpowers/plans/2026-11-23-piano-10-smistamento.md && git commit -m "docs: piano 10 smistamento (selezione, annulla, label)"` con la riga Co-Authored-By.

## Task 2: selezione nella lista

**Produces:** `Model.sel` (set di ID di riga), `x` toggle sulla riga del cursore, `X` seleziona tutte le righe caricate (se tutte già selezionate, azzera), `esc` azzera; reset su cambio cartella o ricerca; casella ☑/☐ nel delegate; "N selezionate" nella barra di stato.

**Test (RED prima):**
- `TestXTogglesRowUnderCursor`: `x` sul cursore seleziona, secondo `x` deseleziona.
- `TestXCapitalSelectsAllLoaded`: `X` seleziona tutte; `X` di nuovo azzera.
- `TestEscClearsSelectionInInbox`: con selezione, `esc` azzera senza altri effetti.
- `TestSelectionResetOnFolderChange` e `TestSelectionResetOnSearch`.
- `TestRowRendersCheckbox` (`internal/ui/inbox`): ☑ per selezionata, ☐ altrimenti.
- `TestStatusBarShowsSelectedCount`: "2 selezionate" con due righe; assente senza selezione.

- [x] RED riportato, implementazione, gate, commit `feat(app): selezione multipla nella lista con x/X/esc (Piano 10, Task 2)`.

## Task 3: azioni su selezione (a, #, s, v)

**Produces:** `internal/app/bulk.go`: con selezione attiva `a` e `#` operano su tutte le conversazioni selezionate (`conversationIDs` per riga, `guardBox` sulla cartella corrente); `s` e `v` con la regola Gmail (D10-V1). Dopo l'azione la selezione si azzera (D10-S3). Il refresh è profondo per archivia/cestino.

**Test (RED prima), con Mock:**
- `TestBulkArchiveArchivesAllSelected`: 3 conversazioni selezionate → fuori dall'INBOX nel Mock, selezione vuota.
- `TestBulkTrashAllSelected`; in cartella Cestino la riga è rifiutata con "già nel cestino", nessuna op.
- `TestBulkStarRule` e `TestBulkReadRule`: le due regole D10-V1 nei due sensi.
- `TestBulkOpStaleResultDropped`: risultato con `listGen` vecchio non tocca la lista.

- [x] RED riportato, implementazione, gate, commit `feat(app): a/#/s/v su tutta la selezione (Piano 10, Task 3)`.

## Task 4: contratto e Mock per annulla e label

**Produces:** in `internal/mail/store.go`: `Trash` con ritorno `[]msg.MessageID` (D10-T2); `Restore` (D10-T3); `AllMailIDs` (D10-T4); `ModifyLabels` (D10-T5). Mock con gli stessi comportamenti: `Restore` rifiuta ID fuori dal cestino; `AllMailIDs` rifiuta senza Gmail (flag del Mock); `ModifyLabels` rifiuta la rimozione fuori dalla cartella-label in modalità senza Gmail.

**Test (RED prima), in `internal/mail`:** `TestMockTrashReturnsIDsInTrash`, `TestMockRestoreMovesBackToOrigin`, `TestMockRestoreRefusesIDsOutsideTrash`, `TestMockAllMailIDsRefusesWithoutGmail`, `TestMockModifyLabelsAddRemove`, `TestMockModifyLabelsRemoveOutsideLabelRefused`.

- [ ] RED riportato (errore di compilazione, poi fallimento comportamentale), implementazione, gate, commit `feat(mail): contratto per annulla e label su selezione (Piano 10, Task 4)`.

## Task 5: backend IMAP

**Produces:** `trash.go` (Trash restituisce `MoveData.DestUIDs`), `restore.go` (MOVE di ritorno con guard trash), `gmail_labels.go` (AllMailIDs: FETCH `X-GM-MSGID` in INBOX + `SEARCH X-GM-MSGID` in All Mail; ModifyLabels: STORE ±X-GM-LABELS con `StoreGmailLabels`, cache con `UpdateGmailCopies`).

**Test (RED prima):** su `imapmemserver`: `TestTrashReturnsNewIDs`, `TestRestoreMovesBackFromTrash`, `TestRestoreRefusesNonTrashIDs` (nessuna chiamata al server), `TestModifyLabelsNoGmailAddCopies`, `TestModifyLabelsNoGmailRemoveOutsideFolderRefused`. Su server scriptato (`net.Pipe`, come `gmail_store_test.go`): `TestAllMailIDsScripted`, `TestModifyLabelsGmailStoreScripted`, `TestAllMailIDsNoGmailErrors`.

- [ ] RED riportato, implementazione, gate, commit `feat(imap): trash con ID, restore e label Gmail via STORE (Piano 10, Task 5)`.

## Task 6: annulla (archivia, cestino) e precedenza di `u`

**Produces:** `internal/app/undo.go`: slot `Model.lastUndo` (kind: send | archive | trash, con ID e mailbox di ritorno). Archivia: prima `AllMailIDs` (se fallisce, nessun `u`, D10-T6), poi `Archive`. Cestina: salva gli ID restituiti da `Trash`. Toast "archiviate N — u annulla" per `undoBulkWindow` (variabile, 10 s). `u` in viewInbox/viewThread: ultima azione → `Restore` o `ModifyLabels(add=\Inbox)`, poi refresh. Timer con seq stantio ignorato.

**Test (RED prima), con timer iniettato:** `TestUndoArchiveRestoresInbox`, `TestUndoTrashRestoresOrigin`, `TestUndoNotOfferedWithoutGmail`, `TestUndoExpiredIgnored`, `TestUndoPrecedenceLastActionWins`, `TestSendWindowReplacedByLaterArchive` (l'invio in attesa parte comunque allo scadere), `TestUndoIgnoredInComposerAndSearch`.

- [ ] RED riportato, implementazione, gate, commit `feat(app): annulla archivia e cestino con u, precedenza all'ultima azione (Piano 10, Task 6)`.

## Task 7: pannello label su selezione

**Produces:** `labels_picker.go` e `internal/ui/labelpick/`: elenco delle label (`ListMailboxes` con ruolo vuoto) più quelle digitate; stato per label: ☑ se in tutte le selezionate, ◪ se in alcune, ☐ se in nessuna. Spazio cambia stato; digitare filtra; invio → una sola `ModifyLabels(add, remove)`; nome nuovo digitato → aggiunto. Senza X-GM-EXT-1 le rimozioni sono offerte solo per la cartella corrente.

**Test (RED prima):** `TestLabelPickerMixedState`, `TestLabelPickerSpaceToggles`, `TestLabelPickerFilterByTyping`, `TestLabelPickerEnterAppliesAddAndRemoveTogether`, `TestLabelPickerNewLabelByName`, `TestLabelPickerNoGmailRemoveOnlyCurrentFolder`.

- [ ] RED riportato, implementazione, gate, commit `feat(app): pannello label su selezione con stato misto (Piano 10, Task 7)`.

## Task 8: sicurezza MCP e cache

**Produces:** test che fallisce se `internal/mcpserver` vede `Restore`, `AllMailIDs` o `ModifyLabels`; la suite di sicurezza resta verde. Cache: `ModifyLabels` aggiorna `Ref.Labels` delle copie per GmailID.

- [ ] RED riportato, implementazione, gate, commit `test(mcpserver): metodi di scrittura del Piano 10 fuori dal server (Piano 10, Task 8)`.

## Task 9: demo, tmux e documentazione

**Produces:** demo con selezione, annulla e label (`internal/mail/demo.go`); prova in tmux con HOME temporanea (selezione, `u`, elenco label); sezione Piano 10 in `docs/smoke-checklist.md` (in Gmail web: archivia → `u` → la conversazione torna in INBOX; cestina → `u` → torna; label con stato misto; rimozione da un'altra cartella); README (tasti), CHANGELOG, piano dei gap, AGENTS.md.

- [ ] Demo verificata in tmux, documentazione aggiornata, commit `docs: Piano 10 (README, CHANGELOG, smoke, piano dei gap)`.

## Task 10: gate finale

- [ ] `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `golangci-lint` a 0 issues.
- [ ] Build `CGO_ENABLED=0` per linux, darwin, windows.
- [ ] `govulncheck ./...` pulito.

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Rimuovere `\Inbox` su Gmail non equivale sempre ad archiviare | Test scriptato del comando; verifica reale in smoke; dichiarato in report fino ad allora. |
| Annulla cestino non ripristina le label perse col cestino | Dichiarato (D10-T3): il MOVE ripristina la cartella d'origine. |
| `u` con un invio in attesa | Precedenza esplicita (D10-U2) con test: l'invio non si perde. |
| `AllMailIDs` lento su selezioni grandi (una SEARCH per messaggio) | Accettato in v1; da rivedere se serve. |
| MOVE di ritorno su UID cambiati | Errore nel toast, lista al refresh; nessun EXPUNGE generico. |

## Revisione indipendente

Dopo il Task 9: review da un agente diverso sul diff di `internal/mail/imap` (guard e comandi) e su `internal/app/undo.go` (precedenza di `u`).
