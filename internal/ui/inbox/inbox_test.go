package inbox

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// now fissa per i test: 2026-11-21 è sabato, 09:05 UTC.
func testNow() time.Time {
	return time.Date(2026, 11, 21, 9, 5, 0, 0, time.UTC)
}

// testRefs costruisce ref con date NON in ordine desc: l'ordine di arrivo
// deve essere preservato dalla lista (lo store ordina, l'inbox no).
func testRefs() []msg.MessageRef {
	now := testNow()
	return []msg.MessageRef{
		{ID: msg.NewID("INBOX", 1), From: "a@x.dev", Subject: "prima", Date: now.Add(-50 * time.Minute), Unread: true, Starred: true},
		{ID: msg.NewID("INBOX", 2), From: "b@x.dev", Subject: "Re: riepilogo", Date: time.Date(2026, 11, 20, 23, 59, 0, 0, time.UTC)},
		{
			ID: msg.NewID("INBOX", 3), From: "maria.chiari.lunghissima@azienda-italiana.example.com",
			Subject: "terza", Date: time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC),
		},
	}
}

// newTestModel costruisce l'inbox sized (80×10) con clock fisso.
func newTestModel(t *testing.T, refs []msg.MessageRef) Model {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	m := New(refs, styles.New(th))
	m.SetSize(80, 10)
	clock := testNow()
	m.SetClock(func() time.Time { return clock })
	return m
}

// rows spezza la View in righe (la lista senza decorazioni inizia con le
// righe degli item alla riga 0).
func rows(m Model) []string {
	return strings.Split(m.View(), "\n")
}

// --- RelativeDate -----------------------------------------------------------

func TestRelativeDate(t *testing.T) {
	now := testNow()
	cases := []struct {
		name string
		d    time.Time
		want string
	}{
		{"oggi", now.Add(-50 * time.Minute), "08:15"},
		{"oggi mezzanotte", time.Date(2026, 11, 21, 0, 7, 0, 0, time.UTC), "00:07"},
		{"ieri", time.Date(2026, 11, 20, 23, 59, 0, 0, time.UTC), "ieri"},
		{"ieri stessa ora", time.Date(2026, 11, 20, 9, 5, 0, 0, time.UTC), "ieri"},
		{"stesso anno", time.Date(2026, 11, 15, 8, 0, 0, 0, time.UTC), "15 nov"},
		{"primo gennaio", time.Date(2026, 1, 3, 8, 0, 0, 0, time.UTC), "3 gen"},
		{"anno diverso", time.Date(2025, 1, 3, 8, 0, 0, 0, time.UTC), "3 gen 25"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativeDate(tc.d, now); got != tc.want {
				t.Errorf("RelativeDate(%v) = %q, voglio %q", tc.d, got, tc.want)
			}
		})
	}
}

// --- rendering riga ---------------------------------------------------------

func TestRenderUnreadAndStar(t *testing.T) {
	m := newTestModel(t, testRefs())
	lines := rows(m)
	if len(lines) < 3 {
		t.Fatalf("righe renderizzate = %d, voglio almeno 3", len(lines))
	}
	first := lines[0]
	if !strings.Contains(first, "●") {
		t.Errorf("riga 0 = %q, voglio il marker unread ●", first)
	}
	if !strings.Contains(first, "★") {
		t.Errorf("riga 0 = %q, voglio la stella ★", first)
	}
	if !strings.Contains(first, "a@x.dev") {
		t.Errorf("riga 0 = %q, voglio il mittente", first)
	}
	if !strings.Contains(first, "prima") {
		t.Errorf("riga 0 = %q, voglio l'oggetto", first)
	}
	if !strings.Contains(first, "08:15") {
		t.Errorf("riga 0 = %q, voglio la data relativa 08:15", first)
	}
}

func TestRenderReadAndStarredSlot(t *testing.T) {
	m := newTestModel(t, testRefs())
	lines := rows(m)
	if !strings.Contains(lines[1], "○") {
		t.Errorf("riga 1 = %q, voglio il marker letto ○", lines[1])
	}
	if strings.Contains(lines[1], "★") {
		t.Errorf("riga 1 = %q, non voglio la stella (ref non starred)", lines[1])
	}
	if !strings.Contains(lines[1], "Re: riepilogo") {
		t.Errorf("riga 1 = %q, il prefisso Re: deve restare com'è", lines[1])
	}
	if !strings.Contains(lines[1], "ieri") {
		t.Errorf("riga 1 = %q, voglio la data relativa ieri", lines[1])
	}
}

func TestRenderSenderTruncated(t *testing.T) {
	m := newTestModel(t, testRefs())
	lines := rows(m)
	const long = "maria.chiari.lunghissima@azienda-italiana.example.com"
	if !strings.Contains(lines[2], "…") {
		t.Errorf("riga 2 = %q, voglio il mittente troncato con …", lines[2])
	}
	if strings.Contains(lines[2], long) {
		t.Errorf("riga 2 = %q, il mittente lungo deve essere troncato", lines[2])
	}
}

