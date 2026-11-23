package imap

import (
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 16, Task 2: stub di mail.Snapshotter, sempre miss. Il Task 8 lo
// riempie leggendo la cache (sola lettura, senza gate ne rete).
var _ mail.Snapshotter = (*Store)(nil)

// CachedConversations: stub, sempre miss (Piano 16, Task 2).
func (s *Store) CachedConversations(mailbox string) (mail.ConversationsSnapshot, bool) {
	return mail.ConversationsSnapshot{}, false
}

// CachedThread: stub, sempre miss (Piano 16, Task 2).
func (s *Store) CachedThread(threadID uint64) (mail.ThreadSnapshot, bool) {
	return mail.ThreadSnapshot{}, false
}

// CachedMessage: stub, sempre miss (Piano 16, Task 2).
func (s *Store) CachedMessage(id msg.MessageID) (*msg.Message, bool) {
	return nil, false
}
