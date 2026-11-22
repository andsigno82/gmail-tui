# gmail-tui — Piano 8: estensioni Gmail (X-GM-EXT-1) — Implementation Plan

> **For agentic workers:** branch `feat/piano-8` (da `main` e6961e2). Leggere `AGENTS.md`. TDD vero (test rosso verificato prima del fix), commit per task, checkbox `- [ ]`. La decisione D8-1 è già presa da Andrea; D8-2 è proposta qui e da confermare. Le domande Q1-Q5 richiedono Andrea prima della Wave 4.

**Goal:** tre capacità Gmail che IMAP standard non dà: (1) la lista mostra una riga per conversazione, e la vista thread mostra tutta la conversazione, comprese le risposte in Inviati; (2) la ricerca `/` passa la sintassi Gmail completa a `X-GM-RAW`; (3) ogni messaggio ha le sue label (`X-GM-LABELS`), visibili in lista e rimovibili da qualunque cartella (prerequisito del Piano 10).

**Stack:** nessuna dipendenza nuova oltre alla copia in-tree di go-imap (D8-1). Nessun CGO. Per il decoding delle label (modified UTF-7) si riusa `github.com/cention-sany/utf7`, già indiretta in `go.mod`.

## Fonti verificate

| Fonte | Cosa fornisce | Verifica |
|---|---|---|
| Google, [IMAP Extensions](https://developers.google.com/workspace/gmail/imap/imap-extensions) | Esempi di wire ufficiali: `FETCH (X-GM-MSGID …)`, `FETCH (X-GM-THRID …)`, `FETCH (X-GM-LABELS (\Inbox \Sent Important "Muy Importante"))`, `STORE +X-GM-LABELS`, `SEARCH X-GM-LABELS`, `SEARCH X-GM-THRID`, `SEARCH X-GM-MSGID` | Verificata solo tramite snippet di ricerca. La pagina va riletta in Wave 1 (Task 2) prima di fissare i fixture |
| jeiang/atrium [#73](https://github.com/jeiang/atrium/issues/73) | Patch di ~255 righe su beta.8: MSGID/THRID uint64 ("one test id is above 2^63"), LABELS con backslash, quoted, modified UTF-7, lista vuota; StoreLabels `+/-/set X-GM-LABELS[.SILENT]`; `SearchCriteria.Gmail`; skip dei msg-att sconosciuti invece di chiudere la connessione | Letta via WebFetch. Non contiene righe di wire: la pagina dichiara "hand-written from Google's docs, not captured" |
| riadafridishibly/go-imap [#12](https://github.com/riadafridishibly/go-imap/issues/12) | Forme: ID decimali uint64 (`1278455344230334865`); `SEARCH X-GM-MSGID n` / `X-GM-THRID n` → `* SEARCH …`; LABELS `("\\Inbox")`; il thread ID coincide con il MSGID di un messaggio del thread; UID dopo gli item Gmail | Letta via WebFetch. Il valore più alto osservato su account reale è 1876409439502535953 (< 2^63) |
| riadafridishibly/go-imap [#19](https://github.com/riadafridishibly/go-imap/issues/19), [PR #29](https://github.com/riadafridishibly/go-imap/pull/29) | X-GM-RAW ("takes Gmail web search syntax as a string"); LABELS con `utf7.Decode` salvo connessione UTF-8 | Solo riferimento: #19 non letta in Wave 0, da leggere in Task 3 |
| excavador/mail-mcp [PR #4](https://github.com/excavador/mail-mcp/pull/4) (fork `v2.0.0-beta.8.gmext.1`) | Uso reale: `search server: true` invia la query "verbatim as X-GM-RAW on the \\All folder"; ID stabile `gm:<MSGID>` quando la capability c'è; test con fake TLS che registra i byte | Letta via WebFetch. Il codice del fork non è riportato in pagina: non è una fonte di forme di risposta |

Nota: nel commento di #73 i riferimenti "#759" e "#334" puntano a PR di atrium non pertinenti. La chiusura di X-GM-LABELS da parte del maintainer upstream è quindi **non verificata**. Non cambia la decisione D8-1.

## Decisioni

| ID | Decisione | Scelta |
|---|---|---|
| D8-1 | Patch go-imap | Copia in-tree in `third_party/go-imap` da `v2.0.0-beta.9` (module cache), `replace` in `go.mod`. Già deciso da Andrea. |
| D8-2 | Formato ID Gmail `uint64` | **Proposta:** `MessageID` resta `MAILBOX:UID` invariato. Gli ID Gmail vanno in campi dedicati e tipizzati: `MessageRef.GmailID` e `MessageRef.ThreadID`, entrambi `uint64` (0 = non disponibile). Motivazioni: (a) `MessageID` è chiave di cache, di Archive/Trash/MarkRead e dei Change di Watch, tutti basati su UID per mailbox: cambiarne il formato toccherebbe ogni op; (b) l'UID IMAP è `uint32` per mailbox, il MSGID è globale e più grande: due spazi diversi non vanno compressi in una stringa; (c) `uint64` evita l'overflow signed (`int64`) segnalato da atrium #73 e confronta i thread senza parsing di stringhe. Conseguenza: i valori `uint64` non vanno in JSON come numero (oltre 2^53 JavaScript perde precisione): ogni serializzazione (MCP, output CLI) li scrive come stringa decimale. |
| D8-2 (approvata) | Formato ID | Confermata dal coordinatore su delega di Andrea: `MessageID` invariato, `GmailID`/`ThreadID` `uint64` dedicati, stringa decimale in JSON. |
| D8-3 (Q1) | Paginazione della lista | **Deviazione accettata dal coordinatore.** Implementata: `Page.Offset`/`Limit` di `ListConversations` contano CONVERSAZIONI, non messaggi. Ogni chiamata scandisce i messaggi dal più recente fino a riempire `offset+limit` thread distinti, con tetto di 500 messaggi letti per chiamata (`maxMessaggiScansionati`). Costo O(offset) per pagina, nessuno stato tra le chiamate. Limite noto: oltre il tetto la pagina può essere più corta del limite, e a offset grandi una conversazione può non comparire. Documentato in `internal/mail/imap/conversation.go`. |
| D8-4 (Q2) | Ambito di `/` | Come Gmail web: senza `in:`/`label:` cerca in `[Gmail]/Tutti i messaggi` (ruolo `all`), con esclusione di spam e cestino applicata da Gmail a X-GM-RAW. Il badge dei risultati lo dichiara. |
| D8-5 (Q3) | Colore delle label | Hash deterministico del nome su un insieme di slot del tema. Ogni slot usato deve rispettare contrasto WCAG >= 4.5:1 rispetto allo sfondo su cui è disegnato; gli slot che non lo rispettano sono scartati. |
| D8-6 (Q4) | Grassetto conversazione | Sì: in grassetto se almeno un messaggio della conversazione è non letto (come Gmail). |
| D8-7 (Q5) | Spam e cestino nella conversazione | Sì: esclusi dalla vista conversazione (come Gmail), salvo quando si è dentro quelle cartelle. |

## Global Constraints

- `mail.MailStore` resta l'unico contratto della UI. `internal/app` e `internal/ui` non importano né `go-imap` né `internal/mail/imap`.
- `go-imap` (anche la copia patchata in `third_party/go-imap`) è importata SOLO da `internal/mail/imap`. La grep della regola 1 di AGENTS.md va verificata sul Task 1 perché `go.mod` e `third_party/` non devono rompere il controllo.
- Ops serializzate (`idleGate`): ogni nuovo metodo di `Store` acquisisce il gate come gli altri. Un metodo che ne chiama un altro già sotto gate usa la variante `heldGate` (il gate non è rientrante).
- Nessun CGO. `CGO_ENABLED=0` in tutti i target.
- Nessuna credenziale reale nei test e nel repo. Nessuna connessione a Gmail reale: i test usano il memserver o uno scripted server locale.
- Colori solo da slot del tema (14 slot in `internal/theme`). Le label colorate usano slot esistenti; nessun colore hard-coded.
- Commenti e doc del codice in italiano; docs pubbliche (README, CHANGELOG) in inglese.
- Breadcrumb `// Brain:` solo se esistono voci Brain pertinenti.
- Compatibilità: paginazione, cartelle, cestino, toast, layout a due colonne e mark-read in anteprima devono funzionare con la riga-conversazione. Un server senza `X-GM-EXT-1` (non-Gmail, o test) ricade sul comportamento attuale: una riga per messaggio, ThreadLen 1.
- Demo e Mock simulano conversazioni e label: la demo senza credenziali mostra il nuovo comportamento.

## File Structure

Nuovi:
- `third_party/go-imap/`: copia di `github.com/emersion/go-imap/v2@v2.0.0-beta.9`, con `LICENSE` originale e `PATCHES.md` (elenco patch e procedura di riallineamento).
- `internal/mail/imap/gmail.go`: flag `Store.gmail` da capability, costruzione criteri `X-GM-RAW`, filtro delle label utente.
- `internal/mail/imap/gmail_test.go`, `internal/mail/imap/testdata/gmail/*.txt`: transcript scriptati (vedi Test strategy nella Wave 1).
- `internal/mail/imap/conversation.go` (+ test): `ListConversations` e `ThreadMessages`.
- `internal/ui/conversation/` oppure estensione di `internal/ui/inbox` (da decidere nel Task 9).

Modificati:
- `go.mod` (`replace github.com/emersion/go-imap/v2 => ./third_party/go-imap`), `go.sum`.
- `internal/msg/message.go` (`MessageRef`: `GmailID`, `ThreadID`, `Labels`), `internal/msg/query.go` (`Query.Raw`).
- `internal/mail/store.go` (`ListConversations`, `ThreadMessages`, `Conversation`).
- `internal/mail/mock.go`, `internal/app/demo.go`.
- `internal/mail/imap/imap.go`, `search.go`, `query.go`, `fetch.go`, `mutate.go`.
- `internal/mail/cache/` (versione schema: nuovi campi in `msg.Message`).
- `internal/ui/inbox/inbox.go`, `internal/ui/thread/thread.go`, `internal/app/model.go`, `ops.go`, `search.go`, `split.go`.
- `README.md`, `CHANGELOG.md`, `docs/smoke-checklist.md`, `AGENTS.md`.

## Waves

| Wave | Contenuto | Task | Dipende da |
|---|---|---|---|
| 1 | Patch go-imap (D8-1): copia, replace, parsing e encoding X-GM-* | 1-4 | — |
| 2 | Contratto: campi `MessageRef`, `Query.Raw`, `ListConversations`, `ThreadMessages`; Mock | 5 | — (parallelizzabile con la Wave 1) |
| 3 | Backend IMAP: capability, conversazioni, ricerca `X-GM-RAW`, label, cache | 6-8 | 1, 2 |
| 4 | UI: riga-conversazione, vista thread, `/` nativo, label colorate | 9-11 | 2, risposte a Q1-Q3 |
| 5 | Demo, documentazione, verifica, smoke su account reale (Andrea) | 12-13 | 3, 4 |

### Task 1: copia in-tree di go-imap e replace

**Files:** `third_party/go-imap/` (nuovo), `go.mod`, `go.sum`, `AGENTS.md` (grep regola 1), `third_party/go-imap/PATCHES.md` (nuovo).

- Copiare `$(go env GOMODCACHE)/github.com/emersion/go-imap/v2@v2.0.0-beta.9` in `third_party/go-imap` (il module cache è read-only: copia con permessi scrivibili). Mantenere `LICENSE`. Il `go.mod` della copia resta `module github.com/emersion/go-imap/v2`.
- `go.mod`: `replace github.com/emersion/go-imap/v2 => ./third_party/go-imap`. Nessun cambio di versione nel `require`.
- `PATCHES.md`: versione upstream, elenco delle patch (una riga per patch, con il task che la introduce), procedura di riallineamento: (1) nuova copia dal tag upstream; (2) `diff -ru` contro la copia precedente per isolare le patch; (3) riapplicare le patch; (4) eseguire i test di `internal/mail/imap`; (5) aggiornare la versione in questo file.
- Verificare che la grep di AGENTS.md resti valida e che il lint (`golangci`/`staticcheck` come da CONTRIBUTING, se presenti) escluda `third_party/` in modo esplicito.
- Verificare che `go build ./...` e `go mod tidy` offline (`GOPROXY=off`) non richiedano moduli nuovi. Se ne richiedono, fermarsi e chiedere.

**Test:** `go test ./third_party/go-imap/...` verde senza modifiche (baseline della copia); `go build ./...` e `go vet ./...` verdi.

- [ ] Baseline (non c'è RED: è una copia) → impl → GREEN → Commit `chore(imap): vendor go-imap v2.0.0-beta.9 in third_party for Gmail patches`

### Task 2: FETCH X-GM-MSGID, X-GM-THRID, X-GM-LABELS e msg-att sconosciuti

**Files:** `third_party/go-imap/fetch.go`, `third_party/go-imap/imapclient/fetch.go`, `third_party/go-imap/imapclient/gmail.go` (nuovo), test accanto.

- Opzioni `FetchOptions.GmailMsgID`, `GmailThreadID`, `GmailLabels bool`. Scrittura come item `X-GM-MSGID X-GM-THRID X-GM-LABELS` quando richiesti.
- Parsing della risposta: MSGID e THRID come `uint64` decimale (errore su overflow, non per valori > 2^63); LABELS come lista di atomi (`\Inbox`), stringhe quotate con escape `\\`, modified UTF-7 decodificato con `cention-sany/utf7` salvo connessione UTF-8, lista vuota `()`.
- Msg-att sconosciuto: ignorato, non errore. Test: una risposta con un attributo inventato non chiude la connessione e gli altri item restano parsati.
- L'ordine degli item sul wire non è garantito: i test confrontano per item, non per stringa.

**Test (RED prima):** FETCH con i tre attributi dai fixture della Wave 1 (esempi Google: `* 1 FETCH (X-GM-LABELS (\Inbox \Sent Important "Muy Importante"))`, `* 3 FETCH (X-GM-LABELS ())`); MSGID `1876409439502535953` e un valore sopra 2^63; attributo sconosciuto; label in modified UTF-7 (`&ANw-bung`).

- [ ] RED → impl → GREEN → Commit `feat(imap-patch): parse X-GM-MSGID/THRID/LABELS and skip unknown fetch attributes`

### Task 3: SEARCH X-GM-RAW, X-GM-THRID, X-GM-MSGID, X-GM-LABELS

**Files:** `third_party/go-imap/search.go`, encoding in `third_party/go-imap/imapwire`, test accanto.

- `SearchCriteria.Gmail` con `Raw string`, `ThreadID uint64`, `MsgID uint64`, `Labels []string`. Encoding: `X-GM-RAW "<stringa>"` con escape di `\` e `"`; gli altri come keyword più numero decimale.
- Prima del test: leggere la issue #19 e la pagina Google, per confermare la sintassi quotata di X-GM-RAW sul fixture ufficiale e non da memoria.
- Test: query con `"` e `\` (es. `subject:"a\b"`) codificata correttamente; THRID come decimale; combinazione con i criteri standard in AND.

- [ ] RED → impl → GREEN → Commit `feat(imap-patch): SEARCH X-GM-RAW, X-GM-THRID, X-GM-MSGID, X-GM-LABELS`

### Task 4: STORE X-GM-LABELS

**Files:** `third_party/go-imap/store.go`, `third_party/go-imap/imapclient/store.go` (o `imapclient/gmail.go`), test accanto.

- `StoreLabels` con modalità `+`, `-`, set, e `.SILENT` opzionale. Nomi quotati se contengono spazi o caratteri speciali; `\Inbox` come atomo.
- Serve alla rimozione label da qualunque cartella (Piano 10). In Piano 8 la primitiva è implementata e testata ma NON esposta in `MailStore`.

- [ ] RED → impl → GREEN → Commit `feat(imap-patch): STORE X-GM-LABELS (+/-/set)`

### Task 5: contratto: `MessageRef`, `Query.Raw`, conversazioni; Mock

**Files:** `internal/msg/message.go`, `internal/msg/query.go`, `internal/mail/store.go`, `internal/mail/mock.go`, test accanto.

Nuove forme (da validare sul contratto):

```go
// msg/message.go
type MessageRef struct {
    // ... campi esistenti ...
    GmailID  uint64   // X-GM-MSGID; 0 = non Gmail
    ThreadID uint64   // X-GM-THRID; 0 = non Gmail
    Labels   []string // solo label utente (senza \Inbox, \Sent, Important, Starred, ecc.)
}

// msg/query.go
type Query struct {
    // ... campi esistenti ...
    Raw string // sintassi Gmail completa, passata come X-GM-RAW (vuota = non attiva)
}

// mail/store.go
type Conversation struct {
    Latest       msg.MessageRef // ultimo messaggio della conversazione nella mailbox richiesta
    ThreadID     uint64
    Count        int            // messaggi nella conversazione (All Mail)
    Participants []string       // mittenti distinti, in ordine cronologico
    Unread       bool           // almeno un messaggio non letto (All Mail)
    Labels       []string       // unione delle label utente
}

// Nuovi metodi di MailStore:
ListConversations(ctx context.Context, q msg.Query, pg Page) ([]Conversation, error)
ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error) // cronologico, All Mail
```

- `Query.IsEmpty()` deve considerare `Raw`. Il test di `Query{}` resta valido.
- `Search` mantiene la firma. Con `Raw` valorizzato, `HasAttachment` non è post-filtrato lato client (X-GM-RAW supporta `has:attachment`).
- `ParseQuery` resta com'è: serve a `fetch --query`, Mock e demo. Nessun token nuovo.
- Mock: `ListConversations` raggruppa per `ThreadID`; `ThreadMessages` restituisce i messaggi con lo stesso `ThreadID`; `Raw` simulata con il sottoinsieme già supportato (from, to, is:unread, label, has:attachment, testo). Sintassi non simulata (es. `older_than:`) produce errore esplicito, per non far passare test falsi: va documentato nel commento del Mock.

**Test:** `IsEmpty` con `Raw`; Mock raggruppa 3 messaggi di 2 thread in 2 conversazioni con Count, Unread, Participants corretti; `ThreadMessages` in ordine cronologico; `Raw` non vuoto non è `IsEmpty`.

- [ ] RED → impl → GREEN → Commit `feat(store): conversations, Gmail ids, labels and raw query in the MailStore contract`

### Task 6: capability X-GM-EXT-1 e modalità Gmail dello Store

**Files:** `internal/mail/imap/imap.go`, `internal/mail/imap/capabilities.go`, `internal/mail/imap/gmail.go` (nuovo), test.

- `Connect` legge la CAPABILITY e imposta `s.gmail = true` solo se è presente `X-GM-EXT-1`. Il flag è scritto in Connect sotto `mu` e riletto dalle op.
- Senza capability: `fetchRefs` non chiede X-GM-*, `GmailID`/`ThreadID`/`Labels` restano zero, `ListConversations` dà una conversazione per messaggio (Count 1). Comportamento identico a oggi.
- `doctor`: riga esplicita "Gmail extensions (X-GM-EXT-1): sì/no" (il Piano 7.1 prevede già le capability in doctor: unire i due lavori).

**Test:** scripted server con CAPABILITY con e senza `X-GM-EXT-1`: il client non invia mai X-GM-* senza la capability (verifica sul transcript).

- [ ] RED → impl → GREEN → Commit `feat(imap): detect X-GM-EXT-1 and gate Gmail extension commands`

### Task 7: backend: conversazioni, thread e ID Gmail

**Files:** `internal/mail/imap/conversation.go` (nuovo), `internal/mail/imap/imap.go` (`fetchRefs`), test.

**Algoritmo di `ListConversations` (default, vedi Q1):**
1. Come `ListMessages`: SELECT della mailbox, UID SEARCH con i criteri, finestra di pagina sui messaggi (D8-3 default).
2. UID FETCH `ENVELOPE FLAGS INTERNALDATE X-GM-MSGID X-GM-THRID X-GM-LABELS` per la pagina.
3. Raggruppamento per `THRID` nella pagina: `Latest` = il messaggio più recente nella mailbox.
4. Per i THRID della pagina, una sola SEARCH in `[Gmail]/Tutti i messaggi` (ruolo `all`, da SPECIAL-USE): `UID SEARCH OR X-GM-THRID a X-GM-THRID b …`, poi FETCH ENVELOPE FLAGS THRID per conteggio, partecipanti e Unread. Il limite di OR va spezzato in gruppi da 20, da misurare sul server reale in smoke.
5. Tutto sotto un solo holder del gate (`defer s.gate.acquire()()`); i passi interni usano la variante `heldGate`.

**`ThreadMessages(threadID)`:** SELECT di `[Gmail]/Tutti i messaggi` (ruolo `all`), UID SEARCH `X-GM-THRID n`, FETCH ENVELOPE FLAGS INTERNALDATE X-GM-LABELS, ordinamento cronologico. Spam e cestino non compaiono in All Mail: comportamento Gmail da confermare (Q5).

**Nota su `FetchMessage`:** un messaggio della conversazione si apre con l'ID di All Mail (`MessageID` invariato). `FetchMessage`, `MarkRead` e `Star` funzionano già su qualunque mailbox: il test copre l'ID di All Mail.

**Test:** transcript scriptato per 2 thread su 3 messaggi: Count, Participants, Unread (basta un non letto), Latest nella mailbox; `ThreadMessages` in ordine; server senza capability (fallback); ID sopra 2^63 propagati senza troncamento.

- [ ] RED → impl → GREEN → Commit `feat(imap): conversations and thread messages from X-GM-THRID`

### Task 8: backend: ricerca `X-GM-RAW`, label, cache

**Files:** `internal/mail/imap/search.go`, `internal/mail/imap/query.go`, `internal/mail/cache/`, test.

- Con `q.Raw != ""` e Gmail attivo: `Search` e `ListConversations` inviano `X-GM-RAW` con la query verbatim, sulla mailbox di `q.Label` se presente, altrimenti su All Mail (D8-4 default, vedi Q2). Nessun post-filtro `HasAttachment` (la query Gmail lo supporta già).
- Senza Gmail (`s.gmail == false`) con `Raw`: errore esplicito "ricerca Gmail non disponibile su questo server", non fallback silenzioso.
- Label: `Labels` valorizzato in `fetchRefs`, solo label utente: si scartano gli attributi `\...`, `Important` e `Starred`. I nomi `[Gmail]/...` non compaiono in X-GM-LABELS.
- Cache: `msg.Message` riceve i nuovi campi. Va alzata la versione dello schema cache (o invalidate le entry) così le entry vecchie non danno `GmailID = 0` a messaggi Gmail. Prima del task verificare in `internal/mail/cache` come viene serializzato il messaggio.

**Test:** `Raw` produce il criterio `X-GM-RAW` con quoting corretto (query con `"`); errore senza Gmail; label filtrate; entry di cache con versione vecchia invalidata.

- [ ] RED → impl → GREEN → Commit `feat(imap): native Gmail search via X-GM-RAW and per-message labels`

### Task 9: UI: riga-conversazione in lista

**Files:** `internal/ui/inbox/inbox.go` e test, `internal/app/model.go`, `messages.go`, `search.go`.

- La lista usa `ListConversations`. Senza Gmail il backend restituisce `Count 1` per ogni messaggio e la lista è identica a oggi.
- Riga: partecipanti (primo + "e altri N" se più di uno, come Gmail), contatore `(N)` se N > 1, oggetto del messaggio più recente, data dell'ultimo, grassetto se `Unread`. Troncamento con `…`.
- Label utente come chip colorati (slot del tema, Q3), troncate se manca spazio: la prima vince, le altre diventano `+N`.
- Selezione (`j/k`), paginazione (`pgup/pgdn`), mouse e toast restano invariati e operano sulla conversazione (l'ID della riga è `Latest.ID`).

**Test:** rendering a larghezza 80 e 140 (partecipanti, contatore, grassetto, chip troncati); fallback senza Gmail con output identico alla lista attuale (test esistenti invariati).

- [ ] RED → impl → GREEN → Commit `feat(inbox): one row per conversation with participants, count and labels`

### Task 10: UI: vista thread stile Gmail

**Files:** `internal/ui/thread/thread.go`, `internal/app/split.go`, `internal/app/model.go`, test.

- Apertura di una riga-conversazione: `ThreadMessages(ThreadID)`, messaggi in ordine cronologico.
- Stato per messaggio: **espanso** = l'ultimo e tutti i non letti; **compresso** = i letti, su una riga (mittente, data, anteprima). `Enter` su una riga la espande. Le risposte dell'utente (in Inviati) sono incluse e compresse se lette.
- Mark-read: espandere un messaggio non letto lo segna letto con la logica esistente. L'anteprima a due colonne mostra la conversazione intera.
- Tasti: nessun nuovo tasto che collida con `internal/app/keys.go` (controllare prima). `f`/`b` restano riservati (gotcha di AGENTS.md).

**Test:** conversazione di 4 messaggi: 3 letti compressi, ultimo espanso; una risposta inviata compare espansa; apertura di un non letto lo segna letto; l'anteprima a due colonne non rompe il focus.

- [ ] RED → impl → GREEN → Commit `feat(thread): Gmail-style conversation view with collapsed read messages`

### Task 11: UI: `/` con sintassi Gmail

**Files:** `internal/app/search.go`, prompt in `internal/ui/inbox`, `internal/app/cmdmode*`, test.

- `/` costruisce `Query{Raw: testo}` quando il backend è Gmail; il testo passa intatto (`older_than:`, `in:`, `-label:`).
- Senza Gmail: comportamento attuale con `ParseQuery`.
- Il badge "N risultati per …" (Piano 7.3) mostra la query raw.
- Errore del server su query non valida: toast d'errore, nessun crash, lista precedente ripristinata.

**Test:** `/older_than:7d` produce `Raw` esatto; senza Gmail usa `ParseQuery`; errore di ricerca ripristina la lista.

- [ ] RED → impl → GREEN → Commit `feat(search): pass Gmail search syntax verbatim as X-GM-RAW`

### Task 12: demo e Mock: conversazioni simulate

**Files:** `internal/app/demo.go`, `internal/app/demo_test.go`, `internal/mail/mock.go`.

- Dataset demo con almeno 3 conversazioni: una con 4 messaggi, una con risposta in Inviati, una con label utente. La demo senza credenziali mostra così il nuovo comportamento.
- La demo non deve mai chiamare la rete (test esistenti).

**Test:** lista con righe-conversazione, contatore e label visibili; apertura di una conversazione mostra la risposta dell'utente.

- [ ] RED → impl → GREEN → Commit `feat(demo): simulated Gmail conversations and labels`

### Task 13: documentazione e verifica (smoke su account reale di Andrea)

- [ ] README (lista a conversazioni, vista thread, `/` con sintassi Gmail, label), CHANGELOG `[Unreleased]` in inglese, `docs/smoke-checklist.md`, `AGENTS.md` (regola 1 aggiornata, gotcha `heldGate` per i nuovi metodi, gotcha ID uint64 come stringa in JSON).
- [ ] Gate completi: `go test ./...`, `CGO_ENABLED=0 go build ./...`, `go vet ./...`, `govulncheck ./...` (obbligatorio: la copia patchata è nel build), lint.
- [ ] `/code-review` sul branch, rilievi corretti.
- [ ] **Smoke su account reale: solo Andrea, mai l'agente.** (1) `gmail-tui doctor` riporta X-GM-EXT-1; (2) una conversazione con risposta in Inviati compare come una riga con il conteggio giusto; (3) `/older_than:30d` dà risultati coerenti con la ricerca Gmail web; (4) una label utente compare in lista e non si duplica dopo `a`; (5) catturare un transcript reale **redatto** (nessun indirizzo, oggetto o corpo) da usare come fixture in `testdata/gmail/`.
- [ ] Merge in `main` e push: chiedere conferma ad Andrea.

## Domande per Andrea (prima della Wave 4)

- **Q1: paginazione della lista.** (a) Default del piano: pagina sui messaggi della mailbox (50 messaggi), raggruppati nella pagina. Veloce, ma una pagina può mostrare meno di 50 righe. (b) Pagina sulle conversazioni: serve un FETCH X-GM-THRID di tutta la mailbox a ogni refresh, costo O(mailbox), attenuabile con il CONDSTORE del Piano 12. Quale?
- **Q2: ambito di `/` senza `in:`.** Default: All Mail, come Gmail web. Alternativa: la cartella corrente, come oggi.
- **Q3: colore delle label.** (a) slot del tema assegnato per hash del nome (stabile, più colori); (b) un solo slot per tutte le label utente (più sobrio).
- **Q4: conteggio e grassetto.** Il piano assume grassetto se almeno un messaggio della conversazione è non letto (in All Mail), come Gmail. Confermare.
- **Q5: spam e cestino nella conversazione.** Default: esclusi, come All Mail di Gmail. Confermare.

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Forme di risposta da snippet o fonti terze sbagliate | Fixture solo da Google (riletta in Task 2) e da transcript reale **redatto** nello smoke; nessun fixture inventato senza marcatura |
| Copia in-tree che diverge da upstream | `PATCHES.md` con procedura di riallineamento; lista esatta delle patch; test della copia |
| `govulncheck` e lint toccano il codice vendorizzato | Task 1 verifica; `third_party` escluso dal lint, incluso in `govulncheck` |
| Costo di `ListConversations` (SEARCH OR per THRID) | Gruppi da 20; misura reale in smoke; CONDSTORE in Piano 12 |
| Cache con entry vecchie senza ID Gmail | Bump della versione dello schema (Task 8) |
| ID sopra 2^53 persi in JSON (MCP Piano 14, output CLI) | `uint64` come stringa decimale; test di serializzazione |
| Regressioni su paginazione, cestino, toast, due colonne | Test esistenti invariati dove possibile; fallback senza Gmail identico all'attuale; Wave 4 dopo Q1 |
| Server senza X-GM-EXT-1 o sintassi diversa | Capability gate (Task 6); errore esplicito su `Raw` senza Gmail |
| Query `X-GM-RAW` non valida o con `"` nel testo | Quoting con escape (Task 3); errore mostrato come toast, lista precedente ripristinata |
| Deadlock sul gate (non rientrante) | Metodi annidati solo con `heldGate`; test con chiamate concorrenti |
| Dipendenza `cention-sany/utf7` | Già indiretta: nessuna nuova dipendenza; `go mod tidy` offline lo verifica |

## Contratto MailStore: cosa cambia (per il server MCP del Piano 14)

- **Nuovi campi:** `msg.MessageRef.GmailID`, `ThreadID` (`uint64`, 0 = non Gmail), `Labels []string` (solo label utente).
- **Nuovo campo:** `msg.Query.Raw string` (sintassi Gmail verbatim).
- **Nuovo tipo:** `mail.Conversation` (Latest, ThreadID, Count, Participants, Unread, Labels).
- **Nuovi metodi di `MailStore`:** `ListConversations(ctx, q, pg) ([]Conversation, error)` e `ThreadMessages(ctx, threadID uint64) ([]msg.MessageRef, error)`. Entrambi in sola lettura. La primitiva STORE X-GM-LABELS resta nella copia patchata e NON è esposta in `MailStore` (sarà del Piano 10).
- **Invariati:** `ListMessages`, `Search` (firma), `FetchMessage`, `MessageID` (`MAILBOX:UID`).
- **Serializzazione:** `GmailID`/`ThreadID` come stringa decimale in ogni output JSON.

## Alternative scartate

- **ID Gmail dentro `MessageID` (`gm:<MSGID>`, come mail-mcp):** scartato. Cambierebbe la chiave di cache, Archive/Trash/MarkRead e i Change di Watch, che lavorano per UID di mailbox. Il campo dedicato lascia intatto il percorso IMAP.
- **`int64` per gli ID:** scartato. Overflow sopra 2^63, segnalato da atrium #73.
- **Server finto come unica strategia di test:** scartato. Il memserver non conosce X-GM-*; estenderlo costa più dei transcript scriptati e non copre la forma del wire. Scelta: transcript scriptati in `testdata/gmail/` per le forme Gmail, memserver invariato per il resto.
- **Fork esterno `excavador/go-imap` come modulo separato:** scartato da D8-1 (dipendenza su un fork non upstream di un singolo autore; la copia in-tree si controlla e si riallinea).
