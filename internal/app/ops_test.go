package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// labelCall registra una chiamata ApplyLabel: il Mock è no-op sulle label
// (IMAP-only), senza un wrapper la chiamata non è osservabile.
type labelCall struct {
	ids   []msg.MessageID
	label string
	on    bool
}

// ApplyLabel implementa la registrazione della chiamata sul recorder
// (delega al wrap per il comportamento).
func (r *recorder) ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error {
	r.labels = append(r.labels, labelCall{
		ids: append([]msg.MessageID(nil), ids...), label: label, on: on,
	})
	return r.MailStore.ApplyLabel(ctx, ids, label, on)
}

// opsRefs costruisce tre ref con date GIÀ in ordine desc: il Mock ordina per
// data desc, quindi l'ordine della lista coincide con l'ordine della slice.
func opsRefs(tb testing.TB) []msg.MessageRef {
	tb.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	return []msg.MessageRef{
		{ID: msg.NewID("INBOX", 1), From: "a@x.dev", Subject: "uno", Date: base.Add(3 * time.Hour), Unread: true},
		{ID: msg.NewID("INBOX", 2), From: "b@x.dev", Subject: "due", Date: base.Add(2 * time.Hour)},
		{ID: msg.NewID("INBOX", 3), From: "c@x.dev", Subject: "tre", Date: base.Add(1 * time.Hour)},
	}
}

// mockMsgs converte i ref di opsRefs nei messaggi del Mock (solo ref: le
// mutazioni osservabili sono i campi Unread/Starred e la rimozione Archive).
func mockMsgs(tb testing.TB) []*msg.Message {
	tb.Helper()
	refs := opsRefs(tb)
	out := make([]*msg.Message, len(refs))
	for i, r := range refs {
		out[i] = &msg.Message{Ref: r}
	}
	return out
}

// opsModel costruisce il Model radice sullo store dato, dimensionato e con il
// load iniziale consolidato sui ref di opsRefs (pending 0, cursore su 0).
func opsModel(tb testing.TB, store mail.MailStore) Model {
	tb.Helper()
	th, err := theme.Load("default")
	if err != nil {
		tb.Fatalf("tema default: %v", err)
	}
	m := New(store, noopSender{}, th)
	m, _ = step(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m, _ = step(m, msgsLoaded{refs: opsRefs(tb)})
	return m
}

// opDoneOf estrae l'opDone dai Msg prodotti da una cmd (fallisce se assente).
func opDoneOf(tb testing.TB, msgs []tea.Msg) opDone {
	tb.Helper()
	for _, ms := range msgs {
		if od, ok := ms.(opDone); ok {
			return od
		}
	}
	tb.Fatalf("nessun opDone nei messaggi prodotti dalla cmd (%d msg)", len(msgs))
	return opDone{}
}

// runRefresh esegue la cmd di refresh restituita dall'opDone e applica il
// msgsLoaded (che DEVE essere silenzioso: non tocca lo status) al modello.
func runRefresh(tb testing.TB, m Model, c tea.Cmd) Model {
	tb.Helper()
	if c == nil {
		tb.Fatal("opDone riuscito: cmd nil, voglio il refresh della lista")
	}
	var loaded msgsLoaded
	for _, ms := range runCmd(c) {
		if l, ok := ms.(msgsLoaded); ok {
			loaded = l
		}
	}
	if loaded.err != nil {
		tb.Fatalf("refresh dopo op: err = %v, voglio nil", loaded.err)
	}
	if !loaded.silent {
		tb.Fatal("il refresh dopo un op deve essere silenzioso (non tocca lo status)")
	}
	mm, _ := step(m, loaded)
	return mm
}

// --- tasti op nell'inbox: chiamata giusta al Mock + feedback + refresh ------

// TestArchiveKeyFromInbox: `a` archivia il ref selezionato — il Mock rimuove
// il messaggio, opDone dà feedback "archiviata" e il refresh silenzioso
// aggiorna la lista senza toccare lo status.
func TestArchiveKeyFromInbox(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock) // cursore su "uno" (indice 0)

	mm, cmd := step(m, pressKey("a"))
	if mm.pending != 1 {
		t.Errorf("pending dopo `a` = %d, voglio 1 (op in flight)", mm.pending)
	}
	od := opDoneOf(t, runCmd(cmd))
	if od.label != "archiviata" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {archiviata nil}", od)
	}
	if !od.refresh {
		t.Error("opDone.refresh = false, voglio true (il refresh segue l'op)")
	}
	if len(mock.Msgs) != 2 {
		t.Fatalf("Mock dopo archive = %d messaggi, voglio 2 (\"uno\" rimosso)", len(mock.Msgs))
	}
	for _, mm := range mock.Msgs {
		if mm.Ref.Subject == "uno" {
			t.Error("il messaggio archiviato è ancora nel Mock")
		}
	}

	mm2, cmd2 := step(mm, od)
	if got := toastText(mm2); got != "archiviata" {
		t.Errorf("status dopo opDone = %q, voglio \"archiviata\"", got)
	}
	mm3 := runRefresh(t, mm2, cmd2)
	if len(mm3.refs) != 2 {
		t.Errorf("refs dopo refresh = %d, voglio 2", len(mm3.refs))
	}
	if body := mm3.body(); strings.Contains(body, "uno") {
		t.Errorf("body inbox dopo refresh = %q, \"uno\" non deve comparire più", body)
	}
	if got := toastText(mm3); got != "archiviata" {
		t.Errorf("status dopo refresh silenzioso = %q, voglio \"archiviata\" (il refresh non tocca lo status)", got)
	}
}

