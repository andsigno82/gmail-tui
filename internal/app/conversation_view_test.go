package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/spinner"

	"github.com/andsigno82/gmail-tui/internal/config"
	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/inbox"
)

// convViewFixture: una conversazione (thread 7) di quattro messaggi in ordine
// cronologico: anna (letto), mario (NON letto), tu in Inviati (letto), anna
// (letto, il più recente, in INBOX). La riga di lista è l'ultimo messaggio.
func convViewFixture(t *testing.T) *mail.Mock {
	t.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(box string, uid uint32, from string, hours int, unread bool) *msg.Message {
		return &msg.Message{
			Ref: msg.MessageRef{
				ID: msg.NewID(box, uid), From: from, Subject: "piano settimana",
				Date: base.Add(time.Duration(hours) * time.Hour), ThreadID: 7, Unread: unread,
			},
			BodyText: fmt.Sprintf("corpo di %s n%d", from, uid),
		}
	}
	return &mail.Mock{Msgs: []*msg.Message{
		mk("INBOX", 1, "anna@x.dev", 0, false),
		mk("INBOX", 2, "mario@x.dev", 1, true),
		mk("[Gmail]/Sent", 3, "tu@x.dev", 2, false),
		mk("INBOX", 4, "anna@x.dev", 3, false),
	}}
}

// unreadInFixture conta i messaggi del Mock ancora non letti.
func unreadInFixture(store *mail.Mock) int {
	n := 0
	for _, mm := range store.Msgs {
		if mm.Ref.Unread {
			n++
		}
	}
	return n
}

// Piano 8, Task 10: aprire una conversazione mostra tutti i suoi messaggi (in
// Inviati incluso) con i letti compressi, e segna letti i non letti.
func TestAperturaConversazioneMostraTuttiEMarcaLetti(t *testing.T) {
	store := convViewFixture(t)
	m := pagedModel(t, store)
	row := m.refs[0]

	m, cmd := step(m, inbox.SelectedMsg{Ref: row})
	var follow []tea.Cmd
	for _, ms := range runCmd(cmd) {
		var c tea.Cmd
		m, c = step(m, ms)
		follow = append(follow, c)
	}
	view := m.thread.View()
	if !strings.Contains(view, "▸ tu@x.dev") {
		t.Errorf("la risposta inviata deve comparire compressa nella conversazione:\n%s", view)
	}
	for _, c := range follow {
		runCmd(c)
	}
	if n := unreadInFixture(store); n != 0 {
		t.Errorf("aprire la conversazione deve segnare letti tutti i non letti: ne restano %d", n)
	}
}

// Piano 8, Task 10: nell'anteprima a due colonne il segna-come-letto dopo 2 s
// vale per tutti i non letti della conversazione, non solo per il più recente.
func TestAnteprimaConversazioneSegnaLettiTuttiDopoDueSecondi(t *testing.T) {
	store := convViewFixture(t)
	m := pagedModel(t, store)
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadDelay)
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = feedPreview(t, m)
	if !m.previewReady() {
		t.Fatalf("anteprima non pronta")
	}
	m, cmd := step(m, previewReadDue{tok: m.prevTok, id: m.prevRef.ID})
	runCmd(cmd)
	if n := unreadInFixture(store); n != 0 {
		t.Errorf("dopo 2 s la conversazione in anteprima deve essere tutta letta: ne restano %d", n)
	}
}

// Piano 8, Task 10 (test mancante): il clic sul titolo di un messaggio compresso
// nell'anteprima a due colonne lo espande, come nella vista a schermo intero.
// La prima riga del thread è il titolo di anna (letta, compressa) a Y=7 (scheda
// di 6 righe più la riga vuota, Piano 15, Task 14); la colonna dell'anteprima inizia dopo la lista.
func TestClickSuTitoloNellAnteprimaEspande(t *testing.T) {
	store := convViewFixture(t)
	m := pagedModel(t, store)
	m.SetPrefs(config.LayoutSplit, config.PreviewMarkReadDelay)
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	m = feedPreview(t, m)
	if !m.previewReady() {
		t.Fatalf("anteprima non pronta")
	}
	if hasBodyLine(m.thread.View(), "corpo di anna@x.dev n1") {
		t.Fatalf("prima del clic il primo messaggio deve essere compresso:\n%s", m.thread.View())
	}
	m = deliverAll(t, m, func() tea.Cmd { _, c := step(m, tea.MouseClickMsg{X: 100, Y: 7, Button: tea.MouseLeft}); return c }())
	if !hasBodyLine(m.thread.View(), "corpo di anna@x.dev n1") {
		t.Errorf("il clic sul titolo nell'anteprima deve espandere il messaggio:\n%s", m.thread.View())
	}
}

