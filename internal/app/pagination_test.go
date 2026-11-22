package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
)

// pagedMsgs costruisce n messaggi INBOX con date desc: il messaggio i ha
// UID i+1 e il più recente è il primo (indice 0), come il Mock ordina.
func pagedMsgs(tb testing.TB, n int) []*msg.Message {
	tb.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	out := make([]*msg.Message, n)
	for i := range out {
		out[i] = &msg.Message{Ref: msg.MessageRef{
			ID:      msg.NewID("INBOX", uint32(i+1)),
			From:    "a@x.dev",
			Subject: "m" + strings.Repeat("x", i%3),
			Date:    base.Add(-time.Duration(i) * time.Hour),
		}}
	}
	return out
}

// pagedModel costruisce il Model radice sullo store dato, dimensionato e con il
// primo load (loadRefs) applicato come farebbe il runtime.
func pagedModel(tb testing.TB, store mail.MailStore) Model {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	m := New(store, noopSender{}, th)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return applyAll(tb, m, loadRefs(store))
}

// applyAll esegue la cmd e applica al modello i msg di risultato (lo spinner
// e i nil del feed sono scartati: il loop spinner non va srotolato nei test).
func applyAll(tb testing.TB, m Model, c tea.Cmd) Model {
	tb.Helper()
	for _, ms := range runCmd(c) {
		switch ms.(type) {
		case nil, spinner.TickMsg:
			continue
		}
		m, _ = step(m, ms)
	}
	return m
}

// settle applica ricorsivamente i risultati che generano altre cmd di dati
// (opDone, msgsLoaded, moreLoaded), con un tetto di profondità.
func settle(tb testing.TB, m Model, c tea.Cmd, depth int) Model {
	tb.Helper()
	if depth == 0 {
		return m
	}
	for _, ms := range runCmd(c) {
		switch ms.(type) {
		case opDone, msgsLoaded, moreLoaded:
			var next tea.Cmd
			m, next = step(m, ms)
			m = settle(tb, m, next, depth-1)
		}
	}
	return m
}

// lastPage è la Page dell'ultima ListMessages registrata dal recorder.
func lastPage(tb testing.TB, rec *recorder) mail.Page {
	tb.Helper()
	if len(rec.pages) == 0 {
		tb.Fatal("nessuna ListMessages registrata")
	}
	return rec.pages[len(rec.pages)-1]
}

// uidsOf restituisce gli UID (in ordine) dei ref del modello.
func uidsOf(tb testing.TB, m Model) []uint32 {
	tb.Helper()
	out := make([]uint32, len(m.refs))
	for i, r := range m.refs {
		_, uid, err := r.ID.Split()
		if err != nil {
			tb.Fatalf("ID %q non valido: %v", r.ID, err)
		}
		out[i] = uid
	}
	return out
}

// hasMoreLoaded dice se la cmd, eseguita, produce una pagina successiva.
func hasMoreLoaded(c tea.Cmd) bool {
	for _, ms := range runCmd(c) {
		if _, ok := ms.(moreLoaded); ok {
			return true
		}
	}
	return false
}

// --- scroll in fondo: pagine successive ----------------------------------------

// TestInboxLoadsNextPageAtEnd: il primo load è di 50; con il cursore in fondo
// la pagina successiva parte (offset = righe caricate), poi arriva l'ultima
// pagina corta e la lista si marca esaurita.
func TestInboxLoadsNextPageAtEnd(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec)
	if len(m.refs) != 50 {
		t.Fatalf("primo load: %d ref, voglio 50", len(m.refs))
	}
	if m.exhausted {
		t.Fatal("dopo una pagina piena la lista non è esaurita")
	}

	m, cmd := step(m, pressKey("G")) // G = fine lista (bubbles/list)
	if cmd == nil || !m.loadingMore {
		t.Fatal("G in fondo: nessuna pagina successiva richiesta")
	}
	m = applyAll(t, m, cmd)
	if len(m.refs) != 100 || m.loadingMore {
		t.Fatalf("dopo la seconda pagina: refs=%d loadingMore=%v, voglio 100 e false", len(m.refs), m.loadingMore)
	}
	if got := lastPage(t, rec); got != (mail.Page{Offset: 50, Limit: 50}) {
		t.Errorf("pagina richiesta = %+v, voglio {Offset:50 Limit:50}", got)
	}

	m, cmd = step(m, pressKey("G"))
	m = applyAll(t, m, cmd)
	if len(m.refs) != 120 || !m.exhausted {
		t.Fatalf("ultima pagina: refs=%d exhausted=%v, voglio 120 e true", len(m.refs), m.exhausted)
	}
	if got := lastPage(t, rec); got != (mail.Page{Offset: 100, Limit: 50}) {
		t.Errorf("ultima pagina richiesta = %+v, voglio {Offset:100 Limit:50}", got)
	}
}

