# gmail-tui — Piano 12a: ritocchi e misura — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Branch `feat/piano-12a` (da `main` 08441f7). Leggere `AGENTS.md`. TDD vero: per ogni task il test scritto prima va visto rosso (RED riportato nel report), poi verde. Un commit per task, con la riga Co-Authored-By usata nel repo, checkbox `- [ ]`. Gate dopo ogni task: `go build ./...`, `go vet ./...`, `go test -race -count=1 ./...`, `golangci-lint run ./...` a 0 issues.

**Goal:** chiudere cinque ritocchi indipendenti a basso rischio (codifica dei messaggi in uscita, righe fantasma, `Connect` interrompibile, "Importanti" tra le speciali, traccia dei passi IMAP) e dare al Piano 12b la misura che oggi manca.

**Architecture:** nessun cambiamento al contratto `mail.MailStore` (solo un nuovo valore di ruolo, `important`). La traccia guadagna un tipo di evento, `step`, emesso dallo store IMAP attorno a ogni comando del percorso di lettura e riassunto da `gmail-tui trace`. `Connect` apre TCP, TLS e login da solo, con il contesto, invece di `imapclient.DialTLS`.

**Tech Stack:** solo libreria standard (`mime/quotedprintable`, `context.AfterFunc`, `net`, `crypto/tls`). Nessuna dipendenza nuova, nessun CGO, nessuna patch a go-imap.

## Perché questo piano

La traccia reale del 23 novembre (`trace-20261123-132119.jsonl`) dice:

| Operazione | Totale | In coda sul gate | Esecuzione |
|---|---|---|---|
| Connect | 28,9 s | 0 | 28,9 s |
| Watch (baseline) | 15,9 s | 0 | 15,9 s |
| Prima lista | 77,1 s | 15,9 s | 61,2 s |
| Refresh dopo un'op | 23–26 s | 6–8 s | 15–20 s |
| Thread, corpo, lettura | 9–33 s | 4–22 s | 2,6–13 s |

- La rete non è il problema: una sonda senza credenziali dalla stessa macchina misura DNS 3–65 ms, TCP 60–75 ms, TLS 76–86 ms, un comando completo 62–64 ms. I secondi sono di Gmail sull'account.
- La traccia non dice quale comando è lento dentro la lista (SELECT, SEARCH o i riepiloghi dei thread su Tutti i messaggi), né cosa ha fatto durare 29 s il Connect. Senza questi numeri il Piano 12b sarebbe disegnato a occhio: la traccia per passo (Task 4 e 5) viene prima.
- Gli altri quattro ritocchi sono indipendenti tra loro e certi nel codice.

## Decisioni

Decisioni di Andrea (2026-11-23, non riaprirle):

