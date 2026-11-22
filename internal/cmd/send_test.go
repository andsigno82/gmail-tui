package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// fakeSender cattura l'Outgoing ricevuto (per gli assert).
type fakeSender struct {
	called bool
	got    msg.Outgoing
	err    error
}

func (f *fakeSender) Send(_ context.Context, m msg.Outgoing) error {
	f.called = true
	if f.err != nil {
		return f.err
	}
	f.got = m
	return nil
}

// TestSendOK: Outgoing corretto (From da config, bcc negli argomenti),
// corpo letto dal reader, output "sent to 2 recipients", exit 0.
func TestSendOK(t *testing.T) {
	fs := &fakeSender{}
	cfg := config.Config{Address: "me@gmail.com"}
	args := SendArgs{To: "a@b.it", Bcc: "c@d.it", Subject: "Saluti"}
	var out bytes.Buffer

	code := Send(&out, fs, cfg, args, strings.NewReader("corpo\n"))
	if code != 0 {
		t.Fatalf("Send exit = %d, want 0", code)
	}
	if !fs.called {
		t.Fatal("sender non chiamato")
	}
	want := msg.Outgoing{
		From:     "me@gmail.com",
		To:       []string{"a@b.it"},
		Bcc:      []string{"c@d.it"},
		Subject:  "Saluti",
		BodyText: "corpo\n",
	}
	if !reflect.DeepEqual(fs.got, want) {
		t.Errorf("Outgoing = %+v, want %+v", fs.got, want)
	}
	if got := out.String(); got != "sent to 2 recipients\n" {
		t.Errorf("output = %q, want %q", got, "sent to 2 recipients\n")
	}
}

// TestSendEmptyBody: corpo vuoto è ok.
func TestSendEmptyBody(t *testing.T) {
	fs := &fakeSender{}
	var out bytes.Buffer
	code := Send(&out, fs, config.Config{Address: "me@gmail.com"},
		SendArgs{To: "a@b.it", Subject: "vuoto"}, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("Send exit = %d, want 0", code)
	}
	if fs.got.BodyText != "" {
		t.Errorf("BodyText = %q, want vuoto", fs.got.BodyText)
	}
}

// TestSendAttachments: gli allegati passano intatti all'Outgoing.
func TestSendAttachments(t *testing.T) {
	fs := &fakeSender{}
	var out bytes.Buffer
	code := Send(&out, fs, config.Config{Address: "me@gmail.com"},
		SendArgs{To: "a@b.it", Subject: "s", Attach: []string{"foto.png", "doc.pdf"}}, strings.NewReader("x"))
	if code != 0 {
		t.Fatalf("Send exit = %d, want 0", code)
	}
	if !reflect.DeepEqual(fs.got.Attachments, []string{"foto.png", "doc.pdf"}) {
		t.Errorf("Attachments = %v, want [foto.png doc.pdf]", fs.got.Attachments)
	}
}

