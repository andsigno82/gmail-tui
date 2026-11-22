// Test del composer a livello radice (Piano 4, Task 3): entry paths c/r/f,
// builders reply/forward/new, cancel con conferma discard, rebind scroll
// (f/b liberati nel thread). Piano 4, Task 4: flusso di invio (conferma
// modale → Sender → feedback inbox/errore).
package app

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/theme"
	"gmail-tui/internal/ui/composer"
	"gmail-tui/internal/ui/inbox"
	"gmail-tui/internal/ui/thread"
)

// Conformance (Piano 4, Task 4): il Mock è anche Sender — in demo mode store e
// sender sono UN solo oggetto.
var _ Sender = (*mail.Mock)(nil)

// testIdentity è l'identità del mittente per i test (via SetIdentity).
const (
	testFrom = "me@example.com"
	testSig  = "la firma"
)

// composerModel prepara il radice col Mock e l'identità del mittente.
func composerModel(t *testing.T, msgs ...*msg.Message) (Model, *mail.Mock) {
	t.Helper()
	m, mk := newTestModelMock(t, msgs...)
	m.SetIdentity(testFrom, testSig)
	return m, mk
}

// teaSize simula il primo WindowSizeMsg del terminale.
func teaSize() tea.WindowSizeMsg { return tea.WindowSizeMsg{Width: 80, Height: 24} }

// inboxSelected costruisce il messaggio di apertura messaggio.
func inboxSelected(ref msg.MessageRef) inbox.SelectedMsg {
	return inbox.SelectedMsg{Ref: ref}
}

// errBoom è l'errore di test.
func errBoom() error { return errors.New("boom") }

// openThread porta il radice nella thread view col messaggio caricato.
func openThread(t *testing.T, m Model, ref msg.MessageRef) Model {
	t.Helper()
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, cmd := step(mm, inboxSelected(ref))
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	return mm
}

// --- builders -----------------------------------------------------------------

// TestReplyForBuilder: To=mittente, Subject="Re: …" idempotente, citazione
// con header "Il <data>, <from> ha scritto:" e ogni riga prefissata "> "
// (le righe vuote diventano ">").
func TestReplyForBuilder(t *testing.T) {
	ref := testRefs(t)[0]
	ref.From = "anna@x.dev"
	full := fullMessage(ref)
	cfg := replyFor(ref, full)

	if cfg.To != "anna@x.dev" {
		t.Errorf("To = %q, voglio il mittente anna@x.dev", cfg.To)
	}
	if cfg.Subject != "Re: prima" {
		t.Errorf("Subject = %q, voglio %q", cfg.Subject, "Re: prima")
	}
	wantHeader := "Il " + thread.FormatDate(ref.Date) + ", anna@x.dev ha scritto:"
	if !strings.HasPrefix(cfg.QuotedBody, wantHeader+"\n") {
		t.Errorf("QuotedBody = %q, voglio header %q in testa", cfg.QuotedBody, wantHeader)
	}
	lines := strings.Split(cfg.QuotedBody, "\n")
	quoted := lines[1:] // dopo l'header
	if len(quoted) != 41 {
		t.Fatalf("righe citate = %d, voglio 41 (40 corpo + riga citata)", len(quoted))
	}
	for i, ln := range quoted {
		if ln == "" {
			t.Errorf("riga citata %d vuota (le vuote diventano \">\")", i)
		}
		if !strings.HasPrefix(ln, ">") {
			t.Errorf("riga citata %d senza prefisso \">\": %q", i, ln)
		}
	}
	// La riga già citata nel corpo viene ri-prefissata ("> " per OGNI riga).
	if !strings.Contains(cfg.QuotedBody, "> > riga citata") {
		t.Errorf("la riga già citata deve essere ri-prefissata: %q", cfg.QuotedBody)
	}
}

// TestReplyForSubjectIdempotent: "Re: …" (qualsiasi capitalizzazione) non
// viene ri-prefissato.
func TestReplyForSubjectIdempotent(t *testing.T) {
	ref := testRefs(t)[0]
	for _, subj := range []string{"Re: prima", "re: prima", "RE: prima"} {
		ref.Subject = subj
		if got := replyFor(ref, fullMessage(ref)).Subject; got != subj {
			t.Errorf("Subject = %q, voglio %q intatto", got, subj)
		}
	}
}

// TestForwardForBuilder: Subject="Fwd: …" idempotente, NESSUN destinatario,
// citazione col blocco allegati indicati (non ri-allegati in v1).
func TestForwardForBuilder(t *testing.T) {
	ref := testRefs(t)[0]
	ref.From = "anna@x.dev"
	cfg := forwardFor(ref, fullMessage(ref))

	if cfg.To != "" {
		t.Errorf("To = %q, voglio vuoto (inoltro senza destinatario)", cfg.To)
	}
	if cfg.Subject != "Fwd: prima" {
		t.Errorf("Subject = %q, voglio %q", cfg.Subject, "Fwd: prima")
	}
	if !strings.Contains(cfg.QuotedBody, "f.pdf") {
		t.Errorf("QuotedBody senza il blocco allegati: %q", cfg.QuotedBody)
	}
	if !strings.Contains(cfg.QuotedBody, "> riga corpo") {
		t.Errorf("QuotedBody senza la citazione: %q", cfg.QuotedBody)
	}
	if len(cfg.Attachments) != 0 {
		t.Errorf("Attachments = %v, voglio vuoto (non ri-allegati in v1)", cfg.Attachments)
	}
	// Idempotenza Fwd:.
	ref.Subject = "Fwd: prima"
	if got := forwardFor(ref, fullMessage(ref)).Subject; got != "Fwd: prima" {
		t.Errorf("Subject = %q, voglio %q intatto", got, "Fwd: prima")
	}
	// Senza allegati: nessun blocco.
	ref.Subject = "prima"
	m2 := fullMessage(ref)
	m2.Attachments = nil
	if cfg2 := forwardFor(ref, m2); strings.Contains(cfg2.QuotedBody, "allegat") {
		t.Errorf("blocco allegati presente senza allegati: %q", cfg2.QuotedBody)
	}
}

