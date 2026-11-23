package app

// Piano 16, Task 9: lista da istantanea e stato provvisorio (R1, R2). Lo store
// dei test e un Mock avvolto da un finto Snapshotter (il Mock da solo non lo e:
// in demo nessuna istantanea) con Connect bloccabile da canale e il registro
// delle chiamate che toccano il server.

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
)

// snapStore e il finto store con istantanee: Connect attende gate (se non nil).
type snapStore struct {
	*mail.Mock
	gate       chan struct{} // Connect attende gate
	threadGate chan struct{} // ThreadMessages attende threadGate (Piano 16, Task 10)

	mu       sync.Mutex
	snaps    map[string]mail.ConversationsSnapshot
	threads  map[uint64]mail.ThreadSnapshot
	bodies   map[msg.MessageID]*msg.Message
	calls    []string
	deadline time.Duration // budget del ctx passato a Connect
}

func newSnapStore(mock *mail.Mock) *snapStore {
	return &snapStore{
		Mock:    mock,
		snaps:   map[string]mail.ConversationsSnapshot{},
		threads: map[uint64]mail.ThreadSnapshot{},
		bodies:  map[msg.MessageID]*msg.Message{},
	}
}

func (s *snapStore) rec(name string) {
	s.mu.Lock()
	s.calls = append(s.calls, name)
	s.mu.Unlock()
}

// called dice quante volte e stato chiamato il metodo name.
func (s *snapStore) called(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, c := range s.calls {
		if c == name {
			n++
		}
	}
	return n
}

// touched elenca le chiamate al server registrate (vuoto = nessuna).
func (s *snapStore) touched() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.calls...)
}

func (s *snapStore) Connect(ctx context.Context) error {
	if dl, ok := ctx.Deadline(); ok {
		s.mu.Lock()
		s.deadline = time.Until(dl)
		s.mu.Unlock()
	}
	if s.gate != nil {
		select {
		case <-s.gate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.Mock.Connect(ctx)
}

func (s *snapStore) CachedConversations(mailbox string) (mail.ConversationsSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sn, ok := s.snaps[mailbox]
	return sn, ok
}

func (s *snapStore) CachedThread(id uint64) (mail.ThreadSnapshot, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sn, ok := s.threads[id]
	return sn, ok
}

func (s *snapStore) CachedMessage(id msg.MessageID) (*msg.Message, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bodies[id]
	return b, ok
}

func (s *snapStore) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	s.rec("FetchMessage:" + string(id))
	return s.Mock.FetchMessage(ctx, id)
}

func (s *snapStore) ThreadMessages(ctx context.Context, id uint64) ([]msg.MessageRef, error) {
	s.rec(fmt.Sprintf("ThreadMessages:%d", id))
	if s.threadGate != nil {
		<-s.threadGate
	}
	return s.Mock.ThreadMessages(ctx, id)
}

func (s *snapStore) ConversationMembers(ctx context.Context, box string, id uint64) ([]msg.MessageID, error) {
	s.rec("ConversationMembers")
	return s.Mock.ConversationMembers(ctx, box, id)
}

func (s *snapStore) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	s.rec(fmt.Sprintf("ListConversations:%d", pg.Offset))
	return s.Mock.ListConversations(ctx, q, pg)
}

func (s *snapStore) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	s.rec("MarkRead")
	return s.Mock.MarkRead(ctx, ids, read)
}

func (s *snapStore) Archive(ctx context.Context, ids []msg.MessageID) error {
	s.rec("Archive")
	return s.Mock.Archive(ctx, ids)
}

func (s *snapStore) Star(ctx context.Context, ids []msg.MessageID, on bool) error {
	s.rec("Star")
	return s.Mock.Star(ctx, ids, on)
}

func (s *snapStore) ApplyLabel(ctx context.Context, ids []msg.MessageID, l string, on bool) error {
	s.rec("ApplyLabel")
	return s.Mock.ApplyLabel(ctx, ids, l, on)
}

