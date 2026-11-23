# gmail-tui — Piano 16: cache locale (istantanee di lista e thread, LRU dei corpi) — Implementation Plan

> **For agentic workers:** branch `feat/piano-16` (da `main` e7718ce). Leggere `AGENTS.md`. TDD vero: per ogni task il test scritto prima va visto rosso (RED riportato nel report), poi verde. Commit per task, checkbox `- [ ]`. Gate dopo ogni task: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `~/go/bin/golangci-lint run ./...` a 0 issues. Le decisioni D16-A* sono di Andrea: non riaprirle. Le D16-T* sono prese in questo piano e si possono contestare nel review. Le domande Q16-* vanno poste ad Andrea prima del task indicato.

**Goal:** la lista compare subito all'avvio e la conversazione già vista si apre subito, anche se Gmail è lento. All'avvio la TUI mostra l'istantanea locale della prima pagina della cartella (prima ancora che `Connect` finisca), con "aggiornamento…" nella barra di stato, e la aggiorna da Gmail in background (stale-while-revalidate). Lo stesso per l'apertura di una conversazione già vista. I corpi in cache sono gli ultimi 200 messaggi aperti (LRU). Nessuna azione fatta su dati in cache può colpire il messaggio sbagliato.

**Stack:** nessuna dipendenza nuova. `go.etcd.io/bbolt v1.5.0` (già diretta: `Compact` in `compact.go`, `Bucket.NextSequence`, `DB.Stats`). Nessun CGO. Nessun import di go-imap fuori da `internal/mail/imap`.

## Perché (misura del 2026-11-23)

Traccia reale di Andrea con `GMAIL_TUI_TRACE=1` (Task 0 qui sotto): `Connect` (dial + LOGIN) 28,9 s; primo `ListConversations` fino a 77 s; `ThreadMessages` mediana 20 s; `FetchMessage` mediana 6 s; `MarkRead` 10 s; lista comparsa dopo 106 s. Frame UI 1-30 ms. Il collo di bottiglia è Gmail (LOGIN e operazioni IMAP), non il rendering: l'unico modo di mostrare la posta prima è mostrare quella che si ha già sul disco.

**Task 0 (già su `main`, e7718ce):** la traccia dei tempi (`internal/trace`, `tracestore`, `tracetea`, `gmail-tui trace`) porta già nei commenti l'etichetta "Piano 16": è lo strumento di misura di questo piano, non va rifatta.

## Stato attuale (verificato sul codice)

| Punto | Fatto | Riferimento |
|---|---|---|
| Cache dei corpi | bbolt, bucket `msgs:<mailbox>` chiave = uid (4 byte), valore = `msg.Message` JSON; `meta:<mailbox>` con `uidvalidity`; nessun limite di dimensione; schema 3 | `internal/mail/cache/cache.go:29`, `SetMessage`, `GetMessage` riga 103 |
| Epoca in lettura | `GetMessage` NON confronta la UIDVALIDITY: un corpo di un'epoca vecchia resta servito finché una `SetMessage` con epoca nuova non svuota il bucket (buco esistente, chiuso da R3) | `internal/mail/cache/cache.go:103` |
| Copie per GmailID | `UpdateGmailCopies` scansiona tutti i bucket `msgs:` (con 200 voci il costo è trascurabile) | `internal/mail/cache/cache.go:184` |
| Fetch cache-first | hit senza gate e senza rete; miss: SELECT + FETCH BODY.PEEK[] + X-GM-*, poi `SetMessage` con la UIDVALIDITY della SELECT | `internal/mail/imap/fetch.go:45-104` |
| Post-filtro allegati | `Search` con `HasAttachment` chiama `fetchMessage(heldGate=true)` su ogni risultato: scrive in cache corpi mai aperti | `internal/mail/imap/search.go:74` |
| Lista e thread | `ListConversations` e `ThreadMessages` scartano il risultato della SELECT (UIDVALIDITY non letta) | `internal/mail/imap/conversation.go:77`, `:181` |
| Mutazioni e cache | `updateCachedFlags` / `updateCachedLabels` aggiornano le copie in cache dopo STORE/COPY riusciti | `internal/mail/imap/mutate.go:228-293` |
| Avvio | `Init` → `beginSession` → `connectCmd` (budget 30 s) → `connResult` → `loadRefs` (listGen++) + `startWatch` | `internal/app/model.go:259-334`, `internal/app/watch.go:59-65` |
| Splash | visibile finché `!listLoaded`; `msgsLoaded` non stantio lo chiude | `internal/app/splash.go:123`, `internal/app/model.go:363` |
| Stantii | `listGen` (lista), `threadGen` (corpi del thread), `prevTok` (anteprima) | `internal/app/model.go:155`, `:60` |
| Apertura conversazione | `fetchMsg` → `loadConversation`: `ThreadMessages` + `FetchMessage` della riga aperta e degli espansi | `internal/app/model.go:1693`, `internal/app/conversation_view.go:24` |
| File della cache | `UserCacheDir/gmail-tui/cache.db` 0600, ma la cartella è creata **0755**; la cache si apre anche in demo (`selectStore(cfg, pw, openCache())`); nessun `Close` all'uscita; nessun legame con l'account | `cmd/gmail-tui/main.go:160`, `:527-541` |
| MCP | lo store del server MCP non usa cache (M4) | `cmd/gmail-tui/mcp.go:19` |
| Traccia | eventi `meta/store/msg/phase/body`; fasi `connected`, `list_loaded`, `first_view`, `first_list_view`; interfaccia opzionale inoltrata dal wrapper (`GmailExt`) | `internal/trace/trace.go`, `internal/trace/tracetea/model.go:122`, `internal/trace/tracestore/store.go` |

## Decisioni

### Decisioni di Andrea (non riaprirle)

| ID | Decisione | Scelta |
|---|---|---|
| D16-A1 | Cosa sta in cache | I corpi degli **ultimi 200 messaggi aperti**, più un'**istantanea della lista** (prima pagina di conversazioni per cartella) e dei **thread** (`ThreadMessages` per thread id). |
| D16-A2 | Avvio e apertura | All'avvio la lista compare SUBITO dall'istantanea, con "aggiornamento…" nella barra di stato, e si aggiorna da Gmail in background (stale-while-revalidate). Lo stesso all'apertura di una conversazione già vista. |
| D16-A3 | Rimozione | Oltre 200: LRU (il meno usato di recente), non FIFO. Limite solo da costante. |
| D16-A4 | Cifratura | Nessuna. File con permessi solo utente: `cache.db` 0600, cartella 0700. |

### Regola di sicurezza delle azioni (R1-R4)