// TestNewMessageBuilder: campi vuoti, identità dalla config.
func TestNewMessageBuilder(t *testing.T) {
	cfg := newMessage(config.Config{Address: testFrom, Signature: testSig})
	if cfg.From != testFrom || cfg.Signature != testSig {
		t.Errorf("newMessage = %+v, voglio From/Signature dalla config", cfg)
	}
	if cfg.To != "" || cfg.Subject != "" || cfg.QuotedBody != "" || len(cfg.Attachments) != 0 {
		t.Errorf("newMessage = %+v, voglio i campi di contenuto vuoti", cfg)
	}
}

// --- entry path: c --------------------------------------------------------------

// TestRootCOpensNewComposer: `c` nell'inbox apre il composer vuoto col
// mittente dalla identity; esc su corpo vuoto torna all'inbox senza conferma.
func TestRootCOpensNewComposer(t *testing.T) {
	m, _ := composerModel(t)
	mm, cmd := step(m, pressKey("c"))
	if cmd != nil {
		t.Errorf("c: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo c = %v, voglio viewComposer", mm.currentView())
	}
	d := mm.composer.Draft()
	if d.From != testFrom {
		t.Errorf("Draft.From = %q, voglio %q (identity)", d.From, testFrom)
	}
	if d.To != nil || d.Subject != "" {
		t.Errorf("Draft = %+v, voglio campi vuoti", d)
	}
	// esc su corpo vuoto: ritorno diretto (nessun modale), anche con firma.
	mm, cmd = step(mm, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc su corpo vuoto: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo esc = %v, voglio inbox (back diretto)", mm.currentView())
	}
	if mm.discardOpen {
		t.Error("corpo vuoto non deve aprire la conferma discard")
	}
}

// --- entry path: r/f nell'inbox (fetch-then-compose) ----------------------------

// TestRootRInInboxFetchesThenComposes: `r` sull'inbox fetcha il messaggio
// completo (cache-first via store.FetchMessage) e apre il composer di
// risposta precompilato; la firma dell'identity finisce nella bozza.
func TestRootRInInboxFetchesThenComposes(t *testing.T) {
	ref := testRefs(t)[1] // già letta: niente auto MarkRead di mezzo
	ref.From = "anna@x.dev"
	m, _ := composerModel(t, fullMessage(ref))
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm.inbox.Select(1)

	mm, cmd := step(mm, pressKey("r"))
	if mm.pending != 1 {
		t.Errorf("pending dopo r = %d, voglio 1 (fetch in flight)", mm.pending)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista durante il fetch = %v, voglio inbox", mm.currentView())
	}
	var loaded composeLoaded
	found := false
	for _, ms := range runCmd(cmd) {
		if cl, ok := ms.(composeLoaded); ok {
			loaded, found = cl, true
		}
	}
	if !found {
		t.Fatalf("cmd di r: voglio composeLoaded dentro il batch (msgs %T)", cmd())
	}
	if loaded.forward || loaded.ref.ID != ref.ID || loaded.m == nil {
		t.Errorf("composeLoaded = %+v, voglio reply sul ref giusto", loaded)
	}

	mm, cmd2 := step(mm, loaded)
	if cmd2 != nil {
		t.Errorf("composeLoaded: cmd = %v, voglio nil", cmd2)
	}
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo il fetch = %v, voglio viewComposer", mm.currentView())
	}
	if mm.pending != 0 {
		t.Errorf("pending = %d, voglio 0", mm.pending)
	}
	d := mm.composer.Draft()
	if d.To[0] != "anna@x.dev" {
		t.Errorf("Draft.To = %v, voglio il mittente", d.To)
	}
	if d.Subject != "Re: seconda" {
		t.Errorf("Draft.Subject = %q, voglio %q", d.Subject, "Re: seconda")
	}
	if !strings.Contains(d.BodyText, "ha scritto:") || !strings.Contains(d.BodyText, "> riga corpo") {
		t.Errorf("Draft.BodyText senza citazione: %q", d.BodyText)
	}
	if !strings.HasSuffix(d.BodyText, "\n\n--\n"+testSig) {
		t.Errorf("Draft.BodyText senza firma: %q", d.BodyText)
	}
}

// TestRootFInInboxComposesForward: `f` nell'inbox = inoltro (fetch-then-compose).
func TestRootFInInboxComposesForward(t *testing.T) {
	ref := testRefs(t)[0]
	m, _ := composerModel(t, fullMessage(ref))
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})

	mm, cmd := step(mm, pressKey("f"))
	var loaded composeLoaded
	for _, ms := range runCmd(cmd) {
		if cl, ok := ms.(composeLoaded); ok {
			loaded = cl
		}
	}
	if !loaded.forward {
		t.Error("f deve produrre un forward (composeLoaded.forward)")
	}
	mm, _ = step(mm, loaded)
	if mm.currentView() != viewComposer {
		t.Fatalf("vista = %v, voglio viewComposer", mm.currentView())
	}
	d := mm.composer.Draft()
	if d.Subject != "Fwd: prima" {
		t.Errorf("Draft.Subject = %q, voglio %q", d.Subject, "Fwd: prima")
	}
	if d.To != nil {
		t.Errorf("Draft.To = %v, voglio vuoto", d.To)
	}
}

