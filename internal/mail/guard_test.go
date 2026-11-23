package mail

// Test della guardia degli ID confermati (Piano 16, Task 5, regola R4): uno
// store finto conta le chiamate che arrivano allo store interno; la guardia
// deve rifiutare con ErrNotConfirmed SENZA chiamarlo.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// guardFake è uno store interno finto: conta le chiamate per metodo e
// restituisce i risultati configurati. Non fa nulla di più.
type guardFake struct {
	mu    sync.Mutex
	calls map[string]int

	refs    []msg.MessageRef
	convs   []Conversation
	members []msg.MessageID
	ids     []msg.MessageID // risultato di Trash, Restore, AllMailIDs
	draft   msg.MessageID
	changes chan Change
	err     error // errore restituito dai metodi di lettura e da Connect
}

func newGuardFake() *guardFake {
	return &guardFake{calls: map[string]int{}, changes: make(chan Change, 4)}
}

func (f *guardFake) hit(name string) {
	f.mu.Lock()
	f.calls[name]++
	f.mu.Unlock()
}

func (f *guardFake) count(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[name]
}

func (f *guardFake) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

func (f *guardFake) setErr(err error) {
	f.mu.Lock()
	f.err = err
	f.mu.Unlock()
}

func (f *guardFake) getErr() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

func (f *guardFake) Connect(context.Context) error { f.hit("Connect"); return f.getErr() }
func (f *guardFake) ListMessages(context.Context, msg.Query, Page) ([]msg.MessageRef, error) {
	f.hit("ListMessages")
	return f.refs, f.getErr()
}
func (f *guardFake) FetchMessage(context.Context, msg.MessageID) (*msg.Message, error) {
	f.hit("FetchMessage")
	return &msg.Message{}, nil
}
func (f *guardFake) Search(context.Context, msg.Query) ([]msg.MessageRef, error) {
	f.hit("Search")
	return f.refs, f.getErr()
}
func (f *guardFake) ListConversations(context.Context, msg.Query, Page) ([]Conversation, error) {
	f.hit("ListConversations")
	return f.convs, f.getErr()
}
func (f *guardFake) ConversationMembers(context.Context, string, uint64) ([]msg.MessageID, error) {
	f.hit("ConversationMembers")
	return f.members, f.getErr()
}
func (f *guardFake) ThreadMessages(context.Context, uint64) ([]msg.MessageRef, error) {
	f.hit("ThreadMessages")
	return f.refs, f.getErr()
}
func (f *guardFake) MarkRead(context.Context, []msg.MessageID, bool) error {
	f.hit("MarkRead")
	return nil
}
func (f *guardFake) Archive(context.Context, []msg.MessageID) error { f.hit("Archive"); return nil }
func (f *guardFake) Star(context.Context, []msg.MessageID, bool) error {
	f.hit("Star")
	return nil
}
func (f *guardFake) ApplyLabel(context.Context, []msg.MessageID, string, bool) error {
	f.hit("ApplyLabel")
	return nil
}
func (f *guardFake) SaveAttachment(context.Context, msg.MessageID, int, string) error {
	f.hit("SaveAttachment")
	return nil
}
func (f *guardFake) Send(context.Context, msg.Outgoing) error { f.hit("Send"); return nil }
func (f *guardFake) ListMailboxes(context.Context) ([]Mailbox, error) {
	f.hit("ListMailboxes")
	return nil, nil
}
func (f *guardFake) UnreadCounts(context.Context) (map[string]int, error) {
	f.hit("UnreadCounts")
	return nil, nil
}
func (f *guardFake) Trash(context.Context, []msg.MessageID) ([]msg.MessageID, error) {
	f.hit("Trash")
	return f.ids, nil
}
func (f *guardFake) Restore(context.Context, []msg.MessageID, string) ([]msg.MessageID, error) {
	f.hit("Restore")
	return f.ids, nil
}
func (f *guardFake) AllMailIDs(context.Context, []msg.MessageID) ([]msg.MessageID, error) {
	f.hit("AllMailIDs")
	return f.ids, nil
}
func (f *guardFake) ModifyLabels(context.Context, []msg.MessageID, []string, []string) error {
	f.hit("ModifyLabels")
	return nil
}
func (f *guardFake) SaveDraft(context.Context, []byte, []msg.MessageID) (msg.MessageID, error) {
	f.hit("SaveDraft")
	return f.draft, nil
}
func (f *guardFake) DeleteDraft(context.Context, []msg.MessageID) error {
	f.hit("DeleteDraft")
	return nil
}
func (f *guardFake) Watch(context.Context) (<-chan Change, error) {
	f.hit("Watch")
	return f.changes, nil
}

