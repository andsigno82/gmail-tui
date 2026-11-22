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
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// watchModel costruisce il modello consolidato (3 ref, cursore su 0, pending
// 0) con il feed Watch ARMATO sul canale del Mock: riproduce ciò che Init fa
// via startWatch → watchStarted. Il cmd waitForChange restituito dallo step
// NON viene eseguito dalla fixture: senza ctx annullato bloccherebbe sulla
// lettura del canale — i test che srotolano i batch passano da drainCmd.
func watchModel(tb testing.TB, mock *mail.Mock) Model {
	tb.Helper()
	m := opsModel(tb, mock)
	ch, err := mock.Watch(context.Background())
	if err != nil {
		tb.Fatalf("Watch: %v", err)
	}
	mm, cmd := step(m, watchStarted{ch: ch})
	if cmd == nil {
		tb.Fatal("watchStarted: cmd nil, voglio waitForChange")
	}
	return mm
}

// drainCmd srotola una cmd DOPO aver annullato il ctx del modello: il rearm
// waitForChange risponde subito nil (loop chiuso dal cancel) invece di
// bloccare sulla lettura del canale.
func drainCmd(tb testing.TB, m Model, c tea.Cmd) []tea.Msg {
	tb.Helper()
	if c == nil {
		return nil
	}
	m.cancel()
	return runCmd(c)
}

// loadedOf estrae l'unico msgsLoaded prodotti dalla cmd (fallisce se assente
// o duplicato).
func loadedOf(tb testing.TB, msgs []tea.Msg) msgsLoaded {
	tb.Helper()
	var loaded msgsLoaded
	found := false
	for _, ms := range msgs {
		if l, ok := ms.(msgsLoaded); ok {
			if found {
				tb.Fatal("più di un msgsLoaded nel drain")
			}
			loaded, found = l, true
		}
	}
	if !found {
		tb.Fatalf("nessun msgsLoaded nel drain (%d msg)", len(msgs))
	}
	return loaded
}

// hasNil segnala se il drain contiene un nil Msg: è il rearm waitForChange
// chiuso dal cancel (il runtime scarta i nil Msg: il loop finisce).
func hasNil(msgs []tea.Msg) bool {
	for _, ms := range msgs {
		if ms == nil {
			return true
		}
	}
	return false
}

// --- Init: il feed Watch parte con il load iniziale -------------------------

// TestInitStartsWatch: Init avvia Watch(ctx del programma) oltre al load e
// allo spinner; al messaggio watchStarted la radice custodisce il canale e
// arma il loop waitForChange, che finisce (nil → runtime lo scarta) quando il
// ctx del programma viene annullato.
func TestInitStartsWatch(t *testing.T) {
	m, _ := newTestModelMock(t, mockMsgs(t)...)

	// Bugfix 2026-11-22: Watch parte SOLO dopo il connect riuscito.
	var ws watchStarted
	found := false
	_, msgs := startSession(t, m)
	for _, ms := range msgs {
		if s, ok := ms.(watchStarted); ok {
			ws, found = s, true
		}
	}
	if !found {
		t.Fatal("Init non avvia il feed Watch (nessun watchStarted nel batch)")
	}
	if ws.err != nil {
		t.Errorf("watchStarted.err = %v, voglio nil", ws.err)
	}
	if ws.ch == nil {
		t.Error("watchStarted.ch = nil, voglio il canale del feed")
	}

	mm, cmd2 := step(m, ws)
	if cmd2 == nil {
		t.Fatal("watchStarted: cmd nil, voglio waitForChange (loop armato)")
	}
	if mm.watchCh == nil {
		t.Error("il canale del feed non è custodito sul modello")
	}
	mm.cancel()
	if res := cmd2(); res != nil {
		t.Errorf("waitForChange dopo cancel = %v, voglio nil (loop finito)", res)
	}
}

// --- ChangeNewMail: re-list silenzioso + feedback "nuova posta" -------------

