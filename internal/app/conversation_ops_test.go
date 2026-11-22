package app

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// convOpsFixture: thread 7 con tre messaggi in INBOX (UID 1 e 3 non letti, 3 il
// più recente), un messaggio singolo INBOX:4, e un messaggio dello stesso thread
// in Inviati non letto (fuori dalla INBOX: non va toccato dalle op in INBOX).
func convOpsFixture(t *testing.T) *mail.Mock {
	t.Helper()
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(box string, uid uint32, thread uint64, hours int, unread bool) *msg.Message {
		return &msg.Message{Ref: msg.MessageRef{
			ID: msg.NewID(box, uid), From: "anna@x.dev", Subject: "discussione",
			Date: base.Add(time.Duration(hours) * time.Hour), ThreadID: thread, Unread: unread,
		}}
	}
	return &mail.Mock{Msgs: []*msg.Message{
		mk("INBOX", 1, 7, 1, true),
		mk("INBOX", 2, 7, 2, false),
		mk("INBOX", 3, 7, 3, true),
		mk("INBOX", 4, 0, 0, false),
		mk("[Gmail]/Sent", 5, 7, 1, true),
	}}
}

// opRec registra le op di mutazione che arrivano allo store. ConversationMembers
// risponde dal Mock (o con override, per il test di guardia).
type opRec struct {
	mail.MailStore
	mock     *mail.Mock
	override []msg.MessageID
	archived [][]msg.MessageID
	trashed  [][]msg.MessageID
	starred  [][]msg.MessageID
	read     [][]msg.MessageID
	labeled  [][]msg.MessageID
}

func (r *opRec) ConversationMembers(_ context.Context, box string, thread uint64) ([]msg.MessageID, error) {
	if r.override != nil {
		return r.override, nil
	}
	var out []msg.MessageID
	for _, mm := range r.mock.Msgs {
		if b, _, err := mm.Ref.ID.Split(); err == nil && b == box && mm.Ref.ThreadID == thread {
			out = append(out, mm.Ref.ID)
		}
	}
	return out, nil
}

func (r *opRec) Archive(ctx context.Context, ids []msg.MessageID) error {
	r.archived = append(r.archived, append([]msg.MessageID(nil), ids...))
	return r.MailStore.Archive(ctx, ids)
}

func (r *opRec) Trash(ctx context.Context, ids []msg.MessageID) error {
	r.trashed = append(r.trashed, append([]msg.MessageID(nil), ids...))
	return r.MailStore.Trash(ctx, ids)
}

func (r *opRec) Star(ctx context.Context, ids []msg.MessageID, on bool) error {
	r.starred = append(r.starred, append([]msg.MessageID(nil), ids...))
	return r.MailStore.Star(ctx, ids, on)
}

func (r *opRec) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	r.read = append(r.read, append([]msg.MessageID(nil), ids...))
	return r.MailStore.MarkRead(ctx, ids, read)
}

func (r *opRec) ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error {
	r.labeled = append(r.labeled, append([]msg.MessageID(nil), ids...))
	return r.MailStore.ApplyLabel(ctx, ids, label, on)
}

// sameIDs confronta due insiemi di ID senza ordine.
func sameIDs(got []msg.MessageID, want ...msg.MessageID) bool {
	if len(got) != len(want) {
		return false
	}
	seen := make(map[msg.MessageID]int, len(got))
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		if seen[id] == 0 {
			return false
		}
		seen[id]--
	}
	return true
}

var (
	idU1 = msg.NewID("INBOX", 1)
	idU2 = msg.NewID("INBOX", 2)
	idU3 = msg.NewID("INBOX", 3)
)

// Piano 8, Task 9b: `#` sulla riga sposta nel cestino TUTTA la conversazione
// nella cartella della riga, non solo il messaggio più recente.
func TestTrashAgisceSullaConversazioneIntera(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := pagedModel(t, rec)
	m, _ = step(m, pressKey("#"))
	m, cmd := step(m, pressKey("s"))
	applyAll(t, m, cmd)
	if len(rec.trashed) != 1 || !sameIDs(rec.trashed[0], idU1, idU2, idU3) {
		t.Errorf("Trash ricevuto = %v, voglio i tre messaggi del thread in INBOX", rec.trashed)
	}
}

