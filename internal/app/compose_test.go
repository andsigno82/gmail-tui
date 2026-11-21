// Test del composer a livello radice (Piano 4, Task 3): entry paths c/r/f,
// builders reply/forward/new, cancel con conferma discard, rebind scroll
// (f/b liberati nel thread), status placeholder per SendRequestedMsg.
package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"gmail-tui/internal/config"
	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
	"gmail-tui/internal/ui/composer"
	"gmail-tui/internal/ui/inbox"
	"gmail-tui/internal/ui/thread"
)

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

// TestBInThreadIsInert: `b` è liberato (uso futuro): non scorre più la
// viewport né fa altro — no-op silenzioso.
func TestBInThreadIsInert(t *testing.T) {
	ref := testRefs(t)[1]
	m, _ := composerModel(t, fullMessage(ref))
	mm := openThread(t, m, ref)
	mm, _ = step(mm, teaSize())
	before := mm.thread.YOffset()
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

// --- send placeholder (il flusso vero è Task 4) -------------------------------------

// TestSendRequestedShowsPlaceholder: ctrl+s nel composer emette
// SendRequestedMsg — per ora la radice custodisce la bozza e mostra lo status
// placeholder; il composer resta aperto.
func TestSendRequestedShowsPlaceholder(t *testing.T) {
	m, _ := composerModel(t)
	mm, _ := step(m, pressKey("c"))
	// Bozza precompilata via config (API esportata del composer).
	mm.composer = composer.New(composer.ComposerConfig{
		From: testFrom, Signature: testSig,
		To: "anna@x.dev", Subject: "oggetto", Body: "corpo",
	}, mm.styles)
	mm, cmd := step(mm, pressKey("ctrl+s"))
	if cmd == nil {
		t.Fatal("ctrl+s: cmd nil, voglio SendRequestedMsg")
	}
	res, ok := cmd().(composer.SendRequestedMsg)
	if !ok {
		t.Fatalf("cmd = %T, voglio composer.SendRequestedMsg", cmd())
	}
	if res.Out.To[0] != "anna@x.dev" || !strings.HasSuffix(res.Out.BodyText, "\n\n--\n"+testSig) {
		t.Errorf("SendRequestedMsg.Out = %+v, voglio la bozza col mittente e la firma", res.Out)
	}
	mm, cmd2 := step(mm, res)
	if cmd2 != nil {
		t.Errorf("cmd dopo SendRequestedMsg = %v, voglio nil", cmd2)
	}
	if mm.currentView() != viewComposer {
		t.Errorf("vista dopo ctrl+s = %v, voglio composer (resta aperto)", mm.currentView())
	}
	if mm.draft.Subject != "oggetto" || !strings.HasSuffix(mm.draft.BodyText, "\n\n--\n"+testSig) {
		t.Errorf("draft custodito = %+v, voglio la bozza con firma", mm.draft)
	}
	if mm.statusText() != "composer pronto — invio in Task 4" {
		t.Errorf("status = %q, voglio il placeholder", mm.statusText())
	}
}

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

// TestComposerHelpEntry: l'help breve della vista composer documenta annulla
// e aiuto (il resto dei tasti è nella riga d'uso del composer).
func TestComposerHelpEntry(t *testing.T) {
	m := newTestModel(t)
	m.view = viewComposer
	descs := bindingDescs(m.shortHelpFor(false, viewComposer))
	if !descs["annulla"] || !descs["aiuto"] {
		t.Errorf("short help composer = %v, voglio annulla+aiuto", descs)
	}
}
