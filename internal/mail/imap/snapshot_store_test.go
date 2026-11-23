package imap

// Piano 16, Task 8: istantanee write-through, regola d'epoca (R3) e patch dopo le
// mutazioni, su server scriptato Gmail (UIDVALIDITY annunciata) e su imapmemserver.

import (
	"bytes"
	"context"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/mail/cache"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/trace"
)

const (
	snapAllMail = "[Gmail]/All Mail"
	snapInboxEp = 7
	snapAllEp   = 9
)

// snapBoxes e il corpus: INBOX con thread 100 (uid 2 letto, uid 3 NON letto) e 200 (uid 1),
// All Mail con in piu uid 4 del thread 100. X-GM-MSGID = 1000+uid.
func snapBoxes() map[string][]gmailFetch {
	inbox := []gmailFetch{
		{uid: 1, thread: 200, login: "cy", date: "01-Nov-2026", read: true},
		{uid: 2, thread: 100, login: "ann", date: "02-Nov-2026", read: true},
		{uid: 3, thread: 100, login: "bob", date: "03-Nov-2026", read: false},
	}
	all := append(append([]gmailFetch{}, inbox...), gmailFetch{uid: 4, thread: 100, login: "me", date: "04-Nov-2026", read: true})
	return map[string][]gmailFetch{"INBOX": inbox, snapAllMail: all}
}

// snapStore collega uno Store con cache al server scriptato.
func snapStore(t *testing.T, c *cache.Cache) *Store {
	t.Helper()
	addr := scriptedGmailServerOpts(t, snapBoxes(), gmailServerOpts{
		epochs: map[string]uint32{"INBOX": snapInboxEp, snapAllMail: snapAllEp},
		trash:  true,
	})
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true, Cache: c})
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// snapFilled e uno Store con l'istantanea di INBOX e quella del thread 100 gia scritte.
func snapFilled(t *testing.T) (*Store, *cache.Cache) {
	t.Helper()
	c := openTestCache(t)
	s := snapStore(t, c)
	ctx := context.Background()
	if _, err := s.ListConversations(ctx, msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if _, err := s.ThreadMessages(ctx, 100); err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	return s, c
}

func rowByThread(rows []mail.Conversation, thread uint64) (mail.Conversation, bool) {
	for _, r := range rows {
		if r.ThreadID == thread {
			return r, true
		}
	}
	return mail.Conversation{}, false
}

// Piano 16, Task 8: i metodi Cached* leggono la cache senza connessione ne gate, anche
// su uno Store mai connesso.
func TestCachedConversationsBeforeConnect(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	var accepts atomic.Int32
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			_ = conn.Close()
		}
	}()

	c := openTestCache(t)
	at := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)
	rows := []mail.Conversation{{Latest: msg.MessageRef{ID: "INBOX:3", ThreadID: 100}, ThreadID: 100, Count: 2}}
	if err := c.PutList("", snapInboxEp, rows, true, at); err != nil {
		t.Fatalf("PutList: %v", err)
	}
	refs := []msg.MessageRef{{ID: "[Gmail]/All Mail:3", GmailID: 1003, ThreadID: 100}}
	if err := c.PutThread(100, snapAllMail, snapAllEp, refs, at); err != nil {
		t.Fatalf("PutThread: %v", err)
	}
	if err := c.SetMessage("INBOX", snapInboxEp, &msg.Message{Ref: msg.MessageRef{ID: "INBOX:3", Subject: "s"}, BodyText: "corpo"}); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}

	s := NewStore(Options{Addr: ln.Addr().String(), Username: "u", Password: "p", Insecure: true, Cache: c})
	snap, ok := s.CachedConversations("")
	if !ok || len(snap.Rows) != 1 || snap.Rows[0].ThreadID != 100 || !snap.Limited || !snap.SavedAt.Equal(at) {
		t.Fatalf("CachedConversations = %+v, %v", snap, ok)
	}
	ts, ok := s.CachedThread(100)
	if !ok || len(ts.Refs) != 1 || ts.Refs[0].GmailID != 1003 {
		t.Fatalf("CachedThread = %+v, %v", ts, ok)
	}
	m, ok := s.CachedMessage("INBOX:3")
	if !ok || m.BodyText != "corpo" {
		t.Fatalf("CachedMessage = %+v, %v", m, ok)
	}
	if _, ok := s.CachedConversations("Altra"); ok {
		t.Error("CachedConversations di una cartella senza istantanea: atteso miss")
	}
	time.Sleep(20 * time.Millisecond)
	if n := accepts.Load(); n != 0 {
		t.Errorf("connessioni aperte = %d, voglio 0 (sola lettura dal disco)", n)
	}

	// Senza cache: sempre miss.
	bare := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p"})
	if _, ok := bare.CachedConversations(""); ok {
		t.Error("senza cache: atteso miss")
	}
}

