package app

// Piano 16, Task 10: conversazione e anteprima da istantanea. La conversazione gia
// vista si apre subito da CachedThread/CachedMessage (provvisoria, R1) e si
// riallinea con thread.Reconcile all'arrivo dei dati freschi; per una riga
// provvisoria il caricamento fresco usa SOLO il ThreadID (D16-T13, R2).

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
	"github.com/andsigno82/gmail-tui/internal/ui/thread"
)

const (
	cvThread = 100
	cvAll    = "[Gmail]/All Mail"
)

// cvFixture: il thread 100 di tre messaggi (GmailID 11, 12, 13) esiste sul server
// come INBOX:20, 21, 22 (21 e 22 non letti) e nell'istantanea come All Mail:51, 52,
// 53 con corpi "corpo istantanea"; la riga della lista provvisoria e INBOX:7
// (GmailID 13): lo stesso UID punta a un altro messaggio sul server. withRow
// false toglie il thread dal server (conversazione sparita).
func cvFixture(withRow bool) (*snapStore, *mail.Mock) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mock := &mail.Mock{}
	st := newSnapStore(mock)
	var snapRefs []msg.MessageRef
	for i := 0; i < 3; i++ {
		date := base.Add(time.Duration(i) * time.Hour)
		unread := i > 0
		gmail := uint64(11 + i)
		if withRow {
			mock.Msgs = append(mock.Msgs, &msg.Message{
				Ref: msg.MessageRef{
					ID: msg.NewID("INBOX", uint32(20+i)), From: "anna@x.dev", Subject: "tema", Date: date,
					Unread: unread, GmailID: gmail, ThreadID: cvThread,
				},
				BodyText: fmt.Sprintf("corpo fresco %d", 20+i),
			})
		}
		ref := msg.MessageRef{
			ID: msg.NewID(cvAll, uint32(51+i)), From: "anna@x.dev", Subject: "tema", Date: date,
			Unread: unread, GmailID: gmail, ThreadID: cvThread,
		}
		snapRefs = append(snapRefs, ref)
		st.bodies[ref.ID] = &msg.Message{Ref: ref, BodyText: fmt.Sprintf("corpo istantanea %d", 51+i)}
	}
	if !withRow {
		// Il server ha solo un'altra conversazione.
		mock.Msgs = append(mock.Msgs, &msg.Message{
			Ref:      msg.MessageRef{ID: msg.NewID("INBOX", 30), From: "z@x.dev", Subject: "altro", Date: base, GmailID: 90, ThreadID: 900},
			BodyText: "altro corpo",
		})
	}
	st.threads[cvThread] = mail.ThreadSnapshot{Refs: snapRefs, SavedAt: time.Now()}
	row := snapRow("INBOX", 7, 13, cvThread, "tema")
	row.Count = 3
	st.snaps[""] = mail.ConversationsSnapshot{Rows: []mail.Conversation{row}, SavedAt: time.Now()}
	return st, mock
}

// splitMsgs separa i risultati di apertura in cached e fresco.
func splitMsgs(msgs []tea.Msg) (cached, fresh *msgLoaded) {
	for _, ms := range msgs {
		if l, ok := ms.(msgLoaded); ok {
			l := l
			if l.cached {
				cached = &l
			} else {
				fresh = &l
			}
		}
	}
	return cached, fresh
}

// cvOpen apre la conversazione della riga provvisoria (o fresca, se la lista e
// gia fresca) e restituisce modello e risultati delle due Cmd.
func cvOpen(t *testing.T, m Model) (Model, *msgLoaded, *msgLoaded) {
	t.Helper()
	m, cmd := step(m, inbox.SelectedMsg{Ref: m.refs[0]})
	if m.view != viewThread {
		t.Fatalf("vista = %v, voglio viewThread", m.view)
	}
	cached, fresh := splitMsgs(fire(cmd))
	return m, cached, fresh
}

