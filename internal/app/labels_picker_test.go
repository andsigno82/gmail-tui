package app

import (
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// convsWithLabels costruisce le conversazioni bersaglio con le label date: una per
// slice, nell'ordine dato (Piano 10, Task 7).
func convsWithLabels(labels ...[]string) []mail.Conversation {
	out := make([]mail.Conversation, 0, len(labels))
	for i, l := range labels {
		out = append(out, mail.Conversation{Latest: msg.MessageRef{ID: msg.NewID("INBOX", uint32(i+1))}, Labels: l})
	}
	return out
}

// Piano 10, Task 7 (D10-L1): la presenza di una label sulle bersaglio è in tutte, in
// alcune (stato misto) o in nessuna.
func TestLabelPickPresence(t *testing.T) {
	convs := convsWithLabels([]string{"Work"}, nil)
	if got := presenceOf("Work", convs); got != presSome {
		t.Errorf("presenza in alcune: %v, voglio presSome", got)
	}
	if got := presenceOf("Work", convsWithLabels([]string{"Work"}, []string{"Work"})); got != presAll {
		t.Errorf("presenza in tutte: %v, voglio presAll", got)
	}
	if got := presenceOf("Lavoro", convs); got != presNone {
		t.Errorf("presenza in nessuna: %v, voglio presNone", got)
	}
}

// Piano 10, Task 7 (D10-L1): lo stato misto si mostra ◪; il primo spazio lo porta a
// ☑, poi ☐, poi ☑ (misto → ☑ → ☐ → ☑).
func TestLabelPickSpaceCycle(t *testing.T) {
	p := pickLabel{name: "Work", start: presSome}
	if got := p.mark(); got != "◪" {
		t.Fatalf("stato iniziale misto: %q, voglio ◪", got)
	}
	p.toggle()
	if got := p.mark(); got != "☑" {
		t.Errorf("dopo il primo spazio: %q, voglio ☑", got)
	}
	p.toggle()
	if got := p.mark(); got != "☐" {
		t.Errorf("dopo il secondo spazio: %q, voglio ☐", got)
	}
	p.toggle()
	if got := p.mark(); got != "☑" {
		t.Errorf("dopo il terzo spazio: %q, voglio ☑", got)
	}
}

// Piano 10, Task 7 (D10-L1): si applica solo ciò che è stato toccato. Lo stato misto
// non toccato non rimuove nulla; una label tolta da tutte si rimuove; una aggiunta
// dove manca si aggiunge; due tocchi che si annullano non cambiano nulla.
func TestLabelPickChanges(t *testing.T) {
	cases := []struct {
		name    string
		start   presence
		toggles int
		add     bool
		remove  bool
	}{
		{"misto non toccato", presSome, 0, false, false},
		{"tutte, tolta", presAll, 1, false, true},
		{"nessuna, aggiunta", presNone, 1, true, false},
		{"misto, portata a ☑", presSome, 1, true, false},
		{"misto, ☑ poi ☐", presSome, 2, false, true},
		{"nessuna, aggiunta e tolta", presNone, 2, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := pickLabel{name: "Work", start: c.start}
			if c.start == presAll {
				p.on = true
			}
			for range c.toggles {
				p.toggle()
			}
			add, remove := p.change()
			if add != c.add || remove != c.remove {
				t.Errorf("change() = (add %v, remove %v), voglio (%v, %v)", add, remove, c.add, c.remove)
			}
		})
	}
}

// Piano 10, Task 7: il filtro digitato restringe l'elenco senza distinguere maiuscole.
func TestLabelPickFilterByTyping(t *testing.T) {
	p := newLabelPicker(convsWithLabels(nil), []string{"Lavoro", "Work", "Newsletter"})
	p.setFilter("lav")
	if got := p.visibleNames(); strings.Join(got, ",") != "Lavoro" {
		t.Errorf("filtro lav: %v, voglio [Lavoro]", got)
	}
	p.setFilter("WORK")
	if got := p.visibleNames(); strings.Join(got, ",") != "Work" {
		t.Errorf("filtro WORK: %v, voglio [Work]", got)
	}
	p.setFilter("")
	if got := p.visibleNames(); len(got) != 3 {
		t.Errorf("filtro vuoto: %v, voglio tutte e tre", got)
	}
}