// TestNoPageRequestWhileLoadingMore: con una pagina in flight non si chiede
// l'altra: né tasti né mouse in fondo producono una seconda ListMessages.
func TestNoPageRequestWhileLoadingMore(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec)
	m, first := step(m, pressKey("G"))
	if !m.loadingMore {
		t.Fatal("prima richiesta non partita")
	}
	m, second := step(m, pressKey("G"))
	if hasMoreLoaded(second) {
		t.Error("seconda richiesta con una pagina in volo")
	}
	m, third := step(m, pressKey("j"))
	if hasMoreLoaded(third) {
		t.Error("j con una pagina in volo ha richiesto un'altra pagina")
	}
	m, fourth := step(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if hasMoreLoaded(fourth) {
		t.Error("rotella con una pagina in volo ha richiesto un'altra pagina")
	}
	m = applyAll(t, m, first)
	if len(m.refs) != 100 {
		t.Errorf("refs = %d, voglio 100 (una sola pagina accodata)", len(m.refs))
	}
}

// TestNoPageRequestWhenExhausted: lista esaurita → scroll in fondo non chiede nulla.
func TestNoPageRequestWhenExhausted(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 30)}}
	m := pagedModel(t, rec)
	if !m.exhausted {
		t.Fatal("30 messaggi con pagina da 50: lista attesa esaurita")
	}
	m, cmd := step(m, pressKey("G"))
	if hasMoreLoaded(cmd) || m.loadingMore {
		t.Error("lista esaurita: pagina richiesta comunque")
	}
}

// TestNoPageRequestDuringSearch: con una query attiva lo scroll non chiede
// pagine (la ricerca non è paginata, contratto invariato).
func TestNoPageRequestDuringSearch(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec)
	m.queryRaw = "bob" // lista di ricerca: nessuna paginazione
	m, cmd := step(m, pressKey("G"))
	if hasMoreLoaded(cmd) || m.loadingMore {
		t.Error("lista di ricerca: pagina richiesta")
	}
}