func (s *snapStore) ModifyLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error {
	s.rec("ModifyLabels")
	return s.Mock.ModifyLabels(ctx, ids, add, remove)
}

func (s *snapStore) Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error) {
	s.rec("Trash")
	return s.Mock.Trash(ctx, ids)
}

func (s *snapStore) SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error {
	s.rec("SaveAttachment")
	return s.Mock.SaveAttachment(ctx, id, part, path)
}

var (
	_ mail.MailStore   = (*snapStore)(nil)
	_ mail.Snapshotter = (*snapStore)(nil)
)

// snapRow costruisce la riga di conversazione di un messaggio d'istantanea.
func snapRow(box string, uid uint32, gmail, thread uint64, subject string) mail.Conversation {
	r := msg.MessageRef{
		ID: msg.NewID(box, uid), From: "mittente@x.dev", Subject: subject,
		Date:    time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC).Add(time.Duration(uid) * time.Minute),
		GmailID: gmail, ThreadID: thread, Unread: true,
	}
	return mail.Conversation{Latest: r, ThreadID: thread, Count: 1, Participants: []string{"mittente@x.dev"}, Unread: true}
}

// snapModel costruisce un Model contro lo store dato, a 100x30, senza lanciare l'Init.
func snapModel(tb testing.TB, st mail.MailStore) Model {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	m := New(st, noopSender{}, th)
	m, _ = step(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	return m
}

// applySnapshot esegue loadSnapshot e applica l'esito al modello.
func applySnapshot(tb testing.TB, m Model, st mail.MailStore) Model {
	tb.Helper()
	c := loadSnapshot(st, m.folder.Name, m.folderGen)
	if c == nil {
		tb.Fatal("loadSnapshot senza Cmd: lo store e uno Snapshotter")
	}
	m, _ = step(m, c())
	return m
}

// fire esegue c senza attendere i timer: ogni Cmd (anche dentro i batch) parte
// in un goroutine e i messaggi pronti entro 150 ms sono restituiti. Serve per
// verificare cosa FA una Cmd (chiamate allo store) senza bloccare sul Tick dei toast.
func fire(c tea.Cmd) []tea.Msg {
	if c == nil {
		return nil
	}
	var (
		mu  sync.Mutex
		out []tea.Msg
		wg  sync.WaitGroup
	)
	var run func(tea.Cmd)
	run = func(c tea.Cmd) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := c()
			if b, ok := res.(tea.BatchMsg); ok {
				for _, sub := range b {
					if sub != nil {
						run(sub)
					}
				}
				return
			}
			mu.Lock()
			out = append(out, res)
			mu.Unlock()
		}()
	}
	run(c)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(150 * time.Millisecond):
	}
	mu.Lock()
	defer mu.Unlock()
	return append([]tea.Msg(nil), out...)
}

// seedSnapshot e un'istantanea di tre righe della INBOX salvata at.
func seedSnapshot(st *snapStore, at time.Time) []mail.Conversation {
	rows := []mail.Conversation{
		snapRow("INBOX", 7, 1, 100, "istantanea uno"),
		snapRow("INBOX", 8, 2, 101, "istantanea due"),
		snapRow("INBOX", 9, 3, 102, "istantanea tre"),
	}
	st.snaps[""] = mail.ConversationsSnapshot{Rows: rows, SavedAt: at}
	return rows
}

// freshMock e il server: due messaggi diversi da quelli dell'istantanea (INBOX:7
// e lo stesso UID di una riga dell'istantanea ma un altro messaggio).
func freshMock() *mail.Mock {
	base := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	mk := func(uid uint32, gmail, thread uint64, subj string) *msg.Message {
		return &msg.Message{
			Ref: msg.MessageRef{
				ID: msg.NewID("INBOX", uid), From: "server@x.dev", Subject: subj,
				Date: base.Add(-time.Duration(uid) * time.Minute), GmailID: gmail, ThreadID: thread, Unread: true,
			},
			BodyText: "corpo " + subj,
		}
	}
	return &mail.Mock{Msgs: []*msg.Message{
		mk(7, 2, 200, "fresco uno"),
		mk(11, 5, 205, "fresco due"),
	}}
}

