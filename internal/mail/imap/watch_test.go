package imap

// Test di Watch (IDLE + riconnessione), piano 2 Task 7. Watch è asincrono:
// i test usano timeout espliciti (5s per gli eventi, 2s per la chiusura del
// canale su ctx cancel), come previsto dal piano.

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// recvChange riceve un Change dal canale entro d o fallisce il test.
func recvChange(t *testing.T, ch <-chan mail.Change, d time.Duration) mail.Change {
	t.Helper()
	select {
	case c, ok := <-ch:
		if !ok {
			t.Fatalf("canale Watch chiuso mentre attendevo un Change")
		}
		return c
	case <-time.After(d):
		t.Fatalf("nessun Change entro %v", d)
		return mail.Change{} // irraggiungibile
	}
}

// assertClosesWithin verifica che il canale si chiuda entro d, drenando gli
// eventuali Change ancora in buffer (il contratto è sulla CHIUSURA, non sul
// vuoto del buffer).
func assertClosesWithin(t *testing.T, ch <-chan mail.Change, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // chiuso
			}
		case <-deadline:
			t.Fatalf("canale Watch non chiuso entro %v", d)
		}
	}
}

// settleIdle attende che il Watch entri effettivamente in IDLE lato server.
// Motivo (limitazione del memserver, non dell'implementazione): i tracker
// della SessionTracker.Idle NON drenano update accodati prima della
// registrazione dell'idle: un APPEND out-of-band nella finestra tra l'ultimo
// poll (fine della baseline) e l'avvio dell'idle resta in coda senza
// notifica. Un server reale (Gmail) consegna comunque l'EXISTS; il test
// attende che l'idle sia registrato prima di seminare.
func settleIdle() {
	time.Sleep(250 * time.Millisecond)
}

// startTestServerOnAddr è la variante della harness che collega un server
// NUOVO su un indirizzo preciso (usata da TestWatchReconnects per simulare il
// ritorno del server dopo un disconnect). Ritorna errore se il bind non riesce
// (le connessioni chiuse del server precedente possono trattenere la porta per
// un istante): il chiamante riprova.
func startTestServerOnAddr(t *testing.T, addr string) (*testServer, error) {
	t.Helper()

	mem := imapmemserver.New()
	user := imapmemserver.NewUser("tester", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		return nil, err
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		InsecureAuth: true, // solo test: LOGIN in chiaro
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		// Close fa terminare Serve con net.ErrServerClosed: ignorato.
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() { _ = srv.Close() }) // doppio Close: ok (errClosed)

	return &testServer{t: t, ln: ln, srv: srv, user: user}, nil
}