// Il gate tenuto da un'op bloccata non ferma i metodi Cached*.
func TestCachedMethodsDoNotTakeGate(t *testing.T) {
	c := openTestCache(t)
	rows := []mail.Conversation{{Latest: msg.MessageRef{ID: "INBOX:1"}, ThreadID: 1, Count: 1}}
	if err := c.PutList("", snapInboxEp, rows, false, time.Now()); err != nil {
		t.Fatalf("PutList: %v", err)
	}
	s := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p", Cache: c})
	release := s.gate.acquire()
	defer release()

	done := make(chan bool, 1)
	go func() {
		_, ok := s.CachedConversations("")
		s.CachedThread(1)
		s.CachedMessage("INBOX:1")
		done <- ok
	}()
	select {
	case ok := <-done:
		if !ok {
			t.Error("CachedConversations: atteso hit")
		}
	case <-time.After(time.Second):
		t.Fatal("i metodi Cached* sono bloccati dal gate")
	}
}

func TestListConversationsWritesSnapshot(t *testing.T) {
	c := openTestCache(t)
	s := snapStore(t, c)
	if _, ok := s.CachedConversations(""); ok {
		t.Fatal("prima della lista l'istantanea non deve esistere")
	}
	convs, err := s.ListConversations(context.Background(), msg.Query{}, mail.Page{})
	if err != nil || len(convs) != 2 {
		t.Fatalf("ListConversations = %d righe, %v", len(convs), err)
	}
	snap, _, _, ok := c.GetList("")
	if !ok || len(snap) != 2 {
		t.Fatalf("istantanea dopo la lista: %d righe, ok=%v", len(snap), ok)
	}
	if r, _ := rowByThread(snap, 100); r.Latest.ID != "INBOX:3" || r.Count != 3 || !r.Unread {
		t.Errorf("riga 100 in istantanea: %+v", r)
	}
	if v, ok, _ := c.UIDValidity("INBOX"); !ok || v != snapInboxEp {
		t.Errorf("UIDVALIDITY registrata = %d, %v; voglio %d (R3)", v, ok, snapInboxEp)
	}
	// Una seconda istanza sulla stessa cache la rivede senza rete.
	again := NewStore(Options{Addr: "127.0.0.1:1", Username: "u", Password: "p", Cache: c})
	if snap, ok := again.CachedConversations(""); !ok || len(snap.Rows) != 2 {
		t.Errorf("CachedConversations = %+v, %v", snap, ok)
	}
}

// Senza X-GM-EXT-1 (memserver) la prima pagina si salva lo stesso, con l'epoca della SELECT.
func TestListConversationsNoGmailWritesSnapshot(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store, c := connectStoreCache(t, ts)

	convs, err := store.ListConversations(t.Context(), msg.Query{}, mail.Page{})
	if err != nil || len(convs) != 5 {
		t.Fatalf("ListConversations = %d, %v", len(convs), err)
	}
	rows, _, _, ok := c.GetList("")
	if !ok || len(rows) != 5 {
		t.Fatalf("istantanea = %d righe, ok=%v; voglio 5", len(rows), ok)
	}
	if _, ok, _ := c.UIDValidity("INBOX"); !ok {
		t.Error("UIDVALIDITY di INBOX non registrata")
	}
}

