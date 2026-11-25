package tracestore

import (
	"bytes"
	"encoding/json"
	"strings"
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

// cacheEvents decodifica gli eventi "cache" del buffer di traccia.
func cacheEvents(t *testing.T, buf *bytes.Buffer) []trace.Event {
	t.Helper()
	var out []trace.Event
	for _, ln := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var e trace.Event
		if err := json.Unmarshal([]byte(ln), &e); err != nil {
			t.Fatal(err)
		}
		if e.K == "cache" {
			out = append(out, e)
		}
	}
	return out
}

// Piano 16, Task 7 (D16-T14): ogni lettura dalla cache emette un evento cache
// con operazione, esito, ruolo e conteggi.
func TestCachedEmettonoHitEMissConRuolo(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	hit := Wrap(snapMock{
		Mock:   newMock(),
		convs:  mail.ConversationsSnapshot{Rows: []mail.Conversation{{ThreadID: 7}, {ThreadID: 8}}},
		thread: mail.ThreadSnapshot{Refs: []msg.MessageRef{{ThreadID: 7}}},
		body:   &msg.Message{BodyText: "abc", BodyHTML: "de"},
	}).(mail.Snapshotter)
	hit.CachedConversations("")
	hit.CachedThread(7)
	hit.CachedMessage(msg.NewID("INBOX", 1))
	miss := Wrap(newMock()).(mail.Snapshotter)
	miss.CachedConversations("[Gmail]/Inviati")
	miss.CachedMessage(msg.NewID("INBOX", 1))

	want := []trace.Event{
		{K: "cache", Op: trace.OpSnapList, Mode: trace.ModeHit, Role: "inbox", N: 2},
		{K: "cache", Op: trace.OpSnapThread, Mode: trace.ModeHit, N: 1},
		{K: "cache", Op: trace.OpSnapBody, Mode: trace.ModeHit, N: 1, Bytes: 5},
		{K: "cache", Op: trace.OpSnapList, Mode: trace.ModeMiss, Role: "label"},
		{K: "cache", Op: trace.OpSnapBody, Mode: trace.ModeMiss},
	}
	got := cacheEvents(t, &buf)
	if len(got) != len(want) {
		t.Fatalf("eventi cache = %d, attesi %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Op != w.Op || g.Mode != w.Mode || (w.Role != "" && g.Role != w.Role) || g.N != w.N || g.Bytes != w.Bytes {
			t.Errorf("evento #%d = %+v, atteso %+v", i, g, w)
		}
	}
}

// Piano 16, Task 7 (D16-T14): gli eventi cache non portano dati personali
// (indirizzi, oggetti, corpi, nomi di label e cartelle, ID): solo ruoli ammessi.
func TestCacheEventsCarryNoPersonalData(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()
	in := snapMock{
		Mock: newMock(),
		convs: mail.ConversationsSnapshot{Rows: []mail.Conversation{{
			Latest:       msg.MessageRef{From: "anna@example.dev", Subject: "segreto-oggetto"},
			ThreadID:     7,
			Participants: []string{"anna@example.dev"},
			Labels:       []string{"EtichettaPrivata"},
		}}},
		thread: mail.ThreadSnapshot{Refs: []msg.MessageRef{{From: "anna@example.dev", Subject: "segreto-oggetto"}}},
		body:   &msg.Message{Ref: msg.MessageRef{ID: msg.NewID("INBOX", 1), From: "anna@example.dev"}, BodyText: "corpo-segreto"},
	}
	sn := Wrap(in).(mail.Snapshotter)
	sn.CachedConversations("EtichettaPrivata")
	sn.CachedConversations("")
	sn.CachedThread(7)
	sn.CachedMessage(msg.NewID("EtichettaPrivata", 9))
	sn.CachedMessage(msg.NewID("INBOX", 1))

	out := buf.String()
	for _, secret := range []string{"anna", "example", "segreto", "corpo", "EtichettaPrivata", "INBOX", "@", "Inviati"} {
		if strings.Contains(out, secret) {
			t.Errorf("la traccia contiene %q:\n%s", secret, out)
		}
	}
	roles := map[string]bool{"inbox": true, "important": true, "sent": true, "drafts": true, "all": true, "trash": true, "junk": true, "flagged": true, "label": true, "": true}
	cache := cacheEvents(t, &buf)
	if len(cache) != 5 {
		t.Fatalf("eventi cache = %d, attesi 5", len(cache))
	}
	for _, e := range cache {
		if !roles[e.Role] {
			t.Errorf("ruolo non ammesso: %q", e.Role)
		}
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