// Piano 10, Task 7 (D10-L1): con il filtro che non corrisponde a nessuna label, il
// commit crea la label nuova e la aggiunge; una label di sistema è rifiutata senza
// modifiche.
func TestLabelPickCommitNewLabel(t *testing.T) {
	p := newLabelPicker(convsWithLabels(nil), []string{"Work"})
	p.setFilter("Nuova")
	add, remove, err := p.commit()
	if err != nil || strings.Join(add, ",") != "Nuova" || len(remove) != 0 {
		t.Errorf("commit nuova = (%v, %v, %v), voglio aggiunta di Nuova", add, remove, err)
	}

	p = newLabelPicker(convsWithLabels(nil), []string{"Work"})
	p.setFilter("INBOX")
	if _, _, err := p.commit(); err == nil {
		t.Error("commit di INBOX: nessun errore, voglio rifiuto (non è una label utente)")
	}
	if len(p.items) != 1 {
		t.Errorf("il rifiuto ha cambiato l'elenco: %d voci", len(p.items))
	}
}

// Piano 10, Task 7: dalla lista dei mailbox restano le sole label utente.
func TestLabelNamesFromMailboxes(t *testing.T) {
	boxes := []mail.Mailbox{
		{Name: "INBOX", Role: "inbox"},
		{Name: "[Gmail]/Tutti i messaggi", Role: "all"},
		{Name: "[Gmail]/Trash", Role: "trash"},
		{Name: `\Inbox`},
		{Name: "Work"},
		{Name: "Lavoro/Clienti"},
	}
	got := labelNames(boxes)
	if strings.Join(got, ",") != "Work,Lavoro/Clienti" {
		t.Errorf("labelNames = %v, voglio [Work Lavoro/Clienti]", got)
	}
}

// labelsFixture è un Gmail con due conversazioni (thread 101 e 102), ciascuna con la
// sua copia in "Tutti i messaggi" (stesso GmailID). La label Work è sulla prima e,
// con workOnBoth, anche sulla seconda. Lavoro esiste ma non è applicata.
func labelsFixture(t *testing.T, workOnBoth bool) *mail.Mock {
	t.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	all := "[Gmail]/Tutti i messaggi"
	mk := func(box string, uid uint32, gid, thread uint64, labels []string) *msg.Message {
		return &msg.Message{Ref: msg.MessageRef{
			ID: msg.NewID(box, uid), GmailID: gid, ThreadID: thread, Labels: labels,
			From: "anna@x.dev", Subject: "tema", Date: base.Add(time.Duration(uid) * time.Hour),
		}}
	}
	second := []string(nil)
	if workOnBoth {
		second = []string{"Work"}
	}
	return &mail.Mock{
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: all, Role: "all"},
			{Name: "[Gmail]/Trash", Role: "trash"},
			{Name: "Work"},
			{Name: "Lavoro"},
		},
		Msgs: []*msg.Message{
			mk("INBOX", 1, 11, 101, []string{"Work"}), mk(all, 9, 11, 101, []string{"Work"}),
			mk("INBOX", 2, 12, 102, second), mk(all, 8, 12, 102, second),
		},
	}
}

// Tasti del pannello: spazio, frecce. (pressKey copre solo i caratteri e enter/esc.)
var (
	keySpace    = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	pickDownKey = tea.KeyPressMsg{Code: tea.KeyDown}
	keyClick    = tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 1}
)