func TestSearchAndPageTwoDoNotWriteSnapshot(t *testing.T) {
	c := openTestCache(t)
	s := snapStore(t, c)
	ctx := context.Background()

	// pagina successiva
	if _, err := s.ListConversations(ctx, msg.Query{}, mail.Page{Offset: 1, Limit: 1}); err != nil {
		t.Fatalf("pagina 2: %v", err)
	}
	// query non di sola cartella
	if _, err := s.ListConversations(ctx, msg.Query{Unread: true}, mail.Page{}); err != nil {
		t.Fatalf("unread: %v", err)
	}
	if _, err := s.ListConversations(ctx, msg.Query{Raw: "is:unread"}, mail.Page{}); err != nil {
		t.Fatalf("raw: %v", err)
	}
	if _, err := s.Search(ctx, msg.Query{From: "bob"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
	if _, _, _, ok := c.GetList(""); ok {
		t.Error("ricerche e pagine successive non devono scrivere l'istantanea della cartella")
	}
	// Una cartella label con prima pagina di sola cartella si salva sotto il suo nome.
	if _, err := s.ListConversations(ctx, msg.Query{Label: snapAllMail}, mail.Page{}); err != nil {
		t.Fatalf("label: %v", err)
	}
	if _, _, _, ok := c.GetList(snapAllMail); !ok {
		t.Error("la prima pagina di una cartella label si salva")
	}
}

func TestThreadMessagesWritesSnapshot(t *testing.T) {
	c := openTestCache(t)
	s := snapStore(t, c)
	if _, err := s.ThreadMessages(context.Background(), 100); err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	refs, _, ok := c.GetThread(100)
	if !ok || len(refs) != 3 {
		t.Fatalf("istantanea del thread = %d ref, ok=%v; voglio 3", len(refs), ok)
	}
	if v, ok, _ := c.UIDValidity(snapAllMail); !ok || v != snapAllEp {
		t.Errorf("UIDVALIDITY di Tutti i messaggi = %d, %v; voglio %d", v, ok, snapAllEp)
	}
	if ts, ok := s.CachedThread(100); !ok || len(ts.Refs) != 3 {
		t.Errorf("CachedThread = %+v, %v", ts, ok)
	}
}

// R3: la UIDVALIDITY nuova del server scarta corpi e istantanee della mailbox PRIMA di
// restituire righe; un thread salvato su Tutti i messaggi cade se cambia la loro epoca.
func TestNewUIDValidityDropsBodiesAndSnapshots(t *testing.T) {
	c := openTestCache(t)
	old := &msg.Message{Ref: msg.MessageRef{ID: "INBOX:1", GmailID: 1001, ThreadID: 200}, BodyText: "vecchio"}
	if err := c.SetMessage("INBOX", snapInboxEp+1, old); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	oldRows := []mail.Conversation{{Latest: msg.MessageRef{ID: "INBOX:1"}, ThreadID: 200, Count: 1}}
	if err := c.PutList("", snapInboxEp+1, oldRows, false, time.Now()); err != nil {
		t.Fatalf("PutList: %v", err)
	}
	oldThread := []msg.MessageRef{{ID: msg.NewID(snapAllMail, 1), GmailID: 1001, ThreadID: 77}}
	if err := c.SetMessage(snapAllMail, snapAllEp+1, &msg.Message{Ref: oldThread[0], BodyText: "x"}); err != nil {
		t.Fatalf("SetMessage all: %v", err)
	}
	if err := c.PutThread(77, snapAllMail, snapAllEp+1, oldThread, time.Now()); err != nil {
		t.Fatalf("PutThread: %v", err)
	}

	s := snapStore(t, c)
	// ListMessages non scrive istantanee: se la lista vecchia sparisce e merito della regola d'epoca.
	if _, err := s.ListMessages(context.Background(), msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if _, _, _, ok := c.GetList(""); ok {
		t.Error("l'istantanea d'epoca vecchia sopravvive alla SELECT con UIDVALIDITY nuova")
	}
	if _, ok, _ := c.GetMessage("INBOX", 1); ok {
		t.Error("il corpo d'epoca vecchia sopravvive alla SELECT con UIDVALIDITY nuova")
	}
	if _, err := s.ThreadMessages(context.Background(), 100); err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	if _, _, ok := c.GetThread(77); ok {
		t.Error("il thread salvato su Tutti i messaggi d'epoca vecchia sopravvive")
	}
	if _, ok, _ := c.GetMessage(snapAllMail, 1); ok {
		t.Error("il corpo di Tutti i messaggi d'epoca vecchia sopravvive")
	}
}

// R3: dopo un cambio di epoca FetchMessage non serve il corpo vecchio dalla cache: va in rete.
func TestFetchHitDoesNotServeOtherEpoch(t *testing.T) {
	c := openTestCache(t)
	stale := &msg.Message{Ref: msg.MessageRef{ID: "INBOX:3", GmailID: 1003, ThreadID: 100}, BodyText: "CORPO D'EPOCA VECCHIA"}
	if err := c.SetMessage("INBOX", snapInboxEp+1, stale); err != nil {
		t.Fatalf("SetMessage: %v", err)
	}
	s := snapStore(t, c)
	ctx := context.Background()
	if _, err := s.ListConversations(ctx, msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	m, err := s.FetchMessage(ctx, "INBOX:3")
	if err != nil {
		t.Fatalf("FetchMessage: %v", err)
	}
	if m.BodyText == stale.BodyText {
		t.Fatal("servito il corpo d'epoca vecchia")
	}
	// E ora il corpo fresco e in cache con l'epoca nuova.
	if got, ok := s.CachedMessage("INBOX:3"); !ok || got.BodyText != m.BodyText {
		t.Errorf("CachedMessage dopo il fetch = %+v, %v", got, ok)
	}
}

func TestFetchTracesCacheHitAndMiss(t *testing.T) {
	var buf bytes.Buffer
	trace.Install(trace.New(&buf, ""))
	defer trace.Uninstall()

	c := openTestCache(t)
	s := snapStore(t, c)
	ctx := context.Background()
	if _, err := s.FetchMessage(ctx, "INBOX:3"); err != nil {
		t.Fatalf("FetchMessage miss: %v", err)
	}
	if _, err := s.FetchMessage(ctx, "INBOX:3"); err != nil {
		t.Fatalf("FetchMessage hit: %v", err)
	}
	out := buf.String()
	if n := bytes.Count([]byte(out), []byte(`"op":"fetch_body"`)); n != 2 {
		t.Fatalf("eventi fetch_body = %d, voglio 2\n%s", n, out)
	}
	if n := bytes.Count([]byte(out), []byte(`"mode":"miss"`)); n != 1 {
		t.Errorf("eventi miss = %d, voglio 1\n%s", n, out)
	}
	if n := bytes.Count([]byte(out), []byte(`"mode":"hit"`)); n != 1 {
		t.Errorf("eventi hit = %d, voglio 1\n%s", n, out)
	}
}

// D16-T10: il post-filtro has:attachment non scrive corpi in cache.
func TestHasAttachmentSearchDoesNotFillCache(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	uid := ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, false)
	store, c := connectStoreCache(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{HasAttachment: true})
	if err != nil || len(refs) != 1 {
		t.Fatalf("Search = %d, %v", len(refs), err)
	}
	for u := uint32(1); u <= uid; u++ {
		if _, ok, _ := c.GetMessage("INBOX", u); ok {
			t.Errorf("INBOX:%d in cache dopo una ricerca has:attachment (D16-T10)", u)
		}
	}
	// Ma un corpo aperto davvero (FetchMessage) entra.
	if _, err := store.FetchMessage(t.Context(), msg.NewID("INBOX", uid)); err != nil {
		t.Fatalf("FetchMessage: %v", err)
	}
	if _, ok, _ := c.GetMessage("INBOX", uid); !ok {
		t.Error("FetchMessage deve scrivere il corpo in cache")
	}
}

func TestArchiveRemovesRowFromSnapshot(t *testing.T) {
	s, c := snapFilled(t)
	if err := s.Archive(context.Background(), []msg.MessageID{"INBOX:3"}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	rows, _, _, _ := c.LookupList("")
	if _, ok := rowByThread(rows, 100); ok {
		t.Error("la riga archiviata e ancora nell'istantanea")
	}
	if _, ok := rowByThread(rows, 200); !ok {
		t.Error("la riga non archiviata e sparita")
	}
}

func TestTrashRemovesFromThreadSnapshots(t *testing.T) {
	s, c := snapFilled(t)
	if _, err := s.Trash(context.Background(), []msg.MessageID{"INBOX:3"}); err != nil {
		t.Fatalf("Trash: %v", err)
	}
	rows, _, _, _ := c.LookupList("")
	if _, ok := rowByThread(rows, 100); ok {
		t.Error("la riga cestinata e ancora nell'istantanea di lista")
	}
	refs, _, ok := c.GetThread(100)
	if !ok || len(refs) != 2 {
		t.Fatalf("istantanea del thread = %d ref, ok=%v; voglio 2 (senza il cestinato)", len(refs), ok)
	}
	for _, r := range refs {
		if r.GmailID == 1003 {
			t.Error("il messaggio cestinato e ancora nell'istantanea del thread")
		}
	}
}

func TestMarkReadPatchesSnapshots(t *testing.T) {
	s, c := snapFilled(t)
	if err := s.MarkRead(context.Background(), []msg.MessageID{"INBOX:3"}, true); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	rows, _, _, _ := c.LookupList("")
	r, ok := rowByThread(rows, 100)
	if !ok || r.Unread || r.Latest.Unread {
		t.Errorf("riga 100 dopo MarkRead: %+v, ok=%v; voglio letta", r, ok)
	}
	refs, _, _ := c.GetThread(100)
	for _, ref := range refs {
		if ref.GmailID == 1003 && ref.Unread {
			t.Error("il ref 1003 del thread e ancora non letto")
		}
	}
}

func TestStarPatchesSnapshots(t *testing.T) {
	s, c := snapFilled(t)
	if err := s.Star(context.Background(), []msg.MessageID{"INBOX:3"}, true); err != nil {
		t.Fatalf("Star: %v", err)
	}
	rows, _, _, _ := c.LookupList("")
	if r, _ := rowByThread(rows, 100); !r.Latest.Starred {
		t.Errorf("riga 100: Latest.Starred = false dopo Star")
	}
	refs, _, _ := c.GetThread(100)
	starred := 0
	for _, ref := range refs {
		if ref.Starred {
			starred++
			if ref.GmailID != 1003 {
				t.Errorf("stella sul ref sbagliato: %d", ref.GmailID)
			}
		}
	}
	if starred != 1 {
		t.Errorf("ref con stella = %d, voglio 1", starred)
	}
}

func TestModifyLabelsPatchesSnapshots(t *testing.T) {
	s, c := snapFilled(t)
	ctx := context.Background()
	if err := s.ModifyLabels(ctx, []msg.MessageID{"INBOX:3"}, []string{"Lavoro"}, nil); err != nil {
		t.Fatalf("ModifyLabels add: %v", err)
	}
	rows, _, _, _ := c.LookupList("")
	r, _ := rowByThread(rows, 100)
	if len(r.Latest.Labels) != 1 || r.Latest.Labels[0] != "Lavoro" || len(r.Labels) != 1 || r.Labels[0] != "Lavoro" {
		t.Errorf("riga 100 dopo +Lavoro: Latest.Labels=%v Labels=%v", r.Latest.Labels, r.Labels)
	}
	refs, _, _ := c.GetThread(100)
	for _, ref := range refs {
		has := len(ref.Labels) == 1 && ref.Labels[0] == "Lavoro"
		if (ref.GmailID == 1003) != has {
			t.Errorf("ref %d: Labels=%v", ref.GmailID, ref.Labels)
		}
	}
	if err := s.ModifyLabels(ctx, []msg.MessageID{"INBOX:3"}, nil, []string{"Lavoro"}); err != nil {
		t.Fatalf("ModifyLabels remove: %v", err)
	}
	rows, _, _, _ = c.LookupList("")
	if r, _ := rowByThread(rows, 100); len(r.Latest.Labels) != 0 || len(r.Labels) != 0 {
		t.Errorf("riga 100 dopo -Lavoro: Latest.Labels=%v Labels=%v", r.Latest.Labels, r.Labels)
	}
}

// ModifyLabels con rimozione di \Inbox e una archiviazione: la riga sparisce da INBOX.
func TestModifyLabelsRemoveInboxRemovesRow(t *testing.T) {
	s, c := snapFilled(t)
	if err := s.ModifyLabels(context.Background(), []msg.MessageID{"INBOX:3"}, nil, []string{`\Inbox`}); err != nil {
		t.Fatalf("ModifyLabels: %v", err)
	}
	rows, _, _, _ := c.LookupList("")
	if _, ok := rowByThread(rows, 100); ok {
		t.Error("la riga resta in INBOX dopo la rimozione di \\Inbox")
	}
}

// Un errore delle istantanee non trasforma un'op riuscita in errore (si ignora come una
// cache assente): la cache chiusa fa fallire ogni accesso, l'archiviazione riesce lo stesso.
func TestSnapshotErrorDoesNotFailOp(t *testing.T) {
	addr := scriptedGmailServerOpts(t, snapBoxes(), gmailServerOpts{
		epochs: map[string]uint32{"INBOX": snapInboxEp, snapAllMail: snapAllEp}, trash: true,
	})
	c, err := cache.Open(filepath.Join(t.TempDir(), "cache.db"))
	if err != nil {
		t.Fatalf("cache open: %v", err)
	}
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true, Cache: c})
	if err := s.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	if _, err := s.ListConversations(ctx, msg.Query{}, mail.Page{}); err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("cache close: %v", err)
	}
	if err := s.Archive(ctx, []msg.MessageID{"INBOX:3"}); err != nil {
		t.Errorf("Archive con le istantanee non scrivibili: %v", err)
	}
	if _, err := s.Trash(ctx, []msg.MessageID{"INBOX:1"}); err != nil {
		t.Errorf("Trash con le istantanee non scrivibili: %v", err)
	}
	if _, err := s.ListConversations(ctx, msg.Query{}, mail.Page{}); err != nil {
		t.Errorf("ListConversations con le istantanee non scrivibili: %v", err)
	}
}
