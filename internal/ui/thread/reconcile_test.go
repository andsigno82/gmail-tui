package thread

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 16, Task 6: conciliazione del thread aperto da istantanea con la
// versione fresca. Identità per ThreadID + GmailID, mai per UID.

// epochMsg costruisce un messaggio del thread 42: gid è il GmailID (identità),
// uid il UID dell'epoca (può cambiare tra istantanea e server).
func epochMsg(uid uint32, gid uint64, from, body string, hours int, unread bool) *msg.Message {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	return &msg.Message{
		Ref: msg.MessageRef{
			ID: msg.NewID("INBOX", uid), From: from, Subject: "piano settimana",
			Date: base.Add(time.Duration(hours) * time.Hour), ThreadID: 42, GmailID: gid, Unread: unread,
		},
		To:       []string{"tu@example.com"},
		BodyText: body,
	}
}

// cachedFixture è la versione provvisoria (UID d'epoca vecchia 101..104).
func cachedFixture() []*msg.Message {
	return []*msg.Message{
		epochMsg(101, 1, "a@x.dev", "primo corpo", 0, false),
		epochMsg(102, 2, "b@x.dev", "secondo corpo", 1, false),
		epochMsg(103, 3, "c@x.dev", "terzo corpo", 2, false),
		epochMsg(104, 4, "d@x.dev", "quarto corpo", 3, false),
	}
}

// freshFixture è la stessa conversazione dal server: stessi GmailID, UID nuovi.
func freshFixture() []*msg.Message {
	return []*msg.Message{
		epochMsg(1, 1, "a@x.dev", "primo corpo", 0, false),
		epochMsg(2, 2, "b@x.dev", "secondo corpo", 1, false),
		epochMsg(3, 3, "c@x.dev", "terzo corpo", 2, false),
		epochMsg(4, 4, "d@x.dev", "quarto corpo", 3, false),
	}
}

func refsOf(ms []*msg.Message) []msg.MessageRef {
	out := make([]msg.MessageRef, len(ms))
	for i, m := range ms {
		out[i] = m.Ref
	}
	return out
}

func bodiesOf(ms []*msg.Message) map[msg.MessageID]*msg.Message {
	out := make(map[msg.MessageID]*msg.Message, len(ms))
	for _, m := range ms {
		out[m.Ref.ID] = m
	}
	return out
}

func gids(m Model) []uint64 {
	var out []uint64
	for _, e := range m.conv {
		out = append(out, e.ref.GmailID)
	}
	return out
}

// TestReconcileReplacesProvisionalIDs: stesso thread con UID diversi, gli ID
// provvisori sono sostituiti da quelli confermati e il corpo in cache resta.
func TestReconcileReplacesProvisionalIDs(t *testing.T) {
	c := cachedFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	f := freshFixture()
	m.Reconcile(refsOf(f), nil) // nessun corpo fresco: restano quelli dell'istantanea

	for i, r := range m.Refs() {
		if r.ID != f[i].Ref.ID {
			t.Errorf("ref %d: ID = %s, voglio il confermato %s", i, r.ID, f[i].Ref.ID)
		}
	}
	for i, e := range m.conv {
		if e.full == nil {
			t.Fatalf("messaggio %d: corpo perso dopo la conciliazione", i)
		}
		if e.full.Ref.ID != f[i].Ref.ID {
			t.Errorf("messaggio %d: il corpo conserva l'ID provvisorio %s", i, e.full.Ref.ID)
		}
	}
	if !hasBodyLine(m.View(), "quarto corpo") {
		t.Errorf("il corpo dell'ultimo messaggio deve restare visibile:\n%s", m.View())
	}
}

// TestReconcileKeepsSelectionByGmailID: la selezione segue il GmailID, non il
// UID (che cambia).
func TestReconcileKeepsSelectionByGmailID(t *testing.T) {
	c := cachedFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	m.move(-2) // seleziona il secondo messaggio (GmailID 2)
	if m.selRef().GmailID != 2 {
		t.Fatalf("precondizione: selezionato GmailID %d", m.selRef().GmailID)
	}
	f := freshFixture()
	m.Reconcile(refsOf(f), bodiesOf(f))
	if got := m.Ref(); got.GmailID != 2 || got.ID != f[1].Ref.ID {
		t.Errorf("selezione = GmailID %d ID %s, voglio GmailID 2 ID %s", got.GmailID, got.ID, f[1].Ref.ID)
	}
}

// longMsg ha un corpo di n righe: serve a tenere il viewport scorrevole.
func longMsg(uid uint32, gid uint64, hours int, n int) *msg.Message {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("riga %d del messaggio %d", i, gid)
	}
	return epochMsg(uid, gid, "d@x.dev", strings.Join(lines, "\n"), hours, false)
}

