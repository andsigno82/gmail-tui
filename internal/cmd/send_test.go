package cmd

import (
	"bytes"
	"context"
	"errors"
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
	args := SendArgs{To: "a@b", Bcc: "c@d", Subject: "Saluti"}
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
		To:       []string{"a@b"},
		Bcc:      []string{"c@d"},
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
		SendArgs{To: "a@b", Subject: "vuoto"}, strings.NewReader(""))
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
		SendArgs{To: "a@b", Subject: "s", Attach: []string{"foto.png", "doc.pdf"}}, strings.NewReader("x"))
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
			SendArgs{To: "a@b", Cc: "a@b", Bcc: "c@d"},
			"sent to 2 recipients\n",
		},
		{
			"duplicati nello stesso campo + spazi + vuoti",
			SendArgs{To: " a@b , a@b ,, c@d ", Subject: "s"},
			"sent to 2 recipients\n",
		},
		{
			"tre unici",
			SendArgs{To: "a@b", Cc: "c@d", Bcc: "e@f"},
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
		{To: "a@b", Cc: "x@"},
		{To: "a@b", Bcc: "<>"},
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
	if code := Send(&out, fs, config.Config{Address: "m@x"}, SendArgs{To: "a@b"}, strings.NewReader("b")); code != 1 {
		t.Errorf("Send exit = %d, want 1", code)
	}
}
