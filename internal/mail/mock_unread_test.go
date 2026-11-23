package mail

import (
	"context"
	"errors"
	"testing"
)

// Piano 15, Task 2: contratti Mailbox.Delimiter e UnreadCounts (solo firme).

func TestMailboxDelimiter(t *testing.T) {
	mb := Mailbox{Name: "Lavoro/Clienti", Delimiter: "/"}
	if mb.Delimiter != "/" {
		t.Fatalf("Delimiter = %q, voglio %q", mb.Delimiter, "/")
	}
}

func TestMockUnreadCountsVuoto(t *testing.T) {
	var s MailStore = &Mock{}
	got, err := s.UnreadCounts(context.Background())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("UnreadCounts = %#v, voglio mappa vuota non nil", got)
	}
}

func TestMockUnreadCountsErr(t *testing.T) {
	boom := errors.New("boom")
	m := &Mock{Err: boom}
	if _, err := m.UnreadCounts(context.Background()); !errors.Is(err, boom) {
		t.Fatalf("err = %v, voglio %v", err, boom)
	}
}