func TestRenderAdaptiveWidth(t *testing.T) {
	m := newTestModel(t, testRefs())
	for i, line := range rows(m)[:3] {
		if w := lipgloss.Width(line); w != 80 {
			t.Errorf("larghezza riga %d = %d, voglio 80 (larghezza lista)", i, w)
		}
	}
	// larghezza adattiva: la riga si adatta alla nuova dimensione
	m.SetSize(40, 10)
	for i, line := range rows(m)[:3] {
		if w := lipgloss.Width(line); w != 40 {
			t.Errorf("larghezza riga %d dopo resize = %d, voglio 40", i, w)
		}
	}
}

func TestRenderEmptyList(t *testing.T) {
	m := newTestModel(t, nil)
	if strings.Contains(m.View(), "prima") {
		t.Error("lista vuota non deve mostrare ref")
	}
	if strings.TrimSpace(m.View()) == "" {
		t.Error("lista vuota: atteso uno stato (No items.)")
	}
}

// --- SetRefs: ordine preservato ----------------------------------------------

func TestNewPreservesOrder(t *testing.T) {
	// date NON desc: l'inbox non deve riordinare
	m := newTestModel(t, testRefs())
	v := m.View()
	i1, i2, i3 := strings.Index(v, "prima"), strings.Index(v, "Re: riepilogo"), strings.Index(v, "terza")
	if i1 < 0 || i2 < 0 || i3 < 0 || i1 > i2 || i2 > i3 {
		t.Errorf("ordine righe alterato: prima=%d riepilogo=%d terza=%d", i1, i2, i3)
	}
}

func TestSetRefsPreservesOrder(t *testing.T) {
	m := newTestModel(t, []msg.MessageRef{{ID: msg.NewID("INBOX", 9), From: "vecchio@x.dev", Subject: "vecchio"}})
	refs := testRefs()
	cmd := m.SetRefs(refs)
	_ = cmd // col filtraggio disabilitato è nil, ma l'API lo espone
	v := m.View()
	if strings.Contains(v, "vecchio") {
		t.Error("SetRefs non ha sostituito i vecchi item")
	}
	i1, i2, i3 := strings.Index(v, "prima"), strings.Index(v, "Re: riepilogo"), strings.Index(v, "terza")
	if i1 < 0 || i2 < 0 || i3 < 0 || i1 > i2 || i2 > i3 {
		t.Errorf("ordine dopo SetRefs alterato: prima=%d riepilogo=%d terza=%d", i1, i2, i3)
	}
	if _, ok := m.SelectedRef(); !ok {
		t.Error("dopo SetRefs deve esserci un ref selezionabile")
	}
	if sel, _ := m.SelectedRef(); sel.ID != refs[0].ID {
		t.Errorf("ref selezionato = %v, voglio la prima riga %v", sel.ID, refs[0].ID)
	}
}

// --- enter → SelectedMsg ------------------------------------------------------

func TestEnterEmitsSelectedMsg(t *testing.T) {
	m := newTestModel(t, testRefs())
	m.Select(1)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter: cmd nil, voglio SelectedMsg")
	}
	res := cmd()
	sel, ok := res.(SelectedMsg)
	if !ok {
		t.Fatalf("enter: msg = %T, voglio SelectedMsg", res)
	}
	if sel.Ref.ID != msg.NewID("INBOX", 2) {
		t.Errorf("SelectedMsg.Ref = %v, voglio la riga 1 (%v)", sel.Ref.ID, msg.NewID("INBOX", 2))
	}
}

func TestEnterOnEmptyListIsNoop(t *testing.T) {
	m := newTestModel(t, nil)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Errorf("enter su lista vuota: cmd = %v, voglio nil", cmd)
	}
}

func TestSelectedRef(t *testing.T) {
	m := newTestModel(t, testRefs())
	if sel, ok := m.SelectedRef(); !ok || sel.ID != msg.NewID("INBOX", 1) {
		t.Errorf("SelectedRef iniziale = %v,%v, voglio riga 0", sel.ID, ok)
	}
	m.Select(2)
	if sel, _ := m.SelectedRef(); sel.ID != msg.NewID("INBOX", 3) {
		t.Errorf("SelectedRef dopo Select(2) = %v, voglio riga 2", sel.ID)
	}
}

// TestSelectedIndexTracksCursor: l'indice globale del cursore (usato dall'app
// per ripristinare la posizione del cursore dopo il refresh della lista di un
// op, Task 7).
func TestSelectedIndexTracksCursor(t *testing.T) {
	m := newTestModel(t, testRefs())
	if got := m.SelectedIndex(); got != 0 {
		t.Errorf("SelectedIndex iniziale = %d, voglio 0", got)
	}
	m.Select(2)
	if got := m.SelectedIndex(); got != 2 {
		t.Errorf("SelectedIndex dopo Select(2) = %d, voglio 2", got)
	}
}

// --- mouse --------------------------------------------------------------------