// TestStarKeyTogglesOnAndOff: `s` alterna la stella — on la prima volta,
// off la seconda (dopo il refresh, che aggiorna la copia in lista).
func TestStarKeyTogglesOnAndOff(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock) // cursore su "uno", non stellato

	mm, cmd := step(m, pressKey("s"))
	od := opDoneOf(t, runCmd(cmd))
	if od.label != "stella on" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {stella on nil}", od)
	}
	if !mock.Msgs[0].Ref.Starred {
		t.Error("Mock dopo `s`: Starred = false, voglio true")
	}
	mm2, cmd2 := step(mm, od)
	mm3 := runRefresh(t, mm2, cmd2)

	// secondo `s` sulla riga aggiornata (ora stellata): stella off
	mm4, cmd4 := step(mm3, pressKey("s"))
	od2 := opDoneOf(t, runCmd(cmd4))
	if od2.label != "stella off" || od2.err != nil {
		t.Errorf("opDone secondo toggle = %+v, voglio {stella off nil}", od2)
	}
	if mock.Msgs[0].Ref.Starred {
		t.Error("Mock dopo secondo `s`: Starred = true, voglio false")
	}
	_ = mm4
}

// TestVKeyTogglesUnread: `v` segna letto un messaggio non letto ("letta") e
// non letto un messaggio letto ("segna non letta") — MarkRead read=!current.
func TestVKeyTogglesUnread(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock) // cursore su "uno" (Unread true)

	mm, cmd := step(m, pressKey("v"))
	od := opDoneOf(t, runCmd(cmd))
	if od.label != "letta" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {letta nil}", od)
	}
	if mock.Msgs[0].Ref.Unread {
		t.Error("Mock dopo `v` su non letto: Unread = true, voglio false (letto)")
	}
	mm2, cmd2 := step(mm, od)
	mm3 := runRefresh(t, mm2, cmd2)

	// secondo `v` sulla riga aggiornata (ora letta): segna non letta
	mm4, cmd4 := step(mm3, pressKey("v"))
	od2 := opDoneOf(t, runCmd(cmd4))
	if od2.label != "segna non letta" || od2.err != nil {
		t.Errorf("opDone secondo toggle = %+v, voglio {segna non letta nil}", od2)
	}
	if !mock.Msgs[0].Ref.Unread {
		t.Error("Mock dopo secondo `v`: Unread = false, voglio true")
	}
	_ = mm4
}

// --- il mini-prompt delle label ---------------------------------------------