// TestSendRecipientsCount: N = unici di To+Cc+Bcc (duplicati e vuoti
// scartati, spazi trimmati — stessa semantica di dedup del Sender SMTP).
func TestSendRecipientsCount(t *testing.T) {
	tests := []struct {
		name string
		args SendArgs
		want string
	}{
		{
			"duplicati fra campi",
			SendArgs{To: "a@b.it", Cc: "a@b.it", Bcc: "c@d.it"},
			"sent to 2 recipients\n",
		},
		{
			"duplicati nello stesso campo + spazi + vuoti",
			SendArgs{To: " a@b.it , a@b.it ,, c@d.it ", Subject: "s"},
			"sent to 2 recipients\n",
		},
		{
			"tre unici",
			SendArgs{To: "a@b.it", Cc: "c@d.it", Bcc: "e@f.it"},
			"sent to 3 recipients\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := &fakeSender{}
			var out bytes.Buffer
			if code := Send(&out, fs, config.Config{Address: "m@x"}, tt.args, strings.NewReader("b")); code != 0 {
				t.Fatalf("Send exit = %d, want 0", code)
			}
			if got := out.String(); got != tt.want {
				t.Errorf("output = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestSendNoRecipients: nessun destinatario ⇒ errore, sender NON chiamato.
func TestSendNoRecipients(t *testing.T) {
	fs := &fakeSender{}
	var out bytes.Buffer
	if code := Send(&out, fs, config.Config{Address: "m@x"}, SendArgs{Subject: "s"}, strings.NewReader("b")); code != 1 {
		t.Errorf("Send exit = %d, want 1", code)
	}
	if fs.called {
		t.Error("sender chiamato senza destinatari")
	}
}

// TestSendInvalidRecipient (Piano 7, Task 2): un indirizzo malformato ⇒
// exit 1 PRIMA del round-trip SMTP: il sender non viene chiamato.
func TestSendInvalidRecipient(t *testing.T) {
	for _, args := range []SendArgs{
		{To: "abc"},
		{To: "a@b.it", Cc: "x@"},
		{To: "a@b.it", Bcc: "<>"},
	} {
		fs := &fakeSender{}
		var out bytes.Buffer
		if code := Send(&out, fs, config.Config{Address: "m@x"}, args, strings.NewReader("b")); code != 1 {
			t.Errorf("%+v: Send exit = %d, want 1", args, code)
		}
		if fs.called {
			t.Errorf("%+v: sender chiamato con un destinatario non valido", args)
		}
	}
}

// TestSendError: sender in errore ⇒ exit 1.
func TestSendError(t *testing.T) {
	fs := &fakeSender{err: errors.New("smtp giù")}
	var out bytes.Buffer
	if code := Send(&out, fs, config.Config{Address: "m@x"}, SendArgs{To: "a@b.it"}, strings.NewReader("b")); code != 1 {
		t.Errorf("Send exit = %d, want 1", code)
	}
}

// captureStderr raccoglie ciò che fn scrive su os.Stderr (Piano 7.1, fix
// code-review: i messaggi di send vanno su stderr).
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	fn()
	os.Stderr = old
	_ = w.Close()
	b, _ := io.ReadAll(r)
	return string(b)
}

// TestSendInvalidRecipientMessage (Piano 7.1, fix code-review): l'errore cita il
// campo con l'etichetta del composer e l'indirizzo, come in ValidateAddresses.
func TestSendInvalidRecipientMessage(t *testing.T) {
	cases := []struct {
		args SendArgs
		want string
	}{
		{SendArgs{To: "abc"}, `send: A: indirizzo non valido "abc"`},
		{SendArgs{To: "a@b.it", Cc: "x@"}, `send: Cc: indirizzo non valido "x@"`},
		{SendArgs{To: "a@b.it", Bcc: "<>"}, `send: Ccn: indirizzo non valido "<>"`},
		{SendArgs{To: "a@x.it, b@@y"}, `send: A: indirizzo non valido "a@x.it, b@@y"`},
	}
	for _, tc := range cases {
		fs := &fakeSender{}
		var code int
		got := captureStderr(t, func() {
			code = Send(&bytes.Buffer{}, fs, config.Config{Address: "m@x.it"}, tc.args, strings.NewReader("b"))
		})
		if !strings.Contains(got, tc.want) || code != 1 {
			t.Errorf("%+v: stderr = %q (exit %d), want %q exit 1", tc.args, got, code, tc.want)
		}
	}
}

// TestSendQuotedLocalPart (Piano 7.1, fix code-review): una local part quotata
// passa come tale al sender e conta un solo destinatario.
func TestSendQuotedLocalPart(t *testing.T) {
	fs := &fakeSender{}
	var out bytes.Buffer
	code := Send(&out, fs, config.Config{Address: "me@gmail.com"},
		SendArgs{To: `"a b"@x.it`, Subject: "s"}, strings.NewReader("b"))
	if code != 0 {
		t.Fatalf("Send exit = %d, want 0", code)
	}
	if want := []string{`"a b"@x.it`}; !reflect.DeepEqual(fs.got.To, want) {
		t.Errorf("To = %q, want %q", fs.got.To, want)
	}
	if got := out.String(); got != "sent to 1 recipients\n" {
		t.Errorf("output = %q, want 1 destinatario", got)
	}
}

// TestBareRecipientsReturnsError (Piano 7.1, fix code-review): un indirizzo non
// parsabile è un errore, non viene scartato in silenzio dal conteggio.
func TestBareRecipientsReturnsError(t *testing.T) {
	if _, err := bareRecipients(msg.Outgoing{To: []string{"abc"}}); err == nil {
		t.Error("bareRecipients(abc): err = nil, want errore")
	}
}

// TestSendDisplayNameRecipientIsOne (Piano 7.1, Task 3): una virgola dentro
// il nome quotato non spezza il destinatario; il nome resta nell'Outgoing.
func TestSendDisplayNameRecipientIsOne(t *testing.T) {
	fs := &fakeSender{}
	var out bytes.Buffer
	code := Send(&out, fs, config.Config{Address: "me@gmail.com"},
		SendArgs{To: `"Rossi, Mario" <m@x.it>`, Subject: "s"}, strings.NewReader("b"))
	if code != 0 {
		t.Fatalf("Send exit = %d, want 0", code)
	}
	if want := []string{`"Rossi, Mario" <m@x.it>`}; !reflect.DeepEqual(fs.got.To, want) {
		t.Errorf("To = %q, want %q", fs.got.To, want)
	}
	if got := out.String(); got != "sent to 1 recipients\n" {
		t.Errorf("output = %q, want un solo destinatario", got)
	}
}

// TestSendIDNNormalized (Piano 7.1, Task 3): il dominio IDN arriva al sender
// in punycode, e la stessa casella in forma IDN e punycode conta una volta.
func TestSendIDNNormalized(t *testing.T) {
	fs := &fakeSender{}
	var out bytes.Buffer
	code := Send(&out, fs, config.Config{Address: "me@gmail.com"},
		SendArgs{To: "x@café.it", Cc: "x@xn--caf-dma.it", Subject: "s"}, strings.NewReader("b"))
	if code != 0 {
		t.Fatalf("Send exit = %d, want 0", code)
	}
	if want := []string{"x@xn--caf-dma.it"}; !reflect.DeepEqual(fs.got.To, want) {
		t.Errorf("To = %q, want %q", fs.got.To, want)
	}
	if got := out.String(); got != "sent to 1 recipients\n" {
		t.Errorf("output = %q, want 1 destinatario unico", got)
	}
}