// TestMoreLoadedDedupesShiftedOffset: posta nuova arrivata fra due pagine fa
// slittare l'offset; la pagina successiva ripete un messaggio già visibile, che
// non deve comparire due volte.
func TestMoreLoadedDedupesShiftedOffset(t *testing.T) {
	mock := &mail.Mock{Msgs: pagedMsgs(t, 120)}
	rec := &recorder{MailStore: mock}
	m := pagedModel(t, rec)

	newest := &msg.Message{Ref: msg.MessageRef{
		ID: msg.NewID("INBOX", 999), From: "n@x.dev", Subject: "nuova",
		Date: time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC),
	}}
	mock.Msgs = append([]*msg.Message{newest}, mock.Msgs...)

	m, cmd := step(m, pressKey("G"))
	m = applyAll(t, m, cmd)
	if len(m.refs) != 99 {
		t.Fatalf("refs = %d, voglio 99 (50 + 49 nuove, 1 duplicata scartata)", len(m.refs))
	}
	seen := map[msg.MessageID]bool{}
	for _, r := range m.refs {
		if seen[r.ID] {
			t.Fatalf("ID duplicato dopo la pagina successiva: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

// TestMoreLoadedErrorKeepsListAndRetries: errore della pagina → status d'errore,
// lista intatta, richiesta non bloccata (il prossimo scroll riprova).
func TestMoreLoadedErrorKeepsListAndRetries(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec)
	m, _ = step(m, pressKey("G"))
	m, _ = step(m, moreLoaded{err: errors.New("boom"), query: ""})
	if !strings.Contains(m.statusText(), "boom") || !m.status.isErr {
		t.Errorf("status = %q, voglio errore con boom", m.statusText())
	}
	if len(m.refs) != 50 || m.loadingMore || m.exhausted {
		t.Errorf("dopo errore: refs=%d loadingMore=%v exhausted=%v", len(m.refs), m.loadingMore, m.exhausted)
	}
	m, cmd := step(m, pressKey("G"))
	if !hasMoreLoaded(cmd) {
		t.Error("dopo l'errore lo scroll non riprova la pagina")
	}
}

// TestStaleMoreLoadedDiscarded: una pagina richiesta prima di una ricerca non
// sovrascrive i risultati della ricerca.
func TestStaleMoreLoadedDiscarded(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec)
	m.queryRaw = "bob"
	m, _ = step(m, msgsLoaded{refs: pagedRefs(t, 2), query: "bob", limit: 0})
	before := len(m.refs)
	m, _ = step(m, moreLoaded{refs: pagedRefs(t, 50), query: ""})
	if len(m.refs) != before {
		t.Errorf("pagina stantia accodata alla ricerca: refs=%d, voglio %d", len(m.refs), before)
	}
}

// pagedRefs restituisce i primi n ref di pagedMsgs.
func pagedRefs(tb testing.TB, n int) []msg.MessageRef {
	tb.Helper()
	msgs := pagedMsgs(tb, n)
	out := make([]msg.MessageRef, n)
	for i, mm := range msgs {
		out[i] = mm.Ref
	}
	return out
}

// TestMouseWheelAtEndLoadsNextPage: la rotella che porta il cursore in fondo
// chiede la pagina successiva come i tasti.
func TestMouseWheelAtEndLoadsNextPage(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec) // 50 righe, cursore in cima
	m.inbox.Select(48)      // penultima riga: la rotella giù porta in fondo
	m, cmd := step(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if !hasMoreLoaded(cmd) {
		t.Error("rotella in fondo: pagina successiva non richiesta")
	}
}

// TestArchivedBetweenPagesNoGap: un messaggio archiviato fra due pagine sposta
// tutto di una riga; il refresh a profondità piena (Archive, D5) ricarica la
// finestra e la pagina successiva riparte dall'offset giusto, senza saltare il
// messaggio che è scivolato nella prima pagina.
func TestArchivedBetweenPagesNoGap(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec) // righe UID 1..50
	for range 10 {
		m, _ = step(m, pressKey("j")) // cursore sul messaggio UID 11
	}
	m, cmd := step(m, pressKey("a"))
	m = settle(t, m, cmd, 4)
	if len(m.refs) != 50 {
		t.Fatalf("dopo Archive: refs = %d, voglio 50 (finestra ricaricata)", len(m.refs))
	}
	if got := uidsOf(t, m); got[len(got)-1] != 51 {
		t.Errorf("ultima riga dopo Archive = UID %d, voglio 51 (lo scivolato dalla pagina successiva)", got[len(got)-1])
	}

	m, cmd = step(m, pressKey("G"))
	m = settle(t, m, cmd, 3)
	if got := lastPage(t, rec); got != (mail.Page{Offset: 50, Limit: 50}) {
		t.Errorf("pagina successiva = %+v, voglio {Offset:50 Limit:50}", got)
	}
	got := uidsOf(t, m)
	if len(got) != 100 || got[50] != 52 {
		t.Errorf("dopo la pagina successiva: refs=%d, riga 50 = UID %d; voglio 100 e UID 52 (nessun salto)", len(got), got[50])
	}
	seen := map[uint32]bool{}
	for _, uid := range got {
		if seen[uid] || uid == 11 {
			t.Fatalf("UID %d duplicato o archiviato ancora in lista", uid)
		}
		seen[uid] = true
	}
}

// TestDeepRefreshSurvivesCoalescing: una richiesta a profondità piena arrivata
// mentre un refresh di prima pagina è in volo non si perde: il follow-up parte
// a profondità piena.
func TestDeepRefreshSurvivesCoalescing(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: pagedMsgs(t, 120)}}
	m := pagedModel(t, rec)
	m, cmd := step(m, pressKey("G"))
	m = applyAll(t, m, cmd) // 100 righe caricate

	m, flags := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeFlags}})
	m, _ = step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}}) // coalesced: solo dirty
	first := loadedOf(t, drainCmd(t, m, flags))
	_, follow := step(m, first)
	if follow == nil {
		t.Fatal("follow-up non richiesto dopo il refresh in volo")
	}
	runCmd(follow)
	if got := lastPage(t, rec); got.Limit != 100 {
		t.Errorf("follow-up dopo ChangeNewMail coalesced: Limit = %d, voglio 100 (profondità piena)", got.Limit)
	}
}