// TestReconcileKeepsExpandedAndScroll: gli espansi per scelta dell'utente e lo
// scorrimento restano; con un messaggio vecchio in più sopra lo scorrimento si
// sposta insieme al messaggio selezionato.
func TestReconcileKeepsExpandedAndScroll(t *testing.T) {
	c := cachedFixture()
	c[3] = longMsg(104, 4, 3, 60)
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 16)
	m.toggle(0) // l'utente espande il primo (letto, compresso di default)
	m.move(3)   // torna sull'ultimo
	if !m.conv[0].expanded {
		t.Fatal("precondizione: primo espanso")
	}
	m.vp.SetYOffset(m.starts[m.sel] + 5)
	rel := m.vp.YOffset() - m.starts[m.sel]

	f := freshFixture()
	f[3] = longMsg(4, 4, 3, 60)
	m.Reconcile(refsOf(f), bodiesOf(f))
	if !m.conv[0].expanded {
		t.Error("il primo messaggio espanso dall'utente si è compresso")
	}
	if m.conv[1].expanded || m.conv[2].expanded {
		t.Error("messaggi compressi diventati espansi")
	}
	if got := m.vp.YOffset() - m.starts[m.sel]; got != rel {
		t.Errorf("scorrimento relativo al selezionato = %d, voglio %d", got, rel)
	}

	// Un messaggio più vecchio in più, compresso (letto): tutto scende di una riga
	// e lo scorrimento lo segue.
	before := m.vp.YOffset()
	g := append([]*msg.Message{epochMsg(9, 9, "z@x.dev", "zero", -1, false)}, f...)
	m.Reconcile(refsOf(g), bodiesOf(g))
	if got := m.vp.YOffset(); got != before+1 {
		t.Errorf("YOffset = %d, voglio %d (selezionato spostato di una riga)", got, before+1)
	}
	if m.selRef().GmailID != 4 {
		t.Errorf("selezione persa: GmailID %d", m.selRef().GmailID)
	}
}

// TestReconcileDoesNotMatchByUIDAcrossEpochs: stesso ID, GmailID diverso = un
// altro messaggio. La selezione, l'espansione e il corpo non lo seguono.
func TestReconcileDoesNotMatchByUIDAcrossEpochs(t *testing.T) {
	c := cachedFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	m.toggle(1) // espande e seleziona il secondo (ID INBOX:102, GmailID 2)
	oldID := m.selRef().ID

	// Fresco: il secondo messaggio non esiste più; un messaggio diverso (GmailID
	// 77) ha ora lo stesso UID 102 dell'epoca vecchia. Ultimo: GmailID 4.
	fr := freshFixture()
	other := epochMsg(102, 77, "q@x.dev", "corpo estraneo", 1, false)
	f := []*msg.Message{fr[0], other, fr[2], fr[3]}
	// Il fresco non porta il corpo del messaggio 77: non deve ereditare quello vecchio.
	m.Reconcile(refsOf(f), bodiesOf([]*msg.Message{fr[0], fr[2], fr[3]}))

	if m.selRef().ID == oldID {
		t.Errorf("la selezione ha seguito l'UID %s verso un altro messaggio", oldID)
	}
	if m.selRef().GmailID != 4 {
		t.Errorf("selezione = GmailID %d, voglio l'ultimo (4)", m.selRef().GmailID)
	}
	e := m.conv[1]
	if e.ref.GmailID != 77 {
		t.Fatalf("ordine inatteso: %v", gids(m))
	}
	if e.full != nil {
		t.Errorf("il messaggio 77 ha ereditato il corpo di un altro messaggio: %q", e.full.BodyText)
	}
	if e.expanded {
		t.Error("il messaggio 77 ha ereditato l'espansione di un altro messaggio")
	}
}

// TestReconcileNewMessageAppended: un messaggio arrivato nel frattempo compare,
// espanso (ultimo non letto); gli altri e la selezione restano.
func TestReconcileNewMessageAppended(t *testing.T) {
	c := cachedFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	f := freshFixture()
	f = append(f, epochMsg(5, 5, "e@x.dev", "quinto corpo", 4, true))
	m.Reconcile(refsOf(f), bodiesOf(f))

	if n := len(m.conv); n != 5 {
		t.Fatalf("messaggi = %d, voglio 5", n)
	}
	if m.selRef().GmailID != 4 {
		t.Errorf("la selezione deve restare sul messaggio 4, è %d", m.selRef().GmailID)
	}
	if !m.conv[4].expanded || m.conv[4].full == nil {
		t.Errorf("il messaggio nuovo deve essere espanso con corpo: expanded=%v", m.conv[4].expanded)
	}
	if !hasBodyLine(m.View(), "quinto corpo") {
		t.Errorf("il corpo nuovo non si vede:\n%s", m.View())
	}
}