func TestOpenCachedThreadImmediately(t *testing.T) {
	st, _ := cvFixture(true)
	st.threadGate = make(chan struct{}) // il fresco resta bloccato
	t.Cleanup(func() { close(st.threadGate) })
	m := applySnapshot(t, snapModel(t, st), st)

	m, cached, fresh := cvOpen(t, m)
	if fresh != nil {
		t.Fatal("il fresco doveva essere ancora bloccato")
	}
	if cached == nil || !cached.cached || len(cached.refs) != 3 {
		t.Fatalf("serve l'esito da istantanea con 3 messaggi, ho %+v", cached)
	}
	m, _ = step(m, *cached)
	v := plain(m)
	if !strings.Contains(v, "corpo istantanea 53") {
		t.Errorf("il thread non si vede subito dall'istantanea\n%s", v)
	}
	if !m.threadCached {
		t.Error("un thread da istantanea e provvisorio (threadCached)")
	}
	if m.pending != 2 { // il Connect d'avvio e il caricamento fresco in volo; il cached non conta
		t.Errorf("pending = %d, voglio 2", m.pending)
	}
}

func TestProvisionalRowFreshLoadUsesThreadIDOnly(t *testing.T) {
	st, _ := cvFixture(true)
	m := applySnapshot(t, snapModel(t, st), st)
	prov := m.refs[0].ID // INBOX:7: UID d'epoca vecchia
	m, cached, fresh := cvOpen(t, m)
	if cached == nil || fresh == nil {
		t.Fatalf("servono entrambi gli esiti: cached=%v fresh=%v", cached, fresh)
	}
	m, _ = step(m, *cached)
	m, _ = step(m, *fresh)
	if st.called(fmt.Sprintf("ThreadMessages:%d", cvThread)) != 1 {
		t.Errorf("ThreadMessages(thread) non chiamato: %v", st.touched())
	}
	for _, c := range st.touched() {
		if c == "FetchMessage:"+string(prov) || c == "ConversationMembers" {
			t.Errorf("il caricamento fresco ha usato l'ID provvisorio %s: %v", prov, st.touched())
		}
	}
	if !strings.Contains(plain(m), "corpo fresco 22") {
		t.Errorf("il corpo fresco non e mostrato\n%s", plain(m))
	}
}

func TestProvisionalRowWithoutThreadIDLoadsOnlyCache(t *testing.T) {
	st, _ := cvFixture(true)
	row := snapRow("INBOX", 8, 5, 0, "solo") // ThreadID 0
	st.snaps[""] = mail.ConversationsSnapshot{Rows: []mail.Conversation{row}, SavedAt: time.Now()}
	st.bodies[row.Latest.ID] = &msg.Message{Ref: row.Latest, BodyText: "corpo solo cache"}
	m := applySnapshot(t, snapModel(t, st), st)
	before := m.pending
	m, cmd := step(m, inbox.SelectedMsg{Ref: m.refs[0]})
	cached, fresh := splitMsgs(fire(cmd))
	if fresh != nil || st.called("FetchMessage:"+string(row.Latest.ID)) != 0 {
		t.Fatalf("nessun caricamento fresco per una riga provvisoria senza ThreadID: %v", st.touched())
	}
	if m.pending != before {
		t.Errorf("pending = %d, voglio %d (nessun fresco in volo)", m.pending, before)
	}
	if cached == nil {
		t.Fatal("i dati in cache dovevano arrivare")
	}
	m, _ = step(m, *cached)
	if !strings.Contains(plain(m), "corpo solo cache") {
		t.Errorf("il corpo in cache non e mostrato\n%s", plain(m))
	}
}

