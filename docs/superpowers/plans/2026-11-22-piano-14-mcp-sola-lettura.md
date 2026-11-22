# gmail-tui — Piano 14: server MCP in sola lettura — Implementation Plan

> **For agentic workers:** branch `feat/piano-14` (da `main` e6961e2). Leggere `AGENTS.md`. TDD vero (test rosso verificato prima del fix), commit per task, checkbox `- [ ]`. Questo documento è la FASE A: nessun codice di produzione. La FASE B parte solo dopo la conferma di Andrea sulle decisioni M1-M6 e sulla dipendenza SDK.

**Goal:** esporre la posta a un agente (Claude Code, Claude Desktop, altri client MCP) tramite `gmail-tui mcp`, un server MCP su stdio sopra `mail.MailStore`. Il server è **sola lettura**: nessun tool invia, salva bozze, archivia, applica label, sposta nel cestino o segna come letto (D14). Ogni risposta che contiene testo di email è trattata come dato non fidato.

**Stack:** una dipendenza nuova: `github.com/modelcontextprotocol/go-sdk` (da approvare, vedi M2). Nessun CGO, nessun import di go-imap fuori da `internal/mail/imap`.

## Decisioni

Scelte delegate da Andrea "per competenza" (D14). Le voci marcate **da confermare** vanno presentate ad Andrea prima della FASE B (AskUserQuestion).

| ID | Decisione | Scelta proposta | Motivazione |
|---|---|---|---|
| M1 | Trasporto | Solo stdio, sottocomando `gmail-tui mcp` | È il trasporto usato da Claude Code e Claude Desktop per server locali. Nessun listener di rete: nessuna superficie esposta, nessuna auth da gestire. |
| M2 | SDK | `github.com/modelcontextprotocol/go-sdk` **v1.8.0** (ultima stabile; GitHub release "Latest", 14 set 2026; v1.7.0 del 28 lug 2026 è la precedente). Versione attesa in `go.mod`: `v1.8.0`, verificata in fase B sul module cache | Ufficiale, mantenuto con Google. Alternative community scartate: non ufficiali, nessuna garanzia di compatibilità col protocollo. **Approvata dal coordinatore (delega di Andrea); passa `govulncheck`.** |
| M3 | Connessione | Lazy: `Connect` alla prima chiamata di tool, timeout 30 s (come `connectCmd`, AGENTS regola 7), riprova alla chiamata successiva se fallisce. Errore restituito come risultato tool con `IsError`, non come errore di protocollo | Il server parte anche senza rete; un avvio che fallisce non blocca il client MCP. |
| M4 | Cache bbolt | **Approvata: nessuna cache nel server MCP.** | `internal/mail/cache/cache.go:25` apre bbolt con `Timeout: time.Second`: con la TUI aperta, un secondo processo sullo stesso file fallisce. |
| M5 | Limiti di risposta | Liste: `limit` default 20, max 100. Corpo: default 8 000 caratteri, max 32 000, troncamento con marcatore esplicito. Risposta totale del tool ≤ 64 KB | Un messaggio enorme o una ricerca larga non devono saturare il contesto dell'agente. Il troncamento dichiara sempre quanto manca. |
| M6 | Password | Nessun cambio alle fonti: `GMAIL_APP_PASSWORD` → keychain → assente (come `resolvePassword`). La password non compare in risultati, errori o log; la documentazione sconsiglia di metterla nella config MCP (il campo `env` finirebbe in file di config leggibili) | Il vincolo "sola lettura" sta nel server: una App Password non si restringe a lettura. Il segreto resta nel portachiavi. |

Tool proposti (tutti con annotazione `readOnlyHint: true`, `destructiveHint: false`):

| Tool | Parametri | Esito |
|---|---|---|
| `list_folders` | nessuno | cartelle e label: `name`, `role` (da `mail.Mailbox`, `store.go:17-22`) |
| `list_recent` | `folder` (default `INBOX`), `limit` (default 20, max 100), `offset` | righe di lista: `id`, `from`, `subject`, `date`, `unread`, `starred` |
| `search_messages` | `query` (sintassi tipo Gmail, parsata da `msg.ParseQuery`, `query.go:32`), `folder` opzionale, `limit`, `offset` | come sopra |
| `read_message` | `id` (formato `MAILBOX:UID`, `message.go:12`), `max_chars` (default 8 000, max 32 000) | intestazioni (`from`, `to`, `cc`, `subject`, `date`), corpo testo troncato, allegati solo come metadati: `filename`, `content_type`, `size` |
| `read_thread` | **solo dopo il Piano 8** (Task 9): `id` del messaggio, `limit` | conversazione via `X-GM-THRID` |