// TestChangeNewMailRefreshesListAndStatus: la nuova posta emette il feedback
// in status bar e il re-list silenzioso della query corrente (che NON tocca
// il feedback); il loop si ri-arm (rearm nel batch).
func TestChangeNewMailRefreshesListAndStatus(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	// un quarto messaggio entra nello store PRIMA dell'evento
	fourth := msg.MessageRef{
		ID: msg.NewID("INBOX", 4), From: "d@x.dev", Subject: "quattro",
		Date: time.Date(2026, 11, 20, 15, 0, 0, 0, time.UTC), Unread: true,
	}
	mock.Msgs = append(mock.Msgs, &msg.Message{Ref: fourth})

	mm, cmd := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}})
	if got := toastText(mm); got != newMailText {
		t.Errorf("status dopo ChangeNewMail = %q, voglio %q", got, newMailText)
	}
	if mm.pending != 1 {
		t.Errorf("pending dopo ChangeNewMail = %d, voglio 1 (re-list in flight)", mm.pending)
	}

	msgs := drainCmd(t, mm, cmd)
	loaded := loadedOf(t, msgs)
	if !loaded.silent {
		t.Error("il re-list da Watch deve essere silenzioso (non tocca lo status)")
	}
	if len(loaded.refs) != 4 {
		t.Errorf("refs dal re-list = %d, voglio 4 (la nuova posta c'è)", len(loaded.refs))
	}
	if !hasNil(msgs) {
		t.Error("nel batch manca il rearm chiuso dal cancel (loop non ri-armato?)")
	}

	mm2, _ := step(mm, loaded)
	if len(mm2.refs) != 4 {
		t.Errorf("refs sul modello = %d, voglio 4 (lista aggiornata)", len(mm2.refs))
	}
	if got := toastText(mm2); got != newMailText {
		t.Errorf("status dopo il re-list silenzioso = %q, voglio %q (intatto)", got, newMailText)
	}
}

// --- ChangeFlags: SOLO re-list silenzioso, nessun feedback ------------------

// TestChangeFlagsRelistsSilentlyWithoutStatus: un cambio di flag (es. un'op
// fatta da un altro client) rilista in silenzio: lo status di un'op
// precedente resta intatto e il re-list riflette il flag aggiornato.
func TestChangeFlagsRelistsSilentlyWithoutStatus(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	mm0, _ := step(m, opDone{label: "stella on"}) // esito d'op: toast (Piano 7.3)
	mock.Msgs[0].Ref.Starred = true

	mm, cmd := step(mm0, changeEvt{c: mail.Change{Kind: mail.ChangeFlags}})
	if got := toastText(mm); got != "stella on" {
		t.Errorf("toast dopo ChangeFlags = %q, voglio \"stella on\" (intatto)", got)
	}
	loaded := loadedOf(t, drainCmd(t, mm, cmd))
	if !loaded.silent {
		t.Error("il re-list da ChangeFlags deve essere silenzioso")
	}
	if len(loaded.refs) == 0 || !loaded.refs[0].Starred {
		t.Error("il re-list non riflette il flag aggiornato nello store")
	}
	mm2, _ := step(mm, loaded)
	if got := toastText(mm2); got != "stella on" {
		t.Errorf("toast dopo il re-list = %q, voglio \"stella on\" (intatto)", got)
	}
}

// --- ChangeDisconnected: indicatore ⟳ finché un msgsLoaded riesce -----------

// TestChangeDisconnectedShowsIndicatorUntilLoad: la disconnessione mostra
// l'indicatore in status bar (persistente) e rilista in silenzio; il primo
// msgsLoaded riuscito lo spegne.
func TestChangeDisconnectedShowsIndicatorUntilLoad(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	mm, cmd := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeDisconnected}})
	if got := mm.statusText(); got != reconnectIndicator {
		t.Errorf("status dopo Disconnected = %q, voglio %q", got, reconnectIndicator)
	}
	if bar := mm.statusBar(); !strings.Contains(bar, "⟳ riconnessione") {
		t.Errorf("status bar = %q, voglio l'indicatore ⟳", bar)
	}

	loaded := loadedOf(t, drainCmd(t, mm, cmd))
	if !loaded.silent {
		t.Error("il re-list da Disconnected deve essere silenzioso")
	}
	mm2, _ := step(mm, loaded)
	if got := mm2.statusText(); got != "" {
		t.Errorf("status dopo il msgsLoaded riuscito = %q, voglio \"\" (indicatore spento)", got)
	}
}