// TestRInInboxWithoutRefIsNoop: senza ref corrente (lista vuota) r/f sono
// no-op silenziosi (come le mutazioni Task 7).
func TestRInInboxWithoutRefIsNoop(t *testing.T) {
	m, _ := composerModel(t)
	// Il load di Init parte con pending=1: lo si consuma (lista vuota) per
	// poter affermare che r/f non incrementano l'op counter.
	mm, _ := step(m, msgsLoaded{})
	for _, k := range []string{"r", "f"} {
		mm, cmd := step(mm, pressKey(k))
		if cmd != nil {
			t.Errorf("%s senza ref: cmd = %v, voglio nil", k, cmd)
		}
		if mm.pending != 0 {
			t.Errorf("%s senza ref: pending = %d, voglio 0", k, mm.pending)
		}
	}
}

// TestComposeLoadedDroppedWhenViewChanged: il risultato del fetch per compose
// è SCARTATO se nel frattempo l'utente ha cambiato vista (r lenta → open:
// nessun yank nel composer).
func TestComposeLoadedDroppedWhenViewChanged(t *testing.T) {
	ref := testRefs(t)[0]
	m, _ := composerModel(t, fullMessage(ref))
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	_, cmd := step(mm, pressKey("r")) // fetch in volo
	// Nel frattempo l'utente apre un messaggio (cambio vista).
	mm, _ = step(mm, inboxSelected(ref))
	if mm.currentView() != viewThread {
		t.Fatalf("vista = %v, voglio thread (precondizione)", mm.currentView())
	}
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	if mm.currentView() != viewThread {
		t.Errorf("vista dopo composeLoaded stante = %v, voglio thread", mm.currentView())
	}
}

// TestComposeLoadedErrorShowsStatusErr: fetch fallito → status d'errore,
// nessun composer.
func TestComposeLoadedErrorShowsStatusErr(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: testRefs(t)})
	mm, _ = step(mm, pressKey("r"))
	mm, cmd := step(mm, composeLoaded{ref: testRefs(t)[0], err: errBoom()})
	if cmd != nil {
		t.Errorf("cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista = %v, voglio inbox", mm.currentView())
	}
	if !mm.status.isErr || !strings.Contains(mm.statusText(), "risposta") || !strings.Contains(mm.statusText(), "boom") {
		t.Errorf("status = %q (isErr=%v), voglio l'errore di risposta", mm.statusText(), mm.status.isErr)
	}
	// Il forward mostra la sua label.
	mm2, _ := step(m, teaSize())
	mm2, _ = step(mm2, msgsLoaded{refs: testRefs(t)})
	mm2, _ = step(mm2, pressKey("f"))
	mm2, _ = step(mm2, composeLoaded{ref: testRefs(t)[0], err: errBoom(), forward: true})
	if !strings.Contains(mm2.statusText(), "inoltro") {
		t.Errorf("status = %q, voglio la label inoltro", mm2.statusText())
	}
}

// --- entry path: r/f nel thread (msg già caricato) -------------------------------

// TestRootRInThreadUsesLoadedMsg: `r` nel thread usa il messaggio caricato
// DIRETTAMENTE (nessun nuovo fetch: pending resta 0, cmd nil).
func TestRootRInThreadUsesLoadedMsg(t *testing.T) {
	ref := testRefs(t)[1]
	ref.From = "anna@x.dev"
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref)
	if mm.pending != 0 {
		t.Fatalf("pending dopo l'apertura = %d, voglio 0", mm.pending)
	}

	mm, cmd := step(mm, pressKey("r"))
	if cmd != nil {
		t.Errorf("r nel thread: cmd = %v, voglio nil (niente fetch)", cmd)
	}
	if mm.pending != 0 {
		t.Errorf("pending dopo r = %d, voglio 0", mm.pending)
	}
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo r = %v, voglio viewComposer", mm.currentView())
	}
	d := mm.composer.Draft()
	if d.To[0] != "anna@x.dev" || d.Subject != "Re: seconda" {
		t.Errorf("Draft = %+v, voglio la reply precompilata", d)
	}
}

// TestRootFInThreadComposesForward: `f` nel thread inoltra il messaggio
// caricato (Subject Fwd:, senza destinatario, blocco allegati).
func TestRootFInThreadComposesForward(t *testing.T) {
	ref := testRefs(t)[1]
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref)

	mm, cmd := step(mm, pressKey("f"))
	if cmd != nil {
		t.Errorf("f nel thread: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo f = %v, voglio viewComposer", mm.currentView())
	}
	d := mm.composer.Draft()
	if d.Subject != "Fwd: seconda" || d.To != nil {
		t.Errorf("Draft = %+v, voglio il forward precompilato", d)
	}
}

// --- rebind scroll thread (decisione controller, review Task 2) ------------------

// TestFInThreadOpensForwardNotHalfPage: `f` nel thread NON scorre più mezza
// pagina — apre l'inoltro (rebind: f liberato per il composer).
func TestFInThreadOpensForwardNotHalfPage(t *testing.T) {
	ref := testRefs(t)[1]
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref)

	mm, _ = step(mm, pressKey("f"))
	if mm.currentView() != viewComposer {
		t.Fatalf("f nel thread: vista = %v, voglio viewComposer (NON scroll)", mm.currentView())
	}
	if mm.composer.Draft().Subject != "Fwd: seconda" {
		t.Error("f deve precompilare il forward")
	}
}

