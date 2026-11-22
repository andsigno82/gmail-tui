# gmail-tui — Piano 7.1: rifiniture dalle review del Piano 7 — Implementation Plan

> **For agentic workers:** branch `feat/piano-7-1` (da `feat/piano-7`). Leggere `AGENTS.md`. TDD, un commit per task, checkbox `- [ ]`. Decisione D7.1 (Andrea, 2026-11-22): domini IDN in punycode, dominio con punto obbligatorio.

**Goal:** chiudere i rilievi di gravità bassa delle review del Piano 7 e un bug trovato pianificando: gli indirizzi con nome visualizzato arrivano grezzi a `RCPT TO`.

**Tech Stack:** stdlib `net/mail` · `golang.org/x/net/idna` (modulo già in `go.mod` come indiretto: diventa diretto, nessun modulo nuovo) · nessuna altra dipendenza.

## Global Constraints

- `go test -race ./...`, `golangci-lint` 0 issues, `govulncheck` pulito, build `CGO_ENABLED=0` su linux/darwin/windows.
- go-imap solo in `internal/mail/imap`; `internal/cmd` non importa `internal/mail/imap` (il probe IMAP di `doctor` è iniettato dal main).
- Commenti e stringhe in italiano con riferimento `Piano 7.1, Task N`.

---

### Task 1: normalizzazione dei destinatari (dominio)

**Files:** `internal/msg/validate.go` (+test).

**Produces:**
```go
// NormalizeRecipients valida To/Cc/Bcc come ValidateRecipients e restituisce
// una copia di o con ogni indirizzo in forma canonica: mail.Address.String()
// (nome RFC 2047 se non ASCII) e dominio convertito in punycode con
// idna.Lookup.ToASCII. Local part non ASCII → errore (niente SMTPUTF8).
func NormalizeRecipients(o Outgoing) (Outgoing, error)
```
- `ValidateRecipients` resta (usata dove serve solo il controllo) e applica le stesse regole: dominio con almeno un punto (D7.1), dominio IDN convertibile, local part ASCII.
- Elementi vuoti scartati; ordine e duplicati preservati (il dedup resta del Sender).

**Test:** `Mario Rossi <m@x.it>` → `"Mario Rossi" <m@x.it>`; `x@café.it` → `<x@xn--caf-dma.it>`; `a@b` → errore (dominio senza punto); `josé@x.it` → errore (local part non ASCII); casi esistenti di `TestValidateRecipients` aggiornati a D7.1.

- [ ] RED → impl → GREEN → Commit `feat(msg): normalize recipients (punycode IDN, dotted domain)`

### Task 2: RCPT con l'indirizzo nudo

**Files:** `internal/mail/smtp/smtp.go` (+test).

- Bug: `Sender.Send` passa le stringhe di `To/Cc/Bcc` così come sono a `c.Rcpt`, quindi `"Mario Rossi" <m@x.it>` produce un `RCPT TO` non valido. Fix: per ogni destinatario `mail.ParseAddress` e `Rcpt(addr.Address)`; dedup sull'indirizzo nudo (case-sensitive come oggi); un indirizzo non parsabile è un errore prima di `MAIL`.
- Header invariati (`buildMIME` scrive le stringhe già normalizzate).

**Test:** server go-smtp in-process: destinatario con nome visualizzato → il backend riceve `m@x.it`; lo stesso indirizzo in To e Bcc con forme diverse → un solo RCPT.

- [ ] RED → impl → GREEN → Commit `fix(smtp): RCPT uses the bare address of display-name recipients`

### Task 3: CLI `send` e composer usano la normalizzazione

**Files:** `internal/cmd/send.go` (+test), `internal/app/model.go` (punto di invio dopo la conferma) (+test).

- CLI: ogni campo con `mail.ParseAddressList` invece di `splitAddrs` (così `"Rossi, Mario" <m@x.it>` resta un indirizzo); poi `msg.NormalizeRecipients`; errore → stderr, exit 1. Il conteggio "sent to N recipients" conta gli indirizzi nudi unici. Test esistenti con `a@b`/`c@d` aggiornati a domini con punto (D7.1).
- Composer: `msg.NormalizeRecipients` sostituisce `ValidateRecipients` prima della conferma; la bozza confermata e inviata è quella normalizzata.

**Test:** CLI con `--to '"Rossi, Mario" <m@x.it>'` → un destinatario; CLI con IDN → Outgoing in punycode; composer con IDN → conferma mostra l'indirizzo normalizzato.

- [ ] RED → impl → GREEN → Commit `feat(cmd,app): send and composer normalize recipients`

### Task 4: `doctor` controlla le capability Gmail

**Files:** `internal/cmd/doctor.go` (+test), `internal/mail/imap` (funzione che si connette e restituisce le capability) (+test), `cmd/gmail-tui/main.go`.

- `Doctor` riceve un probe opzionale `func(ctx) ([]string, error)` (nil = salta). Il main lo costruisce solo se c'è una password (risoluzione env → keychain come la TUI) e un indirizzo in config.
- Output: `imap: ok (MOVE UIDPLUS SPECIAL-USE LIST-EXTENDED)` oppure `imap: MANCA <cap>` (problema bloccante, exit 1) o `imap: ERRORE connessione: …`. Timeout 30s. Password mai stampata.
- Lato imap: connessione, `Caps()`, chiusura; nessun Select.

**Test:** `Doctor` con probe finto (tutte presenti / una mancante / errore / nil); probe reale contro memserver con capability configurate (helper esistente `startTestServerCaps`).

- [ ] RED → impl → GREEN → Commit `feat(doctor): check Gmail IMAP capabilities used by trash and folders`

### Task 5: `#` decide sulla mailbox della riga

**Files:** `internal/app/ops.go` (+test).

- `askTrash` oggi usa `m.folder.Role == "trash"`. Come per `a`/`l` (fix code-review del Piano 7) il controllo deve guardare la mailbox della riga (`opBox()`): "già nel cestino" solo se la riga viene dalla mailbox con ruolo trash. Se il ruolo della mailbox della riga non è noto all'app (es. ricerca `label:` verso il cestino da un'altra cartella), si apre la conferma e decide il backend, che rifiuta già gli id nel cestino.

**Test:** riga del cestino in vista Cestino → status, nessuna conferma; riga di INBOX → conferma.

- [ ] RED → impl → GREEN → Commit `fix(app): trash guard uses the row mailbox`

### Task 6: verifica finale

- [ ] Gate completi + check go-imap + `govulncheck` (x/net diventa diretto)
- [ ] Review indipendente del diff + `/code-review`
- [ ] README (Notes: IDN, `doctor` online), CHANGELOG [Unreleased], `docs/smoke-checklist.md` (doctor con credenziali reali)

## Escluso, con motivo

- **Round IDLE inutile dopo il MOVE di Trash:** costa una SEARCH per cestino. Filtrarlo richiede di distinguere l'EXPUNGE causato da un'op propria dagli EXISTS/EXPUNGE esterni arrivati nello stesso momento, con il rischio di perdere posta nuova. Non vale il rischio; si risolve con la sincronizzazione CONDSTORE del Piano 12.