func TestFreshThreadReconciles(t *testing.T) {
	st, _ := cvFixture(true)
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, fresh := cvOpen(t, m)
	m, _ = step(m, *cached)
	m, _ = step(m, pressKey("k")) // seleziona il messaggio 12 (indice 1)
	if g := m.thread.Ref().GmailID; g != 12 {
		t.Fatalf("selezione = GmailID %d, voglio 12", g)
	}
	m, _ = step(m, *fresh)
	if m.threadCached {
		t.Error("il thread fresco non e piu provvisorio")
	}
	ref := m.thread.Ref()
	if ref.GmailID != 12 || ref.ID != msg.NewID("INBOX", 21) {
		t.Errorf("selezione dopo Reconcile = %+v, voglio GmailID 12 con l'ID fresco INBOX:21", ref)
	}
	if m.thread.Msg() == nil || !strings.Contains(m.thread.Msg().BodyText, "corpo fresco 21") {
		t.Errorf("il corpo del selezionato deve essere quello fresco: %+v", m.thread.Msg())
	}
}

func TestCachedBodyRequestIgnoredUntilFresh(t *testing.T) {
	st, _ := cvFixture(true)
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, _ := cvOpen(t, m)
	m, _ = step(m, *cached)
	before := m.pending
	m, cmd := step(m, thread.BodyRequested{ID: msg.NewID(cvAll, 51), Anchor: m.thread.AnchorID()})
	fire(cmd)
	if st.called("FetchMessage:"+string(msg.NewID(cvAll, 51))) != 0 || m.pending != before {
		t.Errorf("BodyRequested su thread provvisorio deve essere ignorato: %v pending=%d", st.touched(), m.pending)
	}
}

func TestNoAutoMarkReadOnCachedThread(t *testing.T) {
	st, _ := cvFixture(true)
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, _ := cvOpen(t, m)
	_, cmd := step(m, *cached)
	fire(cmd)
	if st.called("MarkRead") != 0 {
		t.Error("nessun segna-come-letto da un thread da istantanea")
	}
}

// Riga provvisoria: anche col thread fresco il segna-come-letto aspetta la lista
// fresca (R1); poi parte, sugli ID freschi.
func TestAutoMarkReadWaitsForFreshListThenUsesFreshIDs(t *testing.T) {
	st, mock := cvFixture(true)
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, fresh := cvOpen(t, m)
	m, _ = step(m, *cached)
	m, cmd := step(m, *fresh)
	fire(cmd)
	if st.called("MarkRead") != 0 {
		t.Fatal("segna-come-letto con la lista ancora provvisoria")
	}
	// La lista fresca conferma la riga: il segna-come-letto differito parte da qui.
	m, _ = step(m, connResult{})
	for _, ms := range runCmd(loadRefs(st, m.folder.Name, m.listGen)) {
		m, cmd = step(m, ms)
	}
	fire(cmd)
	if st.called("MarkRead") != 1 {
		t.Fatalf("MarkRead = %d, voglio 1 dopo la lista fresca", st.called("MarkRead"))
	}
	if unread := unreadInFixture(mock); unread != 0 {
		t.Errorf("dopo la lista fresca i non letti del server devono essere letti, ne restano %d", unread)
	}
}

func TestAutoMarkReadAfterFreshFromFreshRow(t *testing.T) {
	st, mock := cvFixture(true)
	m := freshList(t, snapModel(t, st), st) // lista fresca: riga azionabile
	m, cached, fresh := cvOpen(t, m)
	if cached == nil || fresh == nil {
		t.Fatalf("servono entrambi gli esiti")
	}
	m, cmd := step(m, *cached)
	fire(cmd)
	if st.called("MarkRead") != 0 {
		t.Fatal("nessun segna-come-letto dal thread da istantanea")
	}
	_, cmd = step(m, *fresh)
	fire(cmd)
	if st.called("MarkRead") != 1 {
		t.Fatalf("MarkRead = %d, voglio 1 dopo il thread fresco", st.called("MarkRead"))
	}
	if unread := unreadInFixture(mock); unread != 0 {
		t.Errorf("i non letti freschi dovevano essere segnati: %d", unread)
	}
}