// guardSnapFake aggiunge Snapshotter e GmailExt allo store finto.
type guardSnapFake struct {
	*guardFake
	conv   ConversationsSnapshot
	thread ThreadSnapshot
	body   *msg.Message
	gmail  bool
}

func (f *guardSnapFake) CachedConversations(string) (ConversationsSnapshot, bool) {
	return f.conv, true
}
func (f *guardSnapFake) CachedThread(uint64) (ThreadSnapshot, bool) { return f.thread, true }
func (f *guardSnapFake) CachedMessage(msg.MessageID) (*msg.Message, bool) {
	return f.body, f.body != nil
}
func (f *guardSnapFake) GmailExt() bool { return f.gmail }

const (
	idA msg.MessageID = "INBOX:1"
	idB msg.MessageID = "INBOX:2"
	idC msg.MessageID = "INBOX:3"
)

// guardIDOps elenca tutti i metodi che ricevono un MessageID, ognuno invocato
// su un solo ID.
func guardIDOps(id msg.MessageID) map[string]func(MailStore) error {
	ctx := context.Background()
	ids := []msg.MessageID{id}
	return map[string]func(MailStore) error{
		"FetchMessage":   func(s MailStore) error { _, err := s.FetchMessage(ctx, id); return err },
		"MarkRead":       func(s MailStore) error { return s.MarkRead(ctx, ids, true) },
		"Archive":        func(s MailStore) error { return s.Archive(ctx, ids) },
		"Star":           func(s MailStore) error { return s.Star(ctx, ids, true) },
		"ApplyLabel":     func(s MailStore) error { return s.ApplyLabel(ctx, ids, "L", true) },
		"SaveAttachment": func(s MailStore) error { return s.SaveAttachment(ctx, id, 0, "/tmp/x") },
		"Trash":          func(s MailStore) error { _, err := s.Trash(ctx, ids); return err },
		"Restore":        func(s MailStore) error { _, err := s.Restore(ctx, ids, "INBOX"); return err },
		"AllMailIDs":     func(s MailStore) error { _, err := s.AllMailIDs(ctx, ids); return err },
		"ModifyLabels":   func(s MailStore) error { return s.ModifyLabels(ctx, ids, []string{"L"}, nil) },
		"DeleteDraft":    func(s MailStore) error { return s.DeleteDraft(ctx, ids) },
		"SaveDraft":      func(s MailStore) error { _, err := s.SaveDraft(ctx, []byte("x"), ids); return err },
	}
}

// RED comportamentale: un ID mai uscito da una risposta fresca è rifiutato da
// ogni metodo con ID, senza chiamare lo store interno.
func TestGuardRefusesUnconfirmedIDs(t *testing.T) {
	for name, call := range guardIDOps(idA) {
		t.Run(name, func(t *testing.T) {
			in := newGuardFake()
			err := call(Guard(in))
			if !errors.Is(err, ErrNotConfirmed) {
				t.Fatalf("%s con ID non confermato: err = %v, voglio ErrNotConfirmed", name, err)
			}
			if n := in.total(); n != 0 {
				t.Errorf("%s: %d chiamate allo store interno, voglio 0 (senza rete)", name, n)
			}
		})
	}
}

// Un solo ID non confermato in un batch basta a rifiutare l'intero batch.
func TestGuardRefusesBatchWithOneUnconfirmed(t *testing.T) {
	in := newGuardFake()
	in.refs = []msg.MessageRef{{ID: idA}}
	g := Guard(in)
	ctx := context.Background()
	if _, err := g.ListMessages(ctx, msg.Query{}, Page{}); err != nil {
		t.Fatal(err)
	}
	err := g.Archive(ctx, []msg.MessageID{idA, idB})
	if !errors.Is(err, ErrNotConfirmed) || in.count("Archive") != 0 {
		t.Fatalf("batch con idB non confermato: err = %v, Archive interno = %d", err, in.count("Archive"))
	}
	if err := g.Archive(ctx, []msg.MessageID{idA}); err != nil || in.count("Archive") != 1 {
		t.Fatalf("solo idA (confermato): err = %v, Archive interno = %d", err, in.count("Archive"))
	}
}