// TestDisconnectedIndicatorUntilFlagsRefreshResult: l'indicatore resta
// visibile attraverso UN ALTRO evento (Flags: status intatto) e si spegne al
// risultato del re-list riuscito; il cambio arrivato durante il refresh è
// coalescato (dirty → follow-up al risultato, senza doppio re-list).
func TestDisconnectedIndicatorUntilFlagsRefreshResult(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	mm, cmdD := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeDisconnected}})
	mm, cmdF := step(mm, changeEvt{c: mail.Change{Kind: mail.ChangeFlags}})
	if got := mm.statusText(); got != reconnectIndicator {
		t.Fatalf("status dopo Flags = %q, voglio ancora %q", got, reconnectIndicator)
	}
	// il refresh dei Flags è coalescato in quello della disconnessione:
	// cmdF non aggiunge un re-list (solo rearm)
	for _, ms := range drainCmd(t, mm, cmdF) {
		if _, ok := ms.(msgsLoaded); ok {
			t.Error("ChangeFlags con refresh in flight: re-list duplicato")
		}
	}

	loaded := loadedOf(t, drainCmd(t, mm, cmdD))
	mm2, follow := step(mm, loaded)
	if got := mm2.statusText(); got != "" {
		t.Errorf("status dopo il risultato = %q, voglio \"\" (indicatore spento)", got)
	}
	if follow == nil {
		t.Fatal("cambio arrivato durante il refresh (dirty): al risultato serve il follow-up")
	}
	floaded := loadedOf(t, drainCmd(t, mm2, follow))
	if !floaded.silent {
		t.Error("il follow-up deve essere silenzioso")
	}
}

// --- coalescing: mai due re-list silenziosi in volo -------------------------

// TestChangeCoalescesWhileRefreshInFlight: un cambio con un re-list già in
// volo NON emette un secondo re-list — marca dirty e il follow-up parte al
// risultato di quello in corso. Il feedback di nuova posta arriva comunque.
func TestChangeCoalescesWhileRefreshInFlight(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)
	m.refreshing = true // refresh silenzioso già in flight (es. da un'op)

	mm, cmd := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}})
	if got := toastText(mm); got != newMailText {
		t.Errorf("status = %q, voglio %q (il feedback arriva comunque)", got, newMailText)
	}
	if !mm.dirty {
		t.Error("dirty = false, voglio true (cambio da rilistare al risultato)")
	}
	for _, ms := range drainCmd(t, mm, cmd) {
		if _, ok := ms.(msgsLoaded); ok {
			t.Error("re-list duplicato con un refresh già in flight")
		}
	}

	// il risultato del refresh in corso sblocca il follow-up
	mm2, follow := step(mm, msgsLoaded{refs: opsRefs(t), silent: true})
	if mm2.dirty {
		t.Error("dirty non consumata dal follow-up")
	}
	if follow == nil {
		t.Fatal("al risultato del refresh in flight serve la cmd di follow-up")
	}
	loaded := loadedOf(t, drainCmd(t, mm2, follow))
	if !loaded.silent {
		t.Error("il follow-up deve essere silenzioso")
	}
}

// TestOpDoneRefreshCoalescesWhileRefreshInFlight: anche il re-list dopo un'op
// (Task 7) passa dal guard: con un refresh già in volo non accatasta, marca
// dirty e il follow-up parte al risultato.
func TestOpDoneRefreshCoalescesWhileRefreshInFlight(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)
	m.refreshing = true

	mm, cmd := step(m, opDone{label: "stella on", refresh: true})
	if mm.pending != 0 {
		t.Errorf("pending = %d, voglio 0 (refresh coalescato: nessuna op aggiunta)", mm.pending)
	}
	if msgs := dropExpired(runCmd(cmd)); len(msgs) != 0 {
		t.Errorf("cmd = %v, voglio nessun refresh (coalescato nel flight)", msgs)
	}
	if !mm.dirty {
		t.Error("dirty = false, voglio true")
	}
	mm2, follow := step(mm, msgsLoaded{refs: opsRefs(t), silent: true})
	if follow == nil {
		t.Fatal("al risultato del refresh in flight serve il follow-up")
	}
	loadedOf(t, drainCmd(t, mm2, follow))
}

// --- ctx cancel e continuità del loop ---------------------------------------

// TestCancelEndsWatchLoop: il loop armato risponde nil all'annullamento del
// ctx del programma (il runtime scarta i nil Msg: il loop finisce senza
// ri-armarsi, nessun goroutine bloccato); eventi emessi DOPO il cancel non
// fanno panic — restano nel buffer del feed, nessuno li consuma più.
func TestCancelEndsWatchLoop(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	mm, cmd := step(m, watchStarted{ch: m.watchCh})
	if cmd == nil {
		t.Fatal("watchStarted: cmd nil, voglio waitForChange")
	}
	mm.cancel()
	if res := cmd(); res != nil {
		t.Errorf("waitForChange dopo cancel = %v, voglio nil (loop finito)", res)
	}
	// nessun panic con eventi dopo il cancel (canale condiviso del Mock)
	mock.Emit(mail.Change{Kind: mail.ChangeNewMail})
}