// TestReconcileMessageRemoved: un messaggio sparito dal server si toglie; se era
// il selezionato la selezione passa all'ultimo.
func TestReconcileMessageRemoved(t *testing.T) {
	c := cachedFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	m.move(-1) // seleziona il terzo (GmailID 3)
	fr := freshFixture()
	f := []*msg.Message{fr[0], fr[1], fr[3]} // il terzo non c'è più
	m.Reconcile(refsOf(f), bodiesOf(f))

	if got := gids(m); len(got) != 3 || got[0] != 1 || got[1] != 2 || got[2] != 4 {
		t.Fatalf("GmailID dopo la rimozione = %v, voglio [1 2 4]", got)
	}
	if m.selRef().GmailID != 4 || m.sel != 2 {
		t.Errorf("selezione = GmailID %d indice %d, voglio l'ultimo (4, 2)", m.selRef().GmailID, m.sel)
	}
}

// TestReconcileIgnoresOtherThread: il risultato stantio di un altro thread (ThreadID
// diverso) e un risultato vuoto non toccano il thread aperto.
func TestReconcileIgnoresOtherThread(t *testing.T) {
	c := cachedFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	before := m.Refs()

	foreign := freshFixture()
	for _, x := range foreign {
		x.Ref.ThreadID = 77
	}
	m.Reconcile(refsOf(foreign), bodiesOf(foreign))
	m.Reconcile(nil, nil)

	got := m.Refs()
	if len(got) != len(before) {
		t.Fatalf("messaggi = %d, voglio %d", len(got), len(before))
	}
	for i := range got {
		if got[i].ID != before[i].ID {
			t.Errorf("ref %d cambiato da un thread estraneo: %s -> %s", i, before[i].ID, got[i].ID)
		}
	}
}

// TestReconcileFreshBodyReplacesCached: il corpo fresco prende il posto di quello
// in cache anche a ID uguale (il rendering condiviso non serve il vecchio).
func TestReconcileFreshBodyReplacesCached(t *testing.T) {
	rc := NewRenderCache()
	c := cachedFixture()
	m := NewLazyCached(c[3].Ref, refsOf(c), bodiesOf(c), testStyles(t), rc)
	m.SetSize(80, 40)
	if !hasBodyLine(m.View(), "quarto corpo") {
		t.Fatalf("precondizione:\n%s", m.View())
	}
	f := cachedFixture() // stessi UID, corpo cambiato sul server
	f[3] = epochMsg(104, 4, "d@x.dev", "quarto corpo modificato", 3, false)
	m.Reconcile(refsOf(f), bodiesOf(f))
	v := m.View()
	if !hasBodyLine(v, "quarto corpo modificato") || hasBodyLine(v, "quarto corpo") {
		t.Errorf("il corpo fresco deve sostituire quello in cache:\n%s", v)
	}
}

// TestReconcileNoRerenderOfUnchangedBodies: con corpi invariati la chiave della
// cache di rendering resta la stessa e la voce non si ricalcola.
func TestReconcileNoRerenderOfUnchangedBodies(t *testing.T) {
	rc := NewRenderCache()
	c := cachedFixture()
	m := NewLazyCached(c[3].Ref, refsOf(c), bodiesOf(c), testStyles(t), rc)
	m.SetSize(80, 40)
	key := cacheKey{id: c[3].Ref.ID, width: 80, theme: m.theme, variant: "text"}
	first, ok := rc.m[key]
	if !ok || len(first.Lines) == 0 {
		t.Fatal("precondizione: corpo dell'ultimo messaggio non in cache")
	}
	ptr, n := &first.Lines[0], len(rc.order)

	m.Reconcile(refsOf(c), bodiesOf(c)) // identico: nulla di nuovo
	again, ok := rc.m[key]
	if !ok || &again.Lines[0] != ptr {
		t.Error("il corpo invariato è stato reso di nuovo")
	}
	if len(rc.order) != n {
		t.Errorf("voci in cache %d -> %d: nessuna voce nuova attesa", n, len(rc.order))
	}
}

// Piano 16, Task 13: MissingBodies elenca selezionato ed espansi senza corpo.
func TestMissingBodiesListsSelectedAndExpandedWithoutBody(t *testing.T) {
	c := cachedFixture()
	bodies := bodiesOf(c)
	delete(bodies, c[0].Ref.ID) // primo: letto, compresso, senza corpo: non richiesto
	delete(bodies, c[3].Ref.ID) // ultimo: espanso e selezionato, senza corpo
	m := NewLazy(c[3].Ref, refsOf(c), bodies, testStyles(t))
	got := m.MissingBodies()
	if len(got) != 1 || got[0] != c[3].Ref.ID {
		t.Fatalf("MissingBodies = %v, voglio solo %s", got, c[3].Ref.ID)
	}
}