// TestLabelPromptOpensAndCapturesKeys: `l` apre il mini-prompt (render nella
// status bar, input focalizzato); finché è aperto i tasti-op sono TESTO del
// prompt (nessuna mutazione) ed esc annulla senza chiamare lo store.
func TestLabelPromptOpensAndCapturesKeys(t *testing.T) {
	// NoGmail: il mini-prompt è il flusso senza X-GM-EXT-1 (Piano 10, Task 7).
	mock := &mail.Mock{Msgs: mockMsgs(t), NoGmail: true}
	m := opsModel(t, mock)

	mm, cmd := step(m, pressKey("l"))
	if !mm.labelOpen {
		t.Fatal("dopo `l` il prompt label non è aperto")
	}
	if !mm.label.Focused() {
		t.Error("il textinput del prompt non è focalizzato dopo `l`")
	}
	if cmd == nil {
		t.Error("cmd dopo `l` = nil, voglio la blink cmd del cursore")
	}
	if bar := mm.statusBar(); !strings.Contains(bar, "etichetta: ") {
		t.Errorf("status bar con prompt aperto = %q, voglio il prompt \"etichetta: \"", bar)
	}

	// `s` col prompt aperto è testo, non mutazione
	mm, _ = step(mm, pressKey("s"))
	if got := mm.label.Value(); got != "s" {
		t.Errorf("valore prompt dopo `s` = %q, voglio \"s\" (testo del prompt)", got)
	}
	if mock.Msgs[0].Ref.Starred {
		t.Error("`s` col prompt aperto ha mutato la stella: deve restare testo")
	}
	if !mm.labelOpen {
		t.Error("il prompt si è chiuso durante la digitazione")
	}

	mm2, cmd := step(mm, pressKey("esc"))
	if mm2.labelOpen {
		t.Error("esc non ha chiuso il prompt")
	}
	if cmd != nil {
		t.Errorf("esc sul prompt: cmd = %v, voglio nil (nessuna op)", cmd)
	}
}

// TestLabelEnterAppliesLabel: enter col nome digitato chiama ApplyLabel sul
// ref selezionato con on=true, dà feedback "etichetta Work applicata" e
// aggiorna la lista col refresh silenzioso.
func TestLabelEnterAppliesLabel(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: mockMsgs(t)}}
	m := opsModel(t, rec)

	mm, _ := step(m, pressKey("l"))
	mm = typeKeys(mm, "Work")
	mm, cmd := step(mm, pressKey("enter"))
	if mm.labelOpen {
		t.Error("enter non ha chiuso il prompt")
	}

	od := opDoneOf(t, runCmd(cmd))
	if od.label != "etichetta Work applicata" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {etichetta Work applicata nil}", od)
	}
	if len(rec.labels) != 1 {
		t.Fatalf("chiamate ApplyLabel = %d, voglio 1", len(rec.labels))
	}
	call := rec.labels[0]
	if call.label != "Work" || !call.on {
		t.Errorf("ApplyLabel = (label %q, on %v), voglio (\"Work\", true)", call.label, call.on)
	}
	if len(call.ids) != 1 || call.ids[0] != opsRefs(t)[0].ID {
		t.Errorf("ApplyLabel ids = %v, voglio il ref selezionato %v", call.ids, opsRefs(t)[0].ID)
	}
	mm2, cmd2 := step(mm, od)
	if got := toastText(mm2); got != "etichetta Work applicata" {
		t.Errorf("status dopo opDone = %q, voglio \"etichetta Work applicata\"", got)
	}
	runRefresh(t, mm2, cmd2)
}

// TestLabelEmptyEnterCancels: enter col prompt vuoto non chiama lo store e
// chiude il prompt (come un annullamento).
func TestLabelEmptyEnterCancels(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: mockMsgs(t)}}
	m := opsModel(t, rec)

	mm, _ := step(m, pressKey("l"))
	mm, cmd := step(mm, pressKey("enter"))
	if mm.labelOpen {
		t.Error("enter su prompt vuoto deve chiudere il prompt")
	}
	if cmd != nil {
		t.Errorf("enter su prompt vuoto: cmd = %v, voglio nil", cmd)
	}
	if len(rec.labels) != 0 {
		t.Errorf("chiamate ApplyLabel = %d, voglio 0 (nome vuoto)", len(rec.labels))
	}
}

