package imap

// Watch (IDLE) con riconnessione e change feed (piano 2, Task 7).
//
// Scelta API (go-imap v2.0.0-beta.9, verificata nel module cache):
//
//   - imapclient.Options.UnilateralDataHandler installa, AL DIAL, i gestori
//     dei dati unilaterali: Mailbox (EXISTS: possibili nuovi messaggi),
//     Fetch (FETCH non richiesto da comandi pendenti: cambio di flag, con
//     UID nella risposta), Expunge. I gestori devono essere rapidi (possono
//     girare nel goroutine di lettura): qui fanno solo una send non
//     bloccante sul canale dei wake. Il gestore Fetch riceve dati in
//     streaming che VANNO drenati (contratto imapclient): Collect() li
//     consuma e restituisce UID+FLAGS.
//   - Client.Idle() → *IdleCommand: Close() scrive DONE e sblocca il client
//     (mentre IDLE gira non si possono inviare comandi), Wait() attende la
//     fine del comando e restituisce l'errore di rete (la morte della
//     connessione chiude decCh e completa i comandi pendenti con errore,
//     cfr. TestIdle_closedConn della libreria).
//
// La differenziazione NewMail/Flags è quindi PRATICA in beta.9 (non serve il
// fallback "ChangeNewMail generico" consentito dal piano): EXISTS →
// ri-verifica "UID SEARCH uid_successivo:*" → ChangeNewMail con i nuovi ID;
// FETCH unilaterale → ChangeFlags con l'ID del messaggio. La ri-verifica
// dopo OGNI wake rende il sistema robusto anche a wake persi (best-effort).

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

const (
	// watchBuffer è il buffer del canale dei Change e dei wake (contratto
	// Task 1: canale bufferizzato, eventi best-effort).
	watchBuffer = 16
	// watchRefresh chiude e riapre l'IDLE con un NOOP in mezzo: il limite
	// del protocollo è ~29 min di inattività; 25 min sta comodo sotto.
	watchRefresh = 25 * time.Minute
	// watchStopTimeout: alla cancellazione del ctx la chiusura dell'IDLE su
	// connessione sana è immediata; su connessione muta può aspettare i
	// timeout di scrittura/lettura del client (~30s): attendiamo al più
	// questo prima di chiudere comunque il canale.
	watchStopTimeout = 1500 * time.Millisecond
	// watchHeaderLimit: quante intestazioni nuove leggere per evento (Piano 11,
	// review punto 2): gli ultimi N UID, il conteggio dei non letti resta reale.
	watchHeaderLimit = 5
)

// watchBackoff è la schedule di riconnessione del piano (1s, 2s, 5s, 10s);
// oltre l'ultima soglia si applica il cap watchBackoffCap.
var watchBackoff = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
}

const watchBackoffCap = 30 * time.Second

// errWatchDone segnala internamente la cancellazione del ctx: il loop chiude
// il canale e ritorna.
var errWatchDone = errors.New("watch: done")

// watchKind classifica il wake arrivato da un dato unilaterale.
type watchKind int

const (
	wakeMailbox watchKind = iota // EXISTS/EXPUNGE: da ri-verificare con SEARCH
	wakeFlags                    // FETCH unilaterale: cambio di flag (uid valido)
)

// watchWake è il segnale spedito dai gestori unilaterali al loop del Watch.
type watchWake struct {
	kind watchKind
	uid  imap.UID // valido per wakeFlags se il FETCH conteneva l'UID
}

// watchState è lo stato del feed tra un ciclo e l'altro.
type watchState struct {
	lastUID  imap.UID // UID più alto visto in INBOX
	uidValid uint32   // UIDVALIDITY della baseline: se cambia, gli UID non sono comparabili
	mailbox  string   // mailbox osservata ("INBOX")
}