| ID | Decisione | Scelta |
|---|---|---|
| D12-S1 | Suddivisione del Piano 12 | Due piani. **12a** (questo): ritocchi e misura. **12b**: prestazioni, con una spec propria scritta dopo una traccia reale presa con 12a. |
| D12-S2 | Codifica delle parti di testo | `7bit` se il testo è ASCII puro con righe fino a 998 byte (resta com'è oggi), altrimenti `quoted-printable`. Scelta fra tre opzioni (le altre: sempre quoted-printable; base64 quando serve). |

Scelte tecniche del piano. Sono state proposte il 2026-11-23 insieme al design, ma **non hanno avuto una risposta esplicita**: Andrea le conferma o le corregge alla review di questo piano; da lì non si riaprono.

| ID | Scelta | Motivazione |
|---|---|---|
| D12-T1 | `Content-Transfer-Encoding` si dichiara sempre, anche `7bit` (prima era implicito). | Un solo posto che scrive l'intestazione; il sorgente dice come viaggia. Ogni invio cambia di una riga rispetto a oggi. |
| D12-T2 | "Importanti" senza contatore dei non letti, come le altre speciali. | Il README promette conteggi solo per Posta in arrivo e label; Gmail web lo mostra, ma aggiungerlo è un altro lavoro. |
| D12-T3 | La traccia per passo copre solo il percorso di lettura (`tcp`, `tls`, `login`, `list`, `select`, `search`, `fetch`). Il LIST si misura ovunque giri, anche dentro cestino, ripristino, bozze e conteggio dei non letti, perché è sempre lo stesso comando (precisato in esecuzione, 25 novembre). | Le mutazioni (STORE, MOVE, APPEND) non sono il sospetto di 12b. |
| D12-T4 | `imap.Options.TLSConfig` esiste solo per i test (CA del certificato autofirmato); nil in produzione. | Il nuovo dial rifà a mano il TLS di produzione, che oggi non ha test: tutti girano in chiaro. |
| D12-T5 | Il budget di `Connect` resta 90 s (Q16-7). | Decisione già presa in Piano 16; con l'annullamento vero si potrà ridiscuterla con una misura. |
| D12-T6 | Design e decisioni stanno in testa a questo piano, nessuna spec separata. | Come i Piani 8–16. |

Fuori da questo piano, per scelta (non dimenticati):

- **Cestino su più cartelle** (voce del piano dei gap): l'app già gestisce l'errore a metà, una conversazione alla volta, con annulla e "N di M nel cestino" (`internal/app/undo.go`). Resta il caso di una sola chiamata con messaggi di cartelle diverse, che l'app non produce.
- **Righe di intestazione sopra 998 byte** (per esempio molti destinatari in `To:`). Gmail le accetta; limite noto.
- **Contesto per le altre operazioni dello store**: controllano ancora solo `ctx.Err()` in ingresso. È materia di 12b.
- **`UnreadCounts` nella traccia per passo**: i suoi STATUS e SEARCH non si misurano (ha già un suo budget e i suoi test con server lento); il suo LIST sì, come ogni LIST.
- **`AllMailIDs` nella traccia per passo** (SELECT, FETCH, SELECT e SEARCH su Tutti i messaggi): è di sola lettura, ma serve all'annulla dell'archivia, non alla lista.

## Global Constraints

- `MailStore` resta l'unico contratto della UI. Nessun metodo nuovo né cambiato; `mail.Mailbox.Role` guadagna il valore `"important"`.
- Nessuna dipendenza nuova. Nessun CGO. go-imap solo in `internal/mail/imap`.
- **Privacy della traccia (vincolo assoluto, `internal/trace/trace.go`)**: solo nomi di operazioni, passi, durate, conteggi e byte. Mai nomi di cartella o label (solo il ruolo), UID, ID, indirizzi, testi di errore.
- Cache: nessun cambiamento di schema. Gli snapshot non contengono ruoli di cartella.
- Server MCP: nessun nuovo metodo. `list_folders` riporta il ruolo `"important"` così com'è.
- Commenti e stringhe utente in italiano, con riferimento "Piano 12a, Task N". Nei commenti nuovi segui lo stile del file in cui scrivi (molti file scrivono senza accenti: "e", "cosi", "puo"); non riscrivere i commenti esistenti.
- TDD: il test scritto prima va visto rosso, e il RED riportato nel report del task. Nessuna rete, nessuna credenziale reale: server in-process (loopback).
- Le API di go-imap v2 `v2.0.0-beta.9` si verificano nel sorgente in `third_party/go-imap`, mai a memoria.

## File Structure

| File | Ruolo |
|---|---|
| `internal/mail/smtp/encoding.go` (nuovo), `mime.go` | scelta e scrittura di `Content-Transfer-Encoding` delle parti di testo (Task 1) |
| `internal/mail/imap/conversation.go`, `imap.go` | niente righe senza dati dal FETCH (Task 2); passo `fetch`, `search`, `select` (Task 5) |
| `internal/mail/imap/dial.go` (nuovo), `imap.go`, `capabilities.go` | `dialConn` e `dialSession`: TCP, TLS e login con il contesto (Task 3); passi `tcp`, `tls`, `login` (Task 5) |
| `internal/trace/trace.go`, `summary.go`, `tracestore/store.go` | evento `step`, `StepTimer`, tabella "Passi IMAP" (Task 4) |
| `internal/mail/imap/steps.go` (nuovo), `mailboxes.go`, `search.go`, `fetch.go`, `watch.go` | `traceRole`, `selectBox`, `uidSearch` e gli altri punti di misura (Task 5) |
| `internal/mail/imap/mailboxes.go`, `internal/mail/store.go`, `internal/app/folders.go`, `internal/app/demo.go`, `internal/ui/foldertree/tree.go` | ruolo `important` e "Importanti" (Task 6) |
| `CHANGELOG.md`, `README.md`, `docs/smoke-checklist.md`, `AGENTS.md`, `ROADMAP.md`, `docs/stato-dei-lavori.md`, `RIEPILOGO.md`, `docs/superpowers/plans/2026-11-22-piano-sviluppo-gap.md` | documentazione (Task 7) |

Gate di ogni task (nei comandi si scrive solo `gate`):

```sh
go build ./... && go vet ./... && go test -race -count=1 ./... && golangci-lint run ./...
```

---

## Task 0: branch e piano

**Produces:** il branch di lavoro e questo file committato.

- [x] `git checkout -b feat/piano-12a` (da `main`). `ROADMAP.md` è già modificato nell'albero di lavoro: resta lì e si committa con il Task 7.
- [x] `git add docs/superpowers/plans/2026-11-23-piano-12a-ritocchi-e-misura.md && git commit -m "docs: Piano 12a (ritocchi e misura)"`, con la riga Co-Authored-By.

## Task 1: codifica di trasferimento delle parti di testo

**Files:**
- Create: `internal/mail/smtp/encoding.go`
- Modify: `internal/mail/smtp/mime.go` (`corpo`, righe 196–231; `buildMIMEWith`, righe 89–115; commento di `buildMIME`, righe 26–47)
- Test: Create `internal/mail/smtp/encoding_test.go`

**Produces:** ogni parte di testo (`text/plain` e `text/html`) di invio, bozza e `gmail-tui send` dichiara `Content-Transfer-Encoding`: `7bit` se ASCII puro senza NUL e con righe fino a 998 byte, `quoted-printable` altrimenti (D12-S2, D12-T1). Il testo già valido esce com'è.

**Interfaces:**
- Produces (pacchetto `smtp`, non esportate): `needsEncoding(s string) bool`; `textPart(body string) (cte string, data []byte, err error)`; `corpo` ritorna `(ct, cte string, data []byte, err error)` con `cte` vuoto solo per i multipart.

- [x] **Step 1: scrivere i test** in `internal/mail/smtp/encoding_test.go`.

```go
package smtp

import (
	"bytes"
	"io"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 12a, Task 1 (D12-S2): le parti di testo dichiarano sempre come viaggiano.

var fissoCTE = time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)

// decodeQP decodifica un corpo quoted-printable col decoder della libreria standard.
func decodeQP(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(quotedprintable.NewReader(r))
	if err != nil {
		t.Fatalf("decode quoted-printable: %v", err)
	}
	return string(b)
}

// corpoQM legge un messaggio a una sola parte e ne decodifica il corpo.
func corpoQM(t *testing.T, out []byte) string {
	t.Helper()
	m, err := mail.ReadMessage(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	return decodeQP(t, m.Body)
}

// rigaLunga verifica che nessuna riga superi 998 byte (CRLF escluso).
func rigaLunga(t *testing.T, out []byte) {
	t.Helper()
	for i, riga := range strings.Split(string(out), "\n") {
		if len(strings.TrimSuffix(riga, "\r")) > 998 {
			t.Errorf("riga %d di %d byte, oltre il limite RFC 5322", i, len(riga))
		}
	}
}

// TestBuildMIMEAsciiStaysSevenBit: testo ASCII con righe corte → 7bit dichiarato
// e corpo identico al sorgente (con CRLF).
func TestBuildMIMEAsciiStaysSevenBit(t *testing.T) {
	m := msg.Outgoing{From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve", BodyText: "riga1\nriga2\n"}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	want := "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 7bit\r\n\r\nriga1\r\nriga2\r\n"
	if !strings.Contains(string(out), want) {
		t.Errorf("testo ASCII non esce 7bit com'e':\n%s", out)
	}
}

// TestBuildMIMEAccentsGoQuotedPrintable: accenti, spazi in fondo alla riga e un
// segno "=" → quoted-printable; il messaggio e tutto ASCII e decodifica al sorgente.
func TestBuildMIMEAccentsGoQuotedPrintable(t *testing.T) {
	body := "Città  \r\nperché x=y\r\npiù su\r\n"
	m := msg.Outgoing{From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve", BodyText: body}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(out), "Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n") {
		t.Errorf("accenti senza quoted-printable:\n%s", out)
	}
	if i := bytes.IndexByte(out, 0x80); i >= 0 {
		t.Errorf("byte non ASCII in posizione %d: %q", i, out[max(0, i-20):min(len(out), i+20)])
	}
	if got := corpoQM(t, out); got != body {
		t.Errorf("corpo decodificato = %q, voglio %q", got, body)
	}
}

// TestBuildMIMELongLineGoQuotedPrintable: una riga sopra i 998 byte →
// quoted-printable, nessuna riga oltre il limite, decodifica al sorgente.
func TestBuildMIMELongLineGoQuotedPrintable(t *testing.T) {
	body := strings.Repeat("a", 1500) + "\r\nfine\r\n"
	m := msg.Outgoing{From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve", BodyText: body}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	if !strings.Contains(string(out), "Content-Transfer-Encoding: quoted-printable\r\n") {
		t.Errorf("riga lunga senza quoted-printable:\n%s", out)
	}
	rigaLunga(t, out)
	if got := corpoQM(t, out); got != body {
		t.Errorf("corpo decodificato di %d byte, voglio %d byte", len(got), len(body))
	}
}

// TestBuildMIMEMarkdownPartsChooseOwnEncoding: le due parti di alternative
// decidono ognuna per se. Una riga di 996 caratteri resta 7bit nel sorgente, ma
// nell'HTML diventa <p>…</p> (1003 byte): quoted-printable. multipart decodifica
// le parti in trasparenza.
func TestBuildMIMEMarkdownPartsChooseOwnEncoding(t *testing.T) {
	riga := strings.Repeat("a", 996)
	m := msg.Outgoing{
		From: "me@example.com", To: []string{"a@b.com"}, Subject: "Md",
		BodyText: riga + "\n", Markdown: true,
	}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	parts := topLevelParts(t, out, "multipart/alternative")
	if cte := parts[0].header.Get("Content-Transfer-Encoding"); cte != "7bit" {
		t.Errorf("text/plain Content-Transfer-Encoding = %q, voglio 7bit", cte)
	}
	if !bytes.Equal(parts[0].body, []byte(riga+"\r\n")) {
		t.Errorf("text/plain di %d byte, voglio il sorgente con CRLF", len(parts[0].body))
	}
	if cte := parts[1].header.Get("Content-Transfer-Encoding"); cte != "quoted-printable" {
		t.Errorf("text/html Content-Transfer-Encoding = %q, voglio quoted-printable", cte)
	}
	if !strings.Contains(string(parts[1].body), "<p>"+riga+"</p>") {
		t.Errorf("text/html decodificato senza il paragrafo: %q", parts[1].body)
	}
}

// TestBuildMIMEAttachmentTextPartDeclaresEncoding: con gli allegati la parte di
// testo dentro multipart/mixed porta la sua codifica.
func TestBuildMIMEAttachmentTextPartDeclaresEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("allegato"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := msg.Outgoing{
		From: "me@example.com", To: []string{"a@b.com"}, Subject: "Salve",
		BodyText: "città", Attachments: []string{path},
	}
	out, err := buildMIME(m, fissoCTE)
	if err != nil {
		t.Fatalf("buildMIME: %v", err)
	}
	parts := topLevelParts(t, out, "multipart/mixed")
	if cte := parts[0].header.Get("Content-Transfer-Encoding"); cte != "quoted-printable" {
		t.Errorf("parte di testo Content-Transfer-Encoding = %q, voglio quoted-printable", cte)
	}
	if !bytes.Equal(parts[0].body, []byte("città")) {
		t.Errorf("parte di testo = %q", parts[0].body)
	}
}

// TestBuildDraftRoundTripsQuotedPrintable: una bozza con accenti e una riga
// lunga si riapre con lo stesso testo (msg.Parse decodifica la parte).
func TestBuildDraftRoundTripsQuotedPrintable(t *testing.T) {
	lunga := strings.Repeat("b", 1200)
	out := msg.Outgoing{
		From: "me@example.dev", To: []string{"anna@example.dev"}, Subject: "Bozza",
		BodyText: "Città\r\n" + lunga + "\r\n",
	}
	raw, err := BuildDraft(out, fissoCTE)
	if err != nil {
		t.Fatalf("BuildDraft: %v", err)
	}
	if !strings.Contains(string(raw), "Content-Transfer-Encoding: quoted-printable\r\n") {
		t.Errorf("bozza senza quoted-printable:\n%s", raw)
	}
	rigaLunga(t, raw)
	parsed, err := msg.Parse(msg.NewID("[Gmail]/Drafts", 1), bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("msg.Parse: %v", err)
	}
	if !strings.Contains(parsed.BodyText, "Città") || !strings.Contains(parsed.BodyText, lunga) {
		t.Errorf("bozza riaperta senza il testo di prima: %q", parsed.BodyText)
	}
}
```

- [x] **Step 2: vedere rosso.**

Run: `go test ./internal/mail/smtp/ -run 'TestBuildMIME|TestBuildDraft' -count=1`
Expected: FAIL. `TestBuildMIMEAsciiStaysSevenBit` non trova `Content-Transfer-Encoding: 7bit`; `...AccentsGoQuotedPrintable` trova i byte non ASCII; `...LongLineGoQuotedPrintable` trova la riga di 1500 byte; `...MarkdownPartsChooseOwnEncoding` e `...AttachmentTextPartDeclaresEncoding` leggono `Content-Transfer-Encoding` vuoto; `TestBuildDraftRoundTripsQuotedPrintable` non trova l'intestazione. I test già esistenti del pacchetto restano verdi.

- [x] **Step 3: implementare.** Creare `internal/mail/smtp/encoding.go`:

```go
package smtp

// Codifica di trasferimento delle parti di testo (Piano 12a, Task 1, D12-S2).
// Prima text/plain e text/html non dichiaravano Content-Transfer-Encoding, quindi
// valeva il default MIME 7bit: valido solo per ASCII puro con righe fino a 998
// byte (RFC 5322, sezione 2.1.1). Il testo con accenti o con righe lunghe (l'HTML
// che goldmark ricava dal markdown ne ha quasi sempre) usciva fuori regola. Da
// qui ogni parte di testo dichiara come viaggia: 7bit se puo, quoted-printable
// altrimenti.

import (
	"fmt"
	"mime/quotedprintable"
	"strings"
)

// maxRiga7bit e la lunghezza massima di una riga di una parte 7bit, CRLF escluso
// (RFC 5322, sezione 2.1.1).
const maxRiga7bit = 998

// Codifiche dichiarate sulle parti di testo.
const (
	cte7bit = "7bit"
	cteQP   = "quoted-printable"
)

// needsEncoding dice se il testo (gia normalizzato a CRLF) non puo viaggiare
// com'e': contiene un byte sopra 127 o un NUL, oppure una riga sopra maxRiga7bit.
func needsEncoding(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 || s[i] == 0 {
			return true
		}
	}
	for riga := range strings.SplitSeq(s, "\n") {
		if len(strings.TrimSuffix(riga, "\r")) > maxRiga7bit {
			return true
		}
	}
	return false
}

// textPart prepara una parte di testo: il valore di Content-Transfer-Encoding e i
// byte da scrivere. Il testo che puo stare in 7bit esce com'e'; gli altri in
// quoted-printable (righe di 76 caratteri, spazi e tab in fondo alla riga
// codificati), col writer della libreria standard.
func textPart(body string) (cte string, data []byte, err error) {
	if !needsEncoding(body) {
		return cte7bit, []byte(body), nil
	}
	var b strings.Builder
	w := quotedprintable.NewWriter(&b)
	if _, err := w.Write([]byte(body)); err != nil {
		return "", nil, fmt.Errorf("mime: codifica quoted-printable: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", nil, fmt.Errorf("mime: codifica quoted-printable: %w", err)
	}
	return cteQP, []byte(b.String()), nil
}
```

In `internal/mail/smtp/mime.go` sostituire `corpo` (righe 196–231):

```go
// corpo costruisce Content-Type, Content-Transfer-Encoding e bytes della parte
// corpo. Senza Markdown e text/plain con il corpo CRLF cosi com'e'. Con Markdown e
// multipart/alternative: text/plain = markdown sorgente, text/html = documento
// minimo con il rendering GFM (meta charset utf-8, nessuno stile o script esterno).
// Piano 12a, Task 1: ogni parte di testo porta la sua codifica (textPart); cte e
// vuoto solo quando ct e un multipart, che non la porta (RFC 2045, sezione 6.4).
func corpo(m msg.Outgoing, body string) (ct, cte string, data []byte, err error) {
	const plain = "text/plain; charset=utf-8"
	if !m.Markdown {
		cte, data, err = textPart(body)
		if err != nil {
			return "", "", nil, err
		}
		return plain, cte, data, nil
	}
	var rendered bytes.Buffer
	if err := mdRenderer.Convert([]byte(m.BodyText), &rendered); err != nil {
		return "", "", nil, fmt.Errorf("mime: markdown: %w", err)
	}
	page := toCRLF(htmlPage(rendered.String()))

	type parte struct {
		contentType, cte string
		data             []byte
	}
	testo, err := textPart(body)
	if err != nil {
		return "", "", nil, err
	}
	html, err := textPart(page)
	if err != nil {
		return "", "", nil, err
	}

	var alt bytes.Buffer
	aw := multipart.NewWriter(&alt)
	for _, p := range []parte{
		{plain, testo.cte, testo.data},
		{"text/html; charset=utf-8", html.cte, html.data},
	} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", p.contentType)
		h.Set("Content-Transfer-Encoding", p.cte)
		w, err := aw.CreatePart(h)
		if err != nil {
			return "", "", nil, fmt.Errorf("mime: parte %s: %w", p.contentType, err)
		}
		if _, err := w.Write(p.data); err != nil {
			return "", "", nil, fmt.Errorf("mime: parte %s: %w", p.contentType, err)
		}
	}
	if err := aw.Close(); err != nil {
		return "", "", nil, fmt.Errorf("mime: chiusura alternative: %w", err)
	}
	return "multipart/alternative; boundary=" + aw.Boundary(), "", alt.Bytes(), nil
}
```

Sempre in `mime.go`, in `buildMIMEWith` (righe 89–115), `corpo` ritorna quattro valori e l'intestazione si scrive nei due punti in cui si scrive `Content-Type`:

```go
	ct, cte, data, err := corpo(m, body)
	if err != nil {
		return nil, err
	}

	if len(m.Attachments) == 0 {
		writeHeader("Content-Type", ct)
		if cte != "" {
			writeHeader("Content-Transfer-Encoding", cte)
		}
		buf.WriteString("\r\n")
		buf.Write(data)
		return buf.Bytes(), nil
	}
```

```go
	// Prima parte: il corpo, con la sua codifica (Piano 12a, Task 1).
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", ct)
	if cte != "" {
		h.Set("Content-Transfer-Encoding", cte)
	}
```

Nel commento di `buildMIME` (righe 26–47) sostituire il punto "Senza allegati" con: `Senza allegati: Content-Type: text/plain; charset=utf-8, Content-Transfer-Encoding: 7bit o quoted-printable (Piano 12a, Task 1), corpo con LF→CRLF (gli eventuali CRLF esistenti restano tali).`

- [x] **Step 4: aggiungere il test della funzione pura**, in coda a `encoding_test.go`:

```go
// TestNeedsEncoding: la soglia e i casi limite della scelta 7bit / quoted-printable.
func TestNeedsEncoding(t *testing.T) {
	corta := strings.Repeat("a", 998)
	cases := []struct {
		name, body string
		want       bool
	}{
		{"ascii corto", "ciao\r\nmondo\r\n", false},
		{"vuoto", "", false},
		{"accento", "città\r\n", true},
		{"nul", "a\x00b", true},
		{"riga di 998", corta + "\r\n", false},
		{"riga di 999", corta + "a\r\n", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := needsEncoding(c.body); got != c.want {
				t.Errorf("needsEncoding(%q) = %v, voglio %v", c.body, c.want)
			}
		})
	}
}
```

- [x] **Step 5: vedere verde.**

Run: `go test ./internal/mail/smtp/ -count=1 -v`
Expected: PASS, compresi i test già esistenti (le parti lette con `multipart.Reader.NextPart` arrivano già decodificate).

- [x] **Step 6: gate**, poi commit `feat(smtp): codifica di trasferimento delle parti di testo (Piano 12a, Task 1)`.

## Task 2: niente righe senza dati dal FETCH

**Files:**
- Modify: `internal/mail/imap/conversation.go` (`gmailRefs`, righe 301–335), `internal/mail/imap/imap.go` (`fetchRefs`, righe 419–459; `toRef`, righe 475–502)
- Test: Create `internal/mail/imap/ghost_test.go`; Modify `internal/mail/imap/conversation_test.go` (`gmailServerOpts`, righe 82–86, e i due rami `UID FETCH`, righe 124–158)

**Produces:** un messaggio che il FETCH non restituisce (espunto tra SEARCH e FETCH) non diventa più una riga vuota in lista. `toRef` riceve sempre un buffer presente.

**Interfaces:**
- Consumes: `scriptedIMAP` (`gmail_scripted_test.go`), `scriptedGmailServerOpts` (`conversation_test.go`).
- Modifies (solo test): `gmailServerOpts` guadagna `vanished map[string]bool`.

- [x] **Step 1: estendere il server scriptato** in `conversation_test.go`. Sostituire il tipo:

```go
// gmailServerOpts sono le varianti del server scriptato (Piano 16, Task 8).
type gmailServerOpts struct {
	epochs map[string]uint32 // UIDVALIDITY annunciata dalla SELECT per mailbox (0/assente = nessuna)
	trash  bool              // LIST con \Trash e capability MOVE
	// vanished elenca "BOX:uid" che il SEARCH riporta ma il FETCH non restituisce:
	// il messaggio e stato espunto fra i due comandi (Piano 12a, Task 2).
	vanished map[string]bool
}
```

Nei due rami `UID FETCH` di `scriptedGmailServerOpts`, subito dopo `if asked != nil && !asked[m.uid] { continue }` (riga 131 nel ramo `BODY.PEEK[]`, riga 149 nel ramo delle intestazioni), aggiungere:

```go
					if o.vanished[box+":"+fmt.Sprint(m.uid)] {
						continue
					}
```

- [x] **Step 2: scrivere i test** in `internal/mail/imap/ghost_test.go`:

```go
package imap

// Piano 12a, Task 2: un messaggio che il FETCH non restituisce (espunto fra
// SEARCH e FETCH) non diventa una riga vuota.

import (
	"context"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Percorso Gmail: una riga per conversazione.
func TestListConversationsSkipsMessageMissingFromFetch(t *testing.T) {
	inbox := []gmailFetch{
		{uid: 1, thread: 100, login: "ann", date: "01-Nov-2026"},
		{uid: 2, thread: 200, login: "bob", date: "02-Nov-2026"}, // sparisce prima del FETCH
		{uid: 3, thread: 300, login: "cy", date: "03-Nov-2026"},
	}
	boxes := map[string][]gmailFetch{"INBOX": inbox, "[Gmail]/All Mail": inbox}
	addr := scriptedGmailServerOpts(t, boxes, gmailServerOpts{vanished: map[string]bool{"INBOX:2": true}})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	convs, err := s.ListConversations(ctx, msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("conversazioni = %d, voglio 2 (uid 3 e 1): %+v", len(convs), convs)
	}
	for i, want := range []string{"INBOX:3", "INBOX:1"} {
		if c := convs[i]; string(c.Latest.ID) != want || c.Latest.Subject == "" || c.Latest.From == "" {
			t.Errorf("conversazione %d = %+v, voglio %q con oggetto e mittente", i, c.Latest, want)
		}
	}
}

// Percorso senza Gmail (ListMessages e Search).
func TestListMessagesSkipsMessageMissingFromFetch(t *testing.T) {
	addr := scriptedIMAP(t, "IMAP4rev1", func(cmd string) []string {
		switch {
		case strings.HasPrefix(cmd, "SELECT"):
			return []string{"* 3 EXISTS"}
		case strings.HasPrefix(cmd, "UID SEARCH"):
			return []string{"* SEARCH 1 2 3"}
		case strings.HasPrefix(cmd, "UID FETCH"):
			return []string{
				`* 1 FETCH (UID 1 ENVELOPE (NIL "uno" (("Ann" NIL "ann" "x.test")) NIL NIL NIL NIL NIL NIL "<1@x>") FLAGS () INTERNALDATE "01-Nov-2026 10:00:00 +0000")`,
				`* 3 FETCH (UID 3 ENVELOPE (NIL "tre" (("Cy" NIL "cy" "x.test")) NIL NIL NIL NIL NIL NIL "<3@x>") FLAGS () INTERNALDATE "03-Nov-2026 10:00:00 +0000")`,
			}
		}
		return nil
	})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	refs, err := s.ListMessages(ctx, msg.Query{}, mail.Page{})
	if err != nil || len(refs) != 2 {
		t.Fatalf("ListMessages: %v, %d refs, voglio 2", err, len(refs))
	}
	if refs[0].Subject != "tre" || refs[1].Subject != "uno" {
		t.Errorf("oggetti = %q, %q, voglio tre, uno", refs[0].Subject, refs[1].Subject)
	}
}
```

- [x] **Step 3: vedere rosso.**

Run: `go test ./internal/mail/imap/ -run 'SkipsMessageMissingFromFetch' -count=1`
Expected: FAIL. `conversazioni = 3, voglio 2` (la riga dell'uid 2 è senza oggetto né thread) e `ListMessages: <nil>, 3 refs, voglio 2`.

- [x] **Step 4: implementare.** In `internal/mail/imap/imap.go` sostituire il ciclo finale di `fetchRefs` (righe 450–458):

```go
	refs := make([]msg.MessageRef, 0, len(uids))
	for i := len(uids) - 1; i >= 0; i-- { // dal più nuovo al più vecchio
		buf := byUID[uids[i]]
		if buf == nil {
			continue
		}
		ref := toRef(mailbox, uids[i], buf)
		if gm {
			setGmailFields(&ref, buf)
		}
		refs = append(refs, ref)
	}
	return refs, nil
```

Aggiornare il commento di `fetchRefs` (righe 419–421) in:

```go
// fetchRefs è il passo 3-5 condiviso da ListMessages e Search: UIDFetch
// ENVELOPE+FLAGS+INTERNALDATE degli uid (nella mailbox già selezionata) e
// mapping in msg.MessageRef, dal più nuovo al più vecchio. Piano 12a, Task 2: gli
// uid che il FETCH non restituisce (messaggio espunto fra SEARCH e FETCH) si
// saltano, mai una riga senza dati.
```

In `toRef` (righe 475–484) togliere il ramo `if buf == nil { return ref }` e cambiare il commento in:

```go
// toRef mappa un buffer di fetch in msg.MessageRef. buf è sempre presente: gli uid
// senza dati si saltano prima di chiamarla (Piano 12a, Task 2).
```

In `internal/mail/imap/conversation.go` sostituire il ciclo finale di `gmailRefs` (righe 323–333):

```go
	refs := make([]msg.MessageRef, 0, len(uids))
	for i := len(uids) - 1; i >= 0; i-- {
		buf := byUID[uids[i]]
		if buf == nil {
			continue // espunto fra SEARCH e FETCH: nessuna riga senza dati (Piano 12a, Task 2)
		}
		ref := toRef(mailbox, uids[i], buf)
		ref.GmailID = buf.GmailMsgID
		ref.ThreadID = buf.GmailThreadID
		ref.Labels = msg.UserLabels(buf.GmailLabels)
		refs = append(refs, ref)
	}
	return refs, nil
```

- [x] **Step 5: vedere verde.**

Run: `go test ./internal/mail/imap/ -count=1`
Expected: PASS. La scansione per conversazioni chiede il blocco successivo finché la pagina non si riempie: lo fa già oggi, non si tocca.

- [x] **Step 6: gate**, poi commit `fix(imap): niente righe vuote per i messaggi spariti fra SEARCH e FETCH (Piano 12a, Task 2)`.

## Task 3: Connect interrompibile, con il TLS provato

**Files:**
- Create: `internal/mail/imap/dial.go`
- Modify: `internal/mail/imap/imap.go` (`Options`, righe 21–28; `Connect`, righe 235–269), `internal/mail/imap/capabilities.go` (tutto il file)
- Test: Create `internal/mail/imap/dial_test.go`; Modify `internal/mail/imap/gmail_scripted_test.go` (`scriptedIMAP`, righe 15–58)

**Produces:** `Connect` e la sonda di `doctor` e del wizard rispettano il contesto per tutto il dial e il login: alla scadenza la connessione si chiude e l'errore è quello del contesto (D12-T5: il budget resta 90 s). Il dial TLS di produzione ha un test (D12-T4).

**Interfaces:**
- Produces: campo `Options.TLSConfig *tls.Config` (solo test); `dialSession(ctx context.Context, o Options, options *imapclient.Options) (*imapclient.Client, error)`; `dialConn(ctx context.Context, o Options) (net.Conn, error)`; costante `dialTimeout`.
- Il Task 5 aggiunge a `dialSession` un parametro `op string`.

- [x] **Step 1: rendere il server a copione servibile anche su TLS** in `gmail_scripted_test.go`. Sostituire `scriptedIMAP` (righe 15–58) con tre funzioni: il corpo del loop resta identico, cambia solo chi crea il listener.

```go
func scriptedIMAP(t *testing.T, caps string, respond func(cmd string) []string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	serveScripted(ln, caps, respond)
	return ln.Addr().String()
}

// scriptedIMAPOn serve il protocollo scriptato su un listener gia pronto: TLS
// implicito nei test del dial (Piano 12a, Task 3).
func scriptedIMAPOn(ln net.Listener, caps string, respond func(cmd string) []string) {
	serveScripted(ln, caps, respond)
}

// serveScripted accetta UNA connessione su ln: risponde a LOGIN e LOGOUT, e per
// ogni altro comando chiama respond (senza tag) e chiude il comando con OK.
func serveScripted(ln net.Listener, caps string, respond func(cmd string) []string) {
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		fmt.Fprintf(conn, "* OK [CAPABILITY %s] ready\r\n", caps)
		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			tag, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
			verb, _, _ := strings.Cut(rest, " ")
			switch strings.ToUpper(verb) {
			case "LOGIN":
				fmt.Fprintf(conn, "%s OK [CAPABILITY %s] LOGIN completed\r\n", tag, caps)
			case "LOGOUT":
				fmt.Fprintf(conn, "* BYE bye\r\n%s OK LOGOUT completed\r\n", tag)
				return
			default:
				status := "OK done"
				for _, l := range respond(rest) {
					// Una riga "NO motivo" e l'esito taggato NO del comando (Piano 10, review punto 4).
					if reason, ok := strings.CutPrefix(l, "NO "); ok {
						status = "NO " + reason
						continue
					}
					fmt.Fprintf(conn, "%s\r\n", l)
				}
				fmt.Fprintf(conn, "%s %s\r\n", tag, status)
			}
		}
	}()
}
```

Run: `go test ./internal/mail/imap/ -count=1`
Expected: PASS (spostamento puro: nessun comportamento cambia).

- [x] **Step 2: scrivere i test della scadenza** in `internal/mail/imap/dial_test.go` (i test TLS arrivano nello Step 5, nello stesso file).

```go
package imap

// Piano 12a, Task 3: Connect (e la sonda di doctor) rispettano il contesto per
// tutto il dial e il login; il dial TLS implicito e provato davvero.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// silentServer e un listener che accetta e non risponde mai al login (con greeting
// vuoto non manda nemmeno quello). Chiude closed quando il client chiude la
// connessione: e la prova che nulla resta aperto.
func silentServer(t *testing.T, greeting string) (addr string, closed <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if greeting != "" {
			if _, err := fmt.Fprint(conn, greeting); err != nil {
				return
			}
		}
		buf := make([]byte, 512)
		for {
			if _, err := conn.Read(buf); err != nil {
				return // EOF: il client ha chiuso
			}
		}
	}()
	return ln.Addr().String(), done
}

// assertScaduto chiama op con un ctx di 300 ms e verifica che torni con l'errore
// del ctx, presto, e che la connessione sia stata chiusa.
func assertScaduto(t *testing.T, closed <-chan struct{}, op func(ctx context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := op(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, voglio context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("tornato dopo %v: non ha rispettato la scadenza", d)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Error("la connessione e rimasta aperta dopo la scadenza")
	}
}

// TestConnectStopsAtContextDeadline: il ctx scade mentre il greeting non arriva,
// o mentre il LOGIN non ha risposta.
func TestConnectStopsAtContextDeadline(t *testing.T) {
	cases := []struct{ name, greeting string }{
		{"senza greeting", ""},
		{"greeting ma login muto", "* OK ready\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr, closed := silentServer(t, c.greeting)
			s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
			assertScaduto(t, closed, s.Connect)
			if _, err := s.connected(); !errors.Is(err, mail.ErrNotConnected) {
				t.Errorf("dopo una Connect scaduta connected() = %v, voglio ErrNotConnected", err)
			}
		})
	}
}

// Stessa cosa per la sonda di doctor e del wizard.
func TestCapabilitiesStopsAtContextDeadline(t *testing.T) {
	addr, closed := silentServer(t, "* OK ready\r\n")
	assertScaduto(t, closed, func(ctx context.Context) error {
		_, err := Capabilities(ctx, Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
		return err
	})
}

// TestConnectSurvivesCallerContextCancel (test di guardia): il ctx vale fino alla
// fine della chiamata. Una volta connessi, la sua cancellazione NON chiude la
// connessione dello Store. Per vederlo rosso: togliere stop() in dialSession.
func TestConnectSurvivesCallerContextCancel(t *testing.T) {
	ts := startTestServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	s := NewStore(Options{Addr: ts.addr(), Username: "tester", Password: "secret", Insecure: true})
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cancel()
	if _, err := s.ListMessages(context.Background(), msg.Query{}, mail.Page{}); err != nil {
		t.Errorf("lettura dopo la cancellazione del ctx di Connect: %v", err)
	}
}
```

- [x] **Step 3: vedere rosso.**

Run: `go test ./internal/mail/imap/ -run 'TestConnectStopsAtContextDeadline|TestCapabilitiesStopsAtContextDeadline' -count=1`
Expected: FAIL, e il rosso è lento perché è il bug stesso. Prima della correzione (misurato su una copia dell'albero `a9d68d6`):

- «senza greeting»: `Connect` torna dopo 60 s con `err = imap login u: in response: cannot read tag: read tcp …: i/o timeout, voglio context.DeadlineExceeded`;
- «greeting ma login muto»: non torna affatto. Dopo il greeting il ciclo di lettura del client attende senza scadenza (`Client.read` chiama `Decoder.EOF` prima che `readResponse` rimetta i 30 s), quindi questo caso si raccoglie con `-timeout` (per esempio `go test -timeout 20s`) e muore con `panic: test timed out`;
- `TestCapabilitiesStopsAtContextDeadline`: `la connessione e rimasta aperta dopo la scadenza`.

(`TestConnectSurvivesCallerContextCancel` è verde già oggi: è una guardia per il codice nuovo.)

- [x] **Step 4: implementare.** In `internal/mail/imap/imap.go` aggiungere l'import `"crypto/tls"` e, in `Options`, il campo:

```go
	// TLSConfig sostituisce la configurazione TLS di default del dial (Piano 12a,
	// Task 3): nil in produzione; nei test porta la CA del certificato autofirmato.
	// Nome del server e ALPN "imap" si riempiono comunque in dialConn se vuoti.
	TLSConfig *tls.Config
```

Sostituire `Connect` (righe 235–269):

```go
// Connect apre la connessione (TLS, o in chiaro se Options.Insecure) ed effettua
// il login, rispettando ctx per tutto il dial (dialSession, Piano 12a, Task 3): alla
// scadenza la connessione si chiude e l'errore e quello del ctx. Se esiste gia un
// client (doppio Connect, o il path di riconnessione di Watch dopo un errore di
// rete) viene chiuso PRIMA di diallare: cosi un secondo Connect non leaka la
// connessione precedente.
func (s *Store) Connect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		// Gia connesso: chiudi la vecchia sessione (il suo errore non e
		// rilevante, la stiamo sostituendo).
		_ = s.client.Close()
		s.client = nil
	}

	c, err := dialSession(ctx, s.opts, s.dialOptions())
	if err != nil {
		return err
	}
	s.gate.reset()                       // riese dopo Close: il gate torna operativo
	s.gmail = c.Caps().Has(capGmailExt1) // Piano 8, Task 6
	s.client = c
	return nil
}
```

Creare `internal/mail/imap/dial.go`:

```go
package imap

// Apertura della sessione (dial, TLS, login) col contesto rispettato (Piano 12a,
// Task 3). Le funzioni di dial di imapclient (beta.9) non prendono un contesto: una
// Connect che scadeva lasciava girare dial e login dietro le spalle dell'app, fino
// ai timeout di lettura del client, o anche senza fine: dopo il greeting il loop di
// lettura del client aspetta senza scadenza (Client.read chiama Decoder.EOF prima
// che readResponse rimetta i 30 s). Qui TCP e TLS partono col contesto e, alla sua
// scadenza, la connessione si CHIUDE: anche l'attesa del greeting e il LOGIN in
// corso tornano subito. Lo usa anche la sonda di doctor (Capabilities).

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
)

// dialTimeout e il tempo massimo di TCP e dell'handshake TLS: lo stesso default di
// imapclient (30 s). Il ctx del chiamante puo essere piu corto.
const dialTimeout = 30 * time.Second

// dialSession apre la connessione (TLS, o in chiaro se o.Insecure) e fa il login.
// A ctx scaduto o annullato la connessione e chiusa e l'errore e quello del ctx
// (riconoscibile con errors.Is): non resta una connessione aperta dietro.
func dialSession(ctx context.Context, o Options, options *imapclient.Options) (*imapclient.Client, error) {
	conn, err := dialConn(ctx, o)
	if err != nil {
		return nil, err
	}
	// Alla scadenza del ctx la connessione si chiude: i comandi in corso falliscono
	// subito, senza aspettare i timeout di lettura del client.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })

	c := imapclient.New(conn, options)
	err = c.Login(o.Username, o.Password).Wait()
	cerr := ctx.Err() // scaduto durante il login: vale il suo errore, non quello di rete
	if err != nil || cerr != nil {
		_ = c.Close()
		if cerr != nil {
			err = cerr
		}
		return nil, fmt.Errorf("imap login %s: %w", o.Username, err)
	}
	stop() // da qui la connessione vive con lo Store: il ctx del chiamante non la chiude
	return c, nil
}

// dialConn apre TCP e, se non e Insecure, fa l'handshake TLS: nome del server
// ricavato dall'indirizzo e ALPN "imap", come faceva imapclient.DialTLS.
// o.TLSConfig, se dato, parte al posto della configurazione di default (nei test
// porta la CA del certificato autofirmato).
func dialConn(ctx context.Context, o Options) (net.Conn, error) {
	raw, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", o.Addr)
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	if o.Insecure {
		return raw, nil
	}
	host, _, err := net.SplitHostPort(o.Addr)
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	cfg := &tls.Config{}
	if o.TLSConfig != nil {
		cfg = o.TLSConfig.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}
	if cfg.NextProtos == nil {
		cfg.NextProtos = []string{"imap"}
	}
	hsCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	tc := tls.Client(raw, cfg)
	if err := tc.HandshakeContext(hsCtx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	return tc, nil
}
```

Sostituire `Capabilities` e `probeCapabilities` in `internal/mail/imap/capabilities.go` (righe 16–65) con una sola funzione; il file perde l'import `"fmt"`:

```go
// Capabilities apre una connessione con le Options (TLS, o in chiaro se
// Insecure), esegue il login e restituisce le capability annunciate dal server
// in ordine alfabetico. Il ctx limita l'attesa: alla scadenza la connessione si
// chiude (dialSession, Piano 12a, Task 3) e torna l'errore del ctx.
func Capabilities(ctx context.Context, o Options) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := dialSession(ctx, o, &imapclient.Options{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	set := c.Caps()
	if set == nil {
		return nil, errors.New("imap: capability non lette dal server")
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, string(name))
	}
	slices.Sort(out)
	return out, nil
}
```

- [x] **Step 5: aggiungere i test TLS** in `dial_test.go`, aggiungendo agli import `"crypto/ecdsa"`, `"crypto/elliptic"`, `"crypto/rand"`, `"crypto/tls"`, `"crypto/x509"`, `"crypto/x509/pkix"`, `"math/big"`:

```go
// selfSignedCert genera un certificato per 127.0.0.1 (solo stdlib), come nei test
// del package smtp.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// tlsScriptedServer avvia il server a copione dietro TLS implicito e ne restituisce
// l'indirizzo e il certificato servito.
func tlsScriptedServer(t *testing.T) (addr string, cert tls.Certificate) {
	t.Helper()
	cert = selfSignedCert(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	scriptedIMAPOn(tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}}), "IMAP4rev1", func(string) []string { return nil })
	return ln.Addr().String(), cert
}

// TestConnectOverTLS: il dial implicito fa l'handshake e il login, e la connessione
// resta dello Store. Per vederlo rosso: togliere in dialConn la riga che riempie
// cfg.ServerName (il dial fallisce con "either ServerName or InsecureSkipVerify").
func TestConnectOverTLS(t *testing.T) {
	addr, cert := tlsScriptedServer(t)
	pool := x509.NewCertPool()
	pool.AddCert(cert.Leaf)
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", TLSConfig: &tls.Config{RootCAs: pool}})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect su TLS: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.connected(); err != nil {
		t.Errorf("dopo Connect su TLS connected() = %v", err)
	}
}

// TestConnectTLSRejectsUntrustedCertificate: il certificato si verifica davvero;
// senza la CA del test il dial fallisce e non si connette.
func TestConnectTLSRejectsUntrustedCertificate(t *testing.T) {
	addr, _ := tlsScriptedServer(t)
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p"})
	if err := s.Connect(t.Context()); err == nil {
		_ = s.Close()
		t.Fatal("Connect accettato con un certificato non fidato")
	}
	if _, err := s.connected(); !errors.Is(err, mail.ErrNotConnected) {
		t.Errorf("connected() = %v, voglio ErrNotConnected", err)
	}
}
```

Il contratto di `dialSession` dice "scaduto o annullato", ma i test fin qui coprono solo la scadenza durante il greeting e il login. Aggiungere in `dial_test.go`, prima del test di guardia, i due casi che mancano (aggiunti il 24 novembre durante l'esecuzione):

```go
// TestConnectStopsAtContextCancel: una cancellazione esplicita vale quanto la
// scadenza (il contratto di dialSession dice "scaduto o annullato").
func TestConnectStopsAtContextCancel(t *testing.T) {
	addr, closed := silentServer(t, "* OK ready\r\n")
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.AfterFunc(300*time.Millisecond, cancel)
	if err := s.Connect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, voglio context.Canceled", err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Error("la connessione e rimasta aperta dopo la cancellazione")
	}
}

// TestConnectStopsDuringTLSHandshake: il ctx scade mentre l'handshake TLS non
// procede (il server accetta la connessione e non risponde).
func TestConnectStopsDuringTLSHandshake(t *testing.T) {
	addr, closed := silentServer(t, "")
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p"})
	assertScaduto(t, closed, s.Connect)
}
```

- [x] **Step 6: vedere verde e controllare che i dial di imapclient non ci siano più.**

Run: `go test ./internal/mail/imap/ -count=1 -v`
Expected: PASS. I tre casi di scadenza tornano in pochi centesimi di secondo.

Run: `grep -rn "imapclient.Dial" internal cmd --include="*.go"`
Expected: nessuna riga.

- [x] **Step 7: gate**, poi commit `fix(imap): Connect e la sonda di doctor rispettano il contesto, TLS provato (Piano 12a, Task 3)`.

## Task 4: evento `step` e tabella nel riepilogo

**Files:**
- Modify: `internal/trace/trace.go` (struct `Event` e suo commento, righe 32–72; `WithSpan`, righe 254–267), `internal/trace/summary.go` (tipo `group`, riga 70; variabili, riga 91–100; `switch`, riga 128; sezione Avvio, righe 202–217; dopo la tabella delle operazioni, riga 230), `internal/trace/tracestore/store.go` (`begin`, righe 143–146)
- Test: Create `internal/trace/step_test.go`; Modify `internal/trace/summary_test.go`, `internal/mail/imap/gate_trace_test.go` (righe 24 e 48), `internal/trace/trace_test.go` (righe 41 e 78, aggiunte in esecuzione)

**Produces:** un nuovo tipo di evento di traccia, `step` (operazione, passo, ruolo, durata, conteggio, byte, esito), con `StepTimer` per misurarlo; `gmail-tui trace` stampa la tabella "Passi IMAP" e mette i passi del Connect nella cronologia dell'avvio. I file di traccia vecchi funzionano come prima.

**Interfaces:**
- Produces: campo `Event.Step string` (JSON `step`); costanti `trace.StepTCP`, `trace.StepTLS`, `trace.StepLogin`, `trace.StepList`, `trace.StepSelect`, `trace.StepSearch`, `trace.StepFetch`; `trace.WithOp(ctx, op) context.Context`; `trace.OpFrom(ctx) string`; `trace.WithSpan(ctx, op string) (context.Context, *Span)` (firma cambiata); tipo `trace.StepTimer` con `trace.BeginStep(ctx, step) StepTimer` e metodo `(StepTimer) End(role string, n, bytes int, err error)`.

- [x] **Step 1: scrivere i test.** Create `internal/trace/step_test.go`:

```go
package trace

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// ultimoEvento legge l'ultimo evento scritto in buf (il primo e sempre la riga meta).
func ultimoEvento(t *testing.T, buf *bytes.Buffer) Event {
	t.Helper()
	righe := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var e Event
	if err := json.Unmarshal([]byte(righe[len(righe)-1]), &e); err != nil {
		t.Fatalf("evento di traccia non valido: %v (%q)", err, buf.String())
	}
	return e
}

// Piano 12a, Task 4: un passo porta operazione, nome, ruolo e misura; l'operazione
// viene dal ctx (WithSpan per le chiamate allo store, WithOp per il Watch).
func TestStepEventCarriesOperationStepAndRole(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()

	ctx, _ := WithSpan(context.Background(), "ListConversations")
	BeginStep(ctx, StepSelect).End("inbox", 12, 0, nil)
	if e := ultimoEvento(t, &buf); e.K != "step" || e.Op != "ListConversations" || e.Step != "select" || e.Role != "inbox" || e.N != 12 || e.Err {
		t.Errorf("evento step = %+v", e)
	}

	BeginStep(WithOp(context.Background(), "Watch"), StepSearch).End("", 0, 0, context.Canceled)
	if e := ultimoEvento(t, &buf); e.Op != "Watch" || e.Step != "search" || !e.Err {
		t.Errorf("passo del Watch = %+v", e)
	}
}

// Senza traccia installata nessun costo: WithSpan non tocca il ctx e il timer e vuoto.
func TestStepWithoutTraceIsNoOp(t *testing.T) {
	Uninstall()
	ctx, sp := WithSpan(context.Background(), "ListConversations")
	if sp != nil || OpFrom(ctx) != "" {
		t.Fatal("senza traccia WithSpan non deve toccare il ctx")
	}
	BeginStep(ctx, StepFetch).End("inbox", 1, 10, nil)
}

// WithSpan porta nel ctx anche il nome dell'operazione, oltre allo Span del gate.
func TestWithSpanNamesTheOperation(t *testing.T) {
	var buf bytes.Buffer
	Install(New(&buf, ""))
	defer Uninstall()
	ctx, sp := WithSpan(context.Background(), "FetchMessage")
	if sp == nil || OpFrom(ctx) != "FetchMessage" {
		t.Fatalf("WithSpan: span=%v op=%q", sp, OpFrom(ctx))
	}
}
```

In `internal/trace/summary_test.go` aggiungere in fondo:

```go
// righeCon ritorna le righe del riepilogo che iniziano con prefisso.
func righeCon(s, prefisso string) []string {
	var out []string
	for riga := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(riga, prefisso) {
			out = append(out, riga)
		}
	}
	return out
}

// Piano 12a, Task 4: la tabella dei passi IMAP, una riga per operazione, passo e
// ruolo; i passi del Connect finiscono nella cronologia dell'avvio.
func TestSummarizeSezionePassi(t *testing.T) {
	in := jsonl(
		Event{K: "meta", Start: "2026-11-23T10:00:00Z", OS: "darwin", Arch: "arm64", Version: "dev"},
		Event{T: 100_000, K: "step", Op: "Connect", Step: "tcp", Dur: 100_000},
		Event{T: 250_000, K: "step", Op: "Connect", Step: "tls", Dur: 150_000},
		Event{T: 1_250_000, K: "step", Op: "Connect", Step: "login", Dur: 1_000_000},
		Event{T: 1_300_000, K: "store", Op: "Connect", Dur: 1_300_000},
		Event{T: 1_600_000, K: "step", Op: "ListConversations", Step: "select", Role: "inbox", Dur: 900_000},
		Event{T: 2_100_000, K: "step", Op: "ListConversations", Step: "search", Role: "inbox", Dur: 500_000},
		Event{T: 2_900_000, K: "step", Op: "ListConversations", Step: "select", Role: "all", Dur: 800_000},
	)
	var out strings.Builder
	if err := Summarize(strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "Passi IMAP") {
		t.Errorf("riepilogo senza la tabella dei passi:\n%s", s)
	}
	// Una riga per operazione, passo e ruolo: due "select" (inbox e all).
	if got := righeCon(s, "ListConversations select"); len(got) != 2 {
		t.Errorf("righe di \"ListConversations select\" = %d, attese 2:\n%s", len(got), s)
	}
	if got := righeCon(s, "ListConversations search"); len(got) != 1 || !strings.Contains(got[0], "inbox") || !strings.Contains(got[0], "500.0") {
		t.Errorf("riga di search attesa con inbox e 500.0:\n%s", s)
	}
	// Cronologia dell'avvio: i passi del Connect, in ordine di tempo.
	i, j, k := strings.Index(s, "Connect tcp"), strings.Index(s, "Connect tls"), strings.Index(s, "Connect login")
	if i < 0 || j < 0 || k < 0 || i >= j || j >= k {
		t.Errorf("i passi del Connect non sono in ordine nell'avvio:\n%s", s)
	}
}
```

E in `TestSummarizeRiepilogo` (stesso file), prima della chiusura della funzione, aggiungere:

```go
	// Un file senza passi non ha la tabella dei passi (Piano 12a, Task 4).
	if strings.Contains(s, "Passi IMAP") {
		t.Errorf("file senza eventi step con la tabella dei passi:\n%s", s)
	}
```

- [x] **Step 2: vedere rosso.**

Run: `go test ./internal/trace/ -count=1`
Expected: FAIL per errore di compilazione (`undefined: StepSelect`, `not enough arguments in call to WithSpan`, `undefined: BeginStep` in questo pacchetto). Tolto quello, `TestSummarizeSezionePassi` fallisce con `riepilogo senza la tabella dei passi`.

- [x] **Step 3: implementare in `internal/trace/trace.go`.** Nella struct `Event`, dopo `Op`:

```go
	Step      string `json:"step,omitempty"`
```

Nel commento di `Event`, dopo il punto di `cache`, aggiungere:

```go
//   - step: Piano 12a, Task 4. Op = operazione di appartenenza, Step = passo (una
//     delle costanti Step*), Dur, N = messaggi o UID del comando, Bytes = byte dei
//     corpi scaricati, Role = ruolo della cartella, Err.
```

Dopo le costanti delle operazioni della cache (dopo `ModeAge`), aggiungere:

```go
// Passi misurati dalla traccia (Piano 12a, Task 4): campo Step dell'evento
// K == "step", Op = operazione di appartenenza. Solo nomi fissi: mai mailbox, UID
// o nomi di label (il ruolo della cartella va in Role).
const (
	StepTCP    = "tcp"    // Connect: DNS e TCP
	StepTLS    = "tls"    // Connect: handshake TLS
	StepLogin  = "login"  // Connect: attesa del greeting e LOGIN
	StepList   = "list"   // LIST delle cartelle
	StepSelect = "select" // SELECT di una mailbox
	StepSearch = "search" // UID SEARCH
	StepFetch  = "fetch"  // UID FETCH (intestazioni o corpi)
)
```

Sostituire `WithSpan` (righe 254–267, cioè `WithSpan` e `SpanFrom`) con:

```go
// opKey e la chiave del ctx col nome dell'operazione in corso.
type opKey struct{}

// WithOp mette il nome dell'operazione nel ctx: i passi (BeginStep) lo riportano
// in Op. Serve a chi misura comandi fuori da una chiamata passata dal wrapper
// della traccia (il Watch, l'apertura della sessione). Senza traccia e il ctx stesso.
func WithOp(ctx context.Context, op string) context.Context {
	if !Enabled() {
		return ctx
	}
	return context.WithValue(ctx, opKey{}, op)
}

// OpFrom e il nome dell'operazione in corso, dal ctx ("" se assente).
func OpFrom(ctx context.Context) string {
	op, _ := ctx.Value(opKey{}).(string)
	return op
}

// WithSpan crea uno Span nel ctx e vi associa il nome dell'operazione, che i passi
// leggono con OpFrom; con la traccia spenta ritorna (ctx, nil).
func WithSpan(ctx context.Context, op string) (context.Context, *Span) {
	if !Enabled() {
		return ctx, nil
	}
	sp := &Span{}
	return context.WithValue(WithOp(ctx, op), spanKey{}, sp), sp
}

// SpanFrom recupera lo Span del ctx (nil se assente).
func SpanFrom(ctx context.Context) *Span {
	sp, _ := ctx.Value(spanKey{}).(*Span)
	return sp
}
```

E in coda al file:

```go
// StepTimer misura un passo di un'operazione (un comando IMAP): BeginStep lo apre,
// End emette l'evento "step". Senza traccia e un valore zero e End non fa nulla.
type StepTimer struct {
	t0 time.Time
	ev Event
}

// BeginStep apre la misura del passo step dell'operazione in corso (nome dal ctx).
func BeginStep(ctx context.Context, step string) StepTimer {
	if !Enabled() {
		return StepTimer{}
	}
	return StepTimer{t0: time.Now(), ev: Event{K: "step", Op: OpFrom(ctx), Step: step}}
}

// End chiude il passo: role e il ruolo della cartella ("" se non si applica), n il
// conteggio (messaggi o UID), bytes quelli scaricati, err l'esito (solo il booleano).
func (t StepTimer) End(role string, n, bytes int, err error) {
	if t.t0.IsZero() {
		return
	}
	t.ev.Role, t.ev.N, t.ev.Bytes = role, n, bytes
	t.ev.Err = err != nil
	t.ev.Dur = time.Since(t.t0).Microseconds()
	Emit(t.ev)
}
```

In `internal/trace/tracestore/store.go`, in `begin`:

```go
	ctx, sp := trace.WithSpan(ctx, op)
```

In `internal/mail/imap/gate_trace_test.go`, righe 24 e 48, e in `internal/trace/trace_test.go`, righe 41 e 78 (queste ultime aggiunte in esecuzione), la chiamata diventa `trace.WithSpan(context.Background(), "ListConversations")`.

- [x] **Step 4: implementare in `internal/trace/summary.go`.** Dopo il tipo `group`:

```go
// stepGroup raccoglie le misure di un passo (operazione, passo e ruolo uguali).
type stepGroup struct {
	label, role string
	dur         []int64
}
```

Nelle variabili di `Summarize`, accanto a `store` e `storeOrder`:

```go
		steps     = map[string]*stepGroup{}
		stepOrder []string
		connect   []Event // passi del Connect, per la cronologia dell'avvio
```

Nel `switch e.K`, un caso nuovo prima di `case "msg":`:

```go
		case "step":
			key := e.Op + "\x00" + e.Step + "\x00" + e.Role
			g, ok := steps[key]
			if !ok {
				g = &stepGroup{label: e.Op + " " + e.Step, role: e.Role}
				steps[key] = g
				stepOrder = append(stepOrder, key)
			}
			g.dur = append(g.dur, e.Dur)
			if e.Op == "Connect" {
				connect = append(connect, e)
			}
```

Nella sezione Avvio, dopo `start = append(start, phases...)`:

```go
	for _, e := range connect {
		e.Op = "Connect " + e.Step + " (passo)"
		start = append(start, e)
	}
```

Subito dopo il blocco `if len(storeOrder) > 0 { ... }` della tabella delle operazioni, prima di `if len(msgOrder) > 0`:

```go
	if len(stepOrder) > 0 {
		fmt.Fprintln(w, "\nPassi IMAP (ms; un comando per passo, per ruolo della cartella)")
		fmt.Fprintln(tw, "operazione passo\truolo\tn\tmediana\tp95\tmax")
		for _, k := range stepOrder {
			g := steps[k]
			fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%s\n", g.label, g.role, len(g.dur),
				ms(median(g.dur)), ms(percentile(g.dur, 95)), ms(maxOf(g.dur)))
		}
		_ = tw.Flush()
	}
```

- [x] **Step 5: vedere verde.**

Run: `go test ./internal/trace/... ./internal/mail/imap/ -count=1`
Expected: PASS.

- [x] **Step 6: gate**, poi commit `feat(trace): evento step per i passi IMAP e tabella nel riepilogo (Piano 12a, Task 4)`.

## Task 5: i passi misurati nello store IMAP

**Files:**
- Create: `internal/mail/imap/steps.go`, `internal/mail/imap/steps_test.go`
- Modify: `internal/mail/imap/imap.go` (campo `roles` nello `Store`; `Connect`; `listMessages`; `selectAndSearch`; `fetchRefs`), `internal/mail/imap/dial.go`, `capabilities.go`, `conversation.go`, `search.go`, `fetch.go`, `mailboxes.go`, `trash.go`, `unseen.go`, `watch.go`; `restore.go` e `draft.go` (aggiunti in esecuzione)
- Test: Modify `internal/mail/imap/watch_refs_test.go` (righe 31 e 46)

**Produces:** ogni comando del percorso di lettura emette un evento `step` con il nome dell'operazione e il ruolo della cartella, mai il suo nome (D12-T3). Senza traccia installata nessun costo.

**Interfaces:**
- Consumes: `trace.BeginStep`, `trace.WithOp`, le costanti `trace.Step*` (Task 4).
- Produces: campo `Store.roles map[string]string`; metodi `(s *Store) noteRoles(boxes []mail.Mailbox)`, `traceRole(mailbox string) string`, `selectBox(ctx, c, mailbox) (*imap.SelectData, error)`, `uidSearch(ctx, c, mailbox, crit, opts) (*imap.SearchData, error)`, `gmailRefs(ctx, c, mailbox, uids) ([]msg.MessageRef, error)`, `fetchRefs(ctx, mailbox, uids) ([]msg.MessageRef, error)`, `fetchRaw(ctx, mailbox, uid) (...)`, `threadSummaries(ctx, c, allMail, threads)`, `baselineState(ctx, c)`, `recheckNewMail(ctx, c, ch, st)`, `highestUID(ctx, c, mailbox)`; funzioni `listMailboxes(ctx, c)`, `listData(ctx, c, status)`, `draftsBoxOf(ctx, c)`; `dialSession(ctx, o, op, options)`.

- [x] **Step 1: scrivere i test** in `internal/mail/imap/steps_test.go`:

```go
package imap

// Piano 12a, Task 5: i comandi del percorso di lettura lasciano un evento step
// per comando, col ruolo della cartella e mai col suo nome.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
	"github.com/andsigno82/gmail-tui/internal/trace/tracestore"
)

// syncBuf e un buffer di traccia leggibile mentre altre goroutine (il Watch)
// potrebbero ancora scrivere: senza il mutex il race detector segnalerebbe la lettura.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// stepTrail ritorna "operazione passo ruolo" per ogni evento step della traccia.
func stepTrail(t *testing.T, buf *syncBuf) []string {
	t.Helper()
	var out []string
	for riga := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(riga), &e); err != nil {
			t.Fatalf("riga di traccia non valida: %v", err)
		}
		if e.K == "step" {
			out = append(out, strings.TrimSpace(e.Op+" "+e.Step+" "+e.Role))
		}
	}
	return out
}

// traceRole non guarda mai il nome della cartella.
func TestTraceRoleNeverUsesMailboxName(t *testing.T) {
	s := NewStore(Options{})
	s.noteRoles([]mail.Mailbox{
		{Name: "INBOX", Role: "inbox"},
		{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
		{Name: "Lavoro"},
	})
	cases := map[string]string{
		"": "inbox", "INBOX": "inbox",
		"[Gmail]/Tutti i messaggi": "all",
		"Lavoro":                   "label",
		"MaiVista":                 "label",
	}
	for mailbox, want := range cases {
		if got := s.traceRole(mailbox); got != want {
			t.Errorf("traceRole(%q) = %q, voglio %q", mailbox, got, want)
		}
	}
}

// Una lista Gmail, comando per comando: LIST per trovare Tutti i messaggi, poi
// SELECT, SEARCH e FETCH in INBOX e SELECT, SEARCH e FETCH su Tutti i messaggi.
func TestListConversationsEmitsOneStepPerCommand(t *testing.T) {
	var buf syncBuf
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	inbox := []gmailFetch{
		{uid: 1, thread: 100, login: "ann", date: "01-Nov-2026"},
		{uid: 2, thread: 200, login: "bob", date: "02-Nov-2026"},
	}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": inbox, "[Gmail]/All Mail": inbox})
	inner := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	st := tracestore.Wrap(inner)
	ctx := context.Background()
	if err := st.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = inner.Close() }()
	if _, err := st.ListConversations(ctx, msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListConversations: %v", err)
	}

	want := []string{
		"Connect tcp", "Connect login",
		"ListConversations list",
		"ListConversations select inbox",
		"ListConversations search inbox",
		"ListConversations fetch inbox",
		"ListConversations select all",
		"ListConversations search all",
		"ListConversations fetch all",
	}
	if got := stepTrail(t, &buf); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("passi = %v, voglio %v", got, want)
	}
	for _, nome := range []string{"INBOX", "All Mail"} {
		if strings.Contains(buf.String(), nome) {
			t.Errorf("la traccia contiene il nome di una cartella: %q", nome)
		}
	}
}

// La base del Watch (SELECT e SEARCH di INBOX) si misura come passo del Watch.
// Si leggono solo i primi due: il round di IDLE che segue rifà la SELECT.
func TestWatchBaselineEmitsSteps(t *testing.T) {
	var buf syncBuf
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	ts := startTestServer(t)
	s := connectStore(t, ts)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	if _, err := s.Watch(ctx); err != nil {
		t.Fatalf("Watch: %v", err)
	}
	got := stepTrail(t, &buf)
	var delWatch []string // solo i passi del Watch: prima ci sono quelli del Connect di connectStore
	for _, passo := range got {
		if strings.HasPrefix(passo, "Watch ") {
			delWatch = append(delWatch, passo)
		}
	}
	if len(delWatch) < 2 || delWatch[0] != "Watch select inbox" || delWatch[1] != "Watch search inbox" {
		t.Errorf("passi = %v, voglio che i passi del Watch inizino con Watch select inbox, Watch search inbox", got)
	}
}
```

- [x] **Step 2: vedere rosso.**

Run: `go test ./internal/mail/imap/ -run 'TestTraceRole|EmitsOneStepPerCommand|TestWatchBaselineEmitsSteps' -count=1`
Expected: FAIL per errore di compilazione (`s.noteRoles undefined`, `s.traceRole undefined`). Con il solo `steps.go` (prima parte dello Step 3) compilano, ma i passi sono ancora zero: `passi = [], voglio [Connect tcp Connect login ...]` e `passi = [], voglio che inizino con Watch select inbox, Watch search inbox`.

- [x] **Step 3: creare `internal/mail/imap/steps.go`.**

```go
package imap

// Misura dei passi IMAP (Piano 12a, Task 5, D12-T3): ogni comando del percorso di
// LETTURA emette un evento "step" della traccia. Il nome dell'operazione viene dal
// ctx: lo mette il wrapper della traccia (trace.WithSpan), oppure trace.WithOp per
// i comandi del Watch e per l'apertura della sessione. Il ruolo della cartella
// arriva dall'ultimo LIST: mai il nome. Le mutazioni (STORE, MOVE, APPEND) non si
// misurano. Senza traccia installata ogni funzione qui e una chiamata vuota.

import (
	"context"
	"fmt"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// noteRoles ricorda il ruolo di ogni mailbox dell'ultimo LIST (Piano 12a, Task 5).
func (s *Store) noteRoles(boxes []mail.Mailbox) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.roles == nil {
		s.roles = make(map[string]string, len(boxes))
	}
	for _, b := range boxes {
		s.roles[b.Name] = b.Role
	}
}

// traceRole e il ruolo da mettere nella traccia per una mailbox: "inbox" per INBOX,
// il ruolo dell'ultimo LIST se noto, altrimenti "label". Mai il nome.
func (s *Store) traceRole(mailbox string) string {
	if mailbox == "" || strings.EqualFold(mailbox, "INBOX") {
		return "inbox"
	}
	s.mu.Lock()
	role := s.roles[mailbox]
	s.mu.Unlock()
	if role == "" {
		return "label"
	}
	return role
}

// selectBox e la SELECT del percorso di lettura: misura il passo "select" (N = i
// messaggi della mailbox) e registra l'epoca (Piano 16, R3), come facevano i
// chiamanti. Gate gia detenuto.
func (s *Store) selectBox(ctx context.Context, c *imapclient.Client, mailbox string) (*imap.SelectData, error) {
	step := trace.BeginStep(ctx, trace.StepSelect)
	sel, err := c.Select(mailbox, nil).Wait()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, fmt.Errorf("imap select %s: %w", mailbox, err)
	}
	s.noteEpoch(mailbox, sel.UIDValidity)
	step.End(s.traceRole(mailbox), int(sel.NumMessages), 0, nil)
	return sel, nil
}

// uidSearch e la UID SEARCH del percorso di lettura: misura il passo "search"
// (N = gli UID restituiti). L'errore arriva nudo: il testo lo aggiunge il chiamante.
// Gate gia detenuto.
func (s *Store) uidSearch(ctx context.Context, c *imapclient.Client, mailbox string, crit *imap.SearchCriteria, opts *imap.SearchOptions) (*imap.SearchData, error) {
	step := trace.BeginStep(ctx, trace.StepSearch)
	data, err := c.UIDSearch(crit, opts).Wait()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, err
	}
	step.End(s.traceRole(mailbox), len(data.AllUIDs()), 0, nil)
	return data, nil
}
```

- [x] **Step 4: cablare i chiamanti.** Ogni punto sotto mostra il codice nuovo; il resto delle funzioni non cambia.

**`imap.go`.** Nello struct `Store`, dopo il campo `clock`:

```go
	// roles (Piano 12a, Task 5) mappa nome di mailbox -> ruolo dell'ultimo LIST: serve
	// alla traccia dei passi (traceRole). Protetta da mu.
	roles map[string]string
```

In `Connect`: `c, err := dialSession(ctx, s.opts, "Connect", s.dialOptions())`.

In `listMessages`: le due chiamate `s.fetchRefs(mailbox, uids)` e `s.fetchRefs(mailbox, uids[start:end])` diventano `s.fetchRefs(ctx, mailbox, uids)` e `s.fetchRefs(ctx, mailbox, uids[start:end])`.

In `selectAndSearch`, le righe dalla `c.Select(mailbox, nil)` al `return` diventano:

```go
	sel, err := s.selectBox(ctx, c, mailbox)
	if err != nil {
		return "", nil, 0, err
	}

	data, err := s.uidSearch(ctx, c, mailbox, searchCriteria(q), nil)
	if err != nil {
		return "", nil, 0, fmt.Errorf("imap uid search in %s: %w", mailbox, err)
	}
	return mailbox, data.AllUIDs(), sel.UIDValidity, nil // UID ascendenti
```

(Il commento "Piano 16, Task 8, R3: l'epoca si registra prima di ogni riga" resta sulla chiamata di `selectBox`.) Aggiornare il commento di `selectAndSearch`: `...e la UIDVALIDITY della SELECT (registrata da selectBox: Piano 16, Task 8, R3).`

`fetchRefs` prende `ctx` e misura il passo:

```go
func (s *Store) fetchRefs(ctx context.Context, mailbox string, uids []imap.UID) ([]msg.MessageRef, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	c, err := s.connected()
	if err != nil {
		return nil, err
	}

	gm := s.gmailOn() // Piano 8, Task 8: X-GM-* solo se il server li annuncia
	cmd := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID:           true,
		Envelope:      true,
		Flags:         true,
		InternalDate:  true,
		GmailMsgID:    gm,
		GmailThreadID: gm,
		GmailLabels:   gm,
	})
	step := trace.BeginStep(ctx, trace.StepFetch)
	bufs, err := cmd.Collect()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, fmt.Errorf("imap fetch envelope in %s: %w", mailbox, err)
	}
	step.End(s.traceRole(mailbox), len(bufs), 0, nil)
```

(seguito invariato: `byUID` e il ciclo dal più nuovo al più vecchio).

**`search.go`.** In `Search`: `refs, err := s.fetchRefs(ctx, mailbox, uids)`.

**`fetch.go`.** Le due chiamate diventano `s.fetchRaw(ctx, mailbox, imap.UID(uid))` (in `fetchMessage` e in `SaveAttachment`); `fetchRaw` prende `ctx`:

```go
func (s *Store) fetchRaw(ctx context.Context, mailbox string, uid imap.UID) (raw []byte, flags []imap.Flag, uidValidity uint32, gx gmailFields, err error) {
	c, err := s.connected()
	if err != nil {
		return nil, nil, 0, gmailFields{}, err
	}
	sel, err := s.selectBox(ctx, c, mailbox)
	if err != nil {
		return nil, nil, 0, gmailFields{}, err
	}

	section := &imap.FetchItemBodySection{Peek: true} // BODY.PEEK[] = messaggio intero, senza \Seen
	gm := s.gmailOn()                                 // X-GM-* del messaggio solo se il server li annuncia (Piano 8)
	cmd := c.Fetch(imap.UIDSetNum(uid), &imap.FetchOptions{
		UID:           true,
		Flags:         true,
		BodySection:   []*imap.FetchItemBodySection{section},
		GmailMsgID:    gm,
		GmailThreadID: gm,
		GmailLabels:   gm,
	})
	step := trace.BeginStep(ctx, trace.StepFetch)
	bufs, err := cmd.Collect()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, nil, 0, gmailFields{}, fmt.Errorf("imap fetch body in %s: %w", mailbox, err)
	}
	var buf *imapclient.FetchMessageBuffer
	for _, b := range bufs {
		if b.UID == uid {
			buf = b
			break
		}
	}
	if buf == nil {
		err = fmt.Errorf("%w: uid %d in %s", ErrMessageNotFound, uid, mailbox)
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, nil, 0, gmailFields{}, err
	}
	raw = buf.FindBodySection(section)
	if raw == nil {
		err = fmt.Errorf("%w: uid %d in %s, corpo assente nella risposta", ErrMessageNotFound, uid, mailbox)
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, nil, 0, gmailFields{}, err
	}
	step.End(s.traceRole(mailbox), 1, len(raw), nil)
	gx = gmailFields{msgID: buf.GmailMsgID, threadID: buf.GmailThreadID, labels: buf.GmailLabels}
	return raw, buf.Flags, sel.UIDValidity, gx, nil
}
```

**`conversation.go`** (aggiungere l'import `"github.com/andsigno82/gmail-tui/internal/trace"`). In `ListConversations`, ramo Gmail, le righe dalla `c.Select(mailbox, nil).Wait()` alla fine di `sort.Slice(uids, ...)` diventano:

```go
	sel, err := s.selectBox(ctx, c, mailbox)
	if err != nil {
		return nil, err
	}
	// Piano 16, Task 8, R3: l'epoca si registra PRIMA di costruire le righe (selectBox).
	data, err := s.uidSearch(ctx, c, mailbox, crit, nil)
	if err != nil {
		return nil, fmt.Errorf("imap uid search in %s: %w", mailbox, err)
	}
```

Nella scansione: `refs, err := s.gmailRefs(ctx, c, mailbox, chunk)`; poi `sums, err := s.threadSummaries(ctx, c, allMail, threads)`.

In `ThreadMessages`:

```go
	if _, err := s.selectBox(ctx, c, allMail); err != nil {
		return nil, err
	}
	data, err := s.uidSearch(ctx, c, allMail, &imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{ThreadID: threadID}}, nil)
	if err != nil {
		return nil, fmt.Errorf("imap thread search: %w", err)
	}
	refs, err := s.gmailRefs(ctx, c, allMail, data.AllUIDs())
```

(Il commento "Piano 16, Task 8, R3" e quello "Piano 16, Task 13, R3: ogni SELECT che produce ID" restano sulle chiamate di `selectBox`.)

In `ConversationMembers`:

```go
	if _, err := s.selectBox(ctx, c, mailbox); err != nil {
		return nil, err
	}
	data, err := s.uidSearch(ctx, c, mailbox, &imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{ThreadID: threadID}}, nil)
	if err != nil {
		return nil, fmt.Errorf("imap thread members search: %w", err)
	}
```

`gmailRefs` diventa un metodo e misura il passo (la firma e l'inizio cambiano; il corpo dopo `byUID` resta quello del Task 2):

```go
// gmailRefs scarica ENVELOPE, FLAGS, INTERNALDATE e X-GM-* per gli UID della
// mailbox già selezionata, dal più recente al più vecchio. Gate già detenuto.
// Piano 12a, Task 5: misura il passo "fetch".
func (s *Store) gmailRefs(ctx context.Context, c *imapclient.Client, mailbox string, uids []imap.UID) ([]msg.MessageRef, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	step := trace.BeginStep(ctx, trace.StepFetch)
	bufs, err := c.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{
		UID:           true,
		Envelope:      true,
		Flags:         true,
		InternalDate:  true,
		GmailMsgID:    true,
		GmailThreadID: true,
		GmailLabels:   true,
	}).Collect()
	if err != nil {
		step.End(s.traceRole(mailbox), 0, 0, err)
		return nil, fmt.Errorf("imap fetch gmail in %s: %w", mailbox, err)
	}
	step.End(s.traceRole(mailbox), len(bufs), 0, nil)
```

`threadSummaries` prende `ctx`; le righe dalla `c.Select(allMail, nil).Wait()` al `return out, nil` diventano:

```go
func (s *Store) threadSummaries(ctx context.Context, c *imapclient.Client, allMail string, threads []uint64) (map[uint64]threadSummary, error) {
	out := make(map[uint64]threadSummary, len(threads))
	if len(threads) == 0 {
		return out, nil
	}
	if _, err := s.selectBox(ctx, c, allMail); err != nil {
		return nil, err
	}

	groups := make(map[uint64][]msg.MessageRef)
	for start := 0; start < len(threads); start += threadPerSearch {
		end := start + threadPerSearch
		if end > len(threads) {
			end = len(threads)
		}
		group := threads[start:end]
		data, err := s.uidSearch(ctx, c, allMail, threadCriteria(group), nil)
		if err != nil {
			return nil, fmt.Errorf("imap thread summary search: %w", err)
		}
		refs, err := s.gmailRefs(ctx, c, allMail, data.AllUIDs())
		if err != nil {
			return nil, err
		}
		for _, r := range refs {
			groups[r.ThreadID] = append(groups[r.ThreadID], r)
		}
	}
	for id, refs := range groups {
		out[id] = summarizeThread(refs)
	}
	return out, nil
}
```

(Il commento "Piano 16, Task 8, R3: anche i thread salvati su Tutti i messaggi" resta sulla chiamata di `selectBox`; il corpo di aggregazione non cambia.) Aggiornare il commento della funzione: `Gate detenuto. ctx per la misura dei passi (Piano 12a, Task 5).`

In `searchRaw`:

```go
	if _, err := s.selectBox(ctx, c, mailbox); err != nil {
		return nil, err
	}
	data, err := s.uidSearch(ctx, c, mailbox, &imap.SearchCriteria{Gmail: &imap.SearchCriteriaGmail{Raw: q.Raw}}, nil)
	if err != nil {
		return nil, fmt.Errorf("imap raw search in %s: %w", mailbox, err)
	}
	return s.gmailRefs(ctx, c, mailbox, data.AllUIDs())
```

**`mailboxes.go`.** `ListMailboxes` diventa:

```go
func (s *Store) ListMailboxes(ctx context.Context) ([]mail.Mailbox, error) {
	defer s.acquire(ctx)()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.connected()
	if err != nil {
		return nil, err
	}
	boxes, err := listMailboxes(ctx, c)
	if err != nil {
		return nil, err
	}
	s.noteRoles(boxes) // Piano 12a, Task 5: i ruoli servono alla traccia dei passi
	return boxes, nil
}
```

`listMailboxes` prende `ctx` e chiama `listData(ctx, c, nil)`; `listData` prende `ctx` e misura il passo:

```go
func listData(ctx context.Context, c *imapclient.Client, status *imap.StatusOptions) ([]*imap.ListData, error) {
	step := trace.BeginStep(ctx, trace.StepList)
	data, err := c.List("", "*", &imap.ListOptions{ReturnSpecialUse: true, ReturnStatus: status}).Collect()
	if err != nil {
		step.End("", 0, 0, err)
		return nil, fmt.Errorf("imap list: %w", err)
	}
	step.End("", len(data), 0, nil)
	return data, nil
}
```

(`mailboxes.go` importa `internal/trace`.) Aggiornare i due commenti: `listMailboxes è il corpo di ListMailboxes SENZA gate ... (ctx per la misura del passo "list", Piano 12a, Task 5)`.

**`trash.go`.** In `trashMove`: `boxes, err := listMailboxes(ctx, c)`.

**`restore.go` e `draft.go`.** Stessa chiamata (`listMailboxes(ctx, c)`), rispettivamente in `Restore` e in `draftsBoxOf`. `draftsBoxOf` non aveva `ctx`: la firma diventa `draftsBoxOf(ctx context.Context, c *imapclient.Client)`, con le due chiamate in `SaveDraft` e `DeleteDraft` aggiornate. Questi due file non erano nell'elenco della prima stesura: li ha trovati il compilatore.

**`unseen.go`.** In `unreadCounts`: `listData(ctx, c, withStatus)`.

**`dial.go`.** `dialSession` prende `op` e misura il login; `dialConn` misura `tcp` e `tls` (import `internal/trace`):

```go
// dialSession apre la connessione (TLS, o in chiaro se o.Insecure) e fa il login.
// A ctx scaduto o annullato la connessione e chiusa e l'errore e quello del ctx
// (riconoscibile con errors.Is): non resta una connessione aperta dietro.
// op e il nome dell'operazione per la traccia dei passi (Piano 12a, Task 5).
func dialSession(ctx context.Context, o Options, op string, options *imapclient.Options) (*imapclient.Client, error) {
	ctx = trace.WithOp(ctx, op)
	conn, err := dialConn(ctx, o)
	if err != nil {
		return nil, err
	}
	// Alla scadenza del ctx la connessione si chiude: i comandi in corso falliscono
	// subito, senza aspettare i timeout di lettura del client.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })

	c := imapclient.New(conn, options)
	step := trace.BeginStep(ctx, trace.StepLogin)
	err = c.Login(o.Username, o.Password).Wait()
	cerr := ctx.Err() // scaduto durante il login: vale il suo errore, non quello di rete
	if err != nil || cerr != nil {
		_ = c.Close()
		if cerr != nil {
			err = cerr
		}
		step.End("", 0, 0, err)
		return nil, fmt.Errorf("imap login %s: %w", o.Username, err)
	}
	step.End("", 0, 0, nil)
	stop() // da qui la connessione vive con lo Store: il ctx del chiamante non la chiude
	return c, nil
}
```

In `dialConn`: la `DialContext` e la `HandshakeContext` si misurano.

```go
func dialConn(ctx context.Context, o Options) (net.Conn, error) {
	step := trace.BeginStep(ctx, trace.StepTCP)
	raw, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", o.Addr)
	if err != nil {
		step.End("", 0, 0, err)
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	step.End("", 0, 0, nil)
	if o.Insecure {
		return raw, nil
	}
```

```go
	step = trace.BeginStep(ctx, trace.StepTLS)
	tc := tls.Client(raw, cfg)
	if err := tc.HandshakeContext(hsCtx); err != nil {
		step.End("", 0, 0, err)
		_ = raw.Close()
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	step.End("", 0, 0, nil)
	return tc, nil
}
```

**`capabilities.go`.** `c, err := dialSession(ctx, o, "Capabilities", &imapclient.Options{})`.

**`watch.go`** (import `internal/trace`). In `Watch`, subito dopo il controllo iniziale di `ctx.Err()`:

```go
	ctx = trace.WithOp(ctx, "Watch") // i comandi del feed si misurano come passi del Watch (Piano 12a, Task 5)
```

e la baseline è `st, err := s.baselineState(ctx, c)`. Le funzioni:

```go
// baselineState seleziona INBOX e ne ricava lo stato iniziale: UIDVALIDITY e
// UID più alto esistente (0 se vuota).
func (s *Store) baselineState(ctx context.Context, c *imapclient.Client) (*watchState, error) {
	sel, err := s.selectBox(ctx, c, "INBOX") // Piano 16, Task 13, R3: anche la SELECT del Watch registra l'epoca
	if err != nil {
		return nil, err
	}
	last, err := s.highestUID(ctx, c, "INBOX")
	if err != nil {
		return nil, err
	}
	return &watchState{lastUID: last, uidValid: sel.UIDValidity, mailbox: "INBOX"}, nil
}
```

In `idleRound`, le tre righe della `c.Select(st.mailbox, nil).Wait()` diventano (l'errore perde le parole "prima di idle": nessun test le legge):

```go
	if _, err := s.selectBox(ctx, c, st.mailbox); err != nil {
		return err
	}
```

`recheckNewMail` prende `ctx`. L'inizio e il punto del fetch:

```go
func (s *Store) recheckNewMail(ctx context.Context, c *imapclient.Client, ch chan<- mail.Change, st *watchState) error {
	sel, err := s.selectBox(ctx, c, st.mailbox) // Piano 16, Task 13, R3: gli ID degli eventi escono da questa SELECT
	if err != nil {
		return err
	}
	if sel.UIDValidity != st.uidValid {
		// Nuova epoca della mailbox: ri-baseline senza eventi (gli UID
		// vecchi e nuovi non sono confrontabili).
		st.uidValid = sel.UIDValidity
		last, err := s.highestUID(ctx, c, st.mailbox)
		if err != nil {
			return err
		}
		st.lastUID = last
		return nil
	}

	var set imap.UIDSet
	set.AddRange(st.lastUID+1, 0) // 0 = "*" ⇒ "UID (last+1):*"
	data, err := s.uidSearch(ctx, c, st.mailbox, &imap.SearchCriteria{UID: []imap.UIDSet{set}}, nil)
	if err != nil {
		return fmt.Errorf("imap uid search %s: %w", st.mailbox, err)
	}
```

```go
	unread := len(newUIDs)
	if u, err := s.uidSearch(ctx, c, st.mailbox, &imap.SearchCriteria{UID: []imap.UIDSet{set}, NotFlag: []imap.Flag{imap.FlagSeen}}, nil); err == nil {
```

```go
	fetch := func(mailbox string, uids []imap.UID) ([]msg.MessageRef, error) {
		return s.fetchRefs(ctx, mailbox, uids)
	}
	if s.headersFn != nil {
		fetch = s.headersFn
	}
```

`highestUID` diventa un metodo:

```go
// highestUID ritorna l'UID più alto della mailbox selezionata (UIDSearch
// match-all: la lista è ascendente, l'ultimo è il massimo; 0 se vuota).
func (s *Store) highestUID(ctx context.Context, c *imapclient.Client, mailbox string) (imap.UID, error) {
	data, err := s.uidSearch(ctx, c, mailbox, &imap.SearchCriteria{}, nil)
	if err != nil {
		return 0, fmt.Errorf("imap uid search: %w", err)
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return 0, nil
	}
	return uids[len(uids)-1], nil
}
```

I due chiamanti di `recheckNewMail` (in `idleRound` e in `reconnect`) passano `ctx`: `s.recheckNewMail(ctx, c, ch, st)`.

**`watch_refs_test.go`.** Riga 31: `last, err := store.highestUID(t.Context(), c, "INBOX")`. Riga 46: `if err := store.recheckNewMail(t.Context(), c, ch, st); err != nil {`.

- [x] **Step 5: vedere verde.**

Run: `go test ./internal/mail/imap/ -count=1 -v`
Expected: PASS, `TestListConversationsEmitsOneStepPerCommand` con la sequenza attesa.

- [x] **Step 6: controllo che nessun SELECT o SEARCH del percorso di lettura sia rimasto fuori.**

Run: `grep -nE "c\.(Select|UIDSearch)\(" internal/mail/imap/*.go | grep -v _test.go`
Expected: solo questi file, e nessun altro.

- `steps.go`: le due funzioni `selectBox` e `uidSearch`;
- le mutazioni, fuori per scelta (D12-T3): `mutate.go` (tre SELECT), `gmail_labels.go` (quattro SELECT e la UID SEARCH di `AllMailIDs`), `restore.go` (una SELECT), `trash.go` (una SELECT).

Se compare una riga in `conversation.go`, `fetch.go`, `imap.go`, `mailboxes.go` o `watch.go`, un comando del percorso di lettura è rimasto fuori dalla traccia.

- [x] **Step 7: gate**, poi commit `feat(imap): misura dei passi IMAP nel percorso di lettura (Piano 12a, Task 5)`.

## Task 6: "Importanti" tra le cartelle speciali

**Files:**
- Modify: `internal/mail/imap/mailboxes.go` (`specialRoles`, righe 19–31), `internal/mail/store.go` (commento di `Mailbox.Role`, righe 33–34; commento di `ListMailboxes`, righe 157–161), `internal/app/folders.go` (`folderName`, righe 28–48), `internal/ui/foldertree/tree.go` (`rangoSpeciale`, righe 268–285), `internal/app/demo.go` (elenco cartelle, riga 114), `internal/trace/tracestore/store.go` (commento del pacchetto, riga 5)
- Test: Modify `internal/mail/imap/mailboxes_test.go`, `internal/ui/foldertree/tree_test.go`, `internal/app/folders_test.go`, `internal/app/demo_test.go`, `internal/trace/tracestore/snapshot_test.go`

**Produces:** la mailbox con attributo `\Important` ha ruolo `important` e compare come "Importanti" subito dopo Posta in arrivo, prima di Speciali, come in Gmail (D12-T2: senza contatore). Oggi `[Gmail]/Importanti` non ha ruolo e l'albero la annida tra le label sotto un ramo "[Gmail]".

**Interfaces:**
- Produces: nuovo valore `"important"` per `mail.Mailbox.Role`. Nessun metodo cambia.

- [x] **Step 1: scrivere i test (modifiche ai test esistenti).**

In `internal/mail/imap/mailboxes_test.go`, `TestListMailboxesRolesOrderAndNoselect`: dopo `ts.createSpecialMailbox("[Gmail]/Trash", imap.MailboxAttrTrash)` aggiungere

```go
	ts.createSpecialMailbox("[Gmail]/Important", imap.MailboxAttrImportant)
```

e nel commento della funzione "(Speciali, Inviati, Bozze, Tutti, Spam, Cestino)" diventa "(Importanti, Speciali, Inviati, Bozze, Tutti, Spam, Cestino)". In `want`, subito dopo la riga di INBOX:

```go
		{Name: "[Gmail]/Important", Role: "important", Delimiter: "/"},
```

In `internal/ui/foldertree/tree_test.go`: `nomi` (righe 13–24) impara i due ruoli, come fa `app.folderName`:

```go
// nomi è la funzione di etichetta di app (folderName) ridotta ai casi del test.
func nomi(b mail.Mailbox) string {
	switch b.Role {
	case "inbox":
		return "Posta in arrivo"
	case "important":
		return "Importanti"
	case "flagged":
		return "Speciali"
	case "sent":
		return "Inviati"
	case "drafts":
		return "Bozze"
	}
	return b.Name
}
```

In `TestBuildOrdine` aggiungere alle `boxes`:

```go
		{Name: "[Gmail]/Important", Role: "important", Delimiter: "/"},
		{Name: "[Gmail]/Starred", Role: "flagged", Delimiter: "/"},
```

e `want` diventa:

```go
	want := []string{"INBOX", "[Gmail]/Important", "[Gmail]/Starred", "[Gmail]/Sent Mail", "[Gmail]/Drafts", "alfa", "Beta/x", "Zeta"}
```

In `internal/app/folders_test.go`, `TestFolderNames`: aggiungere al map `cases`:

```go
		"Importanti":      {Name: "[Gmail]/Important", Role: "important"},
```

In `internal/app/demo_test.go`, `TestDemoStoreFolders`: dopo il controllo di `[Gmail]/Inviati` aggiungere:

```go
	if roles["[Gmail]/Importanti"] != "important" {
		t.Errorf("cartelle = %+v, voglio [Gmail]/Importanti con ruolo important", boxes)
	}
```

In `internal/trace/tracestore/snapshot_test.go`, riga 140: aggiungere `"important": true` al map `roles`.

- [x] **Step 2: vedere rosso.**

Run: `go test ./internal/mail/imap/ ./internal/ui/foldertree/ ./internal/app/ ./internal/trace/... -run 'TestListMailboxesRolesOrderAndNoselect|TestBuildOrdine|TestFolderNames|TestDemoStoreFolders' -count=1`
Expected: FAIL. `ListMailboxes[1] = {Name:[Gmail]/Starred ...}, voglio {Name:[Gmail]/Important Role:important}`; `ordine: got [INBOX [Gmail]/Sent Mail [Gmail]/Drafts [Gmail]/Important [Gmail]/Starred] ...`; `folderName(...) = "[Gmail]/Important", voglio "Importanti"`; `cartelle = ..., voglio [Gmail]/Importanti con ruolo important`.

- [x] **Step 3: implementare.**

In `internal/mail/imap/mailboxes.go`:

```go
// specialRoles associa gli attributi SPECIAL-USE ai ruoli, nell'ordine di
// presentazione (Piano 7, Task 1: Speciali, Inviati, Bozze, Tutti, Spam, Cestino;
// Piano 12a, Task 6: Importanti prima di tutti, come in Gmail).
var specialRoles = []struct {
	attr imap.MailboxAttr
	role string
}{
	{imap.MailboxAttrImportant, "important"},
	{imap.MailboxAttrFlagged, "flagged"},
	{imap.MailboxAttrSent, "sent"},
	{imap.MailboxAttrDrafts, "drafts"},
	{imap.MailboxAttrAll, "all"},
	{imap.MailboxAttrJunk, "junk"},
	{imap.MailboxAttrTrash, "trash"},
}
```

In `internal/mail/store.go`, commento di `Mailbox.Role`:

```go
	// Valori: "" (label) | "inbox" | "important" | "sent" | "drafts" | "trash" | "junk" | "all" | "flagged"
```

e commento di `ListMailboxes`:

```go
	// Ordine: INBOX, poi le speciali (important, flagged, sent, drafts, all, junk,
	// trash), poi le label in ordine alfabetico. Le mailbox \Noselect (contenitori
	// come "[Gmail]") sono escluse.
```

In `internal/app/folders.go`, primo caso di `folderName` dopo `"inbox"`:

```go
	case "important":
		return "Importanti"
```

In `internal/ui/foldertree/tree.go`:

```go
// rangoSpeciale fissa l'ordine delle speciali (come Gmail: Importanti, Speciali,
// Inviati, Bozze, Tutti, Spam, Cestino); ruoli sconosciuti in fondo.
// Piano 12a, Task 6: "important" per primo.
func rangoSpeciale(role string) int {
	switch role {
	case "important":
		return 0
	case "flagged":
		return 1
	case "sent":
		return 2
	case "drafts":
		return 3
	case "all":
		return 4
	case "junk":
		return 5
	case "trash":
		return 6
	}
	return 7
}
```

In `internal/app/demo.go`, riga 114, dopo `{Name: "INBOX", Role: "inbox"},`:

```go
		{Name: "[Gmail]/Importanti", Role: "important"}, // Piano 12a, Task 6
```

In `internal/trace/tracestore/store.go`, commento del pacchetto (riga 5): l'elenco dei ruoli diventa `(inbox/important/sent/drafts/all/trash/junk/flagged/label)`.

- [x] **Step 4: vedere verde.**

Run: `go test ./internal/mail/... ./internal/ui/foldertree/ ./internal/app/ ./internal/trace/... ./internal/mcpserver/ -count=1`
Expected: PASS. Il server MCP riporta il ruolo `"important"` così com'è: nessun test suo cambia.

- [x] **Step 5: gate**, poi commit `feat(mail): Importanti tra le cartelle speciali, dopo Posta in arrivo (Piano 12a, Task 6)`.

## Task 7: documenti

**Files:**
- Modify: `CHANGELOG.md`, `README.md`, `docs/smoke-checklist.md`, `AGENTS.md`, `ROADMAP.md`, `docs/superpowers/plans/2026-11-22-piano-sviluppo-gap.md`, `docs/stato-dei-lavori.md`, `RIEPILOGO.md`

**Produces:** la documentazione pubblica e interna al passo con il codice (CHANGELOG e README in inglese, il resto in italiano).

- [x] **Step 1: `CHANGELOG.md`**, sotto `## [Unreleased]`. In cima a `### Added`:

```markdown
- **Important folder** — `[Gmail]/Important` (`\Important`) is listed with the special folders, right after Inbox, as "Importanti". Before, it appeared nested under a "[Gmail]" branch among the labels.
- **Trace steps** — with `GMAIL_TUI_TRACE=1` every IMAP command of the read path is a `step` event (`tcp`, `tls`, `login`, `list`, `select`, `search`, `fetch`). `gmail-tui trace` adds a "Passi IMAP" table (median, p95 and max per operation, step and folder role) and shows the Connect steps in the startup timeline.
```

In cima a `### Changed`:

```markdown
- **Connect honours its deadline** — when the 90 s budget expires, the dial and the login are interrupted and the connection is closed. Before, they kept running in the background and could finish after the app had reported the failure. The same applies to the connection test of the wizard and `gmail-tui doctor`.
```

In cima a `### Fixed`:

```markdown
- **Outgoing message encoding** — the text parts of sent messages, drafts and `gmail-tui send` declare `Content-Transfer-Encoding`: `7bit` for plain ASCII with lines up to 998 bytes, `quoted-printable` otherwise (accents, longer lines: most HTML parts). Before, no encoding was declared and those messages broke RFC 5322.
- **Ghost rows** — a message expunged between the IMAP search and the fetch no longer leaves an empty row in the list.
```

- [x] **Step 2: `README.md`**, cinque punti.

1. Riga 96 (punto **Folders** della sezione Rendering): `(not Drafts, Sent, Spam, Trash or All Mail)` diventa `(not Important, Drafts, Sent, Spam, Trash or All Mail)`, e in fondo al punto si aggiunge: `The Important folder ("Importanti") sits with the special folders, right after Inbox.`
2. Riga 236 (punto **Markdown** delle Notes), in fondo: `Both text parts declare their \`Content-Transfer-Encoding\` (\`7bit\`, or \`quoted-printable\` for accents and lines above 998 bytes).`
3. Riga 256 (descrizione del riepilogo della traccia), in fondo: `A **Passi IMAP** table follows: one row per operation, IMAP command (\`tcp\`, \`tls\`, \`login\`, \`list\`, \`select\`, \`search\`, \`fetch\`) and folder role, which tells which command is slow on a real account; the startup timeline also lists the Connect steps.`
4. Riga 258: `kinds are \`meta\`, \`store\`, \`msg\`, \`phase\` and \`body\`` diventa `kinds are \`meta\`, \`store\`, \`step\`, \`cache\`, \`msg\`, \`phase\` and \`body\``.
5. Riga 260 (Privacy): l'elenco dei ruoli diventa `(\`inbox\`, \`important\`, \`sent\`, \`drafts\`, \`all\`, \`trash\`, \`junk\`, \`flagged\` or \`label\`)`.

- [x] **Step 3: `docs/smoke-checklist.md`**, in fondo al file (in inglese, come le sezioni vicine):

```markdown
## Piano 12a — fixes (real account, run by hand)

Run on a real Gmail account; nothing here is covered by tests.

- [ ] `m`: "Importanti" is the second entry, right after Inbox, with the other special folders and no `[Gmail]` branch above it. Opening it lists the messages Gmail marks as important. It shows no unread count.
- [ ] Compose a message with accented letters and a line longer than 1000 characters (paste a long URL), send it to yourself. In Gmail web, "Show original": the text part has `Content-Transfer-Encoding: quoted-printable`, no line above 998 characters, and the message reads correctly in Gmail and in another client.
- [ ] Send a plain ASCII message: "Show Original" shows `Content-Transfer-Encoding: 7bit` and the text as typed.
- [ ] Markdown message with accents: both parts of the `multipart/alternative` open correctly. Save it as a draft with `Esc`, reopen it from Drafts: the text is the same.
- [ ] Delete a message from Gmail web while the list is open: after the refresh there is no empty row.
- [ ] Turn Wi-Fi off and start: the splash shows the error within the 90 s budget; turn Wi-Fi on and press `R`, the app connects (the timed-out attempt left no connection behind).

## Timing trace, steps (Piano 12a) — real account

This is the measurement the Piano 12b design is built on. Do it first and paste the summary in the chat.

- [ ] `GMAIL_TUI_TRACE=1 gmail-tui`: start, wait for the list, open 2 conversations (one with several messages), press `v` on a row, switch to another folder with `m`, quit.
- [ ] `gmail-tui trace <path>`: the summary has the "Passi IMAP" table, with rows for `Connect tcp`, `Connect tls` and `Connect login`, and `list`, `select`, `search` and `fetch` for `ListConversations`, `ThreadMessages` and `FetchMessage`.
- [ ] Startup timeline: it lists the three Connect steps. Note the time of `Connect login` (the 28.9 s of the 23 November trace were all inside Connect).
- [ ] First `ListConversations`: note the slowest step, and whether it is in `inbox` or in `all`.
- [ ] Privacy: `grep -ciE "inbox|all mail|\[Gmail\]" <path>` prints 0.
- [ ] Paste the "Passi IMAP" table and the startup timeline in the chat: they decide the order of Piano 12b.
```

- [x] **Step 4: `AGENTS.md`.**

1. Regola 7: «(30s timeout)» diventa «(90 s timeout)», e in fondo alla regola si aggiunge: `` `Store.Connect` honours its ctx for the whole dial and login: it closes the connection when the ctx expires (`dialSession`, `internal/mail/imap/dial.go`). ``
2. Testing conventions, nuovo punto dopo quello sugli orologi iniettabili: `- **TLS dial**: `Options.TLSConfig` (test-only) carries the CA of a self-signed certificate, as the SMTP tests do. Never add `InsecureSkipVerify` to production code.`
3. Gotchas, quattro punti nuovi dopo quello su **Gated store methods**:

```markdown
- **`Connect` is the only ctx-aware store path** (Piano 12a): go-imap beta.9 has no ctx API, so
  `dialSession` opens TCP, TLS and login itself and closes the socket when the ctx expires. The other
  ops still check only `ctx.Err()` at entry.
- **Rows always carry FETCH data** (Piano 12a): `gmailRefs` and `fetchRefs` skip the UIDs missing
  from the FETCH response (message expunged in between); `toRef` takes a non-nil buffer.
- **Trace steps** (Piano 12a): the SELECT and UID SEARCH of the read path go through `selectBox`
  and `uidSearch` (`internal/mail/imap/steps.go`), which emit the `step` events with the folder
  role (`traceRole`, never the name). A read command added with a raw `c.Select`/`c.UIDSearch`
  is invisible in the trace. Mutations are not traced (D12-T3).
- **Text parts declare their `Content-Transfer-Encoding`** (`textPart`,
  `internal/mail/smtp/encoding.go`): `7bit`, or `quoted-printable` when the text has bytes above
  127, a NUL or a line above 998 bytes. `TestBuildMIMEAccentsGoQuotedPrintable` guards the wire
  format; the standard `quotedprintable` writer does the encoding.
```

- [x] **Step 5: `ROADMAP.md`** (già modificato nell'albero di lavoro, ora allineato a `v0.1.0` e `Unreleased — delivered`). In `Unreleased — delivered`, in coda all'elenco:

```markdown
- **Message encoding** — the text parts of outgoing messages declare their
  `Content-Transfer-Encoding`: `7bit`, or `quoted-printable` for accents and long lines.
- **Important folder** — "Importanti" sits with the special folders, right after Inbox.
- **Trace steps** — each IMAP command of the read path is measured (`tcp`, `tls`, `login`,
  `list`, `select`, `search`, `fetch`) and summarized by `gmail-tui trace`.
```

In `Next — planned` togliere il punto `Outgoing message encoding` e lasciare `Incremental sync` così: `- **Incremental sync and performance (Piano 12b)** — designed on a real-account trace: CONDSTORE-based refresh, fewer IMAP round trips in the list, paged search.`

- [x] **Step 6: `docs/superpowers/plans/2026-11-22-piano-sviluppo-gap.md`**, sotto il titolo `## Piano 12 — Sincronizzazione incrementale e prestazioni`, prima dell'elenco:

```markdown
**Stato (23 novembre 2026): diviso in due.** **12a** — ritocchi e misura, in `docs/superpowers/plans/2026-11-23-piano-12a-ritocchi-e-misura.md`: codifica MIME, `Connect` con contesto, ruolo di `\Important`, traccia per passo. La voce "Trash su più cartelle" è chiusa senza codice: l'app già gestisce l'errore a metà una conversazione alla volta. **12b** — CONDSTORE, prestazioni della lista e ricerca paginata: piano proprio, scritto dopo una traccia reale presa con 12a.
```

- [x] **Step 7: `docs/stato-dei-lavori.md`.** Nella tabella di `Fatto`, nuova prima riga (il commit è quello del merge):

```markdown
| 23 nov | 12a, ritocchi e misura | Codifica delle parti di testo, niente righe fantasma, Connect interrompibile con il TLS provato, Importanti tra le speciali, traccia per passo IMAP; ROADMAP allineata a `v0.1.0` | `<merge>` |
```

Nella sezione `Da fare: smoke test sull'account reale` aggiungere, prima delle voci del Piano 16: `- [ ] **Piano 12a**: "Importanti" dopo Posta in arrivo, messaggio con accenti e riga lunga in "Show original", nessuna riga vuota dopo una cancellazione da Gmail web (passi in \`docs/smoke-checklist.md\`)` e `- [ ] **Piano 12b, la misura**: una sessione con \`GMAIL_TUI_TRACE=1\` e la tabella "Passi IMAP" incollata in chat: decide l'ordine di 12b`. Sostituire la sezione `Da fare: Piano 12 e passi manuali` con:

```markdown
## Da fare: Piano 12b

Il Piano 12 è diviso in due. Il **12a** (ritocchi e misura) è in `main`. Resta il **12b**, prestazioni, che avrà una spec propria dopo la traccia reale con i passi per comando (sezione precedente):

1. Niente rilista dopo `s` o `v`: solo i flag cambiati, con CONDSTORE
2. Meno comandi nella lista: oggi ogni lista rilancia il LIST di tutte le cartelle e ogni operazione rifà la SELECT
3. La base del Watch senza la SEARCH di tutti gli UID di INBOX
4. Ricerca oltre i primi risultati
5. Contesto per le altre operazioni dello store, da interrompere in modo pulito

Altri lavori aperti:

- [ ] **Release**: tutto quello che c'è dopo `v0.1.0` è solo in `main`. Serve un tag (per esempio `v0.2.0`) per pubblicare i binari
- [ ] **Social preview di GitHub**: l'immagine è in `.superpowers/sdd/social-preview.png`, da caricare a mano in Settings → General (non esiste un'API)
- [ ] **Tap Homebrew** (facoltativo): serve il repo `homebrew-gmail-tui` e il token `TAP_GITHUB_TOKEN`, poi si attiva la sezione in `.goreleaser.yaml`
- [ ] **Lentezza di Gmail**: la traccia ha mostrato Connect a 29 s e liste fino a 77 s. La cache la nasconde ma non la risolve: la tabella "Passi IMAP" dice quale comando è lento, e vale la pena controllare anche le impostazioni IMAP e di sicurezza dell'account
```

Il punto «`ROADMAP.md` è indietro» esce dalla lista perché è fatto; i due punti sul Piano 16 che aspettano una decisione di Andrea ("azioni bloccate dopo `R`" e "toast store non connesso") restano dove sono.

- [x] **Step 8: `RIEPILOGO.md`.** Dopo la sezione del Piano 16 aggiungere `### Piano 12a — Ritocchi e misura (2026-11-23)`, nello stesso formato delle sezioni vicine: le cinque voci consegnate, le decisioni D12-S1, D12-S2 e D12-T1…T6 in sintesi, i file nuovi (`encoding.go`, `dial.go`, `steps.go`) e i passi manuali (smoke di 12a e la traccia per passo per il 12b). In `Passi manuali rimasti` aggiungere gli stessi due passi.

- [x] **Step 9: gate**, poi commit `docs: Piano 12a (ritocchi e misura) in CHANGELOG, README, smoke checklist, AGENTS e stato`.

## Revisione e chiusura

- [ ] Revisione indipendente per task (subagent diverso da quello che ha implementato): comportamento, privacy della traccia, nessun nome di cartella negli eventi, commenti in italiano con il riferimento al task.
- [ ] Revisione dell'intero branch: `go test -race -count=1 ./...`, `golangci-lint run ./...`, `CGO_ENABLED=0 go build ./...`, `govulncheck ./...` pulito (nessuna dipendenza nuova: deve restare com'è).
- [ ] Smoke in demo mode: `go run ./cmd/gmail-tui`, wizard, `s`: `m` mostra Importanti dopo Posta in arrivo; nessuna regressione visibile.
- [ ] Merge in `main` e push **solo con l'ok di Andrea** (decisione "Lavoro con agenti", stato dei lavori).

## Dopo il merge: la misura per il Piano 12b

Andrea fa la sessione della sezione "Timing trace, steps" dello smoke checklist e incolla in chat la tabella "Passi IMAP" e la cronologia dell'avvio. Con quei numeri si scrive la spec di 12b. I candidati, in ordine di valore atteso (l'ordine vero lo decide la misura):

1. **Niente rilista dopo `s` o `v`.** Oggi, dopo una stella o un cambio di lettura, l'app rilista 50 righe (15–20 s di esecuzione nella traccia) per rileggere flag che ha appena cambiato lei: `refreshLimit` in `internal/app/ops.go`. Con CONDSTORE si rileggono solo i messaggi cambiati, a qualunque profondità, e cade il compromesso D5 del Piano 7.
2. **Meno comandi nella lista.** Ogni lista rilancia il LIST di tutte le cartelle per ritrovare "Tutti i messaggi" (`allMailName`, `internal/mail/imap/conversation.go`), e ogni operazione rifà la SELECT anche a cartella già selezionata. La copia di go-imap nel repo legge già `HIGHESTMODSEQ` e `UIDNEXT` dalla SELECT: CONDSTORE non richiede patch.
3. **La base del Watch senza la SEARCH di tutti gli UID.** `baselineState` fa SELECT e una SEARCH match-all di INBOX (15,9 s nella traccia) solo per ricavare l'UID più alto, che la SELECT restituisce (`UIDNEXT`).
4. **Ricerca oltre i primi risultati**, oggi ferma a `searchLimit`.
5. **Contesto per le altre operazioni dello store**: chiude la connessione come fa `Connect`, oppure una via meno drastica da valutare con la misura.