// TestWatchLoopConsumesChannelEvents: il rearm del cambio consuma il
// PROSSIMO evento dal canale del feed (contratto Mock: un canale condiviso)
// — il loop resta vivo finché il ctx non viene annullato.
func TestWatchLoopConsumesChannelEvents(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	mock.Emit(mail.Change{Kind: mail.ChangeFlags}) // in coda sul canale
	_, cmd := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}})
	var rearm changeEvt
	found := false
	for _, ms := range runCmd(cmd) { // senza cancel: il rearm trova l'evento in coda e risponde subito
		if e, ok := ms.(changeEvt); ok {
			rearm, found = e, true
		}
	}
	if !found {
		t.Fatal("il rearm non ha consumato l'evento in coda (loop rotto)")
	}
	if rearm.c.Kind != mail.ChangeFlags {
		t.Errorf("rearm.kind = %v, voglio ChangeFlags", rearm.c.Kind)
	}
}

// --- errori del Watch --------------------------------------------------------

// TestWatchStartedErrorShowsStatus: un Watch fallito finisce in un toast d'errore
// (errore) e NON arma il loop — la TUI resta usabile (nessun push, i re-list
// espliciti funzionano).
func TestWatchStartedErrorShowsStatus(t *testing.T) {
	m := newTestModel(t)
	mm, cmd := step(m, watchStarted{err: errors.New("rete giù")})
	if cmd != nil {
		t.Error("cmd = non-nil, voglio nil (nessun loop da armare)")
	}
	if !toastIs(mm, toast.Error) || !strings.Contains(toastText(mm), "rete giù") {
		t.Errorf("status = %q (errore=%v), voglio l'errore del Watch", toastText(mm), toastIs(mm, toast.Error))
	}
}

// TestStaleRefreshFollowsUpPendingDirty (Piano 7, fix code-review 2): R parte in
// INBOX, si cambia lista (listGen+1), ChangeNewMail marca dirty mentre R è in
// volo, il load della nuova lista rientra e coalesce, poi R rientra stantio.
// Il token va liberato E il cambio pendente va rilistato: il follow-up non può
// perdersi nel ramo stantio.
func TestStaleRefreshFollowsUpPendingDirty(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)
	m.refreshing = true // R in volo nella lista precedente
	m.refreshGen = m.listGen
	m.listGen++ // cambio di lista: R diventa stante

	mm, _ := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeNewMail}})
	if !mm.dirty {
		t.Fatal("setup: dirty = false dopo ChangeNewMail con refresh in volo")
	}
	mm, _ = step(mm, msgsLoaded{refs: opsRefs(t), gen: mm.listGen}) // load della nuova lista: coalesce
	if !mm.dirty {
		t.Fatal("setup: il load non deve consumare dirty")
	}

	mm2, follow := step(mm, msgsLoaded{refs: opsRefs(t), silent: true, gen: m.refreshGen})
	if mm2.refreshing != true {
		t.Error("refreshing = false: il follow-up non è partito")
	}
	if mm2.dirty {
		t.Error("dirty non consumata dal follow-up nel ramo stantio")
	}
	if follow == nil {
		t.Fatal("R stantio con dirty pendente: serve la cmd di follow-up")
	}
	loadedOf(t, drainCmd(t, mm2, follow))
}

// TestRefreshStartedByOpAndWatchIsReturned (Piano 7, fix code-review 3): il
// re-list avviato da opDone o da un cambio del feed deve stare nel Model
// restituito. `return m, m.requestRefresh(...)` legge m in ordine non specificato.
func TestRefreshStartedByOpAndWatchIsReturned(t *testing.T) {
	mock := &mail.Mock{Msgs: mockMsgs(t)}
	m := watchModel(t, mock)

	mm, cmd := step(m, opDone{label: "stella on", refresh: true})
	if !mm.refreshing {
		t.Error("opDone: refreshing = false nel Model restituito, voglio true")
	}
	if cmd == nil {
		t.Error("opDone: cmd nil, voglio il re-list")
	}

	mm2, cmd2 := step(m, changeEvt{c: mail.Change{Kind: mail.ChangeFlags}})
	if !mm2.refreshing {
		t.Error("changeEvt: refreshing = false nel Model restituito, voglio true")
	}
	if cmd2 == nil {
		t.Error("changeEvt: cmd nil, voglio re-list e rearm")
	}
}