func TestStaleCachedThreadAfterFreshIgnored(t *testing.T) {
	st, _ := cvFixture(true)
	m := freshList(t, snapModel(t, st), st)
	m, cached, fresh := cvOpen(t, m)
	m, _ = step(m, *fresh)
	m, _ = step(m, *cached) // arriva tardi
	if m.threadCached {
		t.Error("un risultato da cache tardivo non declassa il thread fresco")
	}
	v := plain(m)
	if strings.Contains(v, "istantanea") || !strings.Contains(v, "corpo fresco 22") {
		t.Errorf("il thread fresco e stato sovrascritto\n%s", v)
	}
}

func TestPendingRefRealignedByGmailID(t *testing.T) {
	st, mock := cvFixture(true)
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, fresh := cvOpen(t, m)
	m, _ = step(m, *cached)
	m, _ = step(m, *fresh)
	// Thread fresco ma riga ancora provvisoria: `a` rifiutato.
	m, cmd := step(m, pressKey("a"))
	fire(cmd)
	if st.called("Archive") != 0 || toastText(m) != provisionalToast {
		t.Fatalf("a con riga provvisoria: toast=%q chiamate=%v", toastText(m), st.touched())
	}
	m = freshList(t, m, st)
	if m.pendingRef.ID != msg.NewID("INBOX", 22) || m.rowRef.ID != msg.NewID("INBOX", 22) {
		t.Fatalf("pendingRef/rowRef = %s/%s, voglio la riga fresca INBOX:22", m.pendingRef.ID, m.rowRef.ID)
	}
	if m.openProv || m.openGone {
		t.Errorf("openProv=%v openGone=%v dopo la conferma", m.openProv, m.openGone)
	}
	_, cmd = step(m, pressKey("a"))
	fire(cmd)
	if st.called("Archive") != 1 {
		t.Fatalf("Archive = %d, voglio 1 dopo la conferma", st.called("Archive"))
	}
	for _, mm := range mock.Msgs {
		if mm.Ref.ThreadID == cvThread {
			t.Errorf("la conversazione fresca doveva essere archiviata: resta %s", mm.Ref.ID)
		}
	}
}

func TestPendingRefMissingAfterRefreshRefusesConvActions(t *testing.T) {
	st, _ := cvFixture(false) // il server non ha piu il thread
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, _ := cvOpen(t, m)
	m, _ = step(m, *cached)
	m = freshList(t, m, st)
	if !m.openGone {
		t.Fatal("la riga non e piu nella lista fresca: openGone")
	}
	m, cmd := step(m, pressKey("a"))
	fire(cmd)
	if got := toastText(m); got != goneToast {
		t.Errorf("toast = %q, voglio %q", got, goneToast)
	}
	if st.called("Archive") != 0 || st.called("ConversationMembers") != 0 {
		t.Errorf("azione sulla conversazione sparita: %v", st.touched())
	}
}

// --- anteprima a due colonne -------------------------------------------------

func cvSplitModel(t *testing.T, st *snapStore, mode string) Model {
	t.Helper()
	m := snapModel(t, st)
	m.layout, m.markRead = config.LayoutSplit, mode
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	return m
}

// cvPreview porta l'anteprima della prima riga al punto di caricamento e
// restituisce i risultati cached e fresco della sua apertura.
func cvPreview(t *testing.T, m Model) (Model, *previewLoaded, *previewLoaded) {
	t.Helper()
	if !m.prevHas {
		t.Fatal("l'anteprima non e armata sulla prima riga")
	}
	m, cmd := step(m, previewDue{tok: m.prevTok})
	var cached, fresh *previewLoaded
	for _, ms := range fire(cmd) {
		if l, ok := ms.(previewLoaded); ok {
			l := l
			if l.cached {
				cached = &l
			} else {
				fresh = &l
			}
		}
	}
	return m, cached, fresh
}