// freshList applica al modello l'esito della lista fresca dopo un connResult riuscito.
func freshList(tb testing.TB, m Model, st mail.MailStore) Model {
	tb.Helper()
	m, _ = step(m, connResult{})
	return applyAll(tb, m, loadRefs(st, m.folder.Name, m.listGen))
}

// --- avvio da istantanea ------------------------------------------------------

func TestSnapshotShownBeforeConnect(t *testing.T) {
	st := newSnapStore(freshMock())
	st.gate = make(chan struct{})
	t.Cleanup(func() { close(st.gate) })
	seedSnapshot(st, time.Now())
	m := snapModel(t, st)

	// Init lancia in parallelo Connect (bloccato) e la lettura dell'istantanea.
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init non e un batch")
	}
	got := make(chan tea.Msg, len(batch))
	for _, c := range batch {
		go func(c tea.Cmd) { got <- c() }(c)
	}
	var loaded tea.Msg
	timeout := time.After(2 * time.Second)
	for loaded == nil {
		select {
		case ms := <-got:
			if _, is := ms.(snapshotLoaded); is {
				loaded = ms
			}
		case <-timeout:
			t.Fatal("snapshotLoaded non arrivato con Connect ancora bloccato")
		}
	}
	m, _ = step(m, loaded)

	v := plain(m)
	for _, want := range []string{"istantanea uno", "istantanea due"} {
		if !strings.Contains(v, want) {
			t.Errorf("la lista non mostra %q\n%s", want, v)
		}
	}
	if strings.Contains(v, "connessione a imap.gmail.com") || strings.Contains(v, "█") {
		t.Errorf("lo splash deve sparire con l'istantanea\n%s", v)
	}
	if bar := plainBar(m); !strings.Contains(bar, "aggiornamento…") {
		t.Errorf("barra = %q, manca \"aggiornamento…\"", bar)
	}
	if m.connected {
		t.Error("Connect non e ancora finito: connected deve essere false")
	}
}

func TestNoSnapshotKeepsSplash(t *testing.T) {
	st := newSnapStore(freshMock()) // nessuna istantanea
	m := snapModel(t, st)
	m = applySnapshot(t, m, st)
	v := plain(m)
	if !strings.Contains(v, "connessione a imap.gmail.com…") || !strings.Contains(v, "█") {
		t.Errorf("senza istantanea resta lo splash\n%s", v)
	}
}

func TestEmptySnapshotKeepsSplash(t *testing.T) {
	st := newSnapStore(freshMock())
	st.snaps[""] = mail.ConversationsSnapshot{SavedAt: time.Now()} // righe vuote = assente
	m := snapModel(t, st)
	m = applySnapshot(t, m, st)
	if v := plain(m); !strings.Contains(v, "█") {
		t.Errorf("un'istantanea vuota vale come assente: resta lo splash\n%s", v)
	}
	if m.listSource != srcNone {
		t.Errorf("listSource = %v, voglio srcNone", m.listSource)
	}
}

func TestDemoStoreHasNoSnapshot(t *testing.T) {
	// Il Mock (demo) non implementa Snapshotter: nessuna Cmd, comportamento di oggi.
	if c := loadSnapshot(DemoStore(), "", 0); c != nil {
		t.Fatal("la demo non deve avere istantanee")
	}
	m := newTestModel(t)
	for _, ms := range runCmd(m.Init()) {
		if _, is := ms.(snapshotLoaded); is {
			t.Fatal("Init del Mock non deve emettere snapshotLoaded")
		}
	}
}

