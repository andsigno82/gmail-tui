package imap

// Test di FetchMessage (cache-first) e SaveAttachment (piano 2, Task 4).
// Riusa la harness di imap_test.go (startTestServer, connectStore).

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/cache"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// pdfContent è il payload binario (PDF-ish) dell'allegato del seed multipart.
var pdfContent = []byte("%PDF-1.4\nfake pdf body\n%\xe2\x82\xac\n")

// rawMultipartMail costruisce un multipart/mixed con una parte text/plain e
// un allegato application/pdf codificato base64 (CRLF, header Date fisso).
func rawMultipartMail(from, subject, textBody, filename string, content []byte) []byte {
	var b bytes.Buffer
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: tester@example.org\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + testDate.Format("Mon, 02 Jan 2006 15:04:05 -0700") + "\r\n")
	b.WriteString("Message-ID: <multipart-" + strings.ToLower(strings.ReplaceAll(subject, " ", "-")) + "@example.org>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"bnd42\"\r\n")
	b.WriteString("\r\n")
	b.WriteString("--bnd42\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(textBody + "\r\n")
	b.WriteString("--bnd42\r\n")
	b.WriteString("Content-Type: application/pdf; name=\"" + filename + "\"\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + filename + "\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("\r\n")
	enc := base64.StdEncoding.EncodeToString(content)
	for len(enc) > 0 {
		n := min(76, len(enc))
		b.WriteString(enc[:n] + "\r\n")
		enc = enc[n:]
	}
	b.WriteString("--bnd42--\r\n")
	return b.Bytes()
}

// seedMultipartMail appende il messaggio multipart nella mailbox e ne
// restituisce l'UID (variant della harness per i test del Task 4).
func (ts *testServer) seedMultipartMail(name, from, subject, textBody, filename string, content []byte, seen, flagged bool) uint32 {
	ts.t.Helper()
	return ts.seedRaw(name, rawMultipartMail(from, subject, textBody, filename, content), seen, flagged)
}

// openTestCache apre una cache su file temporaneo (con cleanup).
func openTestCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// connectStoreCache è connectStore con la cache collegata.
func connectStoreCache(t *testing.T, ts *testServer) (*Store, *cache.Cache) {
	t.Helper()
	c := openTestCache(t)
	s := NewStore(Options{
		Addr:     ts.addr(),
		Username: "tester",
		Password: "secret",
		Cache:    c,
		Insecure: true,
	})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, c
}

// TestFetchMessageMultipart: fetch (senza cache) di un multipart con
// allegato → BodyText dal text/plain, HasHTML false, allegato con metadata,
// Ref riconciliato dai FLAGS del fetch; il PEEK non imposta \Seen.
func TestFetchMessageMultipart(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, true)
	store := connectStore(t, ts)
	id := msg.NewID("INBOX", uid)

	m, err := store.FetchMessage(t.Context(), id)
	if err != nil {
		t.Fatalf("FetchMessage: %v", err)
	}

	if m.Ref.ID != id {
		t.Errorf("m.Ref.ID = %q, voglio %q", m.Ref.ID, id)
	}
	if m.Ref.Subject != "With attachment" {
		t.Errorf("m.Ref.Subject = %q, voglio %q", m.Ref.Subject, "With attachment")
	}
	if m.Ref.From != "Mia <mia@example.org>" {
		t.Errorf("m.Ref.From = %q, voglio %q", m.Ref.From, "Mia <mia@example.org>")
	}
	if !strings.Contains(m.BodyText, "Hello from the text part.") {
		t.Errorf("m.BodyText = %q, deve venire dalla parte text/plain", m.BodyText)
	}
	if m.HasHTML {
		t.Error("m.HasHTML = true, voglio false (c'è text/plain)")
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("m.Attachments = %d, voglio 1", len(m.Attachments))
	}
	att := m.Attachments[0]
	if att.PartIndex != 0 {
		t.Errorf("att.PartIndex = %d, voglio 0", att.PartIndex)
	}
	if att.Filename != "doc.pdf" {
		t.Errorf("att.Filename = %q, voglio %q", att.Filename, "doc.pdf")
	}
	if att.ContentType != "application/pdf" {
		t.Errorf("att.ContentType = %q, voglio %q", att.ContentType, "application/pdf")
	}
	if att.Size != int64(len(pdfContent)) {
		t.Errorf("att.Size = %d, voglio %d", att.Size, len(pdfContent))
	}

	// Riconciliazione FLAGS (seed: senza \Seen, con \Flagged).
	if !m.Ref.Unread {
		t.Error("m.Ref.Unread = false, voglio true (nessun \\Seen)")
	}
	if !m.Ref.Starred {
		t.Error("m.Ref.Starred = false, voglio true (\\Flagged nel seed)")
	}

	// PEEK: il fetch del corpo NON ha segnato il messaggio come letto.
	refs, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages dopo FetchMessage: %v", err)
	}
	if len(refs) == 0 || !refs[0].Unread {
		t.Error("dopo FetchMessage il messaggio deve restare unread (BODY.PEEK[])")
	}
}

// TestFetchMessageCacheHitNoNetwork: con la cache, il secondo fetch NON
// tocca la rete: dopo il primo fetch si stacca il server (chiude listener e
// connessioni attive) e il fetch deve riuscire dalla cache.
func TestFetchMessageCacheHitNoNetwork(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, false)
	store, _ := connectStoreCache(t, ts)
	id := msg.NewID("INBOX", uid)

	first, err := store.FetchMessage(t.Context(), id)
	if err != nil {
		t.Fatalf("primo FetchMessage (rete): %v", err)
	}

	// Kill del server: il secondo fetch può riuscire solo dalla cache.
	if err := ts.srv.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}

	second, err := store.FetchMessage(t.Context(), id)
	if err != nil {
		t.Fatalf("secondo FetchMessage deve riuscire dalla cache senza rete: %v", err)
	}
	if second.Ref.ID != first.Ref.ID || second.BodyText != first.BodyText ||
		len(second.Attachments) != len(first.Attachments) {
		t.Errorf("secondo fetch = %+v, diverso dal primo (%+v)", second.Ref, first.Ref)
	}
}

