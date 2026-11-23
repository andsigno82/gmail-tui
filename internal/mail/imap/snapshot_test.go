package imap

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 16, Task 2: lo stub dello store IMAP implementa Snapshotter e
// restituisce sempre miss, senza rete (Task 8 lo riempie).
func TestStoreSnapshotStubsMiss(t *testing.T) {
	var sn mail.Snapshotter = NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p"})
	if _, ok := sn.CachedConversations(""); ok {
		t.Error("CachedConversations: atteso miss")
	}
	if _, ok := sn.CachedThread(1); ok {
		t.Error("CachedThread: atteso miss")
	}
	if m, ok := sn.CachedMessage(msg.NewID("INBOX", 1)); ok || m != nil {
		t.Error("CachedMessage: atteso miss")
	}
}
