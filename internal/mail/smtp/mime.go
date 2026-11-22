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
// Il dot-stuffing SMTP non tocca a buildMIME: se ne occupa il DotWriter
// del client go-smtp in Send.
func buildMIME(m msg.Outgoing, now time.Time) ([]byte, error) {
	// Corpo: normalizza a CRLF (LF→CRLF senza raddoppiare i CRLF esistenti).
	body := strings.ReplaceAll(strings.ReplaceAll(m.BodyText, "\r\n", "\n"), "\n", "\r\n")

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
	// Bcc volutamente assente dagli header: solo RCPT in Send.
	// mime.QEncoding.Encode lascia invariato l'ASCII puro.
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	writeHeader("Date", now.Format(time.RFC1123Z))
	writeHeader("Message-ID", fmt.Sprintf("<%s@%s>", randomHex16(), domainOf(m.From)))
	writeHeader("MIME-Version", "1.0")

	if len(m.Attachments) == 0 {
		writeHeader("Content-Type", "text/plain; charset=utf-8")
		buf.WriteString("\r\n")
		buf.WriteString(body)
		return buf.Bytes(), nil
	}

	mw := multipart.NewWriter(&buf)
	writeHeader("Content-Type", "multipart/mixed; boundary="+mw.Boundary())
	buf.WriteString("\r\n")

	// Prima parte: il corpo text/plain.
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", "text/plain; charset=utf-8")
	p, err := mw.CreatePart(h)
	if err != nil {
		return nil, fmt.Errorf("mime: parte testo: %w", err)
	}
	if _, err := p.Write([]byte(body)); err != nil {
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
