# gmail-tui — Piano 2: Backend Gmail (IMAP+SMTP) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Il backend reale: `internal/mail/imap.Store` (MailStore su IMAP Gmail, con IDLE e cache) e `internal/mail/smtp.Sender` (invio via SMTP), entrambi testati **in-process** contro server reali di libreria, più i comandi CLI `fetch` e `send`.

**Architecture:** `imap.Store` implementa `mail.MailStore` (Piano 1) usando `emersion/go-imap/v2/imapclient`, con la cache bbolt del Piano 1 come fonte-cache di FetchMessage. I test usano il server IMAP **in-process** `go.imap/v2/imapserver/imapmemserver` e il server SMTP in-process di `emersion/go-smtp`: stessa copertura protocollo di GreenMail, zero servizi esterni (deviazione dalla spec, documentata). L'invio costruisce MIME con la stdlib (`mime/multipart`) — zero NIH.

**Tech Stack:** go-imap/v2 (imapclient + imapmemserver) · go-smtp v0.26 · stdlib mime · Piano 1 (msg, cache, mail)

**Nota esecuzione:** subagent di implementazione su GLM-5.3-Flash (o glm-5-turbo) quando disponibile nella definizione dell'agente; altrimenti general-purpose.

## Global Constraints (oltre a quelle del Piano 1, ancora valide)

- Module `gmail-tui`, Go 1.25, MIT, golangci-lint pulito a fine task (`golangci-lint run ./...` → 0 issues, versione v2.15.0)
- Dipendenze ammesse SOLO qui: `github.com/emersion/go-imap/v2`, `github.com/emersion/go-smtp` (+loro indirect)
- La UI non esiste ancora: NESSUN import bubbletea/lipgloss in questo piano
- I tipi stanno in `internal/msg` (Piano 1): non modificarli se non per aggiungere campi richiesti da questo piano (nessuno previsto)
- Ogni test IMAP/SMTP gira in-process: `go test ./...` NON richiede rete né Docker
- Le API esatte di go-imap v2 vanno lette dal module cache (`$(go env GOMODCACHE)/go.etcd.io/...`): dove questo piano indica la SEMANTICA, la firma va adattata all'API pinnata; la semantica è il contratto, i test lo verificano
- Semantica Gmail via IMAP (commentarla nel codice): label = cartelle; COPY sulla cartella = aggiunge label; `\Deleted`+EXPUNGE nella cartella = rimuove label (= archivia da INBOX, il messaggio resta in All Mail)

## File Structure

```
internal/mail/store.go                  (Task 1: doc Watch/semantics — modifica)
internal/mail/imap/imap.go              (Task 3: Store, Options, Connect, ListMessages)
internal/mail/imap/query.go             (Task 2: searchCriteria puro)
internal/mail/imap/query_test.go
internal/mail/imap/imap_test.go         (Task 3: harness server + test List)
internal/mail/imap/fetch.go             (Task 4: FetchMessage, SaveAttachment)
internal/mail/imap/mutate.go            (Task 5: MarkRead, Star, Archive, ApplyLabel)
internal/mail/imap/watch.go             (Task 7: Watch IDLE + reconnect)
internal/mail/smtp/mime.go              (Task 8: buildMIME)
internal/mail/smtp/mime_test.go
internal/mail/smtp/smtp.go              (Task 8: Sender)
internal/mail/smtp/smtp_test.go         (server smtp in-process)
internal/cmd/fetch.go, send.go (+test)  (Task 9: CLI)
cmd/gmail-tui/main.go                   (Task 9: dispatch)
```

---

### Task 1: dipendenze + semantica MailStore documentata

**Files:** Modify: `internal/mail/store.go`; `go.mod/go.sum`.

**Interfaces — Produces:** nessuna nuova firma; CONTRATTI documentati su `MailStore` (go doc):
- `Watch`: un solo canale per chiamata; il canale si chiude su `ctx.Done()`; gli eventi sono best-effort (buffer; se il consumer è lento gli eventi si perdono e il consumer deve re-listare); `ChangeKind` `ChangeDisconnected` emesso prima di una riconnessione.
- `Archive` (Gmail/IMAP): rimuove il messaggio dalla mailbox corrente (expunge = rimozione label; il messaggio resta in All Mail).
- `ApplyLabel` (Gmail/IMAP): `on` = copia nella cartella-label; `off` = rimuove dalla cartella-label.
- `ListMessages`/`Search`: `Query.Label` seleziona la mailbox (label) invece di INBOX; `HasAttachment` è best-effort (post-filtro client-side); `ThreadLen` non è disponibile via IMAP ENVELOPE: vale 1, il raggruppamento thread è responsabilità della UI (Piano 3).