// restartTestServer collega un nuovo server su addr, riprovando il bind per
// qualche secondo (finestra TIME_WAIT del server precedente).
func restartTestServer(t *testing.T, addr string) *testServer {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		ts, err := startTestServerOnAddr(t, addr)
		if err == nil {
			return ts
		}
		if time.Now().After(deadline) {
			t.Fatalf("rebind %s: %v", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestWatchNewMail: Watch attivo → seed di una nuova mail → entro 5s arriva
// un Change (kind NewMail, IDs non vuoti, con l'ID della nuova mail).
func TestWatchNewMail(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo base", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settleIdle()

	uid := ts.seedMail("New <new@example.org>", "Watch me", "corpo nuovo", false, false)

	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeNewMail {
		t.Errorf("Kind = %d, voglio ChangeNewMail (%d)", c.Kind, mail.ChangeNewMail)
	}
	if len(c.IDs) == 0 {
		t.Fatal("IDs vuoti, voglio l'ID della nuova mail")
	}
	want := msg.NewID("INBOX", uid)
	if c.IDs[len(c.IDs)-1] != want {
		t.Errorf("IDs = %v, voglio che contenga %s (ultima = nuova)", c.IDs, want)
	}
}

// TestWatchFlags: cambio di flag da una SECONDA sessione (STORE \Seen) → la
// sessione in IDLE riceve il FETCH unilaterale → entro 5s ChangeFlags con
// l'ID del messaggio.
func TestWatchFlags(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMail("Flag <flag@example.org>", "Flag me", "corpo", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	other := connectStore(t, ts) // seconda sessione: la mutazione non è auto-riferita
	if err := other.MarkRead(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, true); err != nil {
		t.Fatalf("MarkRead (seconda sessione): %v", err)
	}

	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeFlags {
		t.Errorf("Kind = %d, voglio ChangeFlags (%d)", c.Kind, mail.ChangeFlags)
	}
	want := msg.NewID("INBOX", uid)
	if len(c.IDs) != 1 || c.IDs[0] != want {
		t.Errorf("IDs = %v, voglio [%s]", c.IDs, want)
	}
}

// TestWatchCancelClosesChannel: ctx cancel ⇒ il canale si chiude entro 2s.
func TestWatchCancelClosesChannel(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo", false, false)
	store := connectStore(t, ts)

	ctx, cancel := context.WithCancel(t.Context())
	ch, err := store.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	cancel()
	assertClosesWithin(t, ch, 2*time.Second)
}

// TestWatchNonBlockingSend: contratto "send non bloccante (drop = consumer
// re-lista)": con il canale mai letto, una raffica di eventi (> buffer 16)
// non blocca il loop — lo dimostriamo chiudendo ctx e verificando che il
// canale venga chiuso comunque entro 2s.
func TestWatchNonBlockingSend(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo", false, false)
	store := connectStore(t, ts)

	ctx, cancel := context.WithCancel(t.Context())
	ch, err := store.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	settleIdle()

	// Raffica di nuove mail: il buffer (16) si satura e i Change in eccesso
	// vengono droppati, MA il loop non deve mai bloccarsi su una send.
	for i := 0; i < 40; i++ {
		ts.seedMail("Burst <burst@example.org>", "Burst", "corpo", false, false)
	}

	cancel()
	assertClosesWithin(t, ch, 2*time.Second) // loop vivo ⇒ chiude puntuale
}

// TestWatchDisconnected: kill del server (listener + connessioni attive) ⇒
// entro 5s arriva ChangeDisconnected. Il loop di riconnessione resta attivo
// in background finché il ctx del test non è annullato (fine test).
func TestWatchDisconnected(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := ts.srv.Close(); err != nil { // simula il network kill
		t.Fatalf("close server: %v", err)
	}

	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeDisconnected {
		t.Errorf("Kind = %d, voglio ChangeDisconnected (%d)", c.Kind, mail.ChangeDisconnected)
	}
}

// waitIdling attende che il Watch entri in IDLE secondo il gate, con l'hook
// di test idleGate.isIdling (poll con deadline). NOTA: isIdling è marcato
// prima della registrazione dell'idle lato server, quindi prima di SEMINARE
// va aggiunto anche settleIdle (gap noto del memserver).
func waitIdling(t *testing.T, s *Store) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !s.gate.isIdling() {
		if time.Now().After(deadline) {
			t.Fatal("il Watch non è entrato in IDLE entro 5s")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// opWithin esegue fn con un watchdog: nei test del gate serve a trasformare
// un'operazione bloccata dall'IDLE attivo (bug pre-fix: l'encoder del client
// è trattenuto dall'IDLE) in un fallimento pulito invece di appendere l'intero
// binary di test.
func opWithin(t *testing.T, d time.Duration, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		t.Fatalf("operazione non completata entro %v (bloccata dall'IDLE attivo?)", d)
		return nil // irraggiungibile
	}
}

// TestStoreOpDuringIdleSucceeds (fix review: idle gate): con il Watch in IDLE,
// un'operazione dello Store sulla STESSA connessione deve riuscire — rompe
// l'IDLE, completa, e NON uccide la connessione (nessun ChangeDisconnected).
func TestStoreOpDuringIdleSucceeds(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo base", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	waitIdling(t, store)

	// L'operazione parte mentre l'IDLE è attivo.
	err = opWithin(t, 10*time.Second, func() error {
		refs, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
		if err == nil && len(refs) == 0 {
			t.Error("ListMessages: nessun ref, attesa la mail seminata")
		}
		return err
	})
	if err != nil {
		t.Fatalf("ListMessages durante l'idle: %v", err)
	}

	// La connessione dev'essere viva: l'operazione successiva riesce.
	if err := opWithin(t, 10*time.Second, func() error {
		_, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
		return err
	}); err != nil {
		t.Fatalf("ListMessages successiva all'operazione: %v (connessione morta?)", err)
	}

	// Nessun ChangeDisconnected nella finestra generosa dopo l'operazione:
	// un drop sarebbe arrivato entro millisecondi dalla violazione.
	select {
	case c, ok := <-ch:
		if ok && c.Kind == mail.ChangeDisconnected {
			t.Fatal("ChangeDisconnected ricevuto: l'operazione ha ucciso la connessione")
		}
	case <-time.After(1 * time.Second):
	}
}

// TestWatchSurvivesManyOps (fix review: idle gate): una sequenza di operazioni
// con il Watch attivo rompe e riappre l'IDLE più volte; il Watch deve restare
// vivo e ri-idlarsi: una mail seminata DOPO le operazioni produce comunque
// ChangeNewMail.
func TestWatchSurvivesManyOps(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo base", false, false)
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	waitIdling(t, store)

	for i := 0; i < 5; i++ {
		if err := opWithin(t, 10*time.Second, func() error {
			_, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
			return err
		}); err != nil {
			t.Fatalf("operazione %d durante il watch: %v", i+1, err)
		}
	}

	// Il watch dev'essere di nuovo in IDLE. Due attese: waitIdling sul gate,
	// poi settleIdle perché isIdling è marcato PRIMA della registrazione
	// dell'idle lato server (gap noto del memserver: seminare in quella
	// finestra non notifica — vedi settleIdle).
	waitIdling(t, store)
	settleIdle()
	uid := ts.seedMail("After <after@example.org>", "After ops", "corpo", false, false)

	want := msg.NewID("INBOX", uid)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case c, ok := <-ch:
			if !ok {
				t.Fatal("canale Watch chiuso mentre attendevo ChangeNewMail")
			}
			if c.Kind != mail.ChangeNewMail {
				continue // eventi spuri (non attesi): interessa solo NewMail
			}
			found := false
			for _, id := range c.IDs {
				if id == want {
					found = true
				}
			}
			if !found {
				t.Fatalf("ChangeNewMail IDs = %v, voglio %s", c.IDs, want)
			}
			return // watch vivo e re-idlato dopo 5 rotture di IDLE
		case <-deadline:
			t.Fatal("nessun ChangeNewMail entro 10s: il watch non è sopravvissuto alle operazioni")
		}
	}
}

// TestWatchReconnects: kill del server → ChangeDisconnected → un nuovo server
// risorge sullo STESSO indirizzo → il backoff riconnette (credenziali di
// Options riusate da Connect) → una mail seminata sul nuovo server produce
// ChangeNewMail. La semina avviene dopo un margine > primo backoff (1s) per
// non cadere nella baseline della riconnessione.
func TestWatchReconnects(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("Base <base@example.org>", "Base", "corpo", false, false)
	addr := ts.addr()
	store := connectStore(t, ts)

	ch, err := store.Watch(t.Context())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := ts.srv.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}
	c := recvChange(t, ch, 5*time.Second)
	if c.Kind != mail.ChangeDisconnected {
		t.Fatalf("Kind = %d, voglio ChangeDisconnected (%d)", c.Kind, mail.ChangeDisconnected)
	}

	// Nuovo server sullo stesso indirizzo: il loop di riconnessione (backoff
	// 1s, 2s, 5s, …) riallaccia da solo con le credenziali di Options.
	ts2 := restartTestServer(t, addr)

	// Il momento esatto della riconnessione dipende dal backoff vs il tempo
	// di rebind della porta: seminiamo ripetutamente finché il Watch
	// riconnesso non riporta la nuova mail (le seed pre-riconnessione finiscono
	// nella baseline, quelle post producono l'evento).
	seeded := make(map[msg.MessageID]bool)
	var got mail.Change
	received := false
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		uid := ts2.seedMail("Back <back@example.org>", "Back online", "corpo", false, false)
		seeded[msg.NewID("INBOX", uid)] = true
		select {
		case c, ok := <-ch:
			if !ok {
				t.Fatal("canale Watch chiuso durante la riconnessione")
			}
			got = c
			received = true
		case <-time.After(1500 * time.Millisecond):
			continue // nessun evento: la riconnessione non è ancora stabilita
		}
		if got.Kind == mail.ChangeNewMail {
			break
		}
		// ChangeDisconnected di troppo (server B non ancora su al dial
		// precedente): continua a seminare.
	}
	if !received || got.Kind != mail.ChangeNewMail {
		t.Fatalf("Kind = %d, voglio ChangeNewMail (%d)", got.Kind, mail.ChangeNewMail)
	}
	found := false
	for _, id := range got.IDs {
		if seeded[id] {
			found = true
		}
	}
	if !found || len(got.IDs) == 0 {
		t.Errorf("IDs = %v, voglio almeno un ID tra le mail seminate (%d seed)", got.IDs, len(seeded))
	}
}