// TestBInThreadIsInert (review Task 3 — test DISCRIMINANTE): `b` è liberato
// (uso futuro). Il test prima scorre giù con `d` (offset > 0: se `b` fosse
// ancora bound alla PageUp del viewport, ora agirebbe) e poi afferma che `b`
// non muove nulla — a offset 0 la vecchia versione non poteva fallire.
func TestBInThreadIsInert(t *testing.T) {
	ref := testRefs(t)[1]
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref)
	mm, _ = step(mm, teaSize())
	mm, _ = step(mm, pressKey("d")) // scroll giù: la vecchia binding b (PageUp) ora agirebbe
	before := mm.thread.YOffset()
	if before <= 0 {
		t.Fatalf("precondizione: d deve scorrere la viewport (offset=%d)", before)
	}
	mm, cmd := step(mm, pressKey("b"))
	if cmd != nil {
		t.Errorf("b: cmd = %v, voglio nil", cmd)
	}
	if mm.thread.YOffset() != before || mm.currentView() != viewThread {
		t.Errorf("b deve essere inerte: offset %d→%d vista %v", before, mm.thread.YOffset(), mm.currentView())
	}
}

// TestScrollKeysStillScrollThread: pgup/pgdn/space/d/u scorrono ancora il
// corpo dopo il rebind; la binding Scroll non documenta più f/b.
func TestScrollKeysStillScrollThread(t *testing.T) {
	for _, k := range keys.Scroll.Keys() {
		if k == "f" || k == "b" {
			t.Errorf("keys.Scroll documenta ancora %q (rebind non fatto)", k)
		}
	}
	ref := testRefs(t)[1]
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref)
	mm, _ = step(mm, teaSize())

	mm, _ = step(mm, pressKey("d"))
	if mm.thread.YOffset() <= 0 {
		t.Errorf("d deve scorrere mezza pagina (offset=%d)", mm.thread.YOffset())
	}
	mm, _ = step(mm, pressKey("u"))
	if mm.thread.YOffset() != 0 {
		t.Errorf("u deve tornare in cima (offset=%d)", mm.thread.YOffset())
	}
}

// --- cancel con conferma ----------------------------------------------------------

// TestComposerCancelWithBodyAsksDiscard: esc su bozza non vuota apre la
// conferma "scartare la bozza?"; s scarta (torna alla vista di provenienza),
// esc riprende nel composer, gli altri tasti sono ignorati.
func TestComposerCancelWithBodyAsksDiscard(t *testing.T) {
	ref := testRefs(t)[1]
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref) // la reply precompilata ha la citazione → corpo non vuoto

	mm, _ = step(mm, pressKey("r"))
	if mm.currentView() != viewComposer {
		t.Fatalf("vista = %v, voglio viewComposer", mm.currentView())
	}
	mm, _ = step(mm, pressKey("esc"))
	if !mm.discardOpen {
		t.Fatal("esc su bozza non vuota deve aprire la conferma discard")
	}
	if !strings.Contains(mm.statusBar(), "scartare la bozza") {
		t.Errorf("status bar = %q, voglio la conferma", mm.statusBar())
	}
	// Tasto estraneo: ignorato (nessuna azione alla cieca, come i modali).
	mm, cmd := step(mm, pressKey("x"))
	if cmd != nil || !mm.discardOpen || mm.currentView() != viewComposer {
		t.Errorf("tasto estraneo nel modale: cmd=%v discardOpen=%v vista=%v", cmd, mm.discardOpen, mm.currentView())
	}
	// esc riprende: composer intatto.
	mm, _ = step(mm, pressKey("esc"))
	if mm.discardOpen || mm.currentView() != viewComposer {
		t.Errorf("esc deve riprendere: discardOpen=%v vista=%v", mm.discardOpen, mm.currentView())
	}
	// esc di nuovo → conferma → s scarta e torna al thread.
	mm, _ = step(mm, pressKey("esc"))
	mm, cmd = step(mm, pressKey("s"))
	if cmd != nil {
		t.Errorf("s: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewThread {
		t.Errorf("vista dopo lo scarto = %v, voglio thread (provenienza)", mm.currentView())
	}
	if mm.discardOpen {
		t.Error("discardOpen resta true dopo lo scarto")
	}
	if mm.statusText() != "bozza scartata" {
		t.Errorf("status = %q, voglio %q", mm.statusText(), "bozza scartata")
	}
}

// TestComposerCancelEmptyBodyGoesBackDirectly: nel composer NUOVO (corpo
// vuoto, firma a parte) esc torna alla vista di provenienza senza conferma.
func TestComposerCancelEmptyBodyGoesBackDirectly(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, pressKey("c"))
	mm, _ = step(mm, pressKey("esc"))
	if mm.discardOpen || mm.currentView() != viewInbox {
		t.Errorf("esc su vuoto: discardOpen=%v vista=%v, voglio false/inbox", mm.discardOpen, mm.currentView())
	}
}

// TestComposerTypingDoesNotTriggerOps: i tasti operazione (r/f/…) digitati
// nel composer sono TESTO — nessun op, nessun cambio vista.
func TestComposerTypingDoesNotTriggerOps(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, pressKey("c"))
	mm, _ = step(mm, pressKey("tab")) // → Oggetto
	mm, _ = step(mm, pressKey("tab")) // → Corpo
	mm, _ = step(mm, pressKey("tab")) // → Allegati
	mm, _ = step(mm, pressKey("tab")) // → A (wrap)
	mm, _ = step(mm, pressKey("r"))   // testo in A
	mm, _ = step(mm, pressKey("f"))   // testo in A
	if mm.currentView() != viewComposer {
		t.Fatalf("vista dopo r/f = %v, voglio composer (testo, non ops)", mm.currentView())
	}
	if got := mm.composer.Draft().To; len(got) != 1 || got[0] != "rf" {
		t.Errorf("To = %v, voglio [rf] (r e f digitati)", got)
	}
}

// --- invio: conferma + Sender (Piano 4, Task 4) -------------------------------------