Il rischio: una riga dell'istantanea porta un `MessageID` = mailbox + UID dell'epoca in cui è stata salvata. Dentro una stessa epoca (stessa UIDVALIDITY) un UID non è mai riusato (RFC 3501 §2.3.1.1): se il messaggio è stato spostato o archiviato altrove, l'UID non esiste più e STORE/EXPUNGE sono no-op, mai un altro messaggio. Il pericolo vero è un cambio di UIDVALIDITY (stesso UID, messaggio diverso) e, in difesa, qualunque percorso dimenticato. Quattro regole, tutte testate:

| ID | Regola | Dove |
|---|---|---|
| **R1** | **Provvisorio = sola lettura locale.** Righe e thread mostrati da un'istantanea sono *provvisori*. Su dati provvisori si può solo navigare, leggere e aprire ciò che sta già in cache (`o` compreso). Sono rifiutati con toast informativo "aggiornamento in corso: azione disponibile tra poco", senza toccare lo store: `a`, `#`, `s`, `v`, `l`, pannello label, `x`/`X` (selezione), `r`, `f`, `:save`/`:open` degli allegati, pagina successiva (scroll in fondo), segna-come-letto automatico (apertura e anteprima). `c` (nuovo messaggio), `/` (ricerca), `m` (cartelle), `R`, `u` (annulla di azioni di questa sessione) restano attivi. | `internal/app` (un solo punto di controllo sui tasti, Task 9) |
| **R2** | **Conferma = sostituzione dal server.** Una riga diventa azionabile solo quando un `ListConversations` fresco della stessa cartella la sostituisce (msgsLoaded non stantio, non da cache): la lista provvisoria è sostituita per intero, non fusa. Un thread aperto da istantanea diventa azionabile solo quando arriva il suo `ThreadMessages` fresco; la riga aperta (`pendingRef`) si riallinea alla riga fresca per **ThreadID + GmailID**, mai per `MessageID`. Se la riga non c'è più nella lista fresca, le azioni sulla conversazione restano rifiutate ("conversazione non più in questa cartella"). | `internal/app` (Task 9, 10), `internal/ui/thread` (Task 6) |
| **R3** | **Regola d'epoca.** Ogni istantanea salva la UIDVALIDITY della sua mailbox (lista: la cartella; thread: Tutti i messaggi). Un'istantanea si usa solo se coincide con la UIDVALIDITY registrata per i corpi della stessa mailbox. Alla prima SELECT di una mailbox in sessione (lista, thread, fetch) con UIDVALIDITY diversa da quella in cache, lo store scarta corpi e istantanee di quella mailbox (e tutte le istantanee di thread se la mailbox è Tutti i messaggi) prima di restituire righe. | `internal/mail/cache` (Task 3, 4), `internal/mail/imap` (Task 8) |
| **R4** | **Difesa in profondità: guardia degli ID confermati.** Lo store della TUI reale è avvolto da `mail.Guard`: ogni chiamata con un `MessageID` (lettura per UID o scrittura) è rifiutata con `mail.ErrNotConfirmed`, **senza rete**, se quell'ID non è uscito da una risposta fresca del server in questa sessione (`ListConversations`, `ListMessages`, `Search`, `ThreadMessages`, `ConversationMembers`, `Trash`, `Restore`, `AllMailIDs`, `SaveDraft`, `Change.IDs`/`Change.Refs` del Watch). Gli ID letti dalle istantanee (`Cached*`) non sono mai registrati. Se R1 dimentica un tasto, R4 ferma comunque la chiamata. | `internal/mail/guard.go` (Task 5), cablato in `cmd/gmail-tui` (Task 11) |

Limite dichiarato: un cambio di UIDVALIDITY **durante** la sessione (dopo che gli ID sono stati confermati) resta fuori da questo piano, come oggi.

### Decisioni tecniche del piano (D16-T*)