func TestPreviewFromSnapshot(t *testing.T) {
	st, _ := cvFixture(true)
	m := cvSplitModel(t, st, config.PreviewMarkReadImmediate)
	m = applySnapshot(t, m, st)
	m, cached, fresh := cvPreview(t, m)
	if cached == nil || fresh == nil {
		t.Fatalf("servono entrambi gli esiti: cached=%v fresh=%v", cached, fresh)
	}
	m, cmd := step(m, *cached)
	fire(cmd)
	if !m.previewReady() || !strings.Contains(plain(m), "corpo istantanea 53") {
		t.Errorf("l'anteprima non mostra l'istantanea (ready=%v)\n%s", m.previewReady(), plain(m))
	}
	if st.called("MarkRead") != 0 {
		t.Error("nessun segna-come-letto da un'anteprima da istantanea")
	}
	m, _ = step(m, pressKey("a"))
	if toastText(m) != provisionalToast {
		t.Errorf("toast = %q: l'anteprima provvisoria e in sola lettura", toastText(m))
	}
	// Il fresco riallinea l'anteprima; la lista e ancora provvisoria: niente segna-come-letto.
	m, cmd = step(m, *fresh)
	fire(cmd)
	if st.called("MarkRead") != 0 {
		t.Error("segna-come-letto con la lista ancora provvisoria")
	}
	if !strings.Contains(plain(m), "corpo fresco 22") {
		t.Errorf("il fresco non e mostrato in anteprima\n%s", plain(m))
	}
}

func TestAutoMarkReadPreviewModes(t *testing.T) {
	for _, c := range []struct {
		mode string
		want int
	}{
		{config.PreviewMarkReadImmediate, 1},
		{config.PreviewMarkReadDelay, 1},
		{config.PreviewMarkReadNever, 0},
	} {
		t.Run(c.mode, func(t *testing.T) {
			st, _ := cvFixture(true)
			m := cvSplitModel(t, st, c.mode)
			m = freshList(t, m, st) // lista fresca: riga azionabile
			m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 40})
			m, cached, fresh := cvPreview(t, m)
			if cached == nil || fresh == nil {
				t.Fatalf("servono entrambi gli esiti")
			}
			m, cmd := step(m, *cached)
			fire(cmd)
			if st.called("MarkRead") != 0 {
				t.Fatal("nessun segna-come-letto da un'anteprima da istantanea")
			}
			m, cmd = step(m, *fresh)
			fire(cmd)
			if c.mode == config.PreviewMarkReadDelay {
				m, cmd = step(m, previewReadDue{tok: m.prevTok, id: m.thread.AnchorID()})
				fire(cmd)
			}
			if got := st.called("MarkRead"); got != c.want {
				t.Errorf("MarkRead = %d, voglio %d", got, c.want)
			}
		})
	}
}

// R2 sull'anteprima: la riga provvisoria si riallinea alla riga fresca (ID nuovo) e
// l'anteprima resta pronta; il segna-come-letto sospeso parte alla conferma.
func TestPreviewRealignedAfterFreshList(t *testing.T) {
	st, _ := cvFixture(true)
	m := cvSplitModel(t, st, config.PreviewMarkReadImmediate)
	m = applySnapshot(t, m, st)
	m, cached, fresh := cvPreview(t, m)
	m, _ = step(m, *cached)
	m, _ = step(m, *fresh)
	if st.called("MarkRead") != 0 {
		t.Fatal("segna-come-letto con la lista ancora provvisoria")
	}
	m, _ = step(m, connResult{})
	var cmd tea.Cmd
	for _, ms := range runCmd(loadRefs(st, m.folder.Name, m.listGen)) {
		m, cmd = step(m, ms)
	}
	fire(cmd)
	if m.prevRef.ID != msg.NewID("INBOX", 22) {
		t.Errorf("prevRef = %s, voglio la riga fresca INBOX:22", m.prevRef.ID)
	}
	if !m.previewReady() {
		t.Error("l'anteprima deve restare pronta dopo il riallineamento")
	}
	if st.called("MarkRead") != 1 {
		t.Errorf("MarkRead = %d, voglio 1 alla conferma della lista", st.called("MarkRead"))
	}
	m, _ = step(m, pressKey("a"))
	if toastText(m) == provisionalToast {
		t.Error("dopo la conferma l'anteprima e azionabile")
	}
}