// Watch ritorna il canale degli eventi push del feed (contratto Task 1):
// Select di INBOX e baseline dell'UID più alto, poi un goroutine fa IDLE e
// su ogni termine dell'idle (dato unilaterale/refresh/errore) emette i
// Change. Su errore di rete emette ChangeDisconnected e riconnette con
// backoff riusando Connect (credenziali di Options). ctx.Done ⇒ canale
// chiuso. Canale buffer 16, send non bloccante: se il consumer è lento gli
// eventi si perdono e deve re-listare.
//
// Gate (Piano 3, Task 1): la baseline sincrona e la ri-verifica nella
// riconnessione sono comandi watch-side FUORI dai round di idle e girano
// dentro un round acquire→release del gate (una op alla volta: non
// interleaveano con op in flight); la ri-verifica DENTRO un round di idle
// gira sotto il hold implicito del round (idling=true), senza acquire.
//
// Nota: con più Watch contemporanei sull'ultimo Store i dati unilaterali
// alimentano solo il più recente (s.wakes è unico): l'uso previsto è UN
// Watch per volta (un solo canale per chiamata).
//
// Nota: chiudere il ctx di Watch PRIMA di chiamare Close: Close rompe
// l'IDLE e marca il gate come chiuso, ma finché il ctx resta vivo il loop
// può ancora tentare riconnessioni sul client già chiuso.
func (s *Store) Watch(ctx context.Context) (<-chan mail.Change, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := s.connected()
	if err != nil {
		return nil, err
	}

	// Baseline sincrona: Select INBOX + UID più alto esistente. Fallire qui
	// (mailbox rotta, connessione morta) è un errore di Watch, non un evento.
	//
	// Gate round (Piano 3, Task 1): la baseline è un comando watch-side FUORI
	// dai round di idle (fa SELECT: senza gate un'op concorrente potrebbe
	// selezionare un'altra mailbox in mezzo) — gira dentro acquire→release
	// per non interleaveare con op in flight. Nessun rischio di deadlock con
	// l'idle loop: qui il watch-goroutine non esiste ancora (niente idling
	// possibile), acquire può solo attendere ops in flight.
	release := s.acquire(ctx)
	st, err := baselineState(c)
	release()
	if err != nil {
		return nil, err
	}

	ch := make(chan mail.Change, watchBuffer)
	wakes := make(chan watchWake, watchBuffer)
	s.mu.Lock()
	s.wakes = wakes
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			// Solo il watch più recente può azzerare s.wakes: se un vecchio
			// goroutine esce dopo l'avvio di un nuovo Watch non deve cancellare
			// il canale del nuovo.
			if s.wakes == wakes {
				s.wakes = nil
			}
			s.mu.Unlock()
			close(ch)
		}()
		s.watchLoop(ctx, ch, wakes, st)
	}()
	return ch, nil
}

// watchLoop è il ciclo principale: un round di IDLE per iterazione; errore ⇒
// ChangeDisconnected + riconnessione con backoff.
func (s *Store) watchLoop(ctx context.Context, ch chan<- mail.Change, wakes <-chan watchWake, st *watchState) {
	backoff := 0
	for {
		err := s.idleRound(ctx, ch, wakes, st)
		switch {
		case err == nil:
			backoff = 0 // round sano: azzera il backoff
			continue
		case errors.Is(err, errWatchDone):
			return // ctx cancellato: il defer chiude il canale
		default:
			// Errore di rete (o del server): avvisa e riconnetta.
			sendChange(ch, mail.Change{Kind: mail.ChangeDisconnected})
			if !s.reconnect(ctx, ch, wakes, st, &backoff) {
				return // ctx cancellato durante il backoff
			}
		}
	}
}

