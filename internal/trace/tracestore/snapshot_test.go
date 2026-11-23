package tracestore

import (
	"bytes"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

// snapMock e un Mock che implementa anche mail.Snapshotter.
type snapMock struct {
	*mail.Mock
	convs  mail.ConversationsSnapshot
	thread mail.ThreadSnapshot
	body   *msg.Message
}

func (s snapMock) CachedConversations(string) (mail.ConversationsSnapshot, bool) {
	return s.convs, true
}
func (s snapMock) CachedThread(uint64) (mail.ThreadSnapshot, bool) { return s.thread, true }
func (s snapMock) CachedMessage(msg.MessageID) (*msg.Message, bool) {
	return s.body, true
}

// Piano 16, Task 2: il wrapper inoltra Snapshotter allo store interno.
func TestWrapForwardsSnapshotter(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	in := snapMock{
		Mock:   newMock(),
		convs:  mail.ConversationsSnapshot{Rows: []mail.Conversation{{ThreadID: 7}}, Limited: true},
		thread: mail.ThreadSnapshot{Refs: []msg.MessageRef{{ThreadID: 7}}},
		body:   &msg.Message{BodyText: "x"},
	}
	sn, ok := Wrap(in).(mail.Snapshotter)
	if !ok {
		t.Fatal("il wrapper deve esporre mail.Snapshotter")
	}
	if c, ok := sn.CachedConversations(""); !ok || len(c.Rows) != 1 || !c.Limited {
		t.Errorf("CachedConversations = %+v, %v", c, ok)
	}
	if th, ok := sn.CachedThread(7); !ok || len(th.Refs) != 1 {
		t.Errorf("CachedThread = %+v, %v", th, ok)
	}
	if m, ok := sn.CachedMessage(msg.NewID("INBOX", 1)); !ok || m == nil || m.BodyText != "x" {
		t.Errorf("CachedMessage = %+v, %v", m, ok)
	}
}

// Piano 16, Task 2: senza Snapshotter nello store interno (Mock) il wrapper
// risponde miss, come se l'interfaccia mancasse.
func TestWrapWithoutSnapshotterMisses(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	sn, ok := Wrap(newMock()).(mail.Snapshotter)
	if !ok {
		t.Fatal("il wrapper deve esporre mail.Snapshotter")
	}
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