// deliverAll esegue la cmd e ne applica i risultati, due livelli di profondità:
// il clic emette BodyRequested (Piano 8, Task 13), che produce il fetch del corpo.
// Spinner e nil sono scartati, come in applyAll.
func deliverAll(t *testing.T, m Model, c tea.Cmd) Model {
	t.Helper()
	for _, ms := range runCmd(c) {
		switch ms.(type) {
		case nil, spinner.TickMsg:
			continue
		}
		var next tea.Cmd
		m, next = step(m, ms)
		for _, ms2 := range runCmd(next) {
			switch ms2.(type) {
			case nil, spinner.TickMsg:
				continue
			}
			m, _ = step(m, ms2)
		}
	}
	return m
}

// hasBodyLine dice se una riga della vista è esattamente s (corpo espanso).
func hasBodyLine(view, s string) bool {
	for _, ln := range strings.Split(view, "\n") {
		if strings.TrimSpace(ln) == s {
			return true
		}
	}
	return false
}

// fetchCounter conta le FetchMessage: il caricamento lazy (Piano 8, Task 13) scarica
// il corpo solo degli espansi all'apertura.
type fetchCounter struct {
	mail.MailStore
	fetched []msg.MessageID
}

func (f *fetchCounter) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	f.fetched = append(f.fetched, id)
	return f.MailStore.FetchMessage(ctx, id)
}

// Piano 8, Task 13: aprire una conversazione scarica il corpo dei soli messaggi
// espansi (ultimo e non letti), non di tutti.
func TestAperturaScaricaSoloICorpiEspansi(t *testing.T) {
	store := convViewFixture(t)
	fc := &fetchCounter{MailStore: store}
	m := pagedModel(t, fc)
	row := m.refs[0]
	fc.fetched = nil

	_, cmd := step(m, inbox.SelectedMsg{Ref: row})
	for _, ms := range runCmd(cmd) {
		if _, ok := ms.(msgLoaded); ok {
			step(m, ms)
		}
	}
	want := map[msg.MessageID]bool{msg.NewID("INBOX", 2): true, msg.NewID("INBOX", 4): true}
	if len(fc.fetched) != len(want) {
		t.Fatalf("corpi scaricati = %v, voglio solo gli espansi (ultimo e non letto)", fc.fetched)
	}
	for _, id := range fc.fetched {
		if !want[id] {
			t.Errorf("scaricato il corpo di %s, che è compresso", id)
		}
	}
}

// Piano 8, Task 13: un corpo richiesto per una generazione superata del thread non
// si installa nel thread riaperto (token anti-stantio). Dopo il clic sul compresso
// la vista deve ancora mostrare il caricamento: il corpo stantio non c'entra.
func TestCorpoStantioNonSiInstallaNelThreadRiaperto(t *testing.T) {
	store := convViewFixture(t)
	m := pagedModel(t, store)
	row := m.refs[0]

	m, cmd := step(m, inbox.SelectedMsg{Ref: row})
	m = deliverAll(t, m, cmd)
	oldGen := m.threadGen
	m, cmd = step(m, inbox.SelectedMsg{Ref: row})
	m = deliverAll(t, m, cmd)
	if m.threadGen == oldGen {
		t.Fatalf("riaprire la conversazione deve creare una nuova generazione del thread")
	}

	stale := store.Msgs[0] // INBOX:1, compressa nel thread riaperto
	m, _ = step(m, bodyLoaded{gen: oldGen, anchor: row.ID, id: stale.Ref.ID, m: stale})

	m2, click := step(m, tea.MouseClickMsg{X: 100, Y: 7, Button: tea.MouseLeft})
	if !strings.Contains(m2.thread.View(), "corpo in caricamento") {
		t.Fatalf("il corpo stantio è stato installato nel thread riaperto:\n%s", m2.thread.View())
	}
	m2 = deliverAll(t, m2, click)
	if !hasBodyLine(m2.thread.View(), "corpo di anna@x.dev n1") {
		t.Errorf("il clic deve caricare il corpo corrente:\n%s", m2.thread.View())
	}
}

// Piano 8, review (punto 3): una lista limitata dal tetto di scansione non è esaurita
// (niente altre pagine per il solo fatto che la pagina è corta) e la barra di stato
// lo dice, suggerendo la ricerca per andare più indietro.
func TestListaLimitataNonEsauritaEBarraLoDice(t *testing.T) {
	m := pagedModel(t, convViewFixture(t))
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	var convs []mail.Conversation
	for i := 0; i < 3; i++ {
		convs = append(convs, mail.SingleConversation(msg.MessageRef{ID: msg.NewID("INBOX", uint32(100+i)), From: "a@x.dev", Subject: "s", Date: base.Add(time.Duration(i) * time.Hour)}))
	}
	m, _ = step(m, msgsLoaded{convs: convs, refs: latestRefs(convs), limit: pageSize, limited: true, mailbox: "", gen: m.listGen})
	if m.exhausted {
		t.Errorf("lista limitata marcata come esaurita: la pagina corta non vuol dire fine della cartella")
	}
	if !strings.Contains(m.statusText(), "limitata") {
		t.Errorf("status = %q, voglio l'avviso di lista limitata", m.statusText())
	}
}