// TestMouseClickIgnoredWhileLabelPromptOpen: col prompt label aperto il mouse
// è bloccato come i tasti (fix merge-gate): un click NON muove il cursore
// dell'inbox e enter applica la label al ref selezionato all'apertura del
// prompt — non alla riga colpita dal click (invariante di confirmLabel).
func TestMouseClickIgnoredWhileLabelPromptOpen(t *testing.T) {
	rec := &recorder{MailStore: &mail.Mock{Msgs: mockMsgs(t)}}
	m := opsModel(t, rec)

	mm, _ := step(m, pressKey("l"))
	if !mm.labelOpen {
		t.Fatal("il prompt label non è aperto")
	}

	// click sulla riga 1: col bug muoverebbe il cursore sotto il prompt
	mm, cmd := step(mm, tea.MouseClickMsg{Y: 1, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("click col prompt aperto: cmd = %v, voglio nil", cmd)
	}
	sel, ok := mm.inbox.SelectedRef()
	if !ok || sel.ID != opsRefs(t)[0].ID {
		t.Fatalf("ref selezionato dopo il click = %v (ok=%v), voglio %v (cursore immobile)",
			sel.ID, ok, opsRefs(t)[0].ID)
	}

	// enter applica comunque la label al ref originale
	mm = typeKeys(mm, "Work")
	mm, cmdEnter := step(mm, pressKey("enter"))
	if mm.labelOpen {
		t.Error("enter non ha chiuso il prompt")
	}
	od := opDoneOf(t, runCmd(cmdEnter))
	if od.label != "etichetta Work applicata" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {etichetta Work applicata nil}", od)
	}
	if len(rec.labels) != 1 {
		t.Fatalf("chiamate ApplyLabel = %d, voglio 1", len(rec.labels))
	}
	if len(rec.labels[0].ids) != 1 || rec.labels[0].ids[0] != opsRefs(t)[0].ID {
		t.Errorf("ApplyLabel ids = %v, voglio il ref selezionato all'apertura %v",
			rec.labels[0].ids, opsRefs(t)[0].ID)
	}
}

// --- op dalla thread view: agiscono sul ref corrente del thread -------------

// TestOpsFromThreadView: nell'inbox. Nel thread `s`/`v`/`a` agiscono sul ref
// del messaggio aperto; la copia locale del ref si aggiorna (toggle consecutivi
// corretti) e il refresh silenzioso aggiorna la lista inbox restanto in thread.
func TestOpsFromThreadView(t *testing.T) {
	ref := opsRefs(t)[1] // "due", già letto: niente auto markread
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock)

	mm, cmd := step(m, inbox.SelectedMsg{Ref: ref})
	var loaded msgLoaded
	for _, ms := range runCmd(cmd) {
		if l, ok := ms.(msgLoaded); ok {
			loaded = l
		}
	}
	mm, _ = step(mm, loaded)
	if mm.currentView() != viewThread || !mm.thread.Loaded() {
		t.Fatalf("vista = %v (loaded %v), voglio thread caricato", mm.currentView(), mm.thread.Loaded())
	}

	// `s` dal thread: stella on sul ref del thread
	mm, cmd = step(mm, pressKey("s"))
	od := opDoneOf(t, runCmd(cmd))
	if od.label != "stella on" || od.err != nil {
		t.Errorf("opDone = %+v, voglio {stella on nil}", od)
	}
	if !mock.Msgs[1].Ref.Starred {
		t.Error("Mock dopo `s` dal thread: Starred = false, voglio true")
	}
	if !mm.thread.Ref().Starred {
		t.Error("copia locale del ref nel thread non aggiornata (toggle consecutivi rotti)")
	}

	// secondo `s` dal thread SENZA refresh intermedio: stella off
	mm, cmd = step(mm, pressKey("s"))
	od = opDoneOf(t, runCmd(cmd))
	if od.label != "stella off" {
		t.Errorf("opDone secondo toggle = %+v, voglio {stella off …}", od)
	}
	if mock.Msgs[1].Ref.Starred {
		t.Error("Mock dopo secondo `s` dal thread: Starred = true, voglio false")
	}

	// `v` dal thread su messaggio letto: segna non letta
	mm, cmd = step(mm, pressKey("v"))
	od = opDoneOf(t, runCmd(cmd))
	if od.label != "segna non letta" {
		t.Errorf("opDone `v` = %+v, voglio {segna non letta …}", od)
	}
	if !mock.Msgs[1].Ref.Unread {
		t.Error("Mock dopo `v` dal thread: Unread = false, voglio true")
	}
	if !mm.thread.Ref().Unread {
		t.Error("copia locale Unread nel thread non aggiornata")
	}

	// `a` dal thread: archivia il ref corrente
	mm, cmd = step(mm, pressKey("a"))
	od = opDoneOf(t, runCmd(cmd))
	if od.label != "archiviata" {
		t.Errorf("opDone `a` = %+v, voglio {archiviata …}", od)
	}
	if len(mock.Msgs) != 2 {
		t.Errorf("Mock dopo archive dal thread = %d messaggi, voglio 2", len(mock.Msgs))
	}

	// il refresh da thread aggiorna la lista inbox in silenzio, resta in thread
	mm2, cmd2 := step(mm, od)
	mm3 := runRefresh(t, mm2, cmd2)
	if len(mm3.refs) != 2 {
		t.Errorf("refs inbox dopo refresh da thread = %d, voglio 2", len(mm3.refs))
	}
	if mm3.currentView() != viewThread {
		t.Errorf("vista dopo refresh = %v, voglio thread (il refresh non cambia vista)", mm3.currentView())
	}
}