// senderRecorder è un Sender di test: registra le bozze consegnate (e il ctx
// ricevuto, per il budget di timeout) e può fallire on demand.
type senderRecorder struct {
	calls []msg.Outgoing
	ctx   context.Context
	err   error
}

func (r *senderRecorder) Send(ctx context.Context, out msg.Outgoing) error {
	r.ctx = ctx
	r.calls = append(r.calls, out)
	return r.err
}

// senderModel prepara il radice con lo store Mock e un Sender dato (recorder
// nei test del flusso di invio, Mock nei test demo — un solo oggetto).
func senderModel(t *testing.T, sender Sender, msgs ...*msg.Message) (Model, *mail.Mock) {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema default: %v", err)
	}
	mock := &mail.Mock{Msgs: msgs}
	return New(mock, sender, th), mock
}

// draftComposer apre il composer del radice sulla bozza data, come farebbe
// l'utente: passa da openComposer (view switch, bozza custodita azzerata,
// fill-if-empty dell'identity — la config del test porta già mittente/firma).
func draftComposer(m Model, cfg composer.ComposerConfig) Model {
	cfg.From, cfg.Signature = testFrom, testSig
	m.openComposer(cfg)
	return m
}

// sendRequested porta il radice allo stato di conferma invio: composer con
// bozza completa + ctrl+s + consegna del SendRequestedMsg alla radice.
func sendRequested(t *testing.T, m Model) Model {
	t.Helper()
	mm, cmd := step(m, pressKey("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s: cmd nil, voglio SendRequestedMsg")
	}
	res, ok := cmd().(composer.SendRequestedMsg)
	if !ok {
		t.Fatalf("cmd = %T, voglio composer.SendRequestedMsg", cmd())
	}
	mm, _ = step(mm, res)
	return mm
}

// ansiRe toglie le sequenze di stile lipgloss (il render della barra porta colori).
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// wsRe collassa le sequenze di whitespace (il wrap a 80 colonne spezza il
// testo della conferma, il padding della barra riempie le righe).
var wsRe = regexp.MustCompile(`\s+`)

// plainBar rende la status bar come testo puro a whitespace normalizzato: il
// testo della conferma può andare a capo ai 80 colonne del terminale di test.
func plainBar(m Model) string {
	return wsRe.ReplaceAllString(ansiRe.ReplaceAllString(m.statusBar(), ""), " ")
}

// TestSendRequestedOpensConfirmation: ctrl+s emette SendRequestedMsg — la
// radice custodisce la bozza e apre la conferma modale ("Invio a …" nella
// status bar); il composer resta la vista attiva e il sender NON è toccato.
func TestSendRequestedOpensConfirmation(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev, bob@y.dev", Subject: "oggetto", Body: "corpo",
		Attachments: []string{"/tmp/a.pdf"},
	})
	mm = sendRequested(t, mm)
	if !mm.confirmSend {
		t.Error("ctrl+s deve aprire la conferma di invio")
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista dopo ctrl+s = %v, voglio composer (resta aperto)", mm.currentView())
	}
	if mm.draft.To[0] != "anna@x.dev" || !strings.HasSuffix(mm.draft.BodyText, "\n\n--\n"+testSig) {
		t.Errorf("draft custodito = %+v, voglio la bozza con firma", mm.draft)
	}
	if len(rec.calls) != 0 {
		t.Errorf("sender chiamato alla sola conferma: %d chiamate", len(rec.calls))
	}
}

// TestSendConfirmationShowsToSubjectAttachments: il testo della conferma porta
// i destinatari decodificati da Out.To (join virgole), l'oggetto e il numero
// di allegati (plurale italiano); senza destinatari il placeholder è esplicito.
func TestSendConfirmationShowsToSubjectAttachments(t *testing.T) {
	cases := []struct {
		name string
		cfg  composer.ComposerConfig
		want []string
		not  string
	}{
		{
			name: "to+oggetto+2 allegati",
			cfg:  composer.ComposerConfig{To: "anna@x.dev, bob@y.dev", Subject: "Re: seconda", Body: "c", Attachments: []string{"/tmp/a.pdf", "/tmp/b.pdf"}},
			want: []string{"Invio a anna@x.dev, bob@y.dev", "Re: seconda", "2 allegati", "s conferma", "esc annulla"},
		},
		{
			name: "1 allegato (singolare)",
			cfg:  composer.ComposerConfig{To: "anna@x.dev", Subject: "s", Body: "c", Attachments: []string{"/tmp/a.pdf"}},
			want: []string{"Invio a anna@x.dev", "1 allegato"},
			not:  "allegati",
		},
		{
			name: "nessun destinatario (inoltro da riempire)",
			cfg:  composer.ComposerConfig{Subject: "Fwd: prima", Body: "c"},
			want: []string{"Invio a (nessun destinatario)", "Fwd: prima"},
			not:  "allegat",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &senderRecorder{}
			m, _ := senderModel(t, rec)
			mm, _ := step(m, teaSize())
			mm = sendRequested(t, draftComposer(mm, tc.cfg))
			bar := plainBar(mm)
			for _, w := range tc.want {
				if !strings.Contains(bar, w) {
					t.Errorf("status bar senza %q: %q", w, bar)
				}
			}
			if tc.not != "" && strings.Contains(bar, tc.not) {
				t.Errorf("status bar contiene %q a sproposito: %q", tc.not, bar)
			}
		})
	}
}