Non esposti: `SaveAttachment` (il contenuto dell'allegato non esce dal server), il corpo HTML grezzo (solo testo, già ricavato da `msg.Parse`), config, percorsi, indirizzo dell'account, qualunque credenziale.

## Difese contro la prompt injection

Fonti verificate con ricerca web (2026-11-22, dai risultati di ricerca, non pagine lette per intero):

- **Google, documentazione Workspace MCP:** avverte che esporre un modello a dati non fidati espone a *indirect prompt injection*, e raccomanda di usare solo server fidati ([developers.google.com/workspace/guides/configure-mcp-servers](https://developers.google.com/workspace/guides/configure-mcp-servers)). Il server Gmail ufficiale (`gmailmcp.googleapis.com/mcp/v1`) espone già strumenti di lettura e di bozze; noi scegliamo di non esporre scritture.
- **apchat-agent/mcp-email:** server MCP per Gmail definito "injection-hardened", fondato sull'idea che l'email è un canale su cui chiunque può mettere testo davanti al modello ([github.com/apchat-agent/mcp-email](https://github.com/apchat-agent/mcp-email)). Da leggere in fase B per i dettagli dei marcatori (non verificati qui).
- **nikolausm/imap-mcp-server:** supporta un'allowlist di tool (solo lettura via `IMAP_MCP_ENABLED_TOOLS`) ([github.com/nikolausm/imap-mcp-server](https://github.com/nikolausm/imap-mcp-server)). Conferma il pattern "limitare i tool, non fidarsi del client".
- **non-dirty/imap-mcp:** dai risultati di ricerca non risultano difese contro injection (da verificare, non usare come modello).
- **ni-c/imap-mcp** ([github.com/ni-c/imap-mcp](https://github.com/ni-c/imap-mcp)): avvolge i corpi tra marcatori con un nonce casuale generato a ogni chiamata; rimuove caratteri zero-width; dichiara `"untrusted": true` in ogni risultato con contenuto di posta; di default è in sola lettura (`IMAP_READ_ONLY=true`). Il formato esatto del marcatore non è riportato nel README: quello del piano è definito qui.

Difese del piano:

1. **Marcatori con nonce per chiamata.** Ogni risposta che contiene testo di email (intestazioni, corpo, nomi file, anteprime di lista) è racchiusa in `BEGIN_UNTRUSTED_EMAIL nonce=<hex>` … `END_UNTRUSTED_EMAIL nonce=<hex>`, dove `<hex>` sono 16 byte da `crypto/rand`, diversi a ogni chiamata. Un messaggio che contiene una falsa riga `END_UNTRUSTED_EMAIL` non può chiudere il blocco perché non conosce il nonce. *Pattern del nonce per chiamata: ni-c/imap-mcp (vedi sopra). Il formato dei marcatori è nostro.*
2. **Descrizioni dei tool** che dichiarano il contenuto non fidato e vietano di eseguire istruzioni trovate nelle email.
3. **Sanificazione:** rimozione di caratteri di controllo e sequenze ANSI/OSC (evita injection nel terminale quando l'output viene mostrato), normalizzazione dei fine riga.
4. **Nessun tool di scrittura:** il server riceve un `Reader` che espone solo metodi di lettura (Task 2); un test verifica che il set di tool registrati sia esattamente quello previsto.
5. **Password mai esposta:** il testo degli errori IMAP passa da una funzione di redazione che sostituisce il segreto con `***`; test con password nota su tutti i percorsi di errore.
6. **Lettura senza effetti collaterali:** `FetchMessage` usa `BODY.PEEK[]` (`internal/mail/imap/fetch.go:171`); il test `internal/mail/imap/fetch_test.go:145-151` verifica che il messaggio resti unread. Il server non chiama mai `MarkRead`: un test sul Mock verifica zero chiamate di scrittura.
7. **stdout riservato al protocollo:** nessun log su stdout; i log vanno su stderr (come `runTUI`). Test che lo impedisce.

---

## Wave 0 — Conferme (prima del codice)

### Task 0: conferme di Andrea

**Files:** nessuno.

- [x] Conferme del coordinatore (delega di Andrea, FASE B): M2 approvata (go-sdk v1.8.0), M4 nessuna cache nel server MCP, M5 e M6 approvati, Wave 1-3 ora, Wave 4 dopo il merge del Piano 8.
- [x] Link `ni-c/imap-mcp`: https://github.com/ni-c/imap-mcp (letto: nonce per chiamata, `untrusted: true`, default read-only).

## Wave 1 — Nucleo senza SDK (logica pura, test puri)

### Task 1: package `internal/mcpserver`, formattazione sicura

**Files:** `internal/mcpserver/wrap.go`, `internal/mcpserver/wrap_test.go`, `internal/mcpserver/redact.go`, `internal/mcpserver/redact_test.go`.

**Produces:**
```go
// Wrap racchiude il testo non fidato in marcatori con nonce casuale.
func Wrap(nonce string, text string) string
func NewNonce() (string, error)                  // 16 byte hex da crypto/rand
func Sanitize(s string) string                   // via caratteri di controllo e sequenze ANSI/OSC
func Truncate(s string, maxRunes, maxBytes int) (string, bool) // su rune e byte, segnala il troncamento
func Redact(err error, secrets ...string) error  // sostituisce i segreti con ***
```

**Test:** nonce diversi a ogni chiamata; un testo con falso `END_UNTRUSTED_EMAIL` non chiude il blocco (il nonce differisce); Sanitize toglie ESC, OSC e `\x00`; Truncate non spezza rune UTF-8 e dichiara il troncamento; Redact non lascia la password nel messaggio d'errore, anche con `%w`.

- [ ] RED → impl → GREEN → Commit `feat(mcpserver): untrusted-content wrapping, sanitizing, redaction`

### Task 2: adattatore di sola lettura

**Files:** `internal/mcpserver/reader.go`, `internal/mcpserver/reader_test.go`.

**Produces:**
```go
// Reader è il sottoinsieme in sola lettura di mail.MailStore usato dal server.
type Reader interface {
    Connect(ctx context.Context) error
    ListMailboxes(ctx context.Context) ([]mail.Mailbox, error)
    ListMessages(ctx context.Context, q msg.Query, pg mail.Page) ([]msg.MessageRef, error)
    Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error)
    FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)
}
```
`mail.MailStore` soddisfa `Reader`; il server non riceve mai lo store completo. È il punto dove il Piano 8 aggiungerà `Query.Raw` e il metodo di conversazione (Wave 4): cambiamenti localizzati qui.

**Test:** `mail.Mock` soddisfa `Reader` (compile-time); un wrapper che conta le chiamate verifica che `read_message` non chiami metodi di scrittura; mapping di `offset`/`limit` su `Page` con i limiti di M5 (limit 0 → 20, >100 → 100, negativi rifiutati).

- [ ] RED → impl → GREEN → Commit `feat(mcpserver): read-only Reader adapter over MailStore`

## Wave 2 — Server MCP con SDK

### Task 3: dipendenza SDK e server stdio

**Files:** `go.mod`, `go.sum`, `internal/mcpserver/server.go`, `internal/mcpserver/server_test.go`.

- [ ] Gate di approvazione (M2) prima di `go get`. Poi `go get github.com/modelcontextprotocol/go-sdk@v1.8.0`; verificare l'API sul module cache (`$(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.8.0`), non sulla memoria: server, registrazione dei tool, transport stdio, client in-memory per i test (nomi da confermare in fase B).
- [ ] `govulncheck ./...` pulito dopo il `go get`.
- [ ] `func NewServer(r Reader, opts Options) *mcp.Server` con i tool di M5 e della tabella. Registrazione dei soli tool previsti; annotazioni `readOnlyHint`.
- [ ] Test: client MCP in-process (trasporto in memoria dell'SDK) contro il server con `*mail.Mock`: `tools/list` restituisce esattamente i 4 tool attesi, nessun altro; ogni tool ha una descrizione che dichiara il contenuto non fidato.
- [ ] RED → impl → GREEN → Commit `feat(mcpserver): MCP server with read-only tools over stdio SDK`

### Task 4: tool di lettura

**Files:** `internal/mcpserver/tools.go`, `internal/mcpserver/tools_test.go`.

- [ ] `list_folders`, `list_recent`, `search_messages` (usa `msg.ParseQuery`, `query.go:32`; `folder` diventa `Query.Label`), `read_message` (intestazioni, corpo testo via `msg.Parse` già esistente, allegati solo metadati).
- [ ] Ogni risultato con testo di email è avvolto da `Wrap` con nonce nuovo; ogni stringa passa da `Sanitize`; i limiti di M5 sono applicati e dichiarati nel risultato.
- [ ] Test con Mock: messaggio con corpo contenente un falso `END_UNTRUSTED_EMAIL` e sequenze ANSI → output avvolto, sanificato, nonce diverso tra due chiamate; corpo di 100 000 caratteri → troncato con dichiarazione; risposta totale ≤ 64 KB; `read_message` su ID inesistente → `IsError` senza dettagli interni; `read_message` non imposta `\Seen` (Mock: nessuna chiamata a `MarkRead`).
- [ ] RED → impl → GREEN → Commit `feat(mcpserver): list, search and read tools with untrusted-content wrapping`

## Wave 3 — Sottocomando, sicurezza, documentazione

### Task 5: sottocomando `gmail-tui mcp`

**Files:** `cmd/gmail-tui/main.go` (dispatch in `main()`, `main.go:38-68`, accanto a `fetch`/`send`), `cmd/gmail-tui/mcp.go` (nuovo, o funzione in `main.go` se coerente con `runFetch`), `cmd/gmail-tui/main_test.go`.

- [ ] `case "mcp": os.Exit(runMCP(os.Args[2:]))`. Config e password con le stesse funzioni di `runFetch`/`appPassword` (`main.go:431`), nessuna nuova logica di risoluzione.
- [ ] Store: `newIMAPStore(cfg)` (`main.go:443`) senza cache secondo M4. Nessuna TUI, nessuna scrittura su stdout.
- [ ] Test: password assente → errore su stderr e uscita non zero, nulla su stdout; password nota non compare in stdout/stderr in nessun caso (regressione con `getenvOf`, `main_test.go:26`).
- [ ] RED → impl → GREEN → Commit `feat(cli): gmail-tui mcp subcommand (stdio)`

### Task 6: test di sicurezza di regressione

**Files:** `internal/mcpserver/security_test.go`.

- [ ] Set di tool registrati uguale all'elenco atteso (nessun nome con send/draft/archive/label/trash/mark/star/save).
- [ ] Il tipo passato a `NewServer` non implementa `Send`, `Archive`, `Trash`, `MarkRead`, `Star`, `ApplyLabel`, `SaveDraft`, `SaveAttachment` (type assertion su interfacce di scrittura, oltre al compile-time di Task 2).
- [ ] Nessuna stringa di configurazione, percorso o password nei risultati di nessun tool (il test inserisce una password nota e la cerca in tutti gli output, anche errori).
- [ ] Stdout di `gmail-tui mcp` contiene solo frame JSON-RPC (test end-to-end con factory iniettata che restituisce Mock).
- [ ] Commit `test(mcpserver): security regression suite (no write tools, no secrets in output)`

### Task 7: documentazione

**Files:** `README.md` (sezione "MCP (sola lettura)" con esempi), `CHANGELOG.md` (voce `[Unreleased]`), `SECURITY.md` (posta come input non fidato, rischi di injection, password fuori dalla config MCP), `AGENTS.md` (riga in Layout per `internal/mcpserver/` e la regola "il server non importa go-imap").

Esempi da verificare in fase B sul comando reale del client:
- Claude Code: `claude mcp add gmail-tui -- /percorso/gmail-tui mcp` (sintassi da controllare con `claude mcp add --help`).
- Claude Desktop, `claude_desktop_config.json`: `"mcpServers": { "gmail-tui": { "command": "/percorso/gmail-tui", "args": ["mcp"] } }`.

Nel README: la password resta nel portachiavi (`gmail-tui login`) o in `GMAIL_APP_PASSWORD` esportata nell'ambiente del client, non nel JSON.

- [ ] Commit `docs: MCP read-only server (README, CHANGELOG, SECURITY, AGENTS)`

### Task 8: smoke con account reale (solo Andrea)

**Files:** `docs/smoke-checklist.md`.

- [ ] Sezione "MCP sola lettura": avvio da Claude Code, `list_folders`, `search_messages` con `from:`, `read_message` su un messaggio con allegato (solo metadati), verifica che il messaggio resti non letto in Gmail web, verifica che la password non appaia nei log del client.
- [ ] **Non eseguita dall'agente:** nessuna connessione a Gmail reale in questo piano. La prova la fa Andrea.

## Wave 4 — Integrazione Piano 8 (dopo il merge del Piano 8)

### Task 9: ricerca con sintassi Gmail e conversazioni

**Prerequisito:** Piano 8 in `main` (`Query.Raw` per `X-GM-RAW`, metodo di conversazione su `MailStore`, label in `MessageRef`).

**Files:** `internal/mcpserver/reader.go` (metodo di conversazione aggiunto a `Reader`), `internal/mcpserver/tools.go`, test.

- [ ] `search_messages` passa la query intera come `X-GM-RAW` (operatori `older_than:`, `in:`, `-label:`) invece del parsing locale.
- [ ] Nuovo tool `read_thread` (conversazione da `X-GM-THRID`, comprese le risposte in Inviati); limiti di M5 per numero di messaggi e per corpo.
- [ ] Label dei messaggi nei risultati di lista (dati non fidati, dentro il marcatore).
- [ ] Verifica che `Reader` resti senza metodi di scrittura dopo le aggiunte (Task 6 rieseguito).
- [ ] Commit `feat(mcpserver): Gmail search syntax and read_thread after Piano 8`

### Task 10: gate finali

- [ ] Gate completi (`CONTRIBUTING.md`), `govulncheck ./...` pulito, `grep -rn "go-imap" internal --include="*.go" -l | grep -v internal/mail/imap` non stampa nulla.
- [ ] `/code-review` sul branch, rilievi corretti.
- [ ] Merge in `main`: chiedere conferma ad Andrea.

---

## Rischi e mitigazioni

| Rischio | Mitigazione |
|---|---|
| Un `fmt.Print` o log su stdout corrompe il protocollo MCP | Log solo su stderr; test end-to-end su stdout (Task 6). |
| TUI e server MCP aprono la cache bbolt insieme: il secondo processo fallisce dopo 1 s | M4: il server non usa la cache della TUI; test di apertura concorrente. |
| Injection da corpo email che imita istruzioni o chiude il blocco | Nonce per chiamata, descrizioni dei tool, nessun tool di scrittura: anche un agente manipolato non può inviare o modificare. |
| Password in output o nei log del client | Redazione su tutti gli errori (Task 1), test di regressione (Task 6), esempi di config senza password. |
| Contesto saturato da messaggi enormi | Limiti di M5 e troncamento dichiarato. |
| Lettura che segna il messaggio come letto | `BODY.PEEK[]` (verificato in `fetch.go:171`) e test su `MarkRead` = 0 chiamate. |
| Nuova dipendenza con vulnerabilità | `govulncheck` dopo `go get`; approvazione esplicita (M2). |
| API dell'SDK diversa da quella attesa | Verifica sul module cache della versione pinnata prima di scrivere (Task 3). |
| Piano 8 cambia `Query` e `MailStore` mentre il server è in corso | Adattatore `Reader` (Task 2) e Wave 4 separata: Wave 1-3 non dipendono dal Piano 8. |
| Client che non gestisce bene troncamento o marcatori | Il marcatore dichiara il troncamento in linguaggio esplicito; verifica nello smoke (Task 8). |

## Dubbi: chiusi dal coordinatore prima della FASE B

1. M2: approvata, go-sdk v1.8.0 (ultima stabile; v1.9.0-pre.1 esiste ma è pre-release, esclusa).
2. M4: nessuna cache nel server MCP.
3. Ordine: Wave 1-3 ora, Wave 4 dopo il merge del Piano 8.
4. `ni-c/imap-mcp`: link ricevuto, citato come fonte del nonce.
5. Limiti M5 (20/100 righe, 8 000/32 000 caratteri, 64 KB): approvati.

## Stato FASE B e deviazioni

Stato per task (branch `feat/piano-14`): Task 0 fatto (`1b39562`); Task 1-7 fatti (commit in `git log`); Task 8 è di Andrea; Task 9-10 dopo il merge del Piano 8.

Deviazioni rispetto alla FASE A, motivate:

- **Reader con `Search`:** oltre ai quattro metodi previsti, `Reader` include `Search` (usato da `search_messages`). `Search` non è paginata nello store: offset e limit si applicano al risultato (`slicePage`). La paginazione vera è del Piano 12.
- **Limiti in byte:** `Truncate` limita anche in byte (`maxBodyBytes` = 48 KB), perché 32 000 rune UTF-8 possono superare 64 KB. Una risposta oltre 64 KB è rifiutata con errore esplicito (`respond`), mai troncata in silenzio. Le liste sono ridotte in coda e dichiarate con `truncated`.
- **Formato del risultato:** ogni tool restituisce un testo con una riga di preambolo, poi il blocco `BEGIN_UNTRUSTED_EMAIL nonce=<hex>` / JSON / `END_UNTRUSTED_EMAIL nonce=<hex>`. Il JSON contiene `"untrusted": true`. Anche `list_folders` passa dal blocco, per uniformità.
- **Sanitize:** oltre a ANSI/OSC e controlli, rimuove i caratteri zero-width e i controlli bidi (come ni-c/imap-mcp). Il formato dei marcatori resta quello del piano: ni-c non ne documenta il formato esatto.
- **Task 3 e Task 4:** i tool di lettura sono stati registrati come stub nel commit del Task 3 (rossi nei test del Task 4), poi implementati nel Task 4.
- **Task 6:** la suite di sicurezza passa sul codice attuale. Non è un rosso: verifica invarianti. Lo stdout è controllato staticamente (nessun `fmt.Print`/`os.Stdout` nel package), non con un processo figlio. Il test end-to-end con processo reale resta a carico dello smoke di Andrea (Task 8).
- **Task 5:** il test di `serveMCP` usa il trasporto in memoria, non stdio. Il trasporto stdio è quello dell'SDK, usato da `runMCP`.
- **Task 7:** README (sezione "MCP server (read-only)" con i comandi `claude mcp add` verificati con `claude mcp add --help`), CHANGELOG, SECURITY, layout di AGENTS.
- **Dipendenze:** `go-sdk` v1.8.0 porta con sé `golang.org/x/oauth2`, `golang-jwt/jwt/v5` e `segmentio/asm` (indiretti). `govulncheck ./...` segnala una vulnerabilità dello stdlib, `GO-2026-6859` (`net/http`, corretta in go1.27.4), con tracce già presenti su `main` (`composer`, `cache`, `cmd/send`). Non è introdotta da questo piano; il go.mod pinna `go1.27.3`. Alzare il toolchain è una decisione di Andrea.

## Fonti

- [Google, Configure the Google Workspace MCP servers](https://developers.google.com/workspace/guides/configure-mcp-servers): avviso di indirect prompt injection, strumenti del server Gmail ufficiale.
- [apchat-agent/mcp-email](https://github.com/apchat-agent/mcp-email): server Gmail MCP "injection-hardened".
- [nikolausm/imap-mcp-server](https://github.com/nikolausm/imap-mcp-server): allowlist di tool per uso in sola lettura.
- [non-dirty/imap-mcp](https://github.com/non-dirty/imap-mcp): server IMAP MCP (senza difese dichiarate nei risultati).
- [modelcontextprotocol/go-sdk, releases](https://github.com/modelcontextprotocol/go-sdk/releases): v1.8.0 come ultima stabile; v1.7.0 del 28 lug 2026 come precedente.
- Codice: `internal/mail/store.go`, `internal/mail/mock.go`, `internal/mail/imap/fetch.go:171`, `internal/mail/imap/fetch_test.go:145-151`, `internal/mail/cache/cache.go:25`, `internal/msg/message.go`, `internal/msg/query.go:32`, `cmd/gmail-tui/main.go:38-68, 431, 443`.