// --- errori e concorrenza ----------------------------------------------------

// TestOpErrorShowsErrorToast: un'op fallita finisce in un toast d'errore
// ("label: errore"), chiude l'op e NON emette refresh.
func TestOpErrorShowsErrorToast(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t), Err: errors.New("rete giù")}
	m := opsModel(t, mock)

	mm, cmd := step(m, pressKey("s"))
	od := opDoneOf(t, runCmd(cmd))
	if od.err == nil || od.err.Error() != "rete giù" {
		t.Fatalf("opDone.err = %v, voglio \"rete giù\"", od.err)
	}
	mm2, cmd2 := step(mm, od)
	if cmd2 != nil {
		t.Errorf("cmd dopo op fallita = %v, voglio nil (niente refresh)", cmd2)
	}
	if !toastIs(mm2, toast.Error) {
		t.Error("toast non di errore, voglio true per un'op fallita")
	}
	if got := toastText(mm2); !strings.Contains(got, "stella on") || !strings.Contains(got, "rete giù") {
		t.Errorf("status = %q, voglio \"stella on: rete giù\"", got)
	}
	if mm2.pending != 0 {
		t.Errorf("pending = %d, voglio 0 dopo il risultato", mm2.pending)
	}
}

// TestOpsAllowedWhilePending: le op NON sono bloccate con op store in flight
// (pending>0): il gate del backend serializza e il Mock è sincrono. Il
// doppio-toggle sullo stesso ref fra press e refresh è accettato in v1.
func TestOpsAllowedWhilePending(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock)

	// ricerca in flight (pending 1)
	mm, _ := step(m, pressKey("/"))
	mm = typeKeys(mm, "from:a")
	mm, _ = step(mm, pressKey("enter"))
	if mm.pending != 1 {
		t.Fatalf("pending dopo enter ricerca = %d, voglio 1", mm.pending)
	}

	// `s` mentre la ricerca è in flight: ammessa (pending 2)
	mm, cmd := step(mm, pressKey("s"))
	if mm.pending != 2 {
		t.Errorf("pending dopo `s` con ricerca in flight = %d, voglio 2 (op ammessa)", mm.pending)
	}
	od := opDoneOf(t, runCmd(cmd))
	if od.label != "stella on" {
		t.Errorf("opDone = %+v, voglio {stella on …}", od)
	}
	if !mock.Msgs[0].Ref.Starred {
		t.Error("Mock dopo `s` con ricerca in flight: Starred = false, voglio true")
	}
}

// --- il refresh preserva la posizione del cursore ----------------------------

// TestRefreshPreservesCursorOnStar: il refresh silenzioso su lista invariata
// lascia il cursore sulla stessa riga.
func TestRefreshPreservesCursorOnStar(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock)
	m.inbox.Select(2) // "tre"

	mm, cmd := step(m, pressKey("s"))
	od := opDoneOf(t, runCmd(cmd))
	mm2, cmd2 := step(mm, od)
	mm3 := runRefresh(t, mm2, cmd2)

	sel, ok := mm3.inbox.SelectedRef()
	if !ok || sel.ID != opsRefs(t)[2].ID {
		t.Errorf("ref selezionato dopo refresh = %v (ok=%v), voglio \"tre\" %v (cursore preservato)",
			sel.ID, ok, opsRefs(t)[2].ID)
	}
}

