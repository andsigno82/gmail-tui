package cache

import (
	"path/filepath"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 8, review (punto 3): lo stesso messaggio Gmail sta in più mailbox (INBOX e
// Tutti i messaggi). UpdateGmailCopies cambia tutte le copie con lo stesso GmailID,
// così un flag cambiato da una mailbox non resta stantio nelle altre.
func TestUpdateGmailCopiesAggiornaTutteLeMailbox(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = c.Close() }()

	inbox := &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 4), GmailID: 1004, Unread: true}}
	all := &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/All Mail", 9), GmailID: 1004, Unread: true}}
	other := &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 5), GmailID: 1005, Unread: true}}
	for _, m := range []*msg.Message{inbox, all, other} {
		if err := c.SetMessage(boxOfID(m.Ref.ID), 1, m); err != nil {
			t.Fatalf("SetMessage: %v", err)
		}
	}

	err = c.UpdateGmailCopies(1004, func(m *msg.Message) bool {
		if !m.Ref.Unread {
			return false
		}
		m.Ref.Unread = false
		return true
	})
	if err != nil {
		t.Fatalf("UpdateGmailCopies: %v", err)
	}
	for _, id := range []msg.MessageID{msg.NewID("INBOX", 4), msg.NewID("[Gmail]/All Mail", 9)} {
		box, uid, _ := id.Split()
		got, ok, err := c.GetMessage(box, uid)
		if err != nil || !ok {
			t.Fatalf("GetMessage %s: ok=%v err=%v", id, ok, err)
		}
		if got.Ref.Unread {
			t.Errorf("copia %s ancora non letta: il flag deve seguire il GmailID", id)
		}
	}
	box, uid, _ := other.Ref.ID.Split()
	if got, _, _ := c.GetMessage(box, uid); got == nil || !got.Ref.Unread {
		t.Errorf("il messaggio con altro GmailID non deve cambiare")
	}
}

// boxOfID è la mailbox di un ID di test (parte prima dei due punti).
func boxOfID(id msg.MessageID) string {
	box, _, _ := id.Split()
	return box
}
