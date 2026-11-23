package tracestore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

func events(t *testing.T, buf *bytes.Buffer) []trace.Event {
	t.Helper()
	var out []trace.Event
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatal(err)
		}
		if e.K == "store" {
			out = append(out, e)
		}
	}
	return out
}

func newMock() *mail.Mock {
	return &mail.Mock{
		Msgs: []*msg.Message{
			{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), From: "anna@example.dev", Subject: "segreto-oggetto"}, BodyText: "corpo-segreto", BodyHTML: "<p>html</p>"},
		},
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Inviati", Role: "sent"},
			{Name: "EtichettaPrivata"},
		},
	}
}

func TestWrapMisuraEConta(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	s := Wrap(newMock())
	ctx := context.Background()

	if err := s.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListMailboxes(ctx); err != nil {
		t.Fatal(err)
	}
	convs, err := s.ListConversations(ctx, msg.Query{}, mail.Page{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListConversations(ctx, msg.Query{Label: "[Gmail]/Inviati"}, mail.Page{Limit: 50}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ListConversations(ctx, msg.Query{Label: "EtichettaPrivata"}, mail.Page{Limit: 50}); err != nil {
		t.Fatal(err)
	}
	m, err := s.FetchMessage(ctx, msg.NewID("INBOX", 1))
	if err != nil || m == nil {
		t.Fatal(err)
	}
	_ = convs
	sn := s.(mail.Snapshotter)
	sn.CachedConversations("EtichettaPrivata")
	sn.CachedMessage(msg.NewID("INBOX", 1))
	if n := len(cacheEvents(t, &buf)); n != 2 {
		t.Errorf("eventi cache = %d, attesi 2", n)
	}

	ev := events(t, &buf)
	if len(ev) != 6 {
		t.Fatalf("attesi 6 eventi store, ho %d", len(ev))
	}
	if ev[0].Op != "Connect" || ev[1].Op != "ListMailboxes" || ev[1].N != 3 {
		t.Errorf("eventi iniziali sbagliati: %+v %+v", ev[0], ev[1])
	}
	for i, want := range []string{"inbox", "sent", "label"} {
		if ev[2+i].Op != "ListConversations" || ev[2+i].Role != want {
			t.Errorf("ListConversations #%d: %+v, ruolo atteso %q", i, ev[2+i], want)
		}
	}
	if ev[2].N != 1 {
		t.Errorf("N = %d, atteso 1", ev[2].N)
	}
	if f := ev[5]; f.Op != "FetchMessage" || f.Bytes != len("corpo-segreto")+len("<p>html</p>") {
		t.Errorf("FetchMessage: %+v", f)
	}
	for _, secret := range []string{"anna@example", "segreto", "EtichettaPrivata", "Inviati", "INBOX"} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("la traccia contiene %q:\n%s", secret, buf.String())
		}
	}
}

func TestWrapSegnalaErroreSenzaTesto(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	mk := newMock()
	mk.Err = errors.New("boom mario@example.com")
	s := Wrap(mk)
	if err := s.Connect(context.Background()); err == nil {
		t.Fatal("errore dell'inner non propagato")
	}
	ev := events(t, &buf)
	if len(ev) != 1 || !ev[0].Err {
		t.Fatalf("atteso 1 evento con err: %+v", ev)
	}
	if strings.Contains(buf.String(), "mario") {
		t.Error("il testo dell'errore e nella traccia")
	}
}

func TestWrapScanLimitNonEErrore(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	s := Wrap(&limited{newMock()})
	_, err := s.ListConversations(context.Background(), msg.Query{}, mail.Page{})
	if !errors.Is(err, mail.ErrScanLimit) {
		t.Fatalf("errore non propagato: %v", err)
	}
	if ev := events(t, &buf); len(ev) != 1 || ev[0].Err {
		t.Errorf("ErrScanLimit non e un errore: %+v", ev)
	}
}

type limited struct{ *mail.Mock }

func (limited) ListConversations(context.Context, msg.Query, mail.Page) ([]mail.Conversation, error) {
	return nil, mail.ErrScanLimit
}

func TestWrapInoltraGmailExt(t *testing.T) {
	mk := newMock()
	s := Wrap(mk)
	g, ok := s.(interface{ GmailExt() bool })
	if !ok || !g.GmailExt() {
		t.Fatal("GmailExt deve passare attraverso il wrapper")
	}
	mk.NoGmail = true
	if g.GmailExt() {
		t.Error("GmailExt deve seguire lo store interno")
	}
}

func TestWrapSenzaTracciaRitornaLoStoreInterno(t *testing.T) {
	trace.Uninstall()
	mk := newMock()
	if Wrap(mk) != mail.MailStore(mk) {
		t.Error("senza traccia Wrap non deve avvolgere")
	}
}