func TestMouseWheelMovesCursor(t *testing.T) {
	m := newTestModel(t, testRefs())
	mm, cmd := m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if cmd != nil {
		t.Errorf("wheel down: cmd = %v, voglio nil", cmd)
	}
	if got := mm.list.Cursor(); got != 1 {
		t.Errorf("cursor dopo wheel down = %d, voglio 1", got)
	}
	mm, _ = mm.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if got := mm.list.Cursor(); got != 0 {
		t.Errorf("cursor dopo wheel up = %d, voglio 0", got)
	}
}

func TestMouseClickSelectsRow(t *testing.T) {
	m := newTestModel(t, testRefs())
	mm, cmd := m.Update(tea.MouseClickMsg{Y: 2, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("singolo click: cmd = %v, voglio nil (solo selezione)", cmd)
	}
	if sel, ok := mm.SelectedRef(); !ok || sel.ID != msg.NewID("INBOX", 3) {
		t.Errorf("ref selezionato dopo click su riga 2 = %v, voglio INBOX:3", sel.ID)
	}
}

func TestMouseDoubleClickOpens(t *testing.T) {
	m := newTestModel(t, testRefs())
	clock := testNow()
	m.SetClock(func() time.Time { return clock })

	m, _ = m.Update(tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	clock = clock.Add(400 * time.Millisecond) // entro la finestra di doppio click
	mm, cmd := m.Update(tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("doppio click: cmd nil, voglio SelectedMsg")
	}
	sel, ok := cmd().(SelectedMsg)
	if !ok || sel.Ref.ID != msg.NewID("INBOX", 2) {
		t.Errorf("doppio click: msg = %T %+v, voglio SelectedMsg INBOX:2", sel, sel)
	}
	_ = mm
}

func TestMouseDoubleClickDifferentRowDoesNotOpen(t *testing.T) {
	m := newTestModel(t, testRefs())
	m, _ = m.Update(tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	_, cmd := m.Update(tea.MouseClickMsg{Y: 0, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("click su righe diverse: cmd = %v, voglio nil", cmd)
	}
}

func TestMouseClickOutsideRowsIsNoop(t *testing.T) {
	m := newTestModel(t, testRefs())
	mm, cmd := m.Update(tea.MouseClickMsg{Y: 7, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("click fuori dalle righe: cmd = %v, voglio nil", cmd)
	}
	if sel, ok := mm.SelectedRef(); !ok || sel.ID != msg.NewID("INBOX", 1) {
		t.Errorf("click fuori dalle righe non deve spostare la selezione: %v", sel.ID)
	}
	// click su colonna non left e Y negativo: no panic, no effetto
	if _, cmd := m.Update(tea.MouseClickMsg{Y: 0, Button: tea.MouseRight}); cmd != nil {
		t.Errorf("click destro: cmd = %v, voglio nil", cmd)
	}
	if _, cmd := m.Update(tea.MouseClickMsg{Y: -1, Button: tea.MouseLeft}); cmd != nil {
		t.Errorf("click con Y negativo: cmd = %v, voglio nil", cmd)
	}
}

// --- delega restante a bubbles/list ------------------------------------------

func TestJAndKMoveCursor(t *testing.T) {
	m := newTestModel(t, testRefs())
	mm, _ := m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if got := mm.list.Cursor(); got != 1 {
		t.Errorf("cursor dopo j = %d, voglio 1", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if got := mm.list.Cursor(); got != 0 {
		t.Errorf("cursor dopo k = %d, voglio 0", got)
	}
}

// --- paginazione (Piano 7, Task 3) ---------------------------------------------

// TestAppendRefsDedupAndKeepsCursor: le pagine successive si accodano, gli ID
// già presenti (posta arrivata fra due pagine, slittamento dell'offset) non si
// duplicano, e il cursore resta sulla stessa riga.
func TestAppendRefsDedupAndKeepsCursor(t *testing.T) {
	m := newTestModel(t, testRefs()) // ID 1, 2, 3
	m.Select(1)
	dup := msg.MessageRef{ID: msg.NewID("INBOX", 3), Subject: "duplicato"}
	fresh := msg.MessageRef{ID: msg.NewID("INBOX", 4), From: "d@x.dev", Subject: "quarta"}
	m.AppendRefs([]msg.MessageRef{dup, fresh})

	if got := len(m.list.Items()); got != 4 {
		t.Fatalf("righe dopo AppendRefs = %d, voglio 4 (dedup per ID)", got)
	}
	if sel, _ := m.SelectedRef(); sel.ID != msg.NewID("INBOX", 2) {
		t.Errorf("cursore spostato: selezionato %v, voglio INBOX:2", sel.ID)
	}
	if !strings.Contains(m.View(), "quarta") {
		t.Error("la riga accodata non è resa")
	}
}

// TestAtEnd: il cursore sull'ultima riga è "in fondo"; lista vuota mai.
func TestAtEnd(t *testing.T) {
	m := newTestModel(t, testRefs())
	if m.AtEnd() {
		t.Error("cursore in cima con 3 righe: AtEnd = true")
	}
	m.Select(2)
	if !m.AtEnd() {
		t.Error("cursore sull'ultima riga: AtEnd = false")
	}
	empty := newTestModel(t, nil)
	if empty.AtEnd() {
		t.Error("lista vuota: AtEnd = true (nessuna pagina da chiedere)")
	}
}
