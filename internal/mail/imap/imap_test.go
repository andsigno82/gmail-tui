package imap

// Harness IMAP in-process (imapmemserver) + test di Store.Connect e
// ListMessages. La harness è riusabile dai test dei Task 4-7 (stesso
// package): startTestServer(t) avvia il server, i metodi di testServer
// fanno seeding diretto via API del memserver.

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// testDate è la Data fissa usata da rawMail e dal seeding (INTERNALDATE
// deterministica: i test asseriscono Date su questo valore).
var testDate = time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)

// testServer è la harness: server IMAP in-process con utente tester/secret.
type testServer struct {
	t    *testing.T
	ln   net.Listener
	srv  *imapserver.Server
	user *imapmemserver.User

	// mu protegge boxes: il LIST di specialUseSession gira nel goroutine del
	// server, i test scrivono dal goroutine del test.
	mu sync.Mutex
	// boxes elenca le mailbox servite nel LIST RETURN (SPECIAL-USE) con i
	// rispettivi attributi. Il memserver beta.9 ignora CreateOptions.SpecialUse
	// (nessun modo di assegnare \Trash e simili): gli attributi vivono qui.
	// Piano 7, Task 1.
	boxes map[string][]imap.MailboxAttr
}

// startTestServer avvia imapmemserver su 127.0.0.1:0 e registra la chiusura
// con t.Cleanup. INBOX esiste sempre; altre mailbox con createMailbox. Le
// capability sono il default del server (solo IMAP4rev1: niente MOVE/UIDPLUS).
func startTestServer(t *testing.T) *testServer {
	t.Helper()
	return startTestServerCaps(t, nil)
}

// startTestServerCaps è startTestServer con le capability annunciate indicate
// (Options.Caps; nil = default IMAP4rev1). Piano 7, Task 1: il Trash richiede
// MOVE o UIDPLUS, che il default del memserver non annuncia.
func startTestServerCaps(t *testing.T, caps imap.CapSet) *testServer {
	t.Helper()

	mem := imapmemserver.New()
	user := imapmemserver.NewUser("tester", "secret")
	if err := user.Create("INBOX", nil); err != nil {
		t.Fatalf("create INBOX: %v", err)
	}
	mem.AddUser(user)

	ts := &testServer{t: t, user: user, boxes: map[string][]imap.MailboxAttr{"INBOX": nil}}
	ts.srv = imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return &specialUseSession{Session: mem.NewSession(), ts: ts}, nil, nil
		},
		// Solo test: LOGIN consentito senza TLS, la connessione è in
		// chiaro (Options.Insecure dello Store).
		InsecureAuth: true,
		Caps:         caps,
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ts.ln = ln
	go func() {
		// Close fa terminare Serve con net.ErrServerClosed: ignorato.
		_ = ts.srv.Serve(ln)
	}()
	t.Cleanup(func() { _ = ts.srv.Close() })

	return ts
}

// specialUseDelim è il delimitatore gerarchico del memserver.
const specialUseDelim = '/'

// specialUseSession avvolge la sessione del memserver (Piano 7, Task 1).
// LIST con RETURN (SPECIAL-USE) è servito da ts.boxes; il resto, MOVE
// incluso, delega alla sessione del memserver. L'embedding dell'interfaccia
// non promuove SessionMove: Move va inoltrato a mano, altrimenti il server
// annuncia MOVE e va in panic (cfr. imapserver/conn.go).
type specialUseSession struct {
	imapserver.Session
	ts *testServer
}