func TestPreviewDelayedReadDroppedWhileCached(t *testing.T) {
	st, _ := cvFixture(true)
	m := cvSplitModel(t, st, config.PreviewMarkReadDelay)
	m = freshList(t, m, st)
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m, cached, _ := cvPreview(t, m)
	m, _ = step(m, *cached)
	m, cmd := step(m, previewReadDue{tok: m.prevTok, id: m.thread.AnchorID()})
	fire(cmd)
	if st.called("MarkRead") != 0 {
		t.Error("il timer del segna-come-letto non vale su un'anteprima da istantanea")
	}
}

// Piano 16, Task 13 (dubbio b): un messaggio espanso a mano mentre il thread era
// provvisorio e senza corpo in cache deve avere il corpo fresco dopo la conciliazione,
// non restare su "corpo in caricamento…".
func TestExpandedWithoutBodyGetsFreshBodyAfterReconcile(t *testing.T) {
	st, _ := cvFixture(true)
	delete(st.bodies, msg.NewID(cvAll, 51)) // il messaggio 0 (GmailID 11, letto) non ha corpo in cache
	m := applySnapshot(t, snapModel(t, st), st)
	m, cached, fresh := cvOpen(t, m)
	m, _ = step(m, *cached)
	m, _ = step(m, pressKey("k"))
	m, _ = step(m, pressKey("k")) // selezione sul messaggio 0
	if g := m.thread.Ref().GmailID; g != 11 {
		t.Fatalf("selezione = GmailID %d, voglio 11", g)
	}
	m, _ = step(m, pressKey("enter")) // lo espande: senza corpo, la richiesta e ignorata (provvisorio)
	m, _ = step(m, pressKey("j"))
	m, _ = step(m, pressKey("j")) // la selezione torna sull'ultimo, che ha il corpo
	if g := m.thread.Ref().GmailID; g != 13 {
		t.Fatalf("selezione = GmailID %d, voglio 13", g)
	}
	m, cmd := step(m, *fresh)
	for _, ms := range fire(cmd) {
		if _, ok := ms.(bodyLoaded); ok {
			m, _ = step(m, ms)
		}
	}
	if st.called("FetchMessage:"+string(msg.NewID("INBOX", 20))) == 0 {
		t.Errorf("il corpo del messaggio espanso senza corpo non e stato richiesto: %v", st.touched())
	}
	if v := plain(m); !strings.Contains(v, "corpo fresco 20") {
		t.Errorf("il messaggio espanso non mostra il corpo fresco\n%s", v)
	}
}

// Piano 16, Task 13: la riga in anteprima si riallinea alla lista fresca PRIMA che
// l'anteprima sia stata caricata (timer di 150 ms ancora in volo) con un ID nuovo: il
// thread che arriva dopo deve risultare pronto, non restare vuoto per un'ancora vecchia.
func TestPreviewRealignedBeforeLoadIsReady(t *testing.T) {
	st, _ := cvFixture(true)
	m := cvSplitModel(t, st, config.PreviewMarkReadNever)
	m = applySnapshot(t, m, st)
	if !m.prevHas || m.prevRef.ID != msg.NewID("INBOX", 7) {
		t.Fatalf("anteprima non armata sulla riga provvisoria: prevRef=%s", m.prevRef.ID)
	}
	m = freshList(t, m, st) // il server dice INBOX:22 per la stessa conversazione
	if m.prevRef.ID != msg.NewID("INBOX", 22) {
		t.Fatalf("prevRef = %s, voglio la riga fresca INBOX:22", m.prevRef.ID)
	}
	m, cmd := step(m, previewDue{tok: m.prevTok})
	for _, ms := range fire(cmd) {
		if _, ok := ms.(previewLoaded); ok {
			m, _ = step(m, ms)
		}
	}
	if !m.previewReady() {
		t.Errorf("anteprima non pronta: ancora del thread %s, prevRef %s, prevReq %s", m.thread.AnchorID(), m.prevRef.ID, m.prevReq)
	}
}
