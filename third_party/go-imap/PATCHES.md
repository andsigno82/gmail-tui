# Patch Gmail su go-imap v2 (third_party/go-imap)

Copia in-tree di `github.com/emersion/go-imap/v2` versione **v2.0.0-beta.9**, presa dal module cache.
Agganciata al progetto con `replace` in `go.mod`. Decisione D8-1 del Piano 8 (`docs/superpowers/plans/2026-11-22-piano-8-estensioni-gmail.md`).

La copia resta identica a upstream salvo i file elencati nella tabella delle patch. Ogni patch ha il task che la introduce.

## Patch

| Task | File | Patch |
|---|---|---|
| 1 | (nessuno) | Copia iniziale, nessuna modifica |
| 2 | `fetch.go` | `FetchOptions.GmailMsgID`, `GmailThreadID`, `GmailLabels` (campi aggiunti in coda) |
| 2 | `imapclient/fetch.go` | scrittura degli item `X-GM-*` in `writeFetchItems`; campi `GmailMsgID`/`GmailThreadID`/`GmailLabels` in `FetchMessageBuffer`; `populateItemData`; case `X-GM-MSGID`/`X-GM-THRID`/`X-GM-LABELS` in `handleFetch`; il `default` che era errore ora scarta il valore (`skipMsgAttValue`) |
| 2 | `imapclient/gmail.go` (nuovo) | tipi `FetchItemDataGmail*`, parser (`readGmailID` uint64, `readGmailLabels`), decoder modified UTF-7 (RFC 3501 §5.1.3, scritto a mano: `cention-sany/utf7` implementa lo UTF-7 con "+", non quello IMAP con "&") |

| 3 | `search.go` | campo `SearchCriteria.Gmail *SearchCriteriaGmail` (`Raw`, `MsgID`, `ThreadID`, `Labels`) e il tipo; `And()` NON unisce questo campo |
| 3 | `imapclient/search.go` | encoding di `X-GM-RAW` (quotato, con escape), `X-GM-MSGID`, `X-GM-THRID`, `X-GM-LABELS` in `writeSearchKey`; `CHARSET UTF-8` se il testo Gmail non è ASCII |
| 4 | `store.go` | tipo `StoreGmailLabels` (`Op`, `Silent`, `Labels`). **Usato dal backend dal Piano 10, Task 5** (`ModifyLabels` in `internal/mail/imap/gmail_labels.go`, con le label codificate in modified UTF-7 prima dell'invio) |
| 4 | `imapclient/gmail.go` | metodo `Client.StoreGmailLabels`: `STORE set [+|-]X-GM-LABELS[.SILENT] ("l1" "l2")`; `Store` upstream invariato. Piano 10, Task 1: le label di sistema (che iniziano con `\`, es. `\Inbox`, `\Important`, `\Starred`) sono scritte come atomi NUDI, non quotati: quotate, Gmail le crea come label utente di nome `\Inbox`. Le label utente restano quotate; le label non ASCII arrivano gia' in modified UTF-7 (ASCII) dal backend |

Test della patch: `imapclient/gmail_test.go` (Task 2), `imapclient/gmail_search_test.go` (Task 3), `imapclient/gmail_store_test.go` (Task 4). Tutti usano un server scriptato su `net.Pipe`, non il memserver, che non conosce X-GM-*.

Deviazione dal `go.mod` upstream: nessuna (la copia usa il `go.mod` di beta.9). La modified UTF-7 è decodificata da `imapclient/gmail.go` senza dipendenze esterne.

Deviazioni dall'upstream oltre alle patch: nessuna. Il `go.mod` della copia è quello upstream (`go 1.18`).

## Procedura di riallineamento a una nuova versione upstream

1. Scaricare la nuova versione in una directory temporanea (`go mod download github.com/emersion/go-imap/v2@<tag>`) e copiarla in `third_party/go-imap.new`.
2. Confrontare la copia attuale con la vecchia versione upstream (`diff -ru <vecchia upstream> third_party/go-imap`): le differenze sono solo le patch elencate sopra.
3. Sostituire `third_party/go-imap` con la nuova upstream e riapplicare le patch, file per file, dalla tabella.
4. Aggiornare la riga "versione" in questo file.
5. Eseguire `cd third_party/go-imap && go test ./...`, poi `go test -race -count=1 ./...` dalla radice e `govulncheck ./...`.
6. Verificare che `go.mod` non richieda dipendenze nuove (`GOPROXY=off go build ./...`).

I test della copia si lanciano dalla sua directory: `third_party/go-imap` è un modulo separato, quindi `go test ./third_party/...` dalla radice fallisce con "setup failed".