// TestFetchMessageNotFound: uid inesistente ⇒ errore che wrappa
// ErrMessageNotFound.
func TestFetchMessageNotFound(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t) // la mailbox non è vuota, ma l'uid non esiste
	store := connectStore(t, ts)

	_, err := store.FetchMessage(t.Context(), msg.NewID("INBOX", 999))
	if err == nil {
		t.Fatal("FetchMessage di uid inesistente: err = nil, voglio errore")
	}
	if !errors.Is(err, ErrMessageNotFound) {
		t.Errorf("err = %v, deve wrappare ErrMessageNotFound", err)
	}
}

// TestSaveAttachmentWritesFile: SaveAttachment scrive il contenuto esatto
// dell'allegato su file.
func TestSaveAttachmentWritesFile(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, false)
	store := connectStore(t, ts)
	path := filepath.Join(t.TempDir(), "doc.pdf")

	if err := store.SaveAttachment(t.Context(), msg.NewID("INBOX", uid), 0, path); err != nil {
		t.Fatalf("SaveAttachment: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leggo %s: %v", path, err)
	}
	if !bytes.Equal(got, pdfContent) {
		t.Errorf("contenuto file = %q, voglio %q", got, pdfContent)
	}
}

// TestSaveAttachmentPartOutOfRange: part=9 (fuori range) ⇒ errore.
func TestSaveAttachmentPartOutOfRange(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, false)
	store := connectStore(t, ts)

	err := store.SaveAttachment(t.Context(), msg.NewID("INBOX", uid), 9, filepath.Join(t.TempDir(), "out.bin"))
	if err == nil {
		t.Fatal("SaveAttachment part=9: err = nil, voglio errore (1 solo allegato)")
	}
}

// rawDegenerateMultipartMail costruisce un multipart/mixed con: parte
// text/plain, una parte DEGENERE (application/octet-stream senza filename e
// con corpo vuoto — enmime la classifica tra gli attachment grezzi) SEGUITA
// da un allegato reale. msg.Parse salta la parte degenere e rinumera:
// l'allegato reale ha PartIndex 0.
func rawDegenerateMultipartMail(from, subject, textBody, filename string, content []byte) []byte {
	var b bytes.Buffer
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: tester@example.org\r\n")
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("Date: " + testDate.Format("Mon, 02 Jan 2006 15:04:05 -0700") + "\r\n")
	b.WriteString("Message-ID: <degen-" + strings.ToLower(strings.ReplaceAll(subject, " ", "-")) + "@example.org>\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/mixed; boundary=\"bnd42\"\r\n")
	b.WriteString("\r\n")
	b.WriteString("--bnd42\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n")
	b.WriteString(textBody + "\r\n")
	// Parte degenere: octet-stream senza nome e senza corpo.
	b.WriteString("--bnd42\r\n")
	b.WriteString("Content-Type: application/octet-stream\r\n")
	b.WriteString("\r\n")
	b.WriteString("--bnd42\r\n")
	b.WriteString("Content-Type: application/pdf; name=\"" + filename + "\"\r\n")
	b.WriteString("Content-Disposition: attachment; filename=\"" + filename + "\"\r\n")
	b.WriteString("Content-Transfer-Encoding: base64\r\n")
	b.WriteString("\r\n")
	enc := base64.StdEncoding.EncodeToString(content)
	for len(enc) > 0 {
		n := min(76, len(enc))
		b.WriteString(enc[:n] + "\r\n")
		enc = enc[n:]
	}
	b.WriteString("--bnd42--\r\n")
	return b.Bytes()
}

// TestSaveAttachmentSkipsDegenerateParts (FIX PartIndex): con una parte
// degenere PRIMA dell'allegato reale, msg.Parse dà all'allegato reale
// PartIndex 0 e SaveAttachment(id, 0) deve scrivere i bytes dell'allegato
// REALE (prima della fix indicizzava gli attachment grezzi e scriveva la
// parte degenere vuota).
func TestSaveAttachmentSkipsDegenerateParts(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedRaw("INBOX",
		rawDegenerateMultipartMail("Dg <dg@example.org>", "Degenerate part", "corpo", "doc.pdf", pdfContent),
		false, false)
	store := connectStore(t, ts)
	id := msg.NewID("INBOX", uid)

	m, err := store.FetchMessage(t.Context(), id)
	if err != nil {
		t.Fatalf("FetchMessage: %v", err)
	}
	if len(m.Attachments) != 1 {
		t.Fatalf("m.Attachments = %d, voglio 1 (msg.Parse salta la parte degenere)", len(m.Attachments))
	}
	if m.Attachments[0].PartIndex != 0 || m.Attachments[0].Filename != "doc.pdf" {
		t.Errorf("m.Attachments[0] = %+v, voglio PartIndex 0 Filename doc.pdf", m.Attachments[0])
	}

	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := store.SaveAttachment(t.Context(), id, 0, path); err != nil {
		t.Fatalf("SaveAttachment(0): %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leggo %s: %v", path, err)
	}
	if !bytes.Equal(got, pdfContent) {
		t.Errorf("contenuto file = %q (%d byte), voglio l'allegato reale %q (%d byte): PartIndex divergente",
			got, len(got), pdfContent, len(pdfContent))
	}
}