- [ ] **Step 1:** `go get github.com/emersion/go-imap/v2@latest github.com/emersion/go-smtp@latest` → `go mod tidy`; `go build ./...` verde
- [ ] **Step 2:** aggiungere i commenti doc sopra ai punti giusti di `store.go` (sull'interfaccia o sul metodo, come go doc idiomatico); NESSUNA modifica a firme
- [ ] **Step 3:** `go vet ./... && go test ./...` verdi; `go doc gmail-tui/internal/mail MailStore` mostra i contratti
- [ ] **Step 4:** Commit `feat(mail): document MailStore IMAP semantics, add go-imap/go-smtp deps`

### Task 2: traduzione Query → IMAP SEARCH (pura)

**Files:** Create `internal/mail/imap/query.go`, `query_test.go`.

**Interfaces — Produces:** `func searchCriteria(q msg.Query) *imap.SearchCriteria` (non esportata; usata da Store in Tasks 3/6). Tabella di traduzione:
- `q.From` → header From; `q.To` → header To; `q.Text` → text (corpo+subject server-side)
- `q.Unread` → senza flag `\Seen`; `q.Starred` → con flag `\Flagged`
- `q.Label` e `q.HasAttachment`: NON traducibili in SEARCH — gestite altrove (mailbox / post-filtro); `searchCriteria` le ignora (commento)
- Query vuota → criteri che matchano tutto

- [ ] **Step 1:** test tabellare `query_test.go`: casi — vuota; from; to; text; unread; starred; combinazione from+unread+text. Ogni caso asserisce i campi del criterio prodotto (adattare le asserzioni ai nomi campo reali dell'API pinnata; la SEMANTICA è il contratto). RED (nessun query.go)
- [ ] **Step 2:** implementare `searchCriteria` secondo la tabella (usare i costruttori/tipi dell'API pinnata, es. `imap.SearchCriteria` con i suoi campi header/text/flags — leggere il module cache). GREEN
- [ ] **Step 3:** `go vet ./... && go test ./...` verdi. Commit `feat(imap): query translation to IMAP SEARCH criteria`

### Task 3: harness IMAP in-process + Store.Connect + ListMessages

**Files:** Create `internal/mail/imap/imap.go`, `imap_test.go` (harness dentro il file di test, riusabile dagli altri task: funzione `startTestServer(t)`).

**Harness (in `imap_test.go`):** avvia `imapmemserver` su `net.Listen("tcp", "127.0.0.1:0")`, crea user `tester`/`secret`, API per seeding: `seedMail(t, from, subject, body string, seen, flagged bool) uint32` (append in INBOX, usando l'API del memserver per creare il messaggio RFC822 — helper `rawMail(from, subject, body string) []byte` con header Date fisso), `addr() string`. Chiude con `t.Cleanup`. Il client di Store si connette in chiaro (vedi Options).

**Interfaces — Produces (firme esatte):**
```go
type Options struct {
    Addr     string // es. "imap.gmail.com:993"
    Username string
    Password string
    Cache    *cache.Cache // può essere nil (no cache)
    Insecure bool // true solo nei test: connessione in chiaro, nessun TLS
}
func NewStore(o Options) *Store
func (s *Store) Connect(ctx context.Context) error
func (s *Store) Close() error
func (s *Store) ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error)
```
**Comportamento ListMessages:**
1. Select della mailbox `q.Label` (o "INBOX" se vuota)
2. `UIDSearch` con criterio "tutti" → lista UID; **paginazione dal più nuovo**: la lista è ascendente, la pagina è la finestra `[len-offset-limit, len-offset)` (offset 0 = più recenti); Limit 0 ⇒ 50
3. `UIDFetch` ENVELOPE+FLAGS+INTERNALDATE solo per gli UID della pagina
4. Mapping: `ID = msg.NewID(mailbox, uid)`; `From` = primo envelope.From formattato "Name <addr>" (o indirizzo nudo; stringa vuota se assente); `Subject` = envelope.Subject; `Date` = INTERNALDATE (fallback envelope date se zero); `Unread` = assenza `\Seen`; `Starred` = presenza `\Flagged`; `ThreadLen` = 1
5. Ordine di ritorno: dal più nuovo al più vecchio

**Test (integration, in-process):**
- seed 5 mail (mix seen/flagged, date crescenti) → List(Query{}, Page{}) restituisce 5 refs in ordine data desc; campi mappati (from/subject/unread/starred/id)
- Page{Offset:1, Limit:2} → saltata la più nuova, 2 refs
- Query{Unread:true} → solo le unseen
- Query{Label:"Archive"} dopo seed su mailbox "Archive" (crearla via harness) → solo quelle

- [ ] **Step 1:** harness + test RED (Store non esiste)
- [ ] **Step 2:** implementare `NewStore/Connect/Close/ListMessages` (Connect: `imapclient.DialTLS` o `imapclient.Dial` se `Insecure`; `Login`; conservare client). GREEN
- [ ] **Step 3:** `go vet ./... && go test ./... && golangci-lint run ./...` verdi. Commit `feat(imap): in-process IMAP harness, Store connect and ListMessages`

### Task 4: FetchMessage (cache-first) + SaveAttachment

**Files:** Create `internal/mail/imap/fetch.go`; test in `imap_test.go` (o `fetch_test.go`).

**Interfaces — Produces:** `func (s *Store) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)`; `func (s *Store) SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error`.

**Comportamento FetchMessage:** `id.Split()` → mailbox+uid → se `Cache != nil`: hit ⇒ return; miss ⇒ Select mailbox, `UIDFetch` body raw con **PEEK** (non segnare letto), `msg.Parse(id, raw)`; `m.Ref.Unread/Starred` riconciliati dai FLAGS del fetch (il peek non li cambia); cache `SetMessage(mailbox, uidvalidity, m)`; return. Miss senza cache idem senza scrittura. Messaggio inesistente ⇒ errore wraps `mail.ErrNotFound`-like (definire `var ErrMessageNotFound = errors.New(...)` in imap e wrapparlo; test asserisce error non-nil).

**Comportamento SaveAttachment:** fetch raw (peek, no cache), `enmime.ReadEnvelope`, `env.Attachments[part]` → `os.WriteFile(path, content, 0o644)`. `part` fuori range ⇒ errore.

**Test:** seed multipart con allegato → Fetch: BodyText dal text/plain, HasHTML=false, allegato con metadata; secondo fetch con cache attiva NON tocca rete (assert: stacca il server dopo il primo fetch — chiudi listener — secondo fetch funziona dalla cache); Fetch di uid inesistente → errore; SaveAttachment scrive il contenuto su file e il file è leggibile; part=9 → errore.

- [ ] **Steps:** RED → implement → GREEN (vet/test/lint) → Commit `feat(imap): cache-first FetchMessage and SaveAttachment`

### Task 5: mutazioni (MarkRead, Star, Archive, ApplyLabel)

**Files:** Create `internal/mail/imap/mutate.go`; test `mutate_test.go`.

**Interfaces — Produces:** i quattro metodi su `*Store` (firme dell'interfaccia MailStore).

**Comportamento:**
- `MarkRead(read=true)`: UIDStore ADD `\Seen` su INBOX; `false`: REMOVE. `Star(on)`: ADD/REMOVE `\Flagged`
- `Archive(ids)`: per la mailbox di ogni id: UIDStore ADD `\Deleted` + `UIDExpunge(uids)` (semantica Gmail: expunge dalla cartella = rimozione label = archiviazione; commento nel codice)
- `ApplyLabel(on=true)`: UIDCopy(uids, mailbox=label) — se la cartella non esiste: crearla con `Create` prima del copy (Gmail crea la label); `off`: Select label, UIDStore `\Deleted` sui uid, UIDExpunge
- Tutte: Select della mailbox di appartenenza degli id prima dell'operazione; batching per mailbox (gli id possono riferire mailbox diverse)

**Test:** MarkRead unread→read (re-list conferma seen); Star on/off; Archive: dopo la chiamata la mail NON è più in INBOX (re-list) — nel memserver l'expunge la rimuove fisicamente: accettabile, il test verifica il comportamento IMAP; ApplyLabel on: la mail appare nella mailbox-label (list lì); ApplyLabel off: scompare.

- [ ] **Steps:** RED → implement → GREEN → Commit `feat(imap): flag mutations, archive and label operations`

### Task 6: Search

**Files:** Modify `internal/mail/imap/imap.go` (o nuovo `search.go`); test in `imap_test.go`.

**Comportamento:** come ListMessages ma con `searchCriteria(q)` alla步骤 2; se `q.HasAttachment`: post-filtro best-effort — per ogni ref candidato (max 200) FetchMessage (cache-first) e tieni solo chi ha `len(Attachments)>0` (commento: IMAP SEARCH non ha criterio allegati).

**Test:** from:x → solo mittente x; text nella subject → match; unread → unseen; starred; has:attachment → solo la multipart con allegato (con e senza cache).

- [ ] **Steps:** RED → implement → GREEN → Commit `feat(imap): server-side search with attachment post-filter`

### Task 7: Watch (IDLE) + riconnessione

**Files:** Create `internal/mail/imap/watch.go`; test `watch_test.go`.

**Interfaces — Produces:** `func (s *Store) Watch(ctx context.Context) (<-chan mail.Change, error)` — contratto Task 1.

**Comportamento:** goroutine: Select INBOX → `Idle`; ogni volta che l'idle termina (dati unilaterali/timeout/errore): se ci sono nuovi messaggi (UID > ultimo visto) ⇒ `ChangeNewMail` con i nuovi ID; se flag cambiati ⇒ `ChangeFlags`; poi ri-idle (NOOP di refresh ogni ~25 min per il limite protocollo). Su errore di rete: `ChangeDisconnected`, riconnessione con backoff (1s,2s,5s,10s, poi 30s cap) riusando le credenziali di Options; se `ctx.Done()` ⇒ chiudi canale e return. Canale buffer 16, send non bloccante (drop = consumer deve re-listare). Suggerimento API: leggere come imapclient v2 espone i dati unilaterali (options di Dial/`UnilateralDataHandler`, oppure terminare l'idle dopo NOTSUPPORTED… usare ciò che l'API pinnata offre; la SEMANTICA è il contratto). L'implementazione può preferire, se l'API rende difficile la differenziazione NewMail/Flags: emettere `ChangeNewMail` generico su qualsiasi evento (la UI farà re-list) — documentato e testato così.

**Test:** Watch attivo → seed nuova mail → entro 5s arriva un Change (kind NewMail o Flags, IDs non vuoti); `ctx cancel` ⇒ il canale si chiude entro 2s; kill del listener (simula disconnect) ⇒ entro timeout arriva `ChangeDisconnected` (se l'implementazione con reconnect è stabile nel test; altrimenti documentare la limitazione nel report).

- [ ] **Steps:** RED → implement → GREEN → Commit `feat(imap): IDLE watch with reconnect and change feed`

### Task 8: SMTP Sender + MIME builder

**Files:** Create `internal/mail/smtp/mime.go`, `smtp.go`, `mime_test.go`, `smtp_test.go`.

**Interfaces — Produces:**
```go
type Options struct { Host string; Port int; Username, Password string; Insecure bool } // Insecure: solo test, no TLS/STARTTLS
func NewSender(o Options) *Sender
func (s *Sender) Send(ctx context.Context, m msg.Outgoing) error
func buildMIME(m msg.Outgoing, now time.Time) ([]byte, error) // non esportata, testata direttamente
```

**buildMIME (semantica precisa):**
- Header: `From` (come fornito), `To` (join ", "), `Cc`, `Subject` (Q-encoded se non-ASCII), `Date` (RFC1123Z di `now`), `Message-ID` `<hex16@dominio-di-From>`, `MIME-Version: 1.0`. **`Bcc` MAI in header** (solo RCPT in Send)
- Senza allegati: `Content-Type: text/plain; charset=utf-8`, corpo con LF→CRLF
- Con allegati: `multipart/mixed`; prima parte text/plain (LF→CRLF); poi per ogni file: parte con `Content-Type` da `mime.TypeByExtension` (fallback `application/octet-stream`), `Content-Disposition: attachment; filename="..."`, `Content-Transfer-Encoding: base64`, contenuto base64 a linee da 76 caratteri CRLF. File inesistente ⇒ errore wrapped

**Sender.Send:** dial `net.Dial` host:port (con ctx), `smtp.NewClient`; se !Insecure: `StartTLS` (tls.Config ServerName=Host) — Gmail: 587 STARTTLS; `Auth` (PLAIN) se Username != ""; `Mail(from)`; RCPT per To+Cc+Bcc (deduplicati); `Data()` → scrivere MIME; `Quit`. Errori wrapped con fase (`dial`, `starttls`, `auth`, `rcpt <addr>`, `data`).

**Test:**
- `mime_test.go` (unit, no rete): senza allegati — header esatti, nessun Bcc, CRLF nel corpo; con allegato tmpfile — multipart, boundary presente, base64 decodifica = contenuto originale, filename presente; subject non-ASCII → Q-encoded; file mancante → errore
- `smtp_test.go` (in-process): server `go-smtp` con backend che cattura (from, rcpts, data); assert: rcpts = To+Cc+Bcc senza duplicati; data contiene To/Cc e NON Bcc; data ha From/Subject; con allegato la data è multipart

- [ ] **Steps:** RED (mime) → implement mime → GREEN → RED (smtp) → implement smtp → GREEN → vet/test/lint → Commit `feat(smtp): MIME builder and SMTP sender with in-process tests`

### Task 9: CLI `fetch` e `send` + wiring main

**Files:** Create `internal/cmd/fetch.go`, `internal/cmd/send.go`, `fetch_test.go`, `send_test.go`; Modify `cmd/gmail-tui/main.go`.

**Interfaces — Produces:**
```go
// internal/cmd/fetch.go
func Fetch(out io.Writer, store mail.MailStore, q msg.Query, limit int) int // 0 ok / 1 errore
// internal/cmd/send.go
type Sender interface { Send(ctx context.Context, m msg.Outgoing) error } // compatibile con *smtp.Sender
func Send(out io.Writer, sender Sender, cfg config.Config, args SendArgs, body io.Reader) int
type SendArgs struct { To, Cc, Bcc, Subject string; Attach []string }
```
- `Fetch`: ListMessages(Query, Page{Limit}) → righe `uid\tfrom\tsubject\tdate(RFC3339)\tflags` (flags: "U" se unread, "*" se starred, concatenate); `limit` default 20
- `Send`: legge body da io.Reader; Outgoing{From: cfg.Address, …}; subject Q-encoding gestita da buildMIME (sender lo fa); stampa `inviato a N destinatari` (inglese: `sent to N recipients`); errori → stderr + 1
- main: `gmail-tui fetch [--limit N] [--query "from:x is:unread"]` e `gmail-tui send --to a@b --subject s [--attach f]... < body`; `--query` parsificata con `msg.ParseQuery(s) msg.Query` — NUOVA funzione in `internal/msg/query.go`: token `from:x to:x is:unread is:starred has:attachment label:x`, resto = testo libero (più token si accumulano in Text separati da spazio; from/to/label: ultimo vince). Factory store in main: Options da costanti Gmail (`imap.gmail.com:993`, `smtp.gmail.com:587`), username = cfg.Address, password = `os.Getenv("GMAIL_APP_PASSWORD")`, cache da UserCacheDir
- NESSUN test di main che tocchi la rete: main è wiring sottile; Fetch/Send testati con mock (Piano 1) + fake Sender

**Test:** Fetch con mail.Mock → output righe attese (uid/from/subject/flag U e *), exit 0; Mock.Err → exit 1; Send con fake Sender che cattura → Outgoing corretto (bcc inclusi negli argomenti), output "sent to 2 recipients", exit 0; body vuoto ok; `msg.ParseQuery`: tabella — `""` vuota; `from:a text` ; `is:unread label:Work x y` → {Unread, Label:"Work", Text:"x y"}; token sconosciuto `foo:bar` → trattato come testo? NO: se ha forma `key:value` con key nota ignota… decisione: `key:value` con key NON riconosciuta resta testo (comportamento permissive, documentato e testato).

- [ ] **Steps:** RED ParseQuery (in internal/msg) → implement → GREEN → RED Fetch/Send (internal/cmd) → implement → main wiring → `go vet ./... && go test ./... && golangci-lint run ./...` + smoke `go run ./cmd/gmail-tui fetch --help` (senza rete: flag parsing) → Commit `feat(cli): fetch and send commands wired to IMAP/SMTP`

## Verifica finale del piano

- [ ] `go vet ./... && go test -race ./... && golangci-lint run ./...` verdi (0 lint issues)
- [ ] `go run ./cmd/gmail-tui fetch --help` e `send --help` funzionano (nessuna rete richiesta per --help)
- [ ] `var _ mail.MailStore = (*imap.Store)(nil)` e `var _ cmd.Sender = (*smtp.Sender)(nil)` compilate (assert di conformance nei rispettivi package)
- [ ] git log: un commit per task

## Prossimo piano

Piano 3 — TUI core (Bubble Tea v2: app, inbox, thread, ricerca, keymap+mouse, status bar; prima però: semantica `Watch` già definita qui, stili da Theme, e collegamento a Store reale con fallback errore amichevole senza credenziali).