// List implementa imapserver.Session: senza RETURN (SPECIAL-USE) delega al
// memserver; con RETURN scrive le mailbox di ts.boxes che matchano i pattern.
func (s *specialUseSession) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	if options == nil || !options.ReturnSpecialUse {
		return s.Session.List(w, ref, patterns, options)
	}
	s.ts.mu.Lock()
	defer s.ts.mu.Unlock()
	names := make([]string, 0, len(s.ts.boxes))
	for name := range s.ts.boxes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !matchAnyPattern(name, ref, patterns) {
			continue
		}
		if err := w.WriteList(&imap.ListData{
			Attrs:   s.ts.boxes[name],
			Delim:   specialUseDelim,
			Mailbox: name,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Move inoltra il MOVE alla sessione del memserver (imapserver.SessionMove).
func (s *specialUseSession) Move(w *imapserver.MoveWriter, numSet imap.NumSet, destName string) error {
	return s.Session.(imapserver.SessionMove).Move(w, numSet, destName)
}

// matchAnyPattern dice se name matcha uno dei pattern LIST (stessa regola
// del memserver: imapserver.MatchList).
func matchAnyPattern(name, ref string, patterns []string) bool {
	for _, p := range patterns {
		if imapserver.MatchList(name, specialUseDelim, ref, p) {
			return true
		}
	}
	return false
}

// addr restituisce l'indirizzo del server di test.
func (ts *testServer) addr() string {
	return ts.ln.Addr().String()
}

// listAs dichiara una mailbox nel LIST con gli attributi indicati, senza
// crearla (es. il contenitore "[Gmail]" con \Noselect di Gmail).
func (ts *testServer) listAs(name string, attrs ...imap.MailboxAttr) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.boxes == nil {
		ts.boxes = map[string][]imap.MailboxAttr{}
	}
	ts.boxes[name] = attrs
}

// createMailbox crea una mailbox (label) lato server.
func (ts *testServer) createMailbox(name string) {
	ts.t.Helper()
	if err := ts.user.Create(name, nil); err != nil {
		ts.t.Fatalf("create mailbox %q: %v", name, err)
	}
	ts.listAs(name)
}

// createSpecialMailbox crea una mailbox con attributi SPECIAL-USE (es.
// \Trash), come farebbe Gmail alla creazione delle proprie cartelle.
func (ts *testServer) createSpecialMailbox(name string, attrs ...imap.MailboxAttr) {
	ts.t.Helper()
	ts.createMailbox(name)
	ts.listAs(name, attrs...)
}

// seedMail appende un messaggio in INBOX e ne restituisce l'UID.
func (ts *testServer) seedMail(from, subject, body string, seen, flagged bool) uint32 {
	return ts.seedInMailbox("INBOX", from, subject, body, seen, flagged)
}

// seedInMailbox appende un messaggio nella mailbox indicata e ne restituisce
// l'UID. Il seeding usa l'APPEND diretto del memserver (API Go, senza
// connessione di rete).
func (ts *testServer) seedInMailbox(name, from, subject, body string, seen, flagged bool) uint32 {
	ts.t.Helper()
	return ts.seedRaw(name, rawMail(from, subject, body), seen, flagged)
}

// seedRaw appende un messaggio RFC822 già formato (grezzo) nella mailbox e
// ne restituisce l'UID: variant della harness per messaggi arbitrarî
// (es. multipart, Task 4).
func (ts *testServer) seedRaw(name string, raw []byte, seen, flagged bool) uint32 {
	ts.t.Helper()

	var flags []imap.Flag
	if seen {
		flags = append(flags, imap.FlagSeen)
	}
	if flagged {
		flags = append(flags, imap.FlagFlagged)
	}
	data, err := ts.user.Append(name, rawLiteral{r: bytes.NewReader(raw)}, &imap.AppendOptions{
		Flags: flags,
		Time:  testDate, // INTERNALDATE deterministica
	})
	if err != nil {
		ts.t.Fatalf("append in %q: %v", name, err)
	}
	return uint32(data.UID)
}

// rawMail costruisce un messaggio RFC822 minimale con Date fissa
// (testDate) e Message-ID derivato dal subject (deterministico).
func rawMail(from, subject, body string) []byte {
	messageID := strings.NewReplacer(" ", "-", "@", "-at-").Replace(strings.ToLower(subject))
	return []byte("From: " + from + "\r\n" +
		"To: tester@example.org\r\n" +
		"Subject: " + subject + "\r\n" +
		"Date: " + testDate.Format(time.RFC1123Z) + "\r\n" +
		"Message-ID: <" + messageID + "@example.org>\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		body + "\r\n")
}

// rawLiteral adatta []byte a imap.LiteralReader per l'APPEND del memserver.
type rawLiteral struct {
	r *bytes.Reader
}

func (l rawLiteral) Read(p []byte) (int, error) { return l.r.Read(p) }
func (l rawLiteral) Size() int64                { return l.r.Size() }

// connectStore crea uno Store collegato al server di test e ne registra la
// chiusura con t.Cleanup.
func connectStore(t *testing.T, ts *testServer) *Store {
	t.Helper()
	s := NewStore(Options{
		Addr:     ts.addr(),
		Username: "tester",
		Password: "secret",
		Insecure: true, // connessione in chiaro verso la harness
	})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// seedFive è il corpus comune: 5 mail in INBOX (mix seen/flagged). L'ordine
// di seeding è a UID crescenti: l'ultima appesa è la più recente.
func (ts *testServer) seedFive(t *testing.T) {
	t.Helper()
	ts.seedMail("Alice <alice@example.org>", "First", "primo corpo", true, false)
	ts.seedMail("Bob <bob@example.org>", "Second", "secondo corpo", false, false)
	ts.seedMail("Carol <carol@example.org>", "Third", "terzo corpo", true, true)
	ts.seedMail("Dave <dave@example.org>", "Fourth", "quarto corpo", false, true)
	ts.seedMail("Eve <eve@example.org>", "Fifth", "quinto corpo", false, false)
}

// subjects estrae i subject nell'ordine (asserzioni compatte).
func subjects(refs []msg.MessageRef) []string {
	out := make([]string, len(refs))
	for i, r := range refs {
		out[i] = r.Subject
	}
	return out
}

// TestListMessagesMapsAndOrders: 5 seed → List(Query{}, Page{}) restituisce
// 5 ref dal più nuovo al più vecchio con i campi mappati (Limit 0 ⇒ default
// 50; offset 0 = più recenti).
func TestListMessagesMapsAndOrders(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	refs, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(refs) != 5 {
		t.Fatalf("refs = %d, voglio 5 (%v)", len(refs), subjects(refs))
	}

	wantSubjects := []string{"Fifth", "Fourth", "Third", "Second", "First"}
	got := subjects(refs)
	for i, want := range wantSubjects {
		if got[i] != want {
			t.Errorf("refs[%d].Subject = %q, voglio %q (ordine dal più nuovo)", i, got[i], want)
		}
	}

	// La più recente (Fifth): mittente formattato "Name <addr>", unseen,
	// non starred, INTERNALDATE = testDate, ThreadLen 1, ID INBOX:uid.
	fifth := refs[0]
	if fifth.From != "Eve <eve@example.org>" {
		t.Errorf("refs[0].From = %q, voglio %q", fifth.From, "Eve <eve@example.org>")
	}
	if !fifth.Unread {
		t.Error("refs[0].Unread = false, voglio true (senza \\Seen)")
	}
	if fifth.Starred {
		t.Error("refs[0].Starred = true, voglio false")
	}
	if !fifth.Date.Equal(testDate) {
		t.Errorf("refs[0].Date = %v, voglio %v (INTERNALDATE)", fifth.Date, testDate)
	}
	if fifth.ThreadLen != 1 {
		t.Errorf("refs[0].ThreadLen = %d, voglio 1 (ENVELOPE non espone il thread)", fifth.ThreadLen)
	}
	if want := msg.NewID("INBOX", 5); fifth.ID != want {
		t.Errorf("refs[0].ID = %q, voglio %q", fifth.ID, want)
	}

	// Terza (Third): seen+flagged → letta e starred.
	third := refs[2]
	if third.Unread {
		t.Error("refs[2].Unread = true, voglio false (\\Seen)")
	}
	if !third.Starred {
		t.Error("refs[2].Starred = false, voglio true (\\Flagged)")
	}
	if want := msg.NewID("INBOX", 3); third.ID != want {
		t.Errorf("refs[2].ID = %q, voglio %q", third.ID, want)
	}
}

// TestListMessagesPagination: Page{Offset:1, Limit:2} salta la più recente
// e restituisce 2 ref, sempre dal più nuovo.
func TestListMessagesPagination(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	refs, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{Offset: 1, Limit: 2})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	want := []string{"Fourth", "Third"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
	}
	for i, s := range want {
		if refs[i].Subject != s {
			t.Errorf("refs[%d].Subject = %q, voglio %q", i, refs[i].Subject, s)
		}
	}
}

// TestListMessagesUnreadFilter: Query{Unread:true} restituisce solo le
// unseen (Fifth, Fourth, Second), dal più nuovo.
func TestListMessagesUnreadFilter(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	refs, err := store.ListMessages(t.Context(), msg.Query{Unread: true}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	want := []string{"Fifth", "Fourth", "Second"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
	}
	for i := range refs {
		if !refs[i].Unread {
			t.Errorf("refs[%d] (%q): Unread = false, voglio true", i, refs[i].Subject)
		}
	}
}

// TestListMessagesLabelSelectsMailbox: Query{Label:"Archive"} seleziona la
// mailbox Archive invece di INBOX (semantica label = cartella).
func TestListMessagesLabelSelectsMailbox(t *testing.T) {
	ts := startTestServer(t)
	ts.createMailbox("Archive")
	uidA := ts.seedInMailbox("Archive", "Arch1 <a@example.org>", "Archived one", "corpo a", true, false)
	uidB := ts.seedInMailbox("Archive", "Arch2 <a@example.org>", "Archived two", "corpo b", false, true)
	ts.seedMail("Inbox <i@example.org>", "Inbox only", "corpo inbox", false, false)
	store := connectStore(t, ts)

	refs, err := store.ListMessages(t.Context(), msg.Query{Label: "Archive"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	want := []string{"Archived two", "Archived one"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
	}
	if refs[0].ID != msg.NewID("Archive", uidB) || refs[1].ID != msg.NewID("Archive", uidA) {
		t.Errorf("IDs = [%s %s], voglio [%s %s]", refs[0].ID, refs[1].ID, msg.NewID("Archive", uidB), msg.NewID("Archive", uidA))
	}
	if !refs[0].Unread || !refs[0].Starred {
		t.Errorf("refs[0] (Archived two): Unread=%v Starred=%v, voglio true/true (seed senza \\Seen, con \\Flagged)", refs[0].Unread, refs[0].Starred)
	}
	if refs[1].Unread || refs[1].Starred {
		t.Errorf("refs[1] (Archived one): Unread=%v Starred=%v, voglio false/false", refs[1].Unread, refs[1].Starred)
	}
}

// TestConcurrentListMessagesNoCrossMailbox (Piano 3, Task 1: mutua esclusione
// op-vs-op del gate): due ListMessages CONCORRENTI su mailbox diverse devono
// restituire SOLO i ref della propria mailbox. Pre-fix il gate serializzava
// solo watch-vs-op: le due op potevano interleaveare i SELECT — SEARCH/FETCH
// agiscono sulla mailbox correntemente selezionata, quindi l'op vedeva (o
// restituiva) i messaggi dell'ALTRA mailbox. Post-fix le operazioni sono
// serializzate dal gate: una op alla volta.
//
// Nota sul RED: la finestra di interleave (tra il SELECT di un'op e il suo
// UIDSearch) dipende dallo scheduling; il test ripete molti round per
// allargarla, ma un RED deterministico non è garantito — in quel caso il test
// resta l'asserzione di correttezza post-fix (cfr. task-p2-7-report.md,
// "P3 Task 1: gate mutual exclusion").
func TestConcurrentListMessagesNoCrossMailbox(t *testing.T) {
	ts := startTestServer(t)
	ts.createMailbox("Archive")
	for i := 1; i <= 5; i++ {
		ts.seedMail(fmt.Sprintf("In%d <in%d@example.org>", i, i), fmt.Sprintf("Inbox %d", i), "corpo", false, false)
		ts.seedInMailbox("Archive", fmt.Sprintf("Arc%d <arc%d@example.org>", i, i), fmt.Sprintf("Archive %d", i), "corpo", false, false)
	}
	store := connectStore(t, ts)

	// checkMailbox: refs deve contenere SOLO messaggi della mailbox richiesta
	// (5, uno per seed) — un ref di un'altra mailbox è la firma dell'interleave
	// dei SELECT pre-fix.
	checkMailbox := func(mailbox string, refs []msg.MessageRef, err error) error {
		if err != nil {
			return fmt.Errorf("ListMessages %s: %w", mailbox, err)
		}
		if len(refs) != 5 {
			return fmt.Errorf("ListMessages %s: %d ref, voglio 5 (%v)", mailbox, len(refs), subjects(refs))
		}
		wantPrefix := "Archive"
		if mailbox == "INBOX" {
			wantPrefix = "Inbox"
		}
		for _, r := range refs {
			mb, _, err := r.ID.Split()
			if err != nil || mb != mailbox {
				return fmt.Errorf("ListMessages %s: ref nella mailbox sbagliata: %s (SELECT interlacciata)", mailbox, r.ID)
			}
			if !strings.HasPrefix(r.Subject, wantPrefix) {
				return fmt.Errorf("ListMessages %s: subject %q di un'altra mailbox", mailbox, r.Subject)
			}
		}
		return nil
	}

	const rounds = 25
	start := make(chan struct{}) // barriera: le due op partono insieme
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, mailbox := range []string{"INBOX", "Archive"} {
		wg.Add(1)
		go func(mailbox string) {
			defer wg.Done()
			<-start
			q := msg.Query{} // label vuota = INBOX
			if mailbox != "INBOX" {
				q.Label = mailbox
			}
			for i := 0; i < rounds; i++ {
				refs, err := store.ListMessages(t.Context(), q, mail.Page{})
				if err := checkMailbox(mailbox, refs, err); err != nil {
					errs <- err
					return
				}
			}
			errs <- nil
		}(mailbox)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

// TestListMessagesEmptyFrom: envelope senza From valido → stringa vuota
// (contratto: "stringa vuota se assente").
func TestListMessagesEmptyFrom(t *testing.T) {
	ts := startTestServer(t)
	ts.seedMail("", "No sender", "corpo", false, false)
	store := connectStore(t, ts)

	refs, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("refs = %d, voglio 1", len(refs))
	}
	if refs[0].From != "" {
		t.Errorf("refs[0].From = %q, voglio \"\"", refs[0].From)
	}
}