// Ogni sorgente fresca conferma i propri ID, che poi passano a tutte le
// operazioni.
func TestGuardAllowsIDsFromFreshResults(t *testing.T) {
	ctx := context.Background()
	sources := map[string]func(in *guardFake, g MailStore){
		"ListMessages": func(in *guardFake, g MailStore) {
			in.refs = []msg.MessageRef{{ID: idA}}
			_, _ = g.ListMessages(ctx, msg.Query{}, Page{})
		},
		"Search": func(in *guardFake, g MailStore) {
			in.refs = []msg.MessageRef{{ID: idA}}
			_, _ = g.Search(ctx, msg.Query{})
		},
		"ListConversations": func(in *guardFake, g MailStore) {
			in.convs = []Conversation{{Latest: msg.MessageRef{ID: idA}}}
			_, _ = g.ListConversations(ctx, msg.Query{}, Page{})
		},
		"ListConversationsScanLimit": func(in *guardFake, g MailStore) {
			in.convs = []Conversation{{Latest: msg.MessageRef{ID: idA}}}
			in.setErr(ErrScanLimit)
			_, _ = g.ListConversations(ctx, msg.Query{}, Page{})
			in.setErr(nil)
		},
		"ThreadMessages": func(in *guardFake, g MailStore) {
			in.refs = []msg.MessageRef{{ID: idA}}
			_, _ = g.ThreadMessages(ctx, 7)
		},
		"ConversationMembers": func(in *guardFake, g MailStore) {
			in.members = []msg.MessageID{idA}
			_, _ = g.ConversationMembers(ctx, "INBOX", 7)
		},
		"Trash": func(in *guardFake, g MailStore) {
			// L'ID di partenza è già confermato: in uscita arrivano quelli del cestino.
			in.members = []msg.MessageID{"INBOX:9"}
			_, _ = g.ConversationMembers(ctx, "INBOX", 7)
			in.ids = []msg.MessageID{idA}
			_, _ = g.Trash(ctx, []msg.MessageID{"INBOX:9"})
		},
		"Restore": func(in *guardFake, g MailStore) {
			in.members = []msg.MessageID{"T:9"}
			_, _ = g.ConversationMembers(ctx, "T", 7)
			in.ids = []msg.MessageID{idA}
			_, _ = g.Restore(ctx, []msg.MessageID{"T:9"}, "INBOX")
		},
		"AllMailIDs": func(in *guardFake, g MailStore) {
			in.members = []msg.MessageID{"INBOX:9"}
			_, _ = g.ConversationMembers(ctx, "INBOX", 7)
			in.ids = []msg.MessageID{idA}
			_, _ = g.AllMailIDs(ctx, []msg.MessageID{"INBOX:9"})
		},
		"SaveDraft": func(in *guardFake, g MailStore) {
			in.draft = idA
			_, _ = g.SaveDraft(ctx, []byte("x"), nil)
		},
	}
	for src, confirm := range sources {
		for op, call := range guardIDOps(idA) {
			t.Run(src+"/"+op, func(t *testing.T) {
				in := newGuardFake()
				g := Guard(in)
				confirm(in, g)
				before := in.count(op)
				if err := call(g); errors.Is(err, ErrNotConfirmed) {
					t.Fatalf("%s dopo conferma da %s: rifiutato %v", op, src, err)
				}
				if in.count(op) != before+1 {
					t.Errorf("%s non è arrivato allo store interno", op)
				}
			})
		}
	}
}