// pickTypeText digita testo nel filtro, un carattere alla volta.
func pickTypeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m, _ = step(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

// labelOn dice se il messaggio con quel GmailID ha la label in tutte le sue copie.
func labelOn(mock *mail.Mock, gid uint64, label string) bool {
	found := false
	for _, mm := range mock.Msgs {
		if mm.Ref.GmailID != gid {
			continue
		}
		found = true
		if !slices.Contains(mm.Ref.Labels, label) {
			return false
		}
	}
	return found
}

// Piano 10, Task 7: con selezione e Gmail, `l` apre il pannello sulle conversazioni
// selezionate; la lista delle label arriva da ListMailboxes.
func TestLabelPickerOpensOnSelection(t *testing.T) {
	mock := labelsFixture(t, false)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	if !m.labelPickOpen {
		t.Fatal("l con selezione su Gmail: il pannello non si apre")
	}
	m = applyAll(t, m, cmd)
	if m.labelPick.loading {
		t.Fatal("ListMailboxes non è arrivato: il pannello resta in caricamento")
	}
	if got := strings.Join(m.labelPick.visibleNames(), ","); got != "Work,Lavoro" {
		t.Errorf("etichette nel pannello = %q, voglio Work,Lavoro", got)
	}
	if len(m.labelPick.convs) != 2 {
		t.Errorf("bersagli = %d, voglio 2 (la selezione)", len(m.labelPick.convs))
	}
}

// Piano 10, Task 7 (D10-L1): spazio su Work (su tutte) la toglie, spazio su Lavoro (su
// nessuna) l'aggiunge; invio applica insieme aggiunta e rimozione a tutte le copie.
// La selezione si azzera e le righe mostrano le label nuove.
func TestLabelPickerEnterAppliesAddAndRemoveTogether(t *testing.T) {
	mock := labelsFixture(t, true)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m, _ = step(m, keySpace)
	m, _ = step(m, pickDownKey)
	m, _ = step(m, keySpace)
	m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = settle(t, m, cmd, 3) // opDone → refresh silenzioso delle righe
	for _, gid := range []uint64{11, 12} {
		if labelOn(mock, gid, "Work") {
			t.Errorf("gid %d: Work ancora presente, voglio rimossa", gid)
		}
		if !labelOn(mock, gid, "Lavoro") {
			t.Errorf("gid %d: Lavoro assente, voglio aggiunta", gid)
		}
	}
	if m.labelPickOpen {
		t.Error("invio: il pannello resta aperto")
	}
	if len(m.sel) != 0 {
		t.Errorf("selezione dopo l'applicazione = %d, voglio 0", len(m.sel))
	}
	if !toastContains(m, "etichette aggiornate su 2") {
		t.Errorf("toast = %v, voglio l'esito su 2 conversazioni", m.toast.Texts())
	}
	for _, c := range m.inbox.Conversations() {
		if !slices.Contains(c.Labels, "Lavoro") {
			t.Errorf("riga %s: labels = %v, voglio Lavoro (righe aggiornate)", c.Latest.ID, c.Labels)
		}
	}
}

// Piano 10, Task 7: digitare un nome che nessuna label corrisponde e premere invio
// crea la label e la applica a tutte le conversazioni selezionate.
func TestLabelPickerNewLabelByName(t *testing.T) {
	mock := labelsFixture(t, false)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m = typeText(t, m, "Nuova")
	if got := strings.Join(m.labelPick.visibleNames(), ","); got != "" {
		t.Errorf("filtro Nuova: righe visibili = %q, voglio nessuna", got)
	}
	m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	settle(t, m, cmd, 3)
	for _, gid := range []uint64{11, 12} {
		if !labelOn(mock, gid, "Nuova") {
			t.Errorf("gid %d: l'etichetta Nuova non è stata applicata", gid)
		}
	}
}

// Piano 10, Task 7: esc chiude il pannello senza cambiare nulla, anche con caselle
// già toccate.
func TestLabelPickerEscChangesNothing(t *testing.T) {
	mock := labelsFixture(t, true)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m, _ = step(m, keySpace)
	m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	applyAll(t, m, cmd)
	if m.labelPickOpen {
		t.Error("esc: il pannello resta aperto")
	}
	if !labelOn(mock, 11, "Work") || !labelOn(mock, 12, "Work") {
		t.Error("esc ha cambiato le label")
	}
}

// Piano 10, Task 7: con il pannello aperto il mouse è disattivato: un click non
// muove il cursore del pannello né la lista dietro, e non parte nessuna op.
func TestLabelPickerIgnoresMouseWhileOpen(t *testing.T) {
	mock := labelsFixture(t, false)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m, _ = step(m, pickDownKey)
	before, pending := m.labelPick.cursor, m.pending
	m, cmd = step(m, keyClick)
	if !m.labelPickOpen || m.labelPick.cursor != before || m.pending != pending || cmd != nil {
		t.Errorf("click con pannello aperto: aperto=%v cursore=%d (era %d), pending=%d (era %d)",
			m.labelPickOpen, m.labelPick.cursor, before, m.pending, pending)
	}
}

// Piano 10, Task 7: un nome di sistema (INBOX, [Gmail]/…) non diventa una label: il
// pannello resta chiuso senza op, con un toast d'errore.
func TestLabelPickerRefusesSystemName(t *testing.T) {
	mock := labelsFixture(t, false)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m = typeText(t, m, "INBOX")
	m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	applyAll(t, m, cmd)
	if m.labelPickOpen {
		t.Error("nome di sistema: il pannello resta aperto")
	}
	if !toastContains(m, "non è un nome di etichetta valido") {
		t.Errorf("toast = %v, voglio il rifiuto del nome", m.toast.Texts())
	}
	if labelOn(mock, 11, "INBOX") {
		t.Error("INBOX è stata applicata come label")
	}
}

// Piano 10, Task 7 (D10-L2): senza Gmail (conversazioni senza thread) `l` resta il
// prompt label di sempre, non il pannello.
func TestLabelWithoutGmailKeepsPrompt(t *testing.T) {
	m := pagedModel(t, undoFixture(t, true))
	m, _ = step(m, pressKey("l"))
	if m.labelPickOpen {
		t.Error("senza Gmail `l` apre il pannello: voglio il prompt di sempre")
	}
	if !m.labelOpen {
		t.Error("senza Gmail `l` non apre il prompt label")
	}
}

// Piano 10, Task 7 (review punto 1): con filtro non vuoto lo spazio è testo del filtro
// e non cambia la casella; così si digita una label con spazi ("Da leggere").
func TestLabelPickerSpaceIsTextWhenFilterSet(t *testing.T) {
	mock := labelsFixture(t, false)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m = typeText(t, m, "Da")
	m, _ = step(m, keySpace)
	m = typeText(t, m, "leggere")
	if got := m.labelPick.filterText(); got != "Da leggere" {
		t.Fatalf("filtro = %q, voglio %q: lo spazio non è stato digitato", got, "Da leggere")
	}
	m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	settle(t, m, cmd, 3)
	for _, gid := range []uint64{11, 12} {
		if !labelOn(mock, gid, "Da leggere") {
			t.Errorf("gid %d: l'etichetta \"Da leggere\" non è stata applicata", gid)
		}
	}
}

// Piano 10, Task 7 (review punto 1): tab cambia sempre la casella sotto il cursore,
// anche con il filtro digitato.
func TestLabelPickerTabTogglesWithFilterSet(t *testing.T) {
	mock := labelsFixture(t, true)
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, cmd := step(m, pressKey("l"))
	m = applyAll(t, m, cmd)
	m = typeText(t, m, "Work")
	m, _ = step(m, tea.KeyPressMsg{Code: tea.KeyTab})
	m, cmd = step(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	settle(t, m, cmd, 3)
	for _, gid := range []uint64{11, 12} {
		if labelOn(mock, gid, "Work") {
			t.Errorf("gid %d: Work ancora presente, tab doveva rimuoverla", gid)
		}
	}
}

// Piano 10, Task 7 (review punto 2): la modalità del pannello dipende dalla capability
// dello store (GmailExt), non da ThreadID: un server senza X-GM-EXT-1 con thread
// resta sul mini-prompt di sempre.
func TestLabelWithThreadsButNoGmailKeepsPrompt(t *testing.T) {
	mock := labelsFixture(t, false)
	mock.NoGmail = true
	m := pagedModel(t, mock)
	m, _ = step(m, pressKey("X"))
	m, _ = step(m, pressKey("l"))
	if m.labelPickOpen {
		t.Error("senza X-GM-EXT-1 `l` apre il pannello: voglio il prompt di sempre")
	}
	if !m.labelOpen {
		t.Error("senza X-GM-EXT-1 `l` non apre il prompt label")
	}
}