func TestFreshListReplacesSnapshot(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now())
	m := applySnapshot(t, snapModel(t, st), st)
	if m.listSource != srcCached {
		t.Fatalf("listSource = %v, voglio srcCached", m.listSource)
	}
	m = freshList(t, m, st)
	if m.listSource != srcFresh {
		t.Fatalf("listSource = %v, voglio srcFresh", m.listSource)
	}
	v := plain(m)
	if !strings.Contains(v, "fresco uno") || !strings.Contains(v, "fresco due") {
		t.Errorf("la lista fresca non e visibile\n%s", v)
	}
	if strings.Contains(v, "istantanea") {
		t.Errorf("la lista provvisoria e sostituita per intero, non fusa\n%s", v)
	}
	if bar := plainBar(m); strings.Contains(bar, "aggiornamento…") {
		t.Errorf("barra = %q: dopo la lista fresca \"aggiornamento…\" sparisce", bar)
	}
	if len(m.refs) != 2 {
		t.Errorf("refs = %d, voglio 2 (solo le fresche)", len(m.refs))
	}
}

func TestStaleSnapshotAfterFreshIgnored(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now())
	m := snapModel(t, st)
	m = freshList(t, m, st) // il fresco arriva per primo
	m = applySnapshot(t, m, st)
	if m.listSource != srcFresh {
		t.Fatalf("listSource = %v: un'istantanea tardiva non declassa la lista fresca", m.listSource)
	}
	if v := plain(m); strings.Contains(v, "istantanea") || !strings.Contains(v, "fresco uno") {
		t.Errorf("l'istantanea tardiva ha sovrascritto il fresco\n%s", v)
	}
}

func TestSnapshotOfOtherFolderIgnored(t *testing.T) {
	st := newSnapStore(freshMock())
	rows := seedSnapshot(st, time.Now())
	m := snapModel(t, st)
	// Cartella diversa e generazione diversa: scartate.
	m, _ = step(m, snapshotLoaded{mailbox: "[Gmail]/Sent", folderGen: m.folderGen, snap: mail.ConversationsSnapshot{Rows: rows}})
	m, _ = step(m, snapshotLoaded{mailbox: "", folderGen: m.folderGen + 1, snap: mail.ConversationsSnapshot{Rows: rows}})
	if m.listSource != srcNone || m.listLoaded {
		t.Errorf("listSource=%v listLoaded=%v: l'istantanea di un'altra cartella o generazione va ignorata", m.listSource, m.listLoaded)
	}
}

func TestSnapshotWithQueryIgnored(t *testing.T) {
	st := newSnapStore(freshMock())
	rows := seedSnapshot(st, time.Now())
	m := snapModel(t, st)
	m.queryRaw = "from:anna"
	m, _ = step(m, snapshotLoaded{mailbox: "", folderGen: m.folderGen, snap: mail.ConversationsSnapshot{Rows: rows}})
	if m.listSource != srcNone {
		t.Errorf("con una ricerca attiva l'istantanea non si applica (listSource = %v)", m.listSource)
	}
}

func TestFolderSwitchShowsFolderSnapshot(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now())
	sent := mail.Mailbox{Name: "[Gmail]/Sent", Role: "sent"}
	st.snaps["[Gmail]/Sent"] = mail.ConversationsSnapshot{
		Rows: []mail.Conversation{snapRow("[Gmail]/Sent", 3, 9, 300, "inviata istantanea")}, SavedAt: time.Now(),
	}
	m := freshList(t, snapModel(t, st), st) // INBOX fresca
	genBefore := m.folderGen

	m, cmd := m.switchFolder(sent)
	if m.folderGen != genBefore+1 {
		t.Fatalf("folderGen = %d, voglio %d", m.folderGen, genBefore+1)
	}
	if m.listSource != srcNone {
		t.Errorf("listSource dopo il cambio = %v, voglio srcNone", m.listSource)
	}
	var snap tea.Msg
	for _, ms := range fire(cmd) {
		if s, ok := ms.(snapshotLoaded); ok {
			snap = s
		}
	}
	if snap == nil {
		t.Fatal("il cambio di cartella non rilegge l'istantanea della cartella nuova")
	}
	m, _ = step(m, snap)
	if v := plain(m); !strings.Contains(v, "inviata istantanea") {
		t.Errorf("l'istantanea della cartella nuova non e visibile\n%s", v)
	}
	// Un'istantanea della cartella precedente arrivata tardi si scarta.
	m, _ = step(m, snapshotLoaded{mailbox: "", folderGen: genBefore, snap: st.snaps[""]})
	if v := plain(m); strings.Contains(v, "istantanea uno") {
		t.Errorf("l'istantanea della cartella precedente non va mostrata\n%s", v)
	}
}