// idleRound esegue un ciclo: attesa del gate (niente ops in volo), IDLE su
// INBOX già selezionata, attesa di wake/refresh/chiusura/break, gestione dei
// cambiamenti. Ritorna nil per ripartire, errWatchDone su ctx cancellato o
// Store chiuso, altro errore per il path di riconnessione.
func (s *Store) idleRound(ctx context.Context, ch chan<- mail.Change, wakes <-chan watchWake, st *watchState) error {
	if _, err := s.connected(); err != nil { // fallimento rapido, senza attendere il gate
		return err
	}

	// Gate: l'IDLE parte solo senza ops in volo (holders == 0); breakCh è
	// chiuso da idleGate.acquire quando un'op vuole la connessione.
	breakCh, ok := s.gate.waitToIdle()
	if !ok {
		return errWatchDone // Store chiuso (Close): il watch termina
	}
	defer s.gate.idleDone() // fine del round, qualunque sia la causa

	// Il client si rilegge DOPO il gate: un'op in volo puo averlo sostituito (dopo uno
	// stallo UnreadCounts riconnette lo Store, cfr. unseen.go); con il vecchio, chiuso,
	// il Watch emetterebbe un Disconnected e aprirebbe una seconda connessione.
	c, err := s.connected()
	if err != nil {
		return err
	}

	// Piano 7, Task 1: un'op su un'altra cartella (ListMessages, ApplyLabel) lascia
	// quella mailbox selezionata sulla connessione. Senza questa SELECT l'IDLE
	// osserverebbe la cartella sbagliata e la posta nuova in INBOX non arriverebbe.
	if _, err := c.Select(st.mailbox, nil).Wait(); err != nil {
		return fmt.Errorf("imap select %s prima di idle: %w", st.mailbox, err)
	}

	idle, err := c.Idle()
	if err != nil {
		return fmt.Errorf("imap idle: %w", err)
	}
	idleDone := make(chan error, 1)
	go func() { idleDone <- idle.Wait() }()

	var (
		flagUIDs []imap.UID // uid dei FETCH unilaterali raccolti in questo round
		noop     bool       // refresh periodico scaduto
	)
	timer := time.NewTimer(watchRefresh)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		stopIdleBounded(idle, idleDone)
		return errWatchDone

	case err := <-idleDone:
		// L'idle è terminato da sé: su errore di rete è il path di
		// disconnessione (nessun Close necessario: il comando è già
		// completato); un ritorno senza errore né Close non è previsto
		// dalla libreria, ma lo trattiamo come wake generico.
		if err != nil {
			return fmt.Errorf("imap idle wait: %w", err)
		}

	case w := <-wakes:
		flagUIDs = collectWakes(wakes, w)

	case <-breakCh:
		// Un'op dello Store vuole la connessione (idleGate.acquire): si esce
		// dall'IDLE con il DONE standard; nessuna azione dedicata, il round
		// prosegue (ri-verifica SEARCH) e al giro successivo si re-idla.

	case <-timer.C:
		// Limite di protocollo: si riapre l'IDLE con un NOOP in mezzo (il
		// NOOP smalta anche eventuali update accodati lato server).
		noop = true
	}

	if err := stopIdle(idle, idleDone); err != nil {
		return err
	}
	if noop {
		if err := c.Noop().Wait(); err != nil {
			return fmt.Errorf("imap noop refresh: %w", err)
		}
	}

	// Ri-verifica dei nuovi messaggi (economica: un UID SEARCH) dopo ogni
	// termine dell'idle: copre wakeMailbox, wake persi e il ritorno dalla
	// riconnessione. Nessun acquire qui (Piano 3, Task 1): siamo DENTRO il
	// round — idling=true È il hold della connessione da parte del watch —
	// e acquire attenderebbe !idling: deadlock per definizione.
	if err := s.recheckNewMail(c, ch, st); err != nil {
		return err
	}

	if len(flagUIDs) > 0 {
		ids := make([]msg.MessageID, len(flagUIDs))
		for i, uid := range flagUIDs {
			ids[i] = msg.NewID(st.mailbox, uint32(uid))
		}
		sendChange(ch, mail.Change{Kind: mail.ChangeFlags, IDs: ids})
	}
	return nil
}

// collectWakes drena (non bloccante) i wake accumulati dietro il primo,
// raccogliendo gli uid dei cambi di flag: un solo round di IDLE per raffica.
func collectWakes(wakes <-chan watchWake, first watchWake) []imap.UID {
	var flagUIDs []imap.UID
	if first.kind == wakeFlags && first.uid != 0 {
		flagUIDs = append(flagUIDs, first.uid)
	}
	for {
		select {
		case w := <-wakes:
			if w.kind == wakeFlags && w.uid != 0 {
				flagUIDs = append(flagUIDs, w.uid)
			}
		default:
			// I flag wake senza UID (FETCH senza UID, raro) e i wakeMailbox
			// scartati qui sono comunque coperti dalla ri-verifica SEARCH.
			return flagUIDs
		}
	}
}

// stopIdle chiude l'IDLE in modo pulito (DONE + attesa del tagged response).
// In caso di errore di Close il Wait abbandonato non leaka: idleDone è
// bufferizzato (cap 1) e il suo goroutine termina comunque.
func stopIdle(idle *imapclient.IdleCommand, idleDone <-chan error) error {
	if err := idle.Close(); err != nil {
		return fmt.Errorf("imap idle close: %w", err)
	}
	if err := <-idleDone; err != nil {
		return fmt.Errorf("imap idle wait: %w", err)
	}
	return nil
}