// TestSendConfirmEscPreservesDraft: esc alla conferma riprende il composer —
// bozza intatta nei campi, nessun invio, conferma chiusa.
func TestSendConfirmEscPreservesDraft(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
		Attachments: []string{"/tmp/a.pdf"},
	}))
	mm, cmd := step(mm, pressKey("esc"))
	if cmd != nil {
		t.Errorf("esc alla conferma: cmd = %v, voglio nil", cmd)
	}
	if mm.confirmSend {
		t.Error("esc deve chiudere la conferma")
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista dopo esc = %v, voglio composer (bozza preservata)", mm.currentView())
	}
	if len(rec.calls) != 0 {
		t.Errorf("esc deve annullare: sender chiamato %d volte", len(rec.calls))
	}
	d := mm.composer.Draft()
	if d.To[0] != "anna@x.dev" || d.Subject != "oggetto" || len(d.Attachments) != 1 {
		t.Errorf("bozza dopo esc = %+v, voglio i campi intatti", d)
	}
}

// TestSendConfirmSendsExactOutgoing: s alla conferma chiama il sender con la
// bozza ESATTA (mittente/firma/destinatari/oggetto/corpo/allegati) dentro un
// ctx con budget di timeout; niente ritorno alla vista finché l'opDone non
// arriva (spinner: pending 1).
func TestSendConfirmSendsExactOutgoing(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{}) // consolida il load iniziale (pending 0)
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev, bob@y.dev", Cc: "cc@x.dev", Subject: "oggetto", Body: "corpo",
		Attachments: []string{"/tmp/a.pdf", "/tmp/b.pdf"},
	}))
	mm, cmd := step(mm, pressKey("s"))
	if mm.pending != 1 {
		t.Errorf("pending dopo s = %d, voglio 1 (invio in flight)", mm.pending)
	}
	if mm.confirmSend {
		t.Error("s deve chiudere la conferma")
	}
	var done opDone
	found := false
	for _, ms := range runCmd(cmd) {
		if od, ok := ms.(opDone); ok {
			done, found = od, true
		}
	}
	if !found {
		t.Fatalf("cmd di s: voglio opDone dentro il batch (msgs %v)", cmd())
	}
	if done.err != nil {
		t.Errorf("opDone err = %v, voglio nil (recorder ok)", done.err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("sender chiamato %d volte, voglio 1", len(rec.calls))
	}
	got := rec.calls[0]
	if got.From != testFrom {
		t.Errorf("Out.From = %q, voglio %q (identity)", got.From, testFrom)
	}
	if len(got.To) != 2 || got.To[0] != "anna@x.dev" || got.To[1] != "bob@y.dev" {
		t.Errorf("Out.To = %v, voglio [anna@x.dev bob@y.dev]", got.To)
	}
	if len(got.Cc) != 1 || got.Cc[0] != "cc@x.dev" {
		t.Errorf("Out.Cc = %v, voglio [cc@x.dev]", got.Cc)
	}
	if got.Subject != "oggetto" {
		t.Errorf("Out.Subject = %q, voglio %q", got.Subject, "oggetto")
	}
	if !strings.HasPrefix(got.BodyText, "corpo") || !strings.HasSuffix(got.BodyText, "\n\n--\n"+testSig) {
		t.Errorf("Out.BodyText = %q, voglio corpo+firma appended", got.BodyText)
	}
	if len(got.Attachments) != 2 || got.Attachments[0] != "/tmp/a.pdf" || got.Attachments[1] != "/tmp/b.pdf" {
		t.Errorf("Out.Attachments = %v, voglio i 2 allegati", got.Attachments)
	}
	// Budget di timeout: il ctx porta una deadline entro ~60s (const condivisa
	// col sottocomando send) e non è già scaduta.
	dl, ok := rec.ctx.Deadline()
	if !ok {
		t.Fatal("ctx senza deadline: voglio il budget sendTimeout")
	}
	if rem := time.Until(dl); rem <= 0 || rem > 61*time.Second {
		t.Errorf("deadline del ctx a %v, voglio ~60s", rem)
	}
	// Finchè l'opDone non arriva il composer resta (spinner in status bar).
	if mm.currentView() != viewComposer {
		t.Errorf("vista durante l'invio = %v, voglio composer", mm.currentView())
	}
}

// TestSendSuccessReturnsToInboxAndRefreshes: opDone{"inviato", nil} → back
// inbox, composer chiuso, feedback "inviato" e re-list silenzioso della lista.
func TestSendSuccessReturnsToInboxAndRefreshes(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{To: "anna@x.dev", Subject: "s", Body: "c"}))
	mm, cmd := step(mm, pressKey("s"))
	var done opDone
	for _, ms := range runCmd(cmd) {
		if od, ok := ms.(opDone); ok {
			done = od
		}
	}
	mm, cmd2 := step(mm, done)
	if cmd2 == nil {
		t.Fatal("opDone ok: cmd nil, voglio il re-list silenzioso")
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo l'invio = %v, voglio inbox", mm.currentView())
	}
	if mm.confirmSend || mm.discardOpen {
		t.Errorf("modali aperte dopo l'invio: confirm=%v discard=%v", mm.confirmSend, mm.discardOpen)
	}
	if mm.draft.Subject != "" || len(mm.draft.To) != 0 || mm.draft.BodyText != "" {
		t.Errorf("draft = %+v, voglio azzerato dopo l'invio", mm.draft)
	}
	if mm.statusText() != "inviato" {
		t.Errorf("status = %q, voglio %q", mm.statusText(), "inviato")
	}
	if !mm.refreshing {
		t.Error("dopo l'invio deve partire il refresh silenzioso della lista")
	}
}