// allMailBox è la cartella Tutti i messaggi di Gmail (namespace diverso da INBOX).
const allMailBox = "[Gmail]/All Mail"

// gmailFixture: la conversazione di convViewFixture con gli ID Gmail (1000+uid).
func gmailFixture(t *testing.T) *mail.Mock {
	t.Helper()
	store := convViewFixture(t)
	for _, mm := range store.Msgs {
		_, uid, _ := mm.Ref.ID.Split()
		mm.Ref.GmailID = 1000 + uint64(uid)
	}
	return store
}

// gmailNS simula Gmail con due namespace (Piano 8, review punto 2 e 4): ThreadMessages
// restituisce gli ID di Tutti i messaggi (stesso uid, stesso GmailID); FetchMessage li
// risolve; Archive registra gli ID ricevuti.
type gmailNS struct {
	*mail.Mock
	archived [][]msg.MessageID
}

func (g *gmailNS) ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error) {
	refs, err := g.Mock.ThreadMessages(ctx, threadID)
	if err != nil {
		return nil, err
	}
	out := make([]msg.MessageRef, 0, len(refs))
	for _, r := range refs {
		_, uid, _ := r.ID.Split()
		r.ID = msg.NewID(allMailBox, uid)
		out = append(out, r)
	}
	return out, nil
}

func (g *gmailNS) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	box, uid, err := id.Split()
	if err != nil {
		return nil, err
	}
	if box != allMailBox {
		return g.Mock.FetchMessage(ctx, id)
	}
	for _, mm := range g.Msgs {
		if _, u, _ := mm.Ref.ID.Split(); u == uid {
			cp, err := g.Mock.FetchMessage(ctx, mm.Ref.ID)
			if err != nil {
				return nil, err
			}
			cp.Ref.ID = id
			return cp, nil
		}
	}
	return nil, fmt.Errorf("messaggio %s non trovato", id)
}

func (g *gmailNS) Archive(ctx context.Context, ids []msg.MessageID) error {
	g.archived = append(g.archived, append([]msg.MessageID(nil), ids...))
	return g.Mock.Archive(ctx, ids)
}

// Piano 8, review punto 2: la conversazione aperta ha un messaggio per GmailID, anche
// quando ThreadMessages (Tutti i messaggi) e la riga (INBOX) usano ID diversi.
func TestConversazioneSenzaDoppioniPerGmailID(t *testing.T) {
	g := &gmailNS{Mock: gmailFixture(t)}
	m := pagedModel(t, g)
	row := m.refs[0]
	m, cmd := step(m, inbox.SelectedMsg{Ref: row})
	m = deliverAll(t, m, cmd)

	refs := m.thread.Refs()
	if len(refs) != 4 {
		t.Fatalf("messaggi nella conversazione = %d, voglio 4 (un doppione per GmailID)", len(refs))
	}
	seen := map[uint64]bool{}
	for _, r := range refs {
		if seen[r.GmailID] {
			t.Errorf("GmailID %d compare due volte: %v", r.GmailID, refs)
		}
		seen[r.GmailID] = true
	}
}

// Piano 8, review punto 4: archivia dal thread agisce sulla conversazione della RIGA
// (INBOX), non su quella del messaggio selezionato (qui un messaggio più vecchio).
func TestArchiviaDalThreadAgiscesLaConversazioneDellaRiga(t *testing.T) {
	g := &gmailNS{Mock: gmailFixture(t)}
	m := pagedModel(t, g)
	row := m.refs[0]
	m, cmd := step(m, inbox.SelectedMsg{Ref: row})
	m = deliverAll(t, m, cmd)
	m, _ = step(m, pressKey("k")) // seleziona il messaggio precedente (Inviati)

	m, cmd = step(m, pressKey("a"))
	deliverAll(t, m, cmd)
	idU1, idU2, idU4 := msg.NewID("INBOX", 1), msg.NewID("INBOX", 2), msg.NewID("INBOX", 4)
	if len(g.archived) != 1 || !sameIDs(g.archived[0], idU1, idU2, idU4) {
		t.Errorf("archiviati = %v, voglio i membri INBOX della conversazione (1,2,4)", g.archived)
	}
}