// stopIdleBounded è stopIdle con attesa limitata: sulla cancellazione del ctx
// non vale la pena aspettare i timeout del client su una connessione muta.
func stopIdleBounded(idle *imapclient.IdleCommand, idleDone <-chan error) {
	done := make(chan struct{})
	go func() {
		_ = stopIdle(idle, idleDone)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(watchStopTimeout):
		// Abbandona: il client resta in IDLE finché non viene chiuso da
		// Close/riconnessione (documentato).
	}
}

// reconnect riconnette con backoff (1s, 2s, 5s, 10s, poi cap 30s) riusando
// le credenziali di Options tramite Connect (che chiude il vecchio client).
// Dopo la riconnessione ri-verifica i nuovi messaggi arrivati mentre eravamo
// offline (baseline aggiornata solo se la UIDVALIDITY è cambiata). Ritorna
// false se il ctx è cancellato mentre si aspetta.
func (s *Store) reconnect(ctx context.Context, ch chan<- mail.Change, wakes <-chan watchWake, st *watchState, attempt *int) bool {
	for {
		delay := watchBackoffCap
		if *attempt < len(watchBackoff) {
			delay = watchBackoff[*attempt]
		}
		*attempt++
		select {
		case <-ctx.Done():
			return false
		case <-time.After(delay):
		}

		if err := s.Connect(ctx); err != nil {
			continue // backoff al prossimo giro
		}
		c, err := s.connected()
		if err != nil {
			continue
		}
		// La ri-verifica post-riconnessione emette anche i ChangeNewMail
		// per le mail arrivate mentre eravamo offline (search da lastUID).
		//
		// Gate round (Piano 3, Task 1): comando watch-side FUORI dai round
		// di idle (fa SELECT) — acquire→release lo esclude da op in flight.
		// Nessun deadlock: qui idling è GIÀ false (idleDone è girato alla
		// fine dell'idleRound fallito, prima della riconnessione) e il watch
		// non detiene nulla mentre attende: il round parte quando le ops in
		// flight hanno finito.
		release := s.acquire(ctx)
		err = s.recheckNewMail(c, ch, st)
		release()
		if err != nil {
			continue // connessione ancora instabile: riprova
		}
		return true
	}
}

// recheckNewMail seleziona la mailbox osservata e cerca gli UID oltre
// l'ultimo visto: se la UIDVALIDITY è cambiata gli UID non sono comparabili
// e la baseline viene rifatta in silenzio (eventi persi durante il cambio
// d'epoca: il consumer re-lista al prossimo Change); altrimenti emette
// ChangeNewMail con i nuovi ID e avanza la baseline.
func (s *Store) recheckNewMail(c *imapclient.Client, ch chan<- mail.Change, st *watchState) error {
	sel, err := c.Select(st.mailbox, nil).Wait()
	if err != nil {
		return fmt.Errorf("imap select %s: %w", st.mailbox, err)
	}
	if sel.UIDValidity != st.uidValid {
		// Nuova epoca della mailbox: ri-baseline senza eventi (gli UID
		// vecchi e nuovi non sono confrontabili).
		st.uidValid = sel.UIDValidity
		last, err := highestUID(c)
		if err != nil {
			return err
		}
		st.lastUID = last
		return nil
	}

	var set imap.UIDSet
	set.AddRange(st.lastUID+1, 0) // 0 = "*" ⇒ "UID (last+1):*"
	data, err := c.UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{set}}, nil).Wait()
	if err != nil {
		return fmt.Errorf("imap uid search %s: %w", st.mailbox, err)
	}
	// Il filtro client-side è dovuto: "n:*" include per RFC anche l'ultimo
	// messaggio quando n è oltre la coda (e il memserver normalizza i range).
	var newUIDs []imap.UID
	for _, uid := range data.AllUIDs() {
		if uid > st.lastUID {
			newUIDs = append(newUIDs, uid)
		}
	}
	if len(newUIDs) == 0 {
		return nil
	}
	// Piano 11, review punto 3: quanti dei nuovi sono NON letti (UID SEARCH NOT SEEN
	// sullo stesso intervallo). Errore di ricerca: si conta come se fossero tutti
	// non letti (nessuna notifica persa).
	unread := len(newUIDs)
	if u, err := c.UIDSearch(&imap.SearchCriteria{UID: []imap.UIDSet{set}, NotFlag: []imap.Flag{imap.FlagSeen}}, nil).Wait(); err == nil {
		unread = 0
		for _, uid := range u.AllUIDs() {
			if uid > st.lastUID {
				unread++
			}
		}
	}
	st.lastUID = newUIDs[len(newUIDs)-1]

	ids := make([]msg.MessageID, len(newUIDs))
	for i, uid := range newUIDs {
		ids[i] = msg.NewID(st.mailbox, uint32(uid))
	}
	// Piano 11, Task 2 e review punto 2: intestazioni (ENVELOPE/FLAGS/INTERNALDATE,
	// nessun BODY, nessun \Seen) solo degli ultimi watchHeaderLimit UID: dopo molta
	// posta offline il gate non resta occupato. Best-effort: un fetch fallito non
	// è un errore di connessione, l'evento parte con i soli ID.
	hdr := newUIDs
	if len(hdr) > watchHeaderLimit {
		hdr = hdr[len(hdr)-watchHeaderLimit:]
	}
	fetch := s.fetchRefs
	if s.headersFn != nil {
		fetch = s.headersFn
	}
	refs, err := fetch(st.mailbox, hdr)
	if err != nil {
		refs = nil
	}
	sendChange(ch, mail.Change{Kind: mail.ChangeNewMail, IDs: ids, Refs: refs, Unread: unread})
	return nil
}

