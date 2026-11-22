package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// gmailRec fa apparire il recorder come store Gmail (server con X-GM-EXT-1):
// lo store dichiara la modalità con GmailExt, come lo Store IMAP (Piano 8, Task 11).
type gmailRec struct{ *recorder }

func (g gmailRec) GmailExt() bool { return true }

// Piano 8, Task 11: con Gmail la query di `/` passa intera a Query.Raw, senza
// riscriverla né ritagliarla: `older_than:` e `-label:` arrivano al backend così come sono.
func TestRicercaGmailPassaLaQueryIntegra(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := pagedModel(t, gmailRec{rec})
	m.search.SetValue("older_than:7d")
	m, cmd := m.submitSearch()
	m = applyAll(t, m, cmd)
	if len(rec.search) != 1 {
		t.Fatalf("ricerche = %d, voglio 1", len(rec.search))
	}
	q := rec.search[0]
	if q.Raw != "older_than:7d" {
		t.Errorf("Query.Raw = %q, voglio \"older_than:7d\" verbatim", q.Raw)
	}
	if q.Label != "" {
		t.Errorf("Query.Label = %q: senza in:/label: la ricerca va in Tutti i messaggi (D8-4), non in una cartella", q.Label)
	}
}

// Piano 8, Task 11: senza Gmail resta il parsing con ParseQuery di prima.
func TestRicercaSenzaGmailUsaParseQuery(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := pagedModel(t, rec)
	m.search.SetValue("from:bob is:unread")
	m, cmd := m.submitSearch()
	m = applyAll(t, m, cmd)
	if len(rec.search) != 1 {
		t.Fatalf("ricerche = %d, voglio 1", len(rec.search))
	}
	q := rec.search[0]
	if q.Raw != "" || q.From != "bob" || !q.Unread {
		t.Errorf("query senza Gmail = %+v, voglio ParseQuery (From bob, Unread, Raw vuoto)", q)
	}
}

// Piano 8, Task 11: il badge dei risultati dichiara l'ambito della ricerca Gmail.
func TestBadgeRicercaGmailDichiaraTuttiIMessaggi(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := pagedModel(t, gmailRec{rec})
	m.search.SetValue("from:me@x.dev")
	m, cmd := m.submitSearch()
	m = applyAll(t, m, cmd)
	want := `risultati per "from:me@x.dev" in Tutti i messaggi`
	if !strings.Contains(m.status.text, want) {
		t.Errorf("status = %q, voglio il badge %q", m.status.text, want)
	}
}

// Piano 8, Task 11: una ricerca con errore del server dà un toast d'errore, lascia
// la lista precedente e riporta lo stato alla query di prima (paginazione e refresh
// riprendono da lì).
func TestErroreRicercaRipristinaLaListaPrecedente(t *testing.T) {
	rec := &recorder{MailStore: folderMock(t)}
	m := pagedModel(t, gmailRec{rec})
	before := len(m.refs)
	rec.err = errors.New("query non valida")
	m.search.SetValue("older_than:7d")
	m, cmd := m.submitSearch()
	m = applyAll(t, m, cmd)
	if !toastIs(m, toast.Error) {
		t.Errorf("nessun toast d'errore dopo la ricerca fallita")
	}
	if len(m.refs) != before {
		t.Errorf("righe = %d, voglio le %d della lista precedente", len(m.refs), before)
	}
	if m.queryRaw != "" {
		t.Errorf("queryRaw = %q dopo l'errore, voglio lo stato della lista precedente (vuoto)", m.queryRaw)
	}
}
