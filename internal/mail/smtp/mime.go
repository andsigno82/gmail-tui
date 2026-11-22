package smtp

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

// buildMIME costruisce il messaggio MIME completo (header + corpo) per
// msg.Outgoing. Semantica:
//
//   - Header: From (come fornito), To/Cc (join ", ", omessi se vuoti),
//     Subject (Q-encoded SOLO se non-ASCII), Date (RFC1123Z di now),
//     Message-ID <hex16@dominio-di-From>, MIME-Version: 1.0.
//   - Bcc MAI negli header: è un destinatario solo RCPT (vedi Sender.Send).
//   - Senza allegati: Content-Type: text/plain; charset=utf-8, corpo con
//     LF→CRLF (gli eventuali CRLF esistenti restano tali).
//   - Con allegati: multipart/mixed; prima parte text/plain, poi una parte
//     per file: Content-Type da mime.TypeByExtension (fallback
//     application/octet-stream), Content-Disposition: attachment;
//     filename="<base del path>", Content-Transfer-Encoding: base64 con
//     righe da 76 caratteri e CRLF.
//   - File inesistente o illeggibile → errore wrapped col path.
//
// Piano 7.2, Task 5: con msg.Outgoing.Markdown il corpo diventa
// multipart/alternative (vedi corpo); con allegati il multipart/alternative
// annidato precede le parti allegato dentro il multipart/mixed.
//
// Il dot-stuffing SMTP non tocca a buildMIME: se ne occupa il DotWriter
// del client go-smtp in Send.
func buildMIME(m msg.Outgoing, now time.Time) ([]byte, error) {
	return buildMIMEWith(m, now, false)
}

// BuildDraft costruisce il MIME di una BOZZA (Piano 9, Task 4): stesso MIME
// dell'invio, ma con l'intestazione Bcc, perché la bozza salvata in Bozze
// conserva i destinatari nascosti (come Gmail) e la ripresa li ricarica. Non va
// MAI usata per l'invio: il Sender usa buildMIME, che non scrive Bcc.
func BuildDraft(m msg.Outgoing, now time.Time) ([]byte, error) {
	return buildMIMEWith(m, now, true)
}

// buildMIMEWith è il builder comune: withBcc aggiunge l'intestazione Bcc
// (solo bozze).
func buildMIMEWith(m msg.Outgoing, now time.Time, withBcc bool) ([]byte, error) {
	// Corpo: normalizza a CRLF (LF→CRLF senza raddoppiare i CRLF esistenti).
	body := toCRLF(m.BodyText)

	var buf bytes.Buffer
	writeHeader := func(key, value string) {
		fmt.Fprintf(&buf, "%s: %s\r\n", key, value)
	}

	writeHeader("From", m.From)
	if len(m.To) > 0 {
		writeHeader("To", strings.Join(m.To, ", "))
	}
	if len(m.Cc) > 0 {
		writeHeader("Cc", strings.Join(m.Cc, ", "))
	}
	// Bcc volutamente assente dagli header dell'invio: solo RCPT in Send.
	// Nella bozza (withBcc) l'intestazione c'è, come in Gmail.
	if withBcc && len(m.Bcc) > 0 {
		writeHeader("Bcc", strings.Join(m.Bcc, ", "))
	}
	// mime.QEncoding.Encode lascia invariato l'ASCII puro.
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	writeHeader("Date", now.Format(time.RFC1123Z))
	writeHeader("Message-ID", fmt.Sprintf("<%s@%s>", randomHex16(), domainOf(m.From)))
	writeHeader("MIME-Version", "1.0")

	// Corpo (Piano 7.2, Task 5): text/plain, o multipart/alternative con HTML.
	ct, data, err := corpo(m, body)
	if err != nil {
		return nil, err
	}

	if len(m.Attachments) == 0 {
		writeHeader("Content-Type", ct)
		buf.WriteString("\r\n")
		buf.Write(data)
		return buf.Bytes(), nil
	}

	mw := multipart.NewWriter(&buf)
	writeHeader("Content-Type", "multipart/mixed; boundary="+mw.Boundary())
	buf.WriteString("\r\n")

	// Prima parte: il corpo text/plain.
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", ct)
	p, err := mw.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("mime: parte testo: %w", err)
	}
	if _, err := p.Write(data); err != nil {
		return nil, fmt.Errorf("mime: corpo: %w", err)
	}

	// Parti degli allegati.
	for _, path := range m.Attachments {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("mime: allegato %q: %w", path, err)
		}
		if err := writeAttachmentPart(mw, path, content); err != nil {
			return nil, err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("mime: chiusura multipart: %w", err)
	}
	return buf.Bytes(), nil
}

// toCRLF normalizza i fine riga a CRLF (LF→CRLF senza raddoppiare i CRLF esistenti).
func toCRLF(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\r\n")
}

// mdRenderer è il convertitore Markdown → HTML (Piano 7.2, Task 5): estensione
// GFM (tabelle, barrato, autolink, liste di task). NESSUN html.WithUnsafe: l'HTML
// scritto a mano nel corpo non passa (goldmark lo sostituisce con un commento).
// html.WithHardWraps: un a capo singolo resta un <br>, così la firma su righe
// separate non si fonde in un unico paragrafo.
//
// Piano 7.2, fix review: rawHTMLEscaper sostituisce il segnaposto "<!-- raw
// HTML omitted -->" con il testo escapato, così "<nome>" resta leggibile.
// Priorità 100: in goldmark v1.8.7 vince la priorità minore (il renderer di
// default è 1000).
var mdRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		renderer.WithNodeRenderers(util.Prioritized(rawHTMLEscaper{w: html.NewWriter()}, 100)),
	),
)

