package mail

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

func newMock() *Mock {
	m := &Mock{Raw: map[msg.MessageID]map[int][]byte{}}
	add := func(uid uint32, from, subject string, unread bool, hoursAgo int, atts ...string) {
		id := msg.NewID("INBOX", uid)
		mm := &msg.Message{Ref: msg.MessageRef{
			ID: id, From: from, Subject: subject, Unread: unread,
			Date: time.Now().Add(-time.Duration(hoursAgo) * time.Hour),
		}}
		for i, name := range atts {
			mm.Attachments = append(mm.Attachments, msg.Attachment{
				PartIndex: i, Filename: name, ContentType: "application/octet-stream", Size: 4,
			})
			if m.Raw[id] == nil {
				m.Raw[id] = map[int][]byte{}
			}
			m.Raw[id][i] = []byte("data")
		}
		m.Msgs = append(m.Msgs, mm)
	}
	add(1, "alice@example.com", "Primo", true, 1)
	add(2, "bob@example.org", "Secondo", false, 2, "doc.pdf")
	add(3, "alice@example.com", "Terzo", true, 3)
	add(4, "carol@example.net", "Quarto", false, 4)
	add(5, "alice@example.com", "Quinto", false, 5)
	return m
}

func TestListSortedAndPaginated(t *testing.T) {
	m := newMock()
	got, err := m.ListMessages(context.Background(), msg.Query{}, Page{Offset: 1, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("attesi 2 ref, avuti %d", len(got))
	}
	if got[0].Subject != "Secondo" || got[1].Subject != "Terzo" {
		t.Fatalf("ordinamento data desc + offset rotto: %q %q", got[0].Subject, got[1].Subject)
	}
}

func TestListFilterUnread(t *testing.T) {
	m := newMock()
	got, _ := m.ListMessages(context.Background(), msg.Query{Unread: true}, Page{})
	if len(got) != 2 {
		t.Fatalf("non letti attesi 2, avuti %d", len(got))
	}
}

func TestSearchText(t *testing.T) {
	m := newMock()
	got, _ := m.Search(context.Background(), msg.Query{Text: "quinto"})
	if len(got) != 1 || got[0].Subject != "Quinto" {
		t.Fatalf("search text: %+v", got)
	}
	got, _ = m.Search(context.Background(), msg.Query{From: "alice@example.com"})
	if len(got) != 3 {
		t.Fatalf("search from: %d risultati", len(got))
	}
}

func TestFetchAndCacheCopySemantics(t *testing.T) {
	m := newMock()
	f1, err := m.FetchMessage(context.Background(), msg.NewID("INBOX", 1))
	if err != nil {
		t.Fatal(err)
	}
	f1.Ref.Unread = false // la copia non deve inquinare lo store
	f2, _ := m.FetchMessage(context.Background(), msg.NewID("INBOX", 1))
	if !f2.Ref.Unread {
		t.Fatal("FetchMessage deve restituire una copia difensiva")
	}
}

func TestMarkReadEmitsChange(t *testing.T) {
	m := newMock()
	ctx := context.Background()
	ch, err := m.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	id := msg.NewID("INBOX", 1)
	if err := m.MarkRead(ctx, []msg.MessageID{id}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-ch:
		if c.Kind != ChangeFlags || len(c.IDs) != 1 || c.IDs[0] != id {
			t.Fatalf("change inatteso: %+v", c)
		}
	default:
		t.Fatal("MarkRead deve emettere ChangeFlags su Watch")
	}
}

func TestArchiveRemoves(t *testing.T) {
	m := newMock()
	if err := m.Archive(context.Background(), []msg.MessageID{msg.NewID("INBOX", 3)}); err != nil {
		t.Fatal(err)
	}
	got, _ := m.ListMessages(context.Background(), msg.Query{}, Page{})
	if len(got) != 4 {
		t.Fatalf("dopo archive attesi 4, avuti %d", len(got))
	}
}

func TestSendRecords(t *testing.T) {
	m := newMock()
	out := msg.Outgoing{From: "me@example.com", To: []string{"a@example.com"}, Subject: "ciao"}
	if err := m.Send(context.Background(), out); err != nil {
		t.Fatal(err)
	}
	if len(m.Sent) != 1 || m.Sent[0].Subject != "ciao" {
		t.Fatalf("Sent = %+v", m.Sent)
	}
}

func TestSaveAttachmentWritesFile(t *testing.T) {
	m := newMock()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := m.SaveAttachment(context.Background(), msg.NewID("INBOX", 2), 0, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "data" {
		t.Fatalf("contenuto allegato sbagliato: %q %v", data, err)
	}
}

func TestErrorInjection(t *testing.T) {
	m := newMock()
	m.Err = errors.New("boom")
	if _, err := m.ListMessages(context.Background(), msg.Query{}, Page{}); err == nil {
		t.Fatal("Err iniettato deve propagarsi")
	}
}