// TestSendErrorStaysInComposerWithStatusErr: errore del sender → StatusErr col
// dettaglio; il composer resta aperto con la bozza intatta (riprovabile con
// ctrl+s — la conferma si riapre).
func TestSendErrorStaysInComposerWithStatusErr(t *testing.T) {
	rec := &senderRecorder{err: errBoom()}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
	}))
	mm, cmd := step(mm, pressKey("s"))
	var done opDone
	for _, ms := range runCmd(cmd) {
		if od, ok := ms.(opDone); ok {
			done = od
		}
	}
	mm, cmd2 := step(mm, done)
	if cmd2 != nil {
		t.Errorf("opDone errore: cmd = %v, voglio nil (nessun refresh)", cmd2)
	}
	if !mm.status.isErr {
		t.Errorf("status isErr = false, voglio errore per %q", mm.statusText())
	}
	if !strings.Contains(mm.statusText(), "invio") || !strings.Contains(mm.statusText(), "boom") {
		t.Errorf("status = %q, voglio il dettaglio del sender", mm.statusText())
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista dopo l'errore = %v, voglio composer (bozza preservata)", mm.currentView())
	}
	d := mm.composer.Draft()
	if d.To[0] != "anna@x.dev" || d.Subject != "oggetto" {
		t.Errorf("bozza dopo l'errore = %+v, voglio i campi intatti (riprova)", d)
	}
	// Riprova: ctrl+s riapre la conferma sulla stessa bozza.
	mm = sendRequested(t, mm)
	if !mm.confirmSend {
		t.Error("ctrl+s dopo l'errore deve riaprire la conferma")
	}
}

// TestSendConfirmOtherKeysAndMouseIgnored: con la conferma aperta i tasti
// estranei (e il mouse) sono ignorati — niente testo nei campi, niente ops,
// nessun invio (stessa regola dei modali discard/help/prompt).
func TestSendConfirmOtherKeysAndMouseIgnored(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
	}))
	for _, k := range []string{"x", "j", "r", "enter", "tab"} {
		mm, cmd := step(mm, pressKey(k))
		if cmd != nil {
			t.Errorf("tasto %s alla conferma: cmd = %v, voglio nil", k, cmd)
		}
		if !mm.confirmSend {
			t.Fatalf("tasto %s ha chiuso la conferma", k)
		}
	}
	if got := mm.composer.Draft().To; len(got) != 1 || got[0] != "anna@x.dev" {
		t.Errorf("To dopo tasti estranei = %v, voglio i campi intatti (niente typing)", got)
	}
	mm, cmd := step(mm, tea.MouseClickMsg{Y: 3, Button: tea.MouseLeft})
	if cmd != nil || !mm.confirmSend {
		t.Errorf("click alla conferma: cmd=%v confirmSend=%v, voglio nil/true", cmd, mm.confirmSend)
	}
	if len(rec.calls) != 0 {
		t.Errorf("sender chiamato senza s: %d chiamate", len(rec.calls))
	}
}

// TestSendDemoMockRecordsSent: in demo mode il Mock è store E sender (un solo
// oggetto): il flusso completo finisce in Mock.Sent — osservabile dai test e
// dal wizard (Piano 4, Task 5).
func TestSendDemoMockRecordsSent(t *testing.T) {
	m, mk := composerModel(t)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
		Attachments: []string{"/tmp/a.pdf"},
	}))
	mm, cmd := step(mm, pressKey("s"))
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	if len(mk.Sent) != 1 {
		t.Fatalf("Mock.Sent = %d messaggi, voglio 1", len(mk.Sent))
	}
	sent := mk.Sent[0]
	if sent.From != testFrom || sent.To[0] != "anna@x.dev" || sent.Subject != "oggetto" {
		t.Errorf("Sent[0] = %+v, voglio la bozza inviata", sent)
	}
	if !strings.HasSuffix(sent.BodyText, "\n\n--\n"+testSig) {
		t.Errorf("Sent[0].BodyText senza firma: %q", sent.BodyText)
	}
	if mm.currentView() != viewInbox {
		t.Errorf("vista = %v, voglio inbox dopo l'invio demo", mm.currentView())
	}
}

// TestSendRequestedStaleDropped: SendRequestedMsg arrivato fuori dal composer
// (bozza stante: cambio vista fra pressione e consegna) è scartato — nessuna
// conferma fantasma.
func TestSendRequestedStaleDropped(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm, cmd := step(mm, composer.SendRequestedMsg{Out: msg.Outgoing{Subject: "stante"}})
	if cmd != nil {
		t.Errorf("cmd = %v, voglio nil", cmd)
	}
	if mm.confirmSend || mm.draft.Subject != "" {
		t.Errorf("SendRequestedMsg stante accettato: confirm=%v draft=%+v", mm.confirmSend, mm.draft)
	}
}

// TestSendInFlightCtrlSDropped: ctrl+s DURANTE un invio in flight (pending 1,
// sender già chiamato una volta — opDone non ancora consegnato) è scartato
// silenziosamente: la conferma NON si riapre e un secondo `s` non può
// consegnare una seconda copia della stessa bozza (recorder resta a 1).
func TestSendInFlightCtrlSDropped(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{}) // consolida il load iniziale (pending 0)
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
	}))
	mm, cmd := step(mm, pressKey("s"))
	if mm.pending != 1 {
		t.Fatalf("pending dopo s = %d, voglio 1 (invio in flight)", mm.pending)
	}
	// Esegue la cmd SENZA consegnare l'opDone: il send parte (recorder 1) ma
	// resta in flight (pending 1), come con un SMTP lento.
	for range runCmd(cmd) {
	}
	if len(rec.calls) != 1 {
		t.Fatalf("sender dopo il primo s = %d chiamate, voglio 1", len(rec.calls))
	}
	// ctrl+s durante l'invio: il composer emette SendRequestedMsg, la radice
	// lo scarta (pending > 0) — nessuna conferma fantasma.
	mm, cmd2 := step(mm, pressKey("ctrl+s"))
	if cmd2 == nil {
		t.Fatal("ctrl+s: cmd nil, voglio SendRequestedMsg dal composer")
	}
	res, ok := cmd2().(composer.SendRequestedMsg)
	if !ok {
		t.Fatalf("cmd = %T, voglio composer.SendRequestedMsg", cmd2())
	}
	mm, _ = step(mm, res)
	if mm.confirmSend {
		t.Error("ctrl+s durante l'invio non deve riaprire la conferma")
	}
	if mm.pending != 1 {
		t.Errorf("pending dopo ctrl+s = %d, voglio 1 (l'invio resta l'unico)", mm.pending)
	}
	// Un secondo `s` (ora testo nel campo, conferma mai aperta) non invia.
	mm, _ = step(mm, pressKey("s"))
	if len(rec.calls) != 1 {
		t.Errorf("sender dopo ctrl+s + s = %d chiamate, voglio 1 (no doppia email)", len(rec.calls))
	}
}