// Le chiamate senza ID e le letture passano sempre, anche a set vuoto, e
// propagano gli errori dello store interno.
func TestGuardNeverBlocksReadsOrIDFreeCalls(t *testing.T) {
	in := newGuardFake()
	g := Guard(in)
	ctx := context.Background()
	_, _ = g.ListMessages(ctx, msg.Query{}, Page{})
	_, _ = g.Search(ctx, msg.Query{})
	_, _ = g.ListConversations(ctx, msg.Query{}, Page{})
	_, _ = g.ThreadMessages(ctx, 1)
	_, _ = g.ConversationMembers(ctx, "INBOX", 1)
	_, _ = g.ListMailboxes(ctx)
	_, _ = g.UnreadCounts(ctx)
	_ = g.Send(ctx, msg.Outgoing{})
	_ = g.Connect(ctx)
	_, _ = g.SaveDraft(ctx, []byte("x"), nil)
	// Batch vuoti: niente da confermare, passano.
	_ = g.Archive(ctx, nil)
	_ = g.DeleteDraft(ctx, []msg.MessageID{})
	for _, n := range []string{"ListMessages", "Search", "ListConversations", "ThreadMessages",
		"ConversationMembers", "ListMailboxes", "UnreadCounts", "Send", "Connect", "SaveDraft", "Archive", "DeleteDraft"} {
		if in.count(n) != 1 {
			t.Errorf("%s: chiamate interne = %d, voglio 1", n, in.count(n))
		}
	}
	boom := errors.New("rete giù")
	in.setErr(boom)
	if _, err := g.ListMessages(ctx, msg.Query{}, Page{}); !errors.Is(err, boom) {
		t.Errorf("l'errore dello store interno va propagato, ho %v", err)
	}
}

// Un errore di lettura non conferma nulla (solo ErrScanLimit porta righe valide).
func TestGuardErrorDoesNotConfirm(t *testing.T) {
	in := newGuardFake()
	in.refs = []msg.MessageRef{{ID: idA}}
	in.setErr(errors.New("rete giù"))
	g := Guard(in)
	_, _ = g.ListMessages(context.Background(), msg.Query{}, Page{})
	if err := g.Archive(context.Background(), []msg.MessageID{idA}); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("ID da lettura fallita: err = %v, voglio ErrNotConfirmed", err)
	}
}

// R4: gli ID visti solo dalle istantanee non sono mai confermati.
func TestGuardSnapshotIDsNeverConfirmed(t *testing.T) {
	in := &guardSnapFake{
		guardFake: newGuardFake(),
		conv:      ConversationsSnapshot{Rows: []Conversation{{Latest: msg.MessageRef{ID: idA}}}},
		thread:    ThreadSnapshot{Refs: []msg.MessageRef{{ID: idB}}},
		body:      &msg.Message{Ref: msg.MessageRef{ID: idC}},
	}
	g := Guard(in)
	sn, ok := g.(Snapshotter)
	if !ok {
		t.Fatal("la guardia deve implementare Snapshotter")
	}
	if c, ok := sn.CachedConversations(""); !ok || len(c.Rows) != 1 {
		t.Fatalf("CachedConversations non inoltrata: %+v %v", c, ok)
	}
	sn.CachedThread(1)
	sn.CachedMessage(idC)
	for _, id := range []msg.MessageID{idA, idB, idC} {
		for name, call := range guardIDOps(id) {
			if err := call(g); !errors.Is(err, ErrNotConfirmed) {
				t.Errorf("%s su %s visto solo da istantanea: err = %v, voglio ErrNotConfirmed", name, id, err)
			}
		}
	}
	if n := in.total(); n != 0 {
		t.Errorf("%d chiamate interne, voglio 0", n)
	}
}