| ID | Scelta | Motivazione |
|---|---|---|
| D16-T1 | Interfaccia **opzionale** `mail.Snapshotter` (`CachedConversations`, `CachedThread`, `CachedMessage`), non metodi nuovi di `MailStore`. Sola lettura dal disco: niente ctx, niente gate, niente `s.mu`, nessuna rete; valida anche prima di `Connect`. L'app la ottiene con un'asserzione di tipo; `tracestore` e `mail.Guard` la inoltrano (stesso schema di `GmailExt`). | Il Mock non la implementa: **in demo nessuna istantanea per costruzione**. Il server MCP usa il suo `Reader` e non la vede: resta com'è. |
| D16-T2 | Le istantanee le scrive **lo store IMAP** (write-through): `ListConversations` con `Offset == 0` e query di sola cartella (`q.Label`, nessun altro criterio, `Raw == ""`) salva le prime `cache.SnapshotRows = 50` righe; `ThreadMessages` salva i ref del thread. Ricerche e pagine successive non si salvano. L'app non scrive mai sul disco. | Un solo punto di scrittura, vicino alla UIDVALIDITY della SELECT. Il refresh dopo ogni op e dopo il Watch riscrive l'istantanea da sé. |
| D16-T3 | Le mutazioni riuscite dello store aggiornano le istantanee: flag e label (MarkRead, Star, ApplyLabel, ModifyLabels) con la stessa funzione `mutate` per GmailID già usata per i corpi; `Unread` della riga ricalcolato dall'istantanea del thread se c'è, altrimenti da `Latest`. Cambi di composizione (Archive, Trash, ApplyLabel off sulla cartella-label, DeleteDraft) tolgono la riga (per ThreadID) dall'istantanea della mailbox d'origine; Trash toglie i messaggi dalle istantanee di thread. Restore e cartelle di destinazione: nulla (il refresh le riscrive). | Le istantanee sono sempre provvisorie (R1): l'aggiornamento serve a non mostrare all'avvio successivo una conversazione già archiviata, non alla sicurezza. |
| D16-T4 | **Unità LRU = messaggio Gmail** (GmailID; `mailbox/uid` se GmailID è 0): le copie dello stesso messaggio in INBOX e Tutti i messaggi occupano un posto solo e si rimuovono insieme. `cache.MaxBodies = 200`. | L'apertura di una conversazione salva sia la copia INBOX della riga sia quella di Tutti i messaggi: contare le copie dimezzerebbe i 200 messaggi. (Q16-2) |
| D16-T5 | Indice LRU in bbolt: bucket `lru:<ns>` con sotto-bucket `seq` (uint64 big-endian da `NextSequence` → chiave dell'unità) e `unit` (chiave → seq + elenco delle voci), contatore `n`. Rimozione: `Cursor.First()` su `seq` finché `n > max`. Tre namespace: `body` (200), `thread` (`MaxThreadSnapshots = 200`), `list` (`MaxListSnapshots = 50`). | O(log n) per tocco e per rimozione; nessuna scansione. |
| D16-T6 | I **tocchi in lettura** (hit di `GetMessage`, `CachedMessage`, `CachedThread`) si accumulano in memoria e si scrivono alla transazione di scrittura successiva o a `Close`. Un crash perde solo l'ordine recente (LRU approssimato). | Su darwin `File.Sync` fa F_FULLFSYNC: una transazione di scrittura per ogni lettura costerebbe millisecondi a ogni apertura. |
| D16-T7 | **Migrazione schema 3 → 4 senza perdere i corpi**: i bucket `msgs:` restano; l'indice LRU si costruisce dalle voci esistenti ordinate per `Ref.Date` (più recente = usato più di recente); oltre 200 unità si rimuove il resto. Versioni < 3: scartate come oggi. Nessuna istantanea esiste prima della 4. | Andrea non perde i corpi già scaricati; il limite vale da subito. |
| D16-T8 | **Compattazione a `Close`**: se il file supera 4 MiB e le pagine libere (`Stats().FreePageN` × page size) sono oltre il 50%, `bolt.Compact` su `cache.db.tmp` (0600, stessa cartella), `fsync`, `rename` atomico. Mai all'apertura. | Non pesa sul tempo alla prima lista; dopo la migrazione il primo `q` è più lento una volta sola. (Q16-6) |
| D16-T9 | **Legame con l'account**: `meta` globale `account` = SHA-256 esadecimale dell'indirizzo in minuscolo. `BindAccount(addr)`: se diverso, la cache si svuota per intero; se assente (cache dello schema 3), si adotta. | Il file è per macchina, non per account: un cambio di account non deve mostrare la posta dell'altro. Nessun indirizzo in chiaro nel file. (Q16-3) |
| D16-T10 | Il post-filtro `HasAttachment` della ricerca legge la cache ma **non scrive** corpi né tocca l'LRU. | Una ricerca con `has:attachment` su 500 messaggi svuoterebbe i 200 messaggi aperti. (Q16-5) |
| D16-T11 | Avvio con istantanea: `Init` lancia in parallelo `connectCmd` e `loadSnapshot` (Cmd, lettura bbolt fuori dal loop UI). Il risultato `snapshotLoaded{mailbox, folderGen}` si applica solo se cartella e `folderGen` coincidono, se non c'è una query, e se la lista della cartella non è già fresca (`listSource != fresh`): un risultato fresco arrivato prima vince sempre. `connResult` incrementa `listGen` ma non `folderGen`: l'istantanea resta valida fino alla lista fresca. Cambio di cartella: `folderGen++`, lettura dell'istantanea della cartella nuova + load fresco. Un'istantanea vuota vale come assente (resta lo splash). | Lo splash sparisce appena c'è un'istantanea, prima di `Connect`. Senza istantanea il comportamento è quello di oggi. |
| D16-T12 | Stato di barra: con lista provvisoria la barra mostra "aggiornamento…" (stato persistente, neutro) con lo spinner. `Connect` fallito con istantanea: la lista resta (sola lettura), barra "connessione fallita — lista non aggiornata, premi R per riprovare", toast d'errore come oggi. | Lettura offline gratuita, senza modalità nuove. (Q16-4) |
| D16-T13 | Apertura di una conversazione: se `CachedThread(threadID)` c'è, il thread si costruisce subito da ref dell'istantanea + corpi da `CachedMessage` (i corpi mancanti mostrano "caricamento…": `BodyRequested` su thread provvisorio si ignora). In parallelo parte il caricamento fresco; per una riga provvisoria il caricamento fresco usa **solo il ThreadID** (X-GM-THRID è globale Gmail, indipendente dall'epoca) e i ref freschi, mai `ref.ID`. Il risultato fresco si concilia con `thread.Reconcile` (messaggio selezionato per GmailID, espansi e scroll preservati). Riga provvisoria con ThreadID 0 (server senza X-GM-EXT-1): solo i dati in cache, nessun caricamento fresco finché la lista non è confermata. Anteprima a due colonne: stesso flusso. | D16-A2; R1/R2 rispettate anche sul percorso di lettura (un `FetchMessage(ref.ID)` con UID d'epoca vecchia mostrerebbe un altro messaggio). |
| D16-T14 | Traccia: nuovo tipo di evento `K: "cache"` con `Op` ∈ {`snap_list`, `snap_thread`, `snap_body`, `fetch_body`}, `Mode` ∈ {`hit`, `miss`, `epoch`} (`epoch` = scartata per R3), `N` (righe/ref), `Bytes`, `Role` (solo ruolo). Fasi nuove: `snapshot_loaded` (primo `snapshotLoaded` applicato); `first_list_view` scatta anche dall'istantanea, con `Mode: "cached"` o `"fresh"`. Il riepilogo di `gmail-tui trace` conta hit/miss per Op. Mai ID, nomi di cartella, oggetti, indirizzi. | Misura del Goal (tempo alla prima lista < 1 s con istantanea). |
| D16-T15 | `openCache(address)`: cartella `gmail-tui` creata 0700 e riportata a 0700 se esiste con permessi più larghi (`os.Chmod`); la cache si apre **solo** con password risolta (mai in demo); `defer c.Close()` in `runTUI` (flush dei tocchi + compattazione). Il wizard (`backendFactory`) apre e lega la cache allo stesso modo. | D16-A4, "nessuna istantanea su disco in demo". |
| D16-T16 | Server MCP invariato: continua senza cache e senza istantanee (nessun dato stantio possibile). Un test fissa che `runMCP` non apre `cache.db`. | Vincolo "MCP non usa istantanee stantie senza dichiararlo": la scelta più semplice è non usarle. |

## Global Constraints

- R1-R4 valgono per ogni task: nessun `MessageID` letto da un'istantanea passa a un metodo di `MailStore`.
- `mail.MailStore` resta il contratto della UI; `Snapshotter` è opzionale e di sola lettura. Nessun import di go-imap fuori da `internal/mail/imap` (`grep -rn "go-imap" internal --include="*.go" -l | grep -v internal/mail/imap` vuoto). `internal/app` non importa `internal/mail/cache`.
- Le istantanee non prendono il gate e non toccano la rete: si leggono prima e durante `Connect`.
- Risultati stantii scartati con `listGen`/`folderGen`/`threadGen`/`prevTok`; un risultato da cache non sovrascrive mai un risultato fresco.
- Limiti solo da costante in `internal/mail/cache/limits.go`: `MaxBodies = 200`, `MaxThreadSnapshots = 200`, `MaxListSnapshots = 50`, `SnapshotRows = 50`. Nessun campo di config.
- Privacy: nessun dato in chiaro fuori da `UserCacheDir/gmail-tui/` (0700; `cache.db` e `cache.db.tmp` 0600). La traccia registra solo Op, Mode, conteggi, byte, ruolo.
- Demo/Mock: nessun file di cache creato, nessuna istantanea (test).
- Colori solo da slot del tema; commenti e stringhe utente in italiano con riferimento "Piano 16, Task N"; tempi da `clock` iniettabile (`SavedAt`, età dell'istantanea).
- Test senza rete: cache su `t.TempDir()`, IMAP su `imapmemserver` o server scriptato, app con `*mail.Mock` avvolto da un finto `Snapshotter` (`Connect` bloccabile da canale).
- API di bbolt verificate sul sorgente della versione fissata (`/Users/andrea/go/pkg/mod/go.etcd.io/bbolt@v1.5.0/`), non a memoria.

## File Structure

| File | Ruolo |
|---|---|
| `internal/mail/snapshot.go` (nuovo) | `Snapshotter`, `ConversationsSnapshot`, `ThreadSnapshot`, `ErrNotConfirmed` (Task 2) |
| `internal/mail/guard.go` (nuovo) | `Guard`: ID confermati, inoltro di `Snapshotter` e `GmailExt` (Task 5) |
| `internal/mail/cache/limits.go` (nuovo) | costanti dei limiti (Task 2) |
| `internal/mail/cache/cache.go`, `lru.go`, `migrate.go`, `account.go`, `compact.go` (nuovi tranne cache.go) | schema 4, LRU, migrazione, account, compattazione, regola d'epoca (Task 3) |
| `internal/mail/cache/snapshot.go` (nuovo) | istantanee di lista e thread, patch e rimozioni (Task 4) |
| `internal/mail/imap/snapshot.go` (nuovo) | `Snapshotter` dello store IMAP, `noteEpoch` (Task 2 stub, Task 8) |
| `internal/mail/imap/{conversation,fetch,search,mutate,trash,restore,gmail_labels,draft}.go` | write-through, R3, patch delle istantanee, D16-T10 (Task 8) |
| `internal/ui/thread/reconcile.go` (nuovo) | `Reconcile(refs, bodies)` (Task 6) |
| `internal/trace/{trace,summary}.go`, `tracestore/store.go`, `tracetea/model.go` | eventi `cache`, fasi, riepilogo (Task 2 inoltro, Task 7) |
| `internal/app/snapshot.go` (nuovo), `provisional.go` (nuovo), `model.go`, `splash.go`, `ops.go`, `select.go`, `bulk.go`, `labels_picker.go`, `folders.go` | lista da istantanea, stato provvisorio, R1 (Task 9) |
| `internal/app/{conversation_view,split,model}.go` | thread e anteprima da istantanea (Task 10) |
| `cmd/gmail-tui/main.go`, `main_test.go`, `mcp_test.go` | cablaggio, permessi, demo, Close, MCP (Task 11) |
| `README.md`, `CHANGELOG.md`, `SECURITY.md`, `AGENTS.md`, `docs/smoke-checklist.md` | documentazione (Task 12) |

## Waves

Ogni wave gira in parallelo solo tra task con file disgiunti. Il Wave 1 fissa i contratti: nessun agente parte prima del suo commit. Modello consigliato per l'agente in colonna.

| Wave | Task | File (ownership esclusiva) | Dipende da | Modello |
|---|---|---|---|---|
| 1 | 2 (contratti) | `internal/mail/snapshot.go`, `internal/mail/cache/limits.go`, `internal/mail/imap/snapshot.go` (stub), `internal/trace/tracestore/store.go` (solo inoltro) | — | sonnet |
| 2a | 3 → 4 (cache: schema 4, LRU, epoca; poi istantanee), un agente in sequenza | `internal/mail/cache/*` | 2 | sonnet |
| 2b | 5 (guardia R4) | `internal/mail/guard.go`, `guard_test.go` | 2 | sonnet |
| 2c | 6 (Reconcile del thread) | `internal/ui/thread/reconcile.go`, `reconcile_test.go` | — | sonnet |
| 2d | 7 (traccia) | `internal/trace/{trace,summary}.go`, `internal/trace/tracestore/store.go`, `internal/trace/tracetea/model.go` + test | 2 | haiku |
| 3a | 8 (store IMAP) | `internal/mail/imap/*` | 2, 3, 4 | sonnet |
| 3b | 9 → 10 (app: lista, poi thread), un agente in sequenza | `internal/app/*` | 2, 5, 6 | sonnet |
| 4a | 11 (cablaggio) | `cmd/gmail-tui/*` | 3, 5, 8 | haiku |
| 4b | 12 (documentazione) | `README.md`, `CHANGELOG.md`, `SECURITY.md`, `AGENTS.md`, `docs/smoke-checklist.md` | 9, 10, 11 | haiku |
| 4c | 13 (gate, misure, revisione) | nessun file di produzione | 11, 12 | sonnet (revisore: agente diverso) |

Nota: `internal/app/model.go` è il file più conteso: solo Wave 3b lo tocca (Task 9 e 10 sequenziali, un agente). 3a e 3b sono indipendenti: l'app si prova con un finto `Snapshotter`, non con lo store IMAP.

## Task 1: piano e commit

**Produces:** questo file, committato da solo.

- [x] Scrivere il piano (questo documento).
- [x] Commit `docs: Piano 16 (cache locale)` con la riga Co-Authored-By.

## Task 2: contratti (solo firme)

**Produces:**

- `internal/mail/snapshot.go`:
  - `type ConversationsSnapshot struct { Rows []Conversation; SavedAt time.Time; Limited bool }`
  - `type ThreadSnapshot struct { Refs []msg.MessageRef; SavedAt time.Time }`
  - `type Snapshotter interface { CachedConversations(mailbox string) (ConversationsSnapshot, bool); CachedThread(threadID uint64) (ThreadSnapshot, bool); CachedMessage(id msg.MessageID) (*msg.Message, bool) }` con il contratto nel commento: sola lettura dal disco, niente rete né gate, valida prima di `Connect`, risultato **provvisorio** (R1), mailbox `""` = INBOX, errori del disco = miss.
  - `var ErrNotConfirmed = errors.New("messaggio non ancora confermato dal server: attendi l'aggiornamento")`.
- `internal/mail/cache/limits.go`: `MaxBodies = 200`, `MaxThreadSnapshots = 200`, `MaxListSnapshots = 50`, `SnapshotRows = 50`.
- `internal/mail/imap/snapshot.go`: i tre metodi su `*Store` che restituiscono miss; assert `var _ mail.Snapshotter = (*Store)(nil)`.
- `internal/trace/tracestore/store.go`: il wrapper inoltra `Snapshotter` se lo store interno lo implementa (altrimenti miss), come `GmailExt`. Nessuna emissione (Task 7).

**Test:** `TestMockIsNotSnapshotter` (`internal/mail`: il Mock non implementa l'interfaccia, quindi la demo non ha istantanee); `TestWrapForwardsSnapshotter` e `TestWrapWithoutSnapshotterMisses` (`tracestore`). RED: non compilano prima dei tipi.

- [ ] RED riportato.
- [ ] Tipi, costanti, stub, inoltro.
- [ ] Gate (incluso il grep go-imap).
- [ ] Commit `feat(mail): contratti delle istantanee locali (Piano 16, Task 2)`.

## Task 3: cache schema 4 (LRU, migrazione, account, epoca, compattazione)

**Produces** (`internal/mail/cache`):

- `schemaVersion = 4`; `migrate` porta la 3 alla 4 senza perdere corpi (D16-T7); < 3 come oggi.
- `lru.go`: indice per namespace (D16-T5) con `touch(ns, unit, entries)`, `evict(ns, max, drop func)`, tocchi in memoria con flush alla prima scrittura e a `Close` (D16-T6). Unità del corpo = `g<GmailID>` o `i<mailbox>\x00<uid>` (D16-T4).
- `SetMessage` tocca l'unità ed espelle oltre `MaxBodies` (tutte le copie dell'unità); `GetMessage` registra il tocco in memoria.
- R3 lato cache: `NoteUIDValidity(mailbox string, v uint32) (changed bool, err error)` registra l'epoca vista dal server; se diversa da quella salvata, svuota `msgs:<mailbox>`, le voci LRU relative e (dal Task 4) le istantanee della mailbox. `GetMessage` non cambia firma.
- `account.go`: `BindAccount(address string) error` (D16-T9).
- `compact.go`: `Close` = flush dei tocchi, poi compattazione condizionata (D16-T8) con `bolt.Compact`, file temporaneo 0600, rename atomico; errori di compattazione ignorati (il file originale resta valido).

**Test (RED prima):**
- `TestLRUEvictsLeastRecentlyUsed`: 201 unità, la meno toccata sparisce; una toccata in lettura sopravvive (dopo flush).
- `TestLRUCopiesShareOneSlot`: stessa GmailID in INBOX e Tutti i messaggi = un posto; l'espulsione toglie entrambe.
- `TestLRUTouchesFlushedOnClose`: ordine conservato dopo Close/Open.
- `TestMigrateV3KeepsNewest200`: database v3 scritto a mano con 250 corpi → 200 restano (i più recenti per Date), nessuna perdita sotto 200.
- `TestMigrateV2StillDrops` (comportamento esistente invariato).
- `TestBindAccountDifferentWipes`, `TestBindAccountAdoptsUnbound`, `TestAccountStoredHashed` (il file non contiene l'indirizzo in chiaro).
- `TestNoteUIDValidityChangeDropsMailbox`.
- `TestCloseCompactsBloatedFile`: 10 MiB scritti e rimossi → file più piccolo dopo Close, permessi 0600; `TestCloseSkipsCompactionSmallFile`.
- Benchmark `BenchmarkGetMessageHit` (nessuna transazione di scrittura per hit).

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(cache): schema 4 con LRU dei 200 messaggi aperti, account ed epoca (Piano 16, Task 3)`.

## Task 4: istantanee nella cache

**Produces** (`internal/mail/cache/snapshot.go`):

- Bucket `snap:list` (chiave = nome mailbox) e `snap:thread` (chiave = thread id 8 byte), valore JSON con `Version`, `SavedAt`, `Mailbox`, `UIDValidity`, righe o ref.
- `PutList(mailbox string, uidValidity uint32, rows []mail.Conversation, limited bool, at time.Time)` (tronca a `SnapshotRows`; righe vuote = cancella), `GetList(mailbox) (rows, at, limited, ok)` con R3: valida solo se `UIDValidity` coincide con `meta:<mailbox>` o se la mailbox non ha corpi (meta assente); altrimenti miss con motivo `epoch`.
- `PutThread(threadID uint64, mailbox string, uidValidity uint32, refs []msg.MessageRef, at time.Time)`, `GetThread(threadID)`: stessa regola d'epoca sulla mailbox del thread.
- LRU `thread` (200) e `list` (50) con lo stesso indice del Task 3.
- Patch (D16-T3): `PatchSnapshotRefs(gmailIDs []uint64, mutate func(*msg.MessageRef) bool)` su `Latest` delle righe e sui ref dei thread, con ricalcolo di `Unread`/`Labels` della riga; `RemoveListRows(mailbox string, threadIDs []uint64, ids []msg.MessageID)`; `RemoveThreadRefs(gmailIDs []uint64)`.
- `NoteUIDValidity` (Task 3) scarta anche `snap:list/<mailbox>` e le istantanee di thread salvate su quella mailbox.

Il package `cache` importa `internal/mail` solo per il tipo `Conversation`; se crea un ciclo d'import, le righe si salvano come tipo locale e la conversione sta nello store IMAP (decisione dell'implementatore, riportata nel report).

**Test (RED prima):** `TestPutGetListRoundTrip`, `TestListTruncatedToSnapshotRows`, `TestListEpochMismatchIsMiss`, `TestThreadEpochMismatchIsMiss`, `TestPatchSnapshotRefsUpdatesUnreadAndLabels`, `TestRemoveListRowsByThread`, `TestThreadSnapshotsLRU201`, `TestListSnapshotsLRU51`, `TestNoteUIDValidityDropsSnapshots`, `TestEmptyListDeletesSnapshot`.

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(cache): istantanee di lista e thread con regola d'epoca (Piano 16, Task 4)`.

## Task 5: guardia degli ID confermati (R4)

**Produces:** `internal/mail/guard.go`: `func Guard(st MailStore) MailStore`. Insieme di ID confermati protetto da mutex. Registra gli ID dei risultati di `ListMessages`, `Search`, `ListConversations` (`Latest.ID`), `ThreadMessages`, `ConversationMembers`, `Trash`, `Restore`, `AllMailIDs`, `SaveDraft`, e degli eventi del canale `Watch` (`IDs`, `Refs`; il wrapper rilancia il canale rispettandone il contratto di chiusura). Rifiuta con `ErrNotConfirmed`, senza chiamare lo store interno, `FetchMessage`, `MarkRead`, `Archive`, `Star`, `ApplyLabel`, `SaveAttachment`, `Trash`, `Restore`, `AllMailIDs`, `ModifyLabels`, `DeleteDraft`, `SaveDraft` (`replace`) se anche un solo ID non è confermato. Inoltra `Snapshotter` e `GmailExt` senza registrare nulla. `Connect`, `Send`, `ListMailboxes`, `UnreadCounts` passano.

**Test (RED prima), con uno store finto che conta le chiamate:**
- `TestGuardRefusesUnconfirmedIDs`: tabella su tutti i metodi con ID → `ErrNotConfirmed`, zero chiamate interne.
- `TestGuardAllowsIDsFromFreshResults`: tabella sulle sorgenti di conferma.
- `TestGuardSnapshotIDsNeverConfirmed`: ID visti solo da `CachedConversations`/`CachedThread` restano rifiutati.
- `TestGuardWatchConfirmsNewMail`, `TestGuardForwardsSnapshotterAndGmailExt`, `TestGuardConcurrentUse` (con `-race`).

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(mail): guardia degli ID confermati dal server (Piano 16, Task 5)`.

## Task 6: conciliazione del thread

**Produces:** `internal/ui/thread/reconcile.go`: `func (m *Model) Reconcile(refs []msg.MessageRef, bodies map[msg.MessageID]*msg.Message)`. Sostituisce l'elenco con i ref freschi; il messaggio selezionato, gli espansi e i corpi già presenti si riportano per **GmailID** (ID se GmailID è 0); lo scroll resta se il messaggio selezionato esiste ancora, altrimenti si seleziona l'ultimo; i corpi freschi sostituiscono quelli in cache; nessun rendering ripetuto dei corpi invariati (chiave della cache di rendering invariata).

**Test (RED prima):** `TestReconcileKeepsSelectionByGmailID`, `TestReconcileKeepsExpandedAndScroll`, `TestReconcileDoesNotMatchByUIDAcrossEpochs` (stesso ID, GmailID diverso = messaggio diverso: la selezione non lo segue), `TestReconcileNewMessageAppended`, `TestReconcileNoRerenderOfUnchangedBodies`.

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(thread): conciliazione del thread con i dati freschi (Piano 16, Task 6)`.

## Task 7: traccia della cache

**Produces:** documentazione del tipo `cache` in `Event` (D16-T14, nessun campo nuovo: si usano `Op`, `Mode`, `N`, `Bytes`, `Role`); `tracestore` emette un evento `cache` per ogni chiamata `Cached*` (hit/miss, ruolo dalla mappa esistente, `"inbox"` per `""`, `"label"` se ignoto); `tracetea` riconosce `.snapshotLoaded` (fase `snapshot_loaded`, e `first_list_view` con `Mode: "cached"`) oltre a `.msgsLoaded` (`Mode: "fresh"`); `summary.go` stampa i conteggi hit/miss per Op e la fase `first_list_view` con la sua origine.

**Test (RED prima):** `TestCacheEventsCarryNoPersonalData` (nessuna stringa fuori dall'insieme ammesso), `TestSnapshotPhaseEmittedOnce`, `TestFirstListViewFromSnapshotIsCached`, `TestSummaryCountsCacheHits`.

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(trace): hit e miss della cache e fase dell'istantanea (Piano 16, Task 7)`.

## Task 8: store IMAP (write-through, epoca, patch)

**Produces** (`internal/mail/imap`):

- `snapshot.go`: i tre metodi `Snapshotter` reali sopra `opts.Cache` (nil = miss), conversione dei tipi, nessun gate; `noteEpoch(mailbox, sel.UIDValidity)` chiamato dopo la SELECT in `ListConversations` (entrambi i rami, anche il fallback senza X-GM-EXT-1 via `ListMessages`), `ThreadMessages`, `fetchRaw`, `ListMessages`, `Search` (R3), **prima** di costruire le righe.
- `ListConversations`: con `Offset == 0` e query di sola cartella, `PutList` delle righe (anche con `ErrScanLimit`: `limited = true`). `ThreadMessages`: `PutThread` con la mailbox Tutti i messaggi e la sua UIDVALIDITY.
- `fetch.go`: evento di traccia `fetch_body` hit/miss (formato del Task 7); con `heldGate=true` (post-filtro) nessuna scrittura (D16-T10).
- Mutazioni (`mutate.go`, `trash.go`, `restore.go`, `gmail_labels.go`, `draft.go`): dopo il successo, patch/rimozioni del D16-T3 accanto agli aggiornamenti dei corpi già esistenti; un errore delle istantanee non trasforma un'op riuscita in errore (si ignora, come una cache assente), coperto da test.
- `Connect` non tocca le istantanee; i metodi `Cached*` funzionano su uno `Store` mai connesso.

**Test (RED prima), su `imapmemserver` e server scriptato (`net.Pipe`):**
- `TestCachedConversationsBeforeConnect`: store non connesso, cache con istantanea → righe, nessuna connessione aperta.
- `TestListConversationsWritesSnapshot` (prima pagina) e `TestSearchAndPageTwoDoNotWriteSnapshot`.
- `TestThreadMessagesWritesSnapshot` (scriptato, X-GM-THRID).
- `TestNewUIDValidityDropsBodiesAndSnapshots`: cache con `meta` = UIDVALIDITY del server + 1 → al list i corpi della mailbox e l'istantanea spariscono; nessun corpo d'epoca vecchia servito dopo.
- `TestFetchHitDoesNotServeOtherEpoch`.
- `TestArchiveRemovesRowFromSnapshot`, `TestTrashRemovesFromThreadSnapshots`, `TestMarkReadPatchesSnapshots`, `TestStarPatchesSnapshots`, `TestModifyLabelsPatchesSnapshots`.
- `TestHasAttachmentSearchDoesNotFillCache`.
- `TestCachedMethodsDoNotTakeGate`: con il gate tenuto da un'op bloccata, `CachedConversations` ritorna subito.

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate (grep go-imap compreso).
- [ ] Commit `feat(imap): istantanee write-through e regola d'epoca (Piano 16, Task 8)`.

## Task 9: app, lista da istantanea e stato provvisorio (R1, R2)

**Produces** (`internal/app`):

- `snapshot.go`: `loadSnapshot(st, box, folderGen) tea.Cmd` (asserzione `mail.Snapshotter`; senza interfaccia nessuna Cmd), messaggio `snapshotLoaded`; `Init`/`beginSession` lo lanciano insieme a `connectCmd` (D16-T11); cambio di cartella idem.
- Stato: `folderGen int`, `listSource` (`none` | `cached` | `fresh`), età dell'istantanea per la barra (clock iniettabile). `snapshotLoaded` applicato: `listLoaded = true` (lo splash sparisce), `listSource = cached`, barra "aggiornamento…" (D16-T12). `msgsLoaded` fresco non stantio: `listSource = fresh`, sostituzione completa (R2), selezione vuota, barra ripristinata. `moreLoaded` e `loadMoreAtEnd` bloccati con lista provvisoria.
- `provisional.go`: `func (m Model) refuseWhileProvisional(k tea.KeyPressMsg) (bool, tea.Cmd)`, un solo punto chiamato dal routing dei tasti di inbox e thread prima di `runOp`, `askTrash`, `confirmLabel`, `bulk`, pannello label, selezione, `r`/`f`, `:save`/`:open` (R1). Il thread è provvisorio se è stato costruito da istantanea e il fresco non è arrivato, o se la riga aperta è provvisoria (Task 10).
- `connResult` con errore e lista provvisoria: barra D16-T12, nessuno splash d'errore (la lista resta).
- Nessuna scrittura su disco dall'app; nessun import di `internal/mail/cache`.

**Test (RED prima), con `*mail.Mock` avvolto da un finto `Snapshotter` e `Connect` bloccato da canale:**
- `TestSnapshotShownBeforeConnect`: con `Connect` ancora bloccato, la View mostra le righe dell'istantanea, nessuno splash, barra "aggiornamento…".
- `TestNoSnapshotKeepsSplash` e `TestDemoStoreHasNoSnapshot` (DemoStore: comportamento di oggi).
- `TestEmptySnapshotKeepsSplash`.
- `TestFreshListReplacesSnapshot` (R2): righe fresche al posto di quelle provvisorie, barra ripulita.
- `TestStaleSnapshotAfterFreshIgnored`: fresco prima, istantanea dopo → ignorata.
- `TestSnapshotOfOtherFolderIgnored` e `TestFolderSwitchShowsFolderSnapshot`.
- `TestProvisionalListRefusesActions`: tabella su `a`, `#`, `s`, `v`, `l`, pannello label, `x`, `X`, `r`, `f`, scroll in fondo → toast informativo, **nessuna chiamata** allo store (Mock invariato, contatore del finto a zero).
- `TestProvisionalAllowsNavigationSearchCompose`: frecce, `/`, `m`, `c` funzionano.
- `TestActionsEnabledAfterFreshList`: stesso tasto `a` dopo il fresco → archiviazione nel Mock.
- `TestEpochChangeSameIDOtherMessage`: istantanea con `INBOX:7` = messaggio A (GmailID 1); lista fresca con `INBOX:7` = messaggio B (GmailID 2); prima del fresco `a` è rifiutato; dopo, `a` archivia B (la riga visibile), mai A per via della selezione o del cursore.
- `TestConnectFailWithSnapshotKeepsReadOnlyList`.
- `TestStatusAgeUsesInjectedClock` (solo se l'età è mostrata, Q16-4).

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(app): lista subito dall'istantanea, azioni solo su righe confermate (Piano 16, Task 9)`.

## Task 10: app, conversazione e anteprima da istantanea

**Produces** (`internal/app/conversation_view.go`, `split.go`, `model.go`):

- Apertura (D16-T13): `fetchMsg`/`fetchPreview` diventano due Cmd: `loadCachedConversation` (solo `CachedThread` + `CachedMessage`, dedup per GmailID come `copyKey`) e `loadConversation` fresca. Per una riga provvisoria la versione fresca salta `FetchMessage(ref.ID)` e lavora solo con `ThreadMessages(ref.ThreadID)` e gli ID che restituisce; ThreadID 0 provvisorio → solo cache.
- `msgLoaded`/`previewLoaded` con campo `cached bool`: il cached si applica solo se il fresco per lo stesso `pendingRef`/`prevTok` non è già arrivato; il fresco su thread già mostrato da cache chiama `thread.Reconcile`.
- `thread.BodyRequested` con thread provvisorio: nessun `FetchMessage`, il corpo arriva col fresco.
- Segna-come-letto automatico (apertura e anteprima, tutte le modalità di `preview_mark_read`) solo a thread fresco, sugli ID freschi.
- Al `msgsLoaded` fresco, il `pendingRef` del thread aperto si riallinea per ThreadID + GmailID (R2); assente → azioni sulla conversazione rifiutate con "conversazione non più in questa cartella".

**Test (RED prima):**
- `TestOpenCachedThreadImmediately`: `ThreadMessages` del finto bloccato → il thread si vede subito dall'istantanea.
- `TestFreshThreadReconciles` (selezione e scroll conservati).
- `TestProvisionalRowFreshLoadUsesThreadIDOnly`: il finto registra che `FetchMessage(ref.ID)` della riga provvisoria non è chiamato.
- `TestCachedBodyRequestIgnoredUntilFresh`.
- `TestNoAutoMarkReadOnCachedThread` e `TestAutoMarkReadAfterFresh` (tre modalità di anteprima).
- `TestStaleCachedThreadAfterFreshIgnored`.
- `TestPendingRefRealignedByGmailID` e `TestPendingRefMissingAfterRefreshRefusesConvActions`.
- `TestPreviewFromSnapshot` (due colonne).

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(app): conversazione subito dall'istantanea con aggiornamento in background (Piano 16, Task 10)`.

## Task 11: cablaggio, permessi, demo, MCP

**Produces** (`cmd/gmail-tui/main.go`): `openCache(address string) *cache.Cache` (D16-T15: cartella 0700 e `Chmod` se più larga, `BindAccount`); `selectStore` apre la cache solo con password; ordine degli involucri `mail.Guard(tracestore.Wrap(imap))` (la guardia vede gli ID come li vede l'app); `defer c.Close()` in `runTUI` dopo `runProgram`; `backendFactory` (wizard) e `newIMAPStore` (CLI `fetch`) con `openCache(address)`; la CLI `fetch` non usa la guardia (non legge istantanee). `openAddressBook` crea la cartella 0700 (stessa cartella).

**Test (RED prima):** `TestDemoCreatesNoCacheFile` (HOME/XDG su `t.TempDir()`), `TestOpenCacheDirIs0700` e `TestOpenCacheTightensExistingDir`, `TestRealStoreIsGuardedAndSnapshotter`, `TestMCPDoesNotOpenCache` (`mcp_test.go`).

- [ ] RED riportato.
- [ ] Implementazione.
- [ ] Gate.
- [ ] Commit `feat(main): cache legata all'account, cartella 0700, nessuna cache in demo (Piano 16, Task 11)`.

## Task 12: documentazione

**Produces:** `README.md` (sezione cache: cosa si salva, 200 messaggi LRU, istantanee, "aggiornamento…", azioni attive dopo l'aggiornamento, dove sta il file e come cancellarlo); `SECURITY.md` (contenuto in chiaro, permessi 0600/0700, legame all'account, MCP senza cache); `CHANGELOG.md`; `AGENTS.md` (gotcha: righe provvisorie e R1-R4, `Snapshotter` opzionale inoltrato dai wrapper, nessun `MessageID` da istantanea verso lo store); `docs/smoke-checklist.md` (avvio a freddo e a caldo, rete staccata con istantanea, azione rifiutata durante l'aggiornamento, cambio account, traccia).

- [ ] Testi in inglese (doc pubbliche), coerenti con il README esistente.
- [ ] Commit `docs: cache locale e istantanee (Piano 16, Task 12)`.

## Task 13: gate finale, misure e revisione

**Produces:**

- Gate: `go build`, `go vet`, `go test -race -count=1 ./...`, `golangci-lint run ./...` a 0 issues, `govulncheck ./...`, build `CGO_ENABLED=0` linux/darwin/windows.
- Benchmark: variante con istantanea in `internal/app/perf_bench_test.go` (finto `Snapshotter`, `Connect` lento simulato): tempo da `Init` alla prima View con righe.
- **Misura reale con la traccia (criterio di accettazione):** Andrea avvia due volte con `GMAIL_TUI_TRACE=1` (la prima popola la cache, la seconda la usa) e lancia `gmail-tui trace <file>` sulla seconda: `first_list_view` con origine `cached` **< 1 s** dall'avvio; `snap_list` hit; apertura di una conversazione già vista: `snap_thread` hit e thread visibile < 1 s dal tasto. Numeri riportati nel report (la traccia non contiene dati personali).
- Revisione indipendente da un agente diverso sul diff di `internal/mail/guard.go`, `internal/app/provisional.go`, R3 in `internal/mail/imap` e migrazione/compattazione in `internal/mail/cache`. Rilievi con test rosso prima del fix.

- [ ] Gate.
- [ ] Benchmark e misura reale.
- [ ] Revisione e fix.

## Domande per Andrea

**Risposte (2026-11-23):** Q16-1 azioni rifiutate con toast durante l'aggiornamento (R1, come proposto). Q16-4 sì: la barra mostra anche l'età dell'istantanea ("aggiornamento… (dati di 2 ore fa)"). Q16-7 budget di Connect portato a 90 s (anche nel wizard e nel probe di doctor, se condividono la costante). Decise dal coordinatore come proposto nel piano: Q16-2 (200 messaggi Gmail distinti), Q16-3 (la cache esistente si adotta), Q16-5 (has:attachment non scrive in cache), Q16-6 (compattazione a Close).

- **Q16-1 (prima del Task 9): azioni durante l'aggiornamento.** Proposta: rifiutate con toast (R1). Alternativa: messe in coda ed eseguite alla conferma della riga (più comodo, più codice e più casi d'errore). Con le misure attuali la conferma arriva dopo Connect + primo list (fino a ~100 s).
- **Q16-2 (prima del Task 3): unità dei 200.** Proposta: 200 messaggi Gmail distinti (le copie INBOX/Tutti i messaggi contano una volta, D16-T4). Alternativa: 200 voci di cache (copie contate a parte).
- **Q16-3 (prima del Task 3): cache esistente senza account.** Proposta: si adotta al primo avvio (i corpi restano; R3 scarta comunque le mailbox con epoca diversa). Alternativa: si svuota una volta.
- **Q16-4 (prima del Task 9): testo in barra.** Solo "aggiornamento…" (come deciso) o anche l'età, es. "aggiornamento… (istantanea di 3 h fa)"?
- **Q16-5 (prima del Task 8): ricerca `has:attachment`.** Con D16-T10 i corpi letti dal post-filtro non entrano in cache: la stessa ricerca ripetuta torna lenta. Accettabile?
- **Q16-6 (prima del Task 3): compattazione.** Proposta: all'uscita (una sola uscita lenta dopo la migrazione). Alternativa: all'avvio (rallenta la prima lista una volta).
- **Q16-7 (fuori piano, da decidere a parte): budget di Connect.** Il LOGIN misurato (28,9 s) è a ridosso del budget di 30 s di `connectCmd`: un LOGIN un po' più lento dà "connessione fallita". Alzarlo (es. 60 s) in un fix separato?

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Azione sul messaggio sbagliato da riga in cache | R1 (tasti), R2 (sostituzione, riallineamento per ThreadID+GmailID), R3 (epoca), R4 (guardia senza rete); test `TestEpochChangeSameIDOtherMessage`, `TestGuardRefusesUnconfirmedIDs` |
| Lettura sbagliata (corpo di un'altra epoca) | R3 in `noteEpoch` prima delle righe fresche; percorso provvisorio solo da cache e solo per ThreadID; `TestFetchHitDoesNotServeOtherEpoch` |
| Istantanea che sovrascrive dati freschi | `listSource`/`folderGen`/`cached` nei messaggi; test degli stantii |
| Un tasto d'azione nuovo dimenticato in R1 | R4 lo ferma comunque; AGENTS.md documenta il punto unico `refuseWhileProvisional` |
| Scritture bbolt lente su macOS (F_FULLFSYNC) | Tocchi in memoria (D16-T6); una transazione per refresh; benchmark del hit |
| File gonfio dalla cache senza limite di oggi | Migrazione con espulsione oltre 200 + compattazione a Close (D16-T7, D16-T8) |
| Cambio account sulla stessa macchina | `BindAccount` con hash (D16-T9) |
| Seconda istanza della TUI (lock bbolt) | `Open` con timeout 1 s → nessuna cache, comportamento di oggi (nessuna istantanea) |
| Cartella creata 0755 da versioni precedenti | `Chmod` a 0700 all'avvio (Task 11) |
| Conflitti su `internal/app/model.go` | Task 9 e 10 sequenziali, un agente |
| Thread che "salta" all'arrivo dei dati freschi | `Reconcile` conserva selezione, espansi e scroll (Task 6) |
| Dati personali nella traccia | Solo Op/Mode/N/Bytes/Role; test di privacy (Task 7) |
| Budget di Connect a ridosso del LOGIN reale | Fuori piano (Q16-7); con istantanea la lista resta leggibile anche se Connect fallisce |

## Revisione indipendente

Dopo il Task 11: review da un agente diverso (Task 13) su R1-R4, migrazione e compattazione. Esito e fix nel report del Task 13.