// --- R1: provvisorio = sola lettura -------------------------------------------

func TestProvisionalListRefusesActions(t *testing.T) {
	for _, k := range []string{"a", "#", "s", "v", "l", "x", "X", "r", "f"} {
		t.Run(k, func(t *testing.T) {
			st := newSnapStore(freshMock())
			seedSnapshot(st, time.Now())
			m := applySnapshot(t, snapModel(t, st), st)
			m, cmd := step(m, pressKey(k))
			if got := toastText(m); got != provisionalToast {
				t.Fatalf("toast = %q, voglio %q", got, provisionalToast)
			}
			fire(cmd)
			if c := st.touched(); len(c) != 0 {
				t.Errorf("lo store e stato toccato su dati provvisori: %v", c)
			}
			if m.trashOpen || m.labelOpen || m.labelPickOpen || m.confirmSend || len(m.sel) != 0 || m.view != viewInbox {
				t.Errorf("stato cambiato: trash=%v label=%v pick=%v sel=%d view=%v", m.trashOpen, m.labelOpen, m.labelPickOpen, len(m.sel), m.view)
			}
		})
	}
}

func TestProvisionalScrollAtEndRefusesNextPage(t *testing.T) {
	st := newSnapStore(freshMock())
	rows := make([]mail.Conversation, pageSize)
	for i := range rows {
		rows[i] = snapRow("INBOX", uint32(100+i), uint64(i+1), uint64(1000+i), fmt.Sprintf("riga %d", i))
	}
	st.snaps[""] = mail.ConversationsSnapshot{Rows: rows, SavedAt: time.Now()}
	m := applySnapshot(t, snapModel(t, st), st)
	m.inbox.Select(pageSize - 1)
	m, cmd := step(m, pressKey("j"))
	if got := toastText(m); got != provisionalToast {
		t.Fatalf("toast = %q, voglio %q", got, provisionalToast)
	}
	fire(cmd)
	if n := st.called("ListConversations:50"); n != 0 || m.loadingMore {
		t.Errorf("pagina successiva richiesta su lista provvisoria (chiamate=%d loadingMore=%v)", n, m.loadingMore)
	}
}

func TestProvisionalShortSnapshotIsExhausted(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now()) // 3 righe < pageSize, non Limited
	m := applySnapshot(t, snapModel(t, st), st)
	if !m.exhausted {
		t.Error("un'istantanea corta non ha pagina successiva (nessun toast a fondo lista)")
	}
}

func TestProvisionalAllowsNavigationSearchCompose(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now())
	m := applySnapshot(t, snapModel(t, st), st)

	m, _ = step(m, pressKey("j"))
	if idx := m.inbox.SelectedIndex(); idx != 1 {
		t.Errorf("j non muove il cursore (indice %d)", idx)
	}
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if idx := m.inbox.SelectedIndex(); idx != 0 {
		t.Errorf("freccia su non muove il cursore (indice %d)", idx)
	}
	m, _ = step(m, pressKey("/"))
	if m.view != viewSearch {
		t.Errorf("/ non apre la ricerca (vista %v)", m.view)
	}
	m, _ = step(m, pressKey("esc"))
	m, _ = step(m, pressKey("m"))
	if !m.folderOpen {
		t.Error("m non apre il selettore cartelle")
	}
	m, _ = step(m, pressKey("esc"))
	m, _ = step(m, pressKey("c"))
	if m.view != viewComposer {
		t.Errorf("c non apre il composer (vista %v)", m.view)
	}
	if got := toastText(m); got == provisionalToast {
		t.Error("navigazione, ricerca, cartelle e composer non vanno rifiutati")
	}
}