// baselineState seleziona INBOX e ne ricava lo stato iniziale: UIDVALIDITY e
// UID più alto esistente (0 se vuota).
func baselineState(c *imapclient.Client) (*watchState, error) {
	sel, err := c.Select("INBOX", nil).Wait()
	if err != nil {
		return nil, fmt.Errorf("imap select INBOX: %w", err)
	}
	last, err := highestUID(c)
	if err != nil {
		return nil, err
	}
	return &watchState{lastUID: last, uidValid: sel.UIDValidity, mailbox: "INBOX"}, nil
}

// highestUID ritorna l'UID più alto della mailbox selezionata (UIDSearch
// match-all: la lista è ascendente, l'ultimo è il massimo; 0 se vuota).
func highestUID(c *imapclient.Client) (imap.UID, error) {
	data, err := c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
	if err != nil {
		return 0, fmt.Errorf("imap uid search: %w", err)
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return 0, nil
	}
	return uids[len(uids)-1], nil
}

// sendChange è la send non bloccante del contratto: se il canale è pieno
// l'evento è PERSO e il consumer deve re-listare.
func sendChange(ch chan<- mail.Change, c mail.Change) {
	select {
	case ch <- c:
	default:
	}
}

// pushWake segnala un dato unilaterale al Watch attivo (se il canale è
// pieno il wake è perso: la ri-verifica SEARCH dopo il prossimo wake o il
// refresh periodico copre comunque i nuovi messaggi).
func (s *Store) pushWake(w watchWake) {
	s.mu.Lock()
	ch := s.wakes
	s.mu.Unlock()
	if ch == nil {
		return // nessun Watch attivo: dato unilaterale non interessante
	}
	select {
	case ch <- w:
	default:
	}
}

// unilateralHandler costruisce i gestori dei dati unilaterali installati a
// ogni dial (Connect). Devono essere rapidi (il gestore Mailbox/Expunge gira
// nel goroutine di lettura del client): solo una pushWake non bloccante.
func (s *Store) unilateralHandler() *imapclient.UnilateralDataHandler {
	return &imapclient.UnilateralDataHandler{
		// EXISTS: NumMessages settato ⇒ possibili nuovi messaggi.
		Mailbox: func(data *imapclient.UnilateralDataMailbox) {
			if data.NumMessages != nil {
				s.pushWake(watchWake{kind: wakeMailbox})
			}
		},
		// FETCH non richiesto da comandi pendenti: cambio di flag (il
		// server invia UID+FLAGS). I dati streaming vanno drenati comunque
		// (contratto imapclient): Collect li consuma e dà l'UID.
		Fetch: func(data *imapclient.FetchMessageData) {
			w := watchWake{kind: wakeFlags}
			if buf, err := data.Collect(); err == nil && buf.UID != 0 {
				w.uid = buf.UID
			}
			s.pushWake(w)
		},
		// EXPUNGE: la lista si restringe, nessun ChangeKind dedicato; il
		// wakeMailbox forza la ri-verifica (coerenza della baseline).
		Expunge: func(uint32) {
			s.pushWake(watchWake{kind: wakeMailbox})
		},
	}
}