// rawHTMLEscaper rende l'HTML grezzo (inline e a blocco) come testo escapato:
// il testo passa dal writer escapante di goldmark e non viene mai emesso come
// tag vivo. Piano 7.2, fix review.
type rawHTMLEscaper struct {
	w html.Writer
}

// RegisterFuncs registra i renderer di ast.KindRawHTML e ast.KindHTMLBlock.
func (e rawHTMLEscaper) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindRawHTML, e.renderRawHTML)
	reg.Register(ast.KindHTMLBlock, e.renderHTMLBlock)
}

func (e rawHTMLEscaper) renderRawHTML(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	seg := n.(*ast.RawHTML).Segments
	for i := range seg.Len() {
		s := seg.At(i)
		e.w.Write(w, s.Value(source))
	}
	return ast.WalkSkipChildren, nil
}

func (e rawHTMLEscaper) renderHTMLBlock(w util.BufWriter, source []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	b := n.(*ast.HTMLBlock)
	if entering {
		for i := range b.Lines().Len() {
			l := b.Lines().At(i)
			e.w.Write(w, l.Value(source))
		}
	} else if b.HasClosure() {
		cl := b.ClosureLine
		e.w.Write(w, cl.Value(source))
	}
	return ast.WalkContinue, nil
}

// corpo costruisce Content-Type e bytes della parte corpo. Senza Markdown è
// text/plain con il corpo CRLF così com'è (invariato). Con Markdown è
// multipart/alternative: text/plain = markdown sorgente, text/html = documento
// minimo con il rendering GFM (meta charset utf-8, nessuno stile o script esterno).
func corpo(m msg.Outgoing, body string) (string, []byte, error) {
	const plain = "text/plain; charset=utf-8"
	if !m.Markdown {
		return plain, []byte(body), nil
	}
	var rendered bytes.Buffer
	if err := mdRenderer.Convert([]byte(m.BodyText), &rendered); err != nil {
		return "", nil, fmt.Errorf("mime: markdown: %w", err)
	}
	page := toCRLF(htmlPage(rendered.String()))

	var alt bytes.Buffer
	aw := multipart.NewWriter(&alt)
	for _, part := range []struct{ contentType, data string }{
		{plain, body},
		{"text/html; charset=utf-8", page},
	} {
		h := textproto.MIMEHeader{}
		h.Set("Content-Type", part.contentType)
		w, err := aw.CreatePart(h)
		if err != nil {
			return "", nil, fmt.Errorf("mime: parte %s: %w", part.contentType, err)
		}
		if _, err := w.Write([]byte(part.data)); err != nil {
			return "", nil, fmt.Errorf("mime: parte %s: %w", part.contentType, err)
		}
	}
	if err := aw.Close(); err != nil {
		return "", nil, fmt.Errorf("mime: chiusura alternative: %w", err)
	}
	return "multipart/alternative; boundary=" + aw.Boundary(), alt.Bytes(), nil
}

// htmlPage incapsula il frammento renderizzato in un documento HTML minimo.
func htmlPage(fragment string) string {
	return "<!DOCTYPE html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n</head>\n<body>\n" +
		fragment + "</body>\n</html>\n"
}

// writeAttachmentPart scrive una parte allegato: Content-Type da
// estensione (fallback application/octet-stream), disposition attachment,
// base64 a righe da 76 caratteri CRLF.
func writeAttachmentPart(mw *multipart.Writer, path string, content []byte) error {
	ct := mime.TypeByExtension(filepath.Ext(path))
	if ct == "" {
		ct = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", ct)
	h.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(path)))
	h.Set("Content-Transfer-Encoding", "base64")

	p, err := mw.CreatePart(h)
	if err != nil {
		return fmt.Errorf("mime: allegato %q: %w", path, err)
	}

	// base64 a righe da 76 caratteri, ognuna chiusa da CRLF (RFC 2045).
	enc := base64.StdEncoding.EncodeToString(content)
	for len(enc) > 76 {
		if _, err := p.Write([]byte(enc[:76] + "\r\n")); err != nil {
			return fmt.Errorf("mime: allegato %q: %w", path, err)
		}
		enc = enc[76:]
	}
	if _, err := p.Write([]byte(enc + "\r\n")); err != nil {
		return fmt.Errorf("mime: allegato %q: %w", path, err)
	}
	return nil
}

// randomHex16 genera 16 cifre esadecimali (8 byte random) per Message-ID.
func randomHex16() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand non fallisce su piattaforme supportate; fallback
		// deterministico per robustezza (non in lattice di test).
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// domainOf estrae il dominio per Message-ID da From: tutto ciò che segue
// l'ultimo '@', troncato a '>' o spazio (From può essere "Nome <a@b.c>").
// Senza '@' → "localhost".
func domainOf(from string) string {
	i := strings.LastIndexByte(from, '@')
	if i < 0 {
		return "localhost"
	}
	d := from[i+1:]
	if j := strings.IndexAny(d, "> \t,\""); j >= 0 {
		d = d[:j]
	}
	if d == "" {
		return "localhost"
	}
	return d
}