func TestActionsEnabledAfterFreshList(t *testing.T) {
	mock := freshMock()
	st := newSnapStore(mock)
	seedSnapshot(st, time.Now())
	m := applySnapshot(t, snapModel(t, st), st)
	m = freshList(t, m, st)

	m, cmd := step(m, pressKey("a"))
	if toastText(m) == provisionalToast {
		t.Fatal("dopo la lista fresca l'azione non va piu rifiutata")
	}
	fire(cmd)
	if st.called("Archive") != 1 {
		t.Fatalf("Archive chiamate = %d, voglio 1", st.called("Archive"))
	}
	for _, mm := range mock.Msgs {
		if mm.Ref.ID == msg.NewID("INBOX", 7) {
			t.Error("INBOX:7 doveva essere archiviato")
		}
	}
}

// R2: stesso ID, messaggio diverso. L'istantanea ha INBOX:7 = A (GmailID 1); la
// lista fresca ha INBOX:7 = B (GmailID 2). Prima della lista fresca `a` e
// rifiutato; dopo archivia B, la riga visibile, mai A.
func TestEpochChangeSameIDOtherMessage(t *testing.T) {
	mock := freshMock() // INBOX:7 = B (GmailID 2, thread 200)
	st := newSnapStore(mock)
	seedSnapshot(st, time.Now()) // INBOX:7 = A (GmailID 1, thread 100)
	m := applySnapshot(t, snapModel(t, st), st)

	a := m.refs[0]
	if a.ID != msg.NewID("INBOX", 7) || a.GmailID != 1 {
		t.Fatalf("riga d'istantanea inattesa: %+v", a)
	}
	m, cmd := step(m, pressKey("a"))
	fire(cmd)
	if st.called("Archive") != 0 || st.called("ConversationMembers") != 0 {
		t.Fatalf("a prima della lista fresca ha toccato lo store: %v", st.touched())
	}

	m = freshList(t, m, st)
	b := m.refs[0]
	if b.GmailID != 2 || b.ThreadID != 200 {
		t.Fatalf("riga fresca inattesa: %+v", b)
	}
	_, cmd = step(m, pressKey("a"))
	fire(cmd)
	if st.called("Archive") != 1 {
		t.Fatalf("Archive = %d, voglio 1", st.called("Archive"))
	}
	for _, mm := range mock.Msgs {
		if mm.Ref.ID == msg.NewID("INBOX", 7) {
			t.Error("la riga fresca B (INBOX:7) doveva essere archiviata")
		}
	}
}

func TestProvisionalDraftsRefusesResume(t *testing.T) {
	st := newSnapStore(freshMock())
	st.snaps["[Gmail]/Drafts"] = mail.ConversationsSnapshot{
		Rows: []mail.Conversation{snapRow("[Gmail]/Drafts", 4, 7, 700, "bozza")}, SavedAt: time.Now(),
	}
	m := snapModel(t, st)
	m.folder = mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}
	m = applySnapshot(t, m, st)
	m, cmd := step(m, inbox.SelectedMsg{Ref: m.refs[0]})
	fire(cmd)
	if c := st.touched(); len(c) != 0 {
		t.Errorf("la ripresa di una bozza provvisoria ha toccato lo store: %v", c)
	}
	if got := toastText(m); got != provisionalToast {
		t.Errorf("toast = %q, voglio %q", got, provisionalToast)
	}
	if m.view == viewComposer {
		t.Error("la bozza provvisoria non si riprende")
	}
}

// --- connessione fallita con istantanea ----------------------------------------