// Piano 8, Task 9b: `a` archivia tutta la conversazione nella INBOX.
func TestArchiviaAgisceSullaConversazioneIntera(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := pagedModel(t, rec)
	m, cmd := step(m, pressKey("a"))
	applyAll(t, m, cmd)
	if len(rec.archived) != 1 || !sameIDs(rec.archived[0], idU1, idU2, idU3) {
		t.Errorf("Archive ricevuto = %v, voglio i tre messaggi del thread in INBOX", rec.archived)
	}
}

// Piano 8, Task 9b: `l` applica la label a tutta la conversazione nella cartella.
func TestEtichettaAgiscSullaConversazioneIntera(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := pagedModel(t, rec)
	m, _ = step(m, pressKey("l"))
	m.label.SetValue("lavoro")
	m, cmd := step(m, pressKey("enter"))
	applyAll(t, m, cmd)
	if len(rec.labeled) != 1 || !sameIDs(rec.labeled[0], idU1, idU2, idU3) {
		t.Errorf("ApplyLabel ricevuto = %v, voglio i tre messaggi del thread in INBOX", rec.labeled)
	}
}

// Piano 8, Task 9b: `s` (stella) resta sul solo messaggio più recente, come Gmail.
func TestStellaRestaSulMessaggioPiuRecente(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := pagedModel(t, rec)
	m, cmd := step(m, pressKey("s"))
	applyAll(t, m, cmd)
	if len(rec.starred) != 1 || !sameIDs(rec.starred[0], idU3) {
		t.Errorf("Star ricevuto = %v, voglio solo INBOX:3", rec.starred)
	}
}

// Piano 8, Task 9b: `v` segna letti tutti i membri nella cartella, e il grassetto
// della riga si spegne subito in base allo stato dell'intera conversazione: il
// messaggio in Inviati è non letto, quindi la conversazione resta in grassetto.
func TestVAggiornaSubitoIlGrassettoDellaConversazione(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := pagedModel(t, rec)
	m, cmd := step(m, pressKey("v"))
	m = applyAll(t, m, cmd)
	if len(rec.read) != 1 || !sameIDs(rec.read[0], idU1, idU2, idU3) {
		t.Errorf("MarkRead ricevuto = %v, voglio i tre messaggi del thread in INBOX", rec.read)
	}
	first := strings.Split(m.inbox.View(), "\n")[0]
	if !strings.Contains(first, "●") {
		t.Errorf("riga = %q: il messaggio in Inviati è non letto, la conversazione deve restare in grassetto", first)
	}
}

// Piano 8, Task 9b, sicurezza dati: se lo store restituisce per la conversazione un
// ID di un'altra mailbox (es. Tutti i messaggi), archivia e cestina rifiutano senza
// chiamare lo store: un expunge lì cancellerebbe il messaggio.
func TestGuardiaRifiutaIdDiAltraMailbox(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	rec.override = []msg.MessageID{idU1, msg.NewID("[Gmail]/Tutti i messaggi", 9)}
	m := pagedModel(t, rec)

	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if len(rec.archived) != 0 {
		t.Errorf("Archive chiamato con ID fuori dalla INBOX: %v", rec.archived)
	}
	if !toastIs(m, toast.Error) {
		t.Errorf("nessun toast d'errore: il rifiuto deve essere visibile")
	}

	m, _ = step(m, pressKey("#"))
	m, cmd = step(m, pressKey("s"))
	applyAll(t, m, cmd)
	if len(rec.trashed) != 0 {
		t.Errorf("Trash chiamato con ID fuori dalla INBOX: %v", rec.trashed)
	}
}

var _ tea.Msg = nil
