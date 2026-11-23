package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// Albero cartelle (Piano 15, Task 15, D15-09): selettore `m` su foldertree.

// treeMock è folderMock con una label annidata "Work/Clienti" (Delimiter "/").
func treeMock(tb testing.TB) *mail.Mock {
	tb.Helper()
	mk := folderMock(tb)
	mk.Mailboxes = []mail.Mailbox{
		{Name: "INBOX", Role: "inbox", Delimiter: "/"},
		{Name: "[Gmail]/Sent", Role: "sent", Delimiter: "/"},
		{Name: "Work", Delimiter: "/"},
		{Name: "Work/Clienti", Delimiter: "/"},
	}
	return mk
}

func pressDown() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }
func pressUp() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyUp} }

// TestFolderTreeNestsLabels: le label annidate sul Delimiter appaiono sotto il
// ramo padre, non col nome completo.
func TestFolderTreeNestsLabels(t *testing.T) {
	rec := &recorder{MailStore: treeMock(t)}
	m := openPicker(t, folderModel(t, rec))
	v := m.View().Content
	if !strings.Contains(v, "Clienti") || strings.Contains(v, "Work/Clienti") {
		t.Errorf("label annidata non resa ad albero: %q", v)
	}
	if !strings.Contains(v, "Inviati") || !strings.Contains(v, "Work") {
		t.Errorf("albero senza Inviati/Work: %q", v)
	}
}

// TestFolderTreeNavigation: j/k e le frecce muovono il cursore (con clamp ai
// bordi) e invio apre la cartella sotto il cursore, anche una label annidata.
func TestFolderTreeNavigation(t *testing.T) {
	j := pressKey("j")
	cases := []struct {
		name string
		keys []tea.KeyPressMsg
		want string
	}{
		{"j", []tea.KeyPressMsg{j}, "[Gmail]/Sent"},
		{"freccia giù", []tea.KeyPressMsg{pressDown()}, "[Gmail]/Sent"},
		{"j j k", []tea.KeyPressMsg{j, j, pressKey("k")}, "[Gmail]/Sent"},
		{"su dal primo resta", []tea.KeyPressMsg{pressUp(), pressKey("k"), pressDown()}, "[Gmail]/Sent"},
		{"label annidata", []tea.KeyPressMsg{j, j, j}, "Work/Clienti"},
		{"oltre l'ultima resta", []tea.KeyPressMsg{j, j, j, j, j, j}, "Work/Clienti"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := &recorder{MailStore: treeMock(t)}
			m := openPicker(t, folderModel(t, rec))
			for _, k := range c.keys {
				m, _ = step(m, k)
			}
			m, cmd := step(m, pressKey("enter"))
			m = applyAll(t, m, cmd)
			if m.folderOpen || m.folder.Name != c.want {
				t.Errorf("cartella = %q (open=%v), voglio %q", m.folder.Name, m.folderOpen, c.want)
			}
		})
	}
}

// TestFolderTreeStartsOnCurrentFolder: il cursore riparte dalla cartella
// corrente: invio subito dopo la riapertura non ricarica nulla.
func TestFolderTreeStartsOnCurrentFolder(t *testing.T) {
	rec := &recorder{MailStore: treeMock(t)}
	m := openPicker(t, folderModel(t, rec))
	m, _ = step(m, pressKey("j"))
	m, cmd := step(m, pressKey("enter"))
	m = applyAll(t, m, cmd)
	m = openPicker(t, m)
	before := len(rec.lists)
	m, cmd = step(m, pressKey("enter"))
	_ = applyAll(t, m, cmd)
	if len(rec.lists) != before || m.folder.Name != "[Gmail]/Sent" {
		t.Errorf("riapertura: liste %d→%d cartella=%q, voglio nessuna rilettura in Inviati", before, len(rec.lists), m.folder.Name)
	}
}