// TestRefreshPreservesCursorAfterArchive: archiviando il messaggio sotto il
// cursore, il cursore cade sul messaggio che ne prende il posto (stesso
// indice, clampato alla lunghezza della nuova lista).
func TestRefreshPreservesCursorAfterArchive(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := opsModel(t, mock)
	m.inbox.Select(1) // "due"

	mm, cmd := step(m, pressKey("a"))
	od := opDoneOf(t, runCmd(cmd))
	mm2, cmd2 := step(mm, od)
	mm3 := runRefresh(t, mm2, cmd2)

	sel, ok := mm3.inbox.SelectedRef()
	if !ok || sel.ID != opsRefs(t)[2].ID {
		t.Errorf("ref selezionato dopo archive = %v (ok=%v), voglio \"tre\" %v (indice clampato)",
			sel.ID, ok, opsRefs(t)[2].ID)
	}
}

// TestRowMailboxDecidesArchiveAndLabel (Piano 7, fix code-review 1): da INBOX una
// ricerca `label:Work` mostra righe della mailbox Work. `a` e `l` agiscono sulla
// mailbox della RIGA (codificata nell'ID), non su quella della cartella mostrata:
// archiviare una riga di Work o applicarle la label Work (COPY nella stessa
// mailbox, duplicato) è rifiutato senza chiamare lo store. La label INBOX resta
// valida (COPY Work→INBOX).
func TestRowMailboxDecidesArchiveAndLabel(t *testing.T) {
	mock := folderMock(t)
	mock.Msgs = append(mock.Msgs, &msg.Message{Ref: msg.MessageRef{
		ID: msg.NewID("Work", 1), From: "bob@x.dev", Subject: "etichettata",
		Date: time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC),
	}})
	rec := &recorder{MailStore: mock}
	m := folderModel(t, rec)
	m, _ = step(m, pressKey("/"))
	m = typeKeys(m, "label:Work bob")
	m, cmd := step(m, pressKey("enter"))
	m = applyAll(t, m, cmd)
	if ref, ok := m.inbox.SelectedRef(); !ok || ref.ID != msg.NewID("Work", 1) {
		t.Fatalf("setup: riga selezionata = %q (ok=%v), voglio la riga di Work", ref.ID, ok)
	}

	before := len(mock.Msgs)
	m, cmd = step(m, pressKey("a"))
	if msgs := dropExpired(runCmd(cmd)); len(msgs) != 0 {
		t.Errorf("a su riga di Work da INBOX: cmd = %v, voglio nessuna op", msgs)
	}
	if len(mock.Msgs) != before {
		t.Errorf("a su riga di Work da INBOX: messaggi %d -> %d, voglio nessuna Archive", before, len(mock.Msgs))
	}
	if want := "archivia è disponibile solo in Posta in arrivo"; toastText(m) != want {
		t.Errorf("status = %q, voglio %q", toastText(m), want)
	}

	m, _ = step(m, pressKey("l"))
	m = typeKeys(m, "Work")
	m, cmd = step(m, pressKey("enter"))
	m = applyAll(t, m, cmd)
	if len(rec.labels) != 0 {
		t.Errorf("label Work su riga di Work: ApplyLabel %d volte, voglio 0 (COPY nella stessa mailbox)", len(rec.labels))
	}
	if !toastIs(m, toast.Error) || !strings.Contains(toastText(m), "cartella corrente") {
		t.Errorf("status = %q (errore=%v), voglio errore sulla cartella corrente", toastText(m), toastIs(m, toast.Error))
	}

	m, _ = step(m, pressKey("l"))
	m = typeKeys(m, "INBOX")
	_, cmd = step(m, pressKey("enter"))
	applyAll(t, m, cmd)
	if len(rec.labels) != 1 {
		t.Errorf("label INBOX su riga di Work: ApplyLabel %d volte, voglio 1", len(rec.labels))
	}
}