func TestConnectFailWithSnapshotKeepsReadOnlyList(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now())
	m := applySnapshot(t, snapModel(t, st), st)
	m, _ = step(m, connResult{err: fmt.Errorf("boom rete")})

	v := plain(m)
	if !strings.Contains(v, "istantanea uno") {
		t.Errorf("la lista resta visibile dopo il Connect fallito\n%s", v)
	}
	if strings.Contains(v, "█") {
		t.Errorf("nessuno splash d'errore con l'istantanea\n%s", v)
	}
	if bar := plainBar(m); !strings.Contains(bar, "connessione fallita — lista non aggiornata, premi R per riprovare") {
		t.Errorf("barra = %q", bar)
	}
	if got := toastText(m); !strings.Contains(got, "connessione fallita: boom rete") {
		t.Errorf("toast d'errore atteso, ho %q", got)
	}
	// Sola lettura: l'azione e ancora rifiutata.
	m2, cmd := step(m, pressKey("a"))
	fire(cmd)
	if got := toastText(m2); got != provisionalToast {
		t.Errorf("toast = %q, voglio %q", got, provisionalToast)
	}
	if st.called("Archive") != 0 {
		t.Error("Archive non deve partire su lista provvisoria")
	}
	// R riprova: la barra torna a "aggiornamento…" senza il testo d'errore.
	m3, _ := step(m, pressKey("R"))
	if bar := plainBar(m3); strings.Contains(bar, "connessione fallita") || !strings.Contains(bar, "aggiornamento…") {
		t.Errorf("dopo R la barra = %q", bar)
	}
}

func TestFreshListErrorKeepsSnapshotReadOnly(t *testing.T) {
	st := newSnapStore(freshMock())
	seedSnapshot(st, time.Now())
	m := applySnapshot(t, snapModel(t, st), st)
	m, _ = step(m, connResult{})
	m, _ = step(m, msgsLoaded{err: fmt.Errorf("lista ko"), limit: pageSize, gen: m.listGen})
	if m.listSource != srcCached {
		t.Errorf("listSource = %v: un fresco in errore non conferma l'istantanea", m.listSource)
	}
	if bar := plainBar(m); strings.Contains(bar, "aggiornamento…") || !strings.Contains(bar, "lista non aggiornata") {
		t.Errorf("barra = %q, voglio \"lista non aggiornata\"", bar)
	}
}

func TestStatusAgeUsesInjectedClock(t *testing.T) {
	st := newSnapStore(freshMock())
	now := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	seedSnapshot(st, now.Add(-2*time.Hour))
	m := snapModel(t, st)
	m.clock = func() time.Time { return now }
	m = applySnapshot(t, m, st)
	if bar := plainBar(m); !strings.Contains(bar, "aggiornamento… (dati di 2 ore fa)") {
		t.Errorf("barra = %q, voglio \"aggiornamento… (dati di 2 ore fa)\"", bar)
	}
}

func TestAgeText(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{10 * time.Second, "meno di un minuto fa"},
		{time.Minute, "1 minuto fa"},
		{12 * time.Minute, "12 minuti fa"},
		{time.Hour, "1 ora fa"},
		{2*time.Hour + 20*time.Minute, "2 ore fa"},
		{24 * time.Hour, "1 giorno fa"},
		{73 * time.Hour, "3 giorni fa"},
		{-time.Minute, "meno di un minuto fa"},
	} {
		if got := ageText(c.d); got != c.want {
			t.Errorf("ageText(%v) = %q, voglio %q", c.d, got, c.want)
		}
	}
}

// Q16-7: il budget del Connect d'avvio e di 90 s.
func TestConnectBudgetIs90Seconds(t *testing.T) {
	st := newSnapStore(freshMock())
	connectCmd(st, context.Background())()
	st.mu.Lock()
	d := st.deadline
	st.mu.Unlock()
	if d < 89*time.Second || d > 90*time.Second {
		t.Errorf("budget del Connect = %v, voglio ~90s", d)
	}
}