// Gli eventi del Watch confermano IDs e Refs prima di arrivare al consumer;
// il canale rilanciato si chiude con quello interno.
func TestGuardWatchConfirmsNewMail(t *testing.T) {
	in := newGuardFake()
	g := Guard(in)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := g.Watch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	in.changes <- Change{Kind: ChangeNewMail, IDs: []msg.MessageID{idA}, Refs: []msg.MessageRef{{ID: idB}}}
	select {
	case c := <-ch:
		if len(c.IDs) != 1 || c.IDs[0] != idA || len(c.Refs) != 1 {
			t.Fatalf("evento alterato: %+v", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("evento non rilanciato")
	}
	for _, id := range []msg.MessageID{idA, idB} {
		if err := g.Archive(context.Background(), []msg.MessageID{id}); err != nil {
			t.Errorf("Archive %s dopo Watch: %v", id, err)
		}
	}
	// Un ID mai visto resta rifiutato.
	if err := g.Archive(context.Background(), []msg.MessageID{idC}); !errors.Is(err, ErrNotConfirmed) {
		t.Errorf("idC mai visto: err = %v", err)
	}
	close(in.changes)
	select {
	case _, open := <-ch:
		if open {
			t.Fatal("canale ancora aperto con evento inatteso")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("il canale rilanciato non si chiude con quello interno")
	}
}

// Un Connect riuscito apre una sessione nuova: le conferme precedenti cadono.
// Un Connect fallito non le tocca.
func TestGuardConnectResetsConfirmed(t *testing.T) {
	in := newGuardFake()
	in.refs = []msg.MessageRef{{ID: idA}}
	g := Guard(in)
	ctx := context.Background()
	_, _ = g.ListMessages(ctx, msg.Query{}, Page{})

	in.setErr(errors.New("connessione fallita"))
	if err := g.Connect(ctx); err == nil {
		t.Fatal("Connect doveva fallire")
	}
	in.setErr(nil)
	if err := g.Archive(ctx, []msg.MessageID{idA}); err != nil {
		t.Fatalf("Connect fallito non deve azzerare le conferme: %v", err)
	}
	if err := g.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if err := g.Archive(ctx, []msg.MessageID{idA}); !errors.Is(err, ErrNotConfirmed) {
		t.Fatalf("dopo Connect riuscito l'ID va riconfermato: err = %v", err)
	}
}

func TestGuardForwardsSnapshotterAndGmailExt(t *testing.T) {
	want := ConversationsSnapshot{Rows: []Conversation{{Count: 3}}, Limited: true}
	in := &guardSnapFake{guardFake: newGuardFake(), conv: want, gmail: true,
		thread: ThreadSnapshot{Refs: []msg.MessageRef{{ThreadID: 5}}}, body: &msg.Message{}}
	g := Guard(in)
	sn, ok := g.(Snapshotter)
	if !ok {
		t.Fatal("la guardia deve implementare Snapshotter")
	}
	if c, ok := sn.CachedConversations("X"); !ok || !c.Limited || c.Rows[0].Count != 3 {
		t.Errorf("CachedConversations = %+v %v", c, ok)
	}
	if th, ok := sn.CachedThread(5); !ok || th.Refs[0].ThreadID != 5 {
		t.Errorf("CachedThread = %+v %v", th, ok)
	}
	if b, ok := sn.CachedMessage(idA); !ok || b == nil {
		t.Errorf("CachedMessage = %v %v", b, ok)
	}
	ge, ok := g.(interface{ GmailExt() bool })
	if !ok || !ge.GmailExt() {
		t.Error("GmailExt deve seguire lo store interno (true)")
	}
	in.gmail = false
	if ge.GmailExt() {
		t.Error("GmailExt deve seguire lo store interno (false)")
	}

	// Senza le interfacce opzionali sullo store interno: miss e false.
	g2 := Guard(newGuardFake())
	sn2, ok := g2.(Snapshotter)
	if !ok {
		t.Fatal("la guardia deve implementare Snapshotter anche senza Snapshotter interno")
	}
	if _, ok := sn2.CachedConversations(""); ok {
		t.Error("senza Snapshotter interno: miss atteso")
	}
	if _, ok := sn2.CachedThread(1); ok {
		t.Error("senza Snapshotter interno: miss atteso")
	}
	if _, ok := sn2.CachedMessage(idA); ok {
		t.Error("senza Snapshotter interno: miss atteso")
	}
	if g2.(interface{ GmailExt() bool }).GmailExt() {
		t.Error("senza GmailExt interno deve valere false")
	}
}

// Uso concorrente (da eseguire con -race): letture che confermano, scritture
// che controllano e Connect che azzera, in parallelo.
func TestGuardConcurrentUse(t *testing.T) {
	in := newGuardFake()
	in.refs = []msg.MessageRef{{ID: idA}, {ID: idB}}
	g := Guard(in)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = g.ListMessages(ctx, msg.Query{}, Page{})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = g.Archive(ctx, []msg.MessageID{idA, idB})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = g.Connect(ctx)
			}
		}()
	}
	wg.Wait()
}