// TestSendConfirmSRefusedWhileSending: difesa in profondità — `s` alla conferma
// con un invio già in flight (sending true; stato non raggiungibile via UI
// perché il SendRequestedMsg è già guardato) è rifutato: nessun secondo
// sendCmd, la conferma resta aperta (esc può chiuderla).
func TestSendConfirmSRefusedWhileSending(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = sendRequested(t, draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
	}))
	mm.sending = true // simula l'invio in flight (da conferma a opDone)
	mm, cmd := step(mm, pressKey("s"))
	if cmd != nil {
		t.Errorf("s con invio in flight: cmd = %v, voglio nil (rifutato)", cmd)
	}
	if !mm.confirmSend {
		t.Error("s rifutato non deve consumare la conferma")
	}
	if !mm.sending {
		t.Error("sending = false, voglio true (l'invio in flight resta l'unico)")
	}
	if len(rec.calls) != 0 {
		t.Errorf("sender chiamato %d volte, voglio 0", len(rec.calls))
	}
}

// TestSendRequestedDiscardOpenDropped (leak confirmSend): ctrl+s col
// SendRequestedMsg in volo, esc apre PRIMA il modale discard — il messaggio
// arriva a stato invalido (discardOpen) ed è scartato; dopo lo scarto i tasti
// dell'inbox funzionano normalmente e `s` (stella) NON invia la bozza scartata.
func TestSendRequestedDiscardOpenDropped(t *testing.T) {
	rec := &senderRecorder{}
	m, _ := senderModel(t, rec)
	mm, _ := step(m, teaSize())
	mm = draftComposer(mm, composer.ComposerConfig{
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
	})
	// ctrl+s: SendRequestedMsg generato ma NON ancora consegnato (cmd in volo).
	mm, cmd := step(mm, pressKey("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s: cmd nil, voglio SendRequestedMsg")
	}
	res, ok := cmd().(composer.SendRequestedMsg)
	if !ok {
		t.Fatalf("cmd = %T, voglio composer.SendRequestedMsg", cmd())
	}
	// esc arriva PRIMA della consegna: il corpo non è vuoto → modale discard.
	mm, _ = step(mm, pressKey("esc"))
	if !mm.discardOpen {
		t.Fatal("esc deve aprire il modale discard (corpo non vuoto)")
	}
	// Il SendRequestedMsg ora consegna a stato invalido: scartato.
	mm, _ = step(mm, res)
	if mm.confirmSend {
		t.Error("SendRequestedMsg con discard aperto accettato: confirmSend leak")
	}
	if !mm.discardOpen {
		t.Error("il modale discard deve restare aperto (messaggio scartato)")
	}
	// `s` scarta la bozza: back alla vista di provenienza (inbox).
	mm, _ = step(mm, pressKey("s"))
	if mm.currentView() != viewInbox {
		t.Errorf("vista dopo lo scarto = %v, voglio inbox", mm.currentView())
	}
	if mm.confirmSend {
		t.Error("confirmSend deve restare false dopo lo scarto")
	}
	// I tasti dell'inbox funzionano normalmente: `s` è stella, NON invia.
	mm, _ = step(mm, pressKey("s"))
	if mm.confirmSend || mm.currentView() != viewInbox {
		t.Errorf("s nell'inbox dopo lo scarto: confirm=%v vista=%v, voglio false/inbox", mm.confirmSend, mm.currentView())
	}
	if len(rec.calls) != 0 {
		t.Errorf("sender chiamato %d volte dopo lo scarto, voglio 0", len(rec.calls))
	}
}

// --- mouse gate ---------------------------------------------------------------------

// --- mouse gate ---------------------------------------------------------------------

// TestMouseIgnoredInComposer: nel composer il mouse è ignorato (click e
// rotella non toccano i campi), come sotto i modali.
func TestMouseIgnoredInComposer(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, pressKey("c"))
	mm, cmd := step(mm, tea.MouseClickMsg{Y: 3, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("click nel composer: cmd = %v, voglio nil", cmd)
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista = %v, voglio composer invariata", mm.currentView())
	}
}

// --- help ---------------------------------------------------------------------------

// TestComposerHelpEntry: l'help breve della vista composer documenta SOLO
// annulla — `?` nel composer è testo nel campo focalizzato, la binding Help
// non va pubblicizzata (review Task 3, precedente viewSearch).
func TestComposerHelpEntry(t *testing.T) {
	m := newTestModel(t)
	m.view = viewComposer
	descs := bindingDescs(m.shortHelpFor(false, viewComposer))
	if !descs["annulla"] {
		t.Errorf("short help composer = %v, voglio annulla", descs)
	}
	if descs["aiuto"] {
		t.Errorf("short help composer pubblica ? ma il tasto è irraggiungibile: %v", descs)
	}
}
