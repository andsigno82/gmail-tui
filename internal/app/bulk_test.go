package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// flagRec registra anche il valore dei flag passati a Star e MarkRead (opRec
// registra solo gli ID). Piano 10, Task 3.
type flagRec struct {
	opRec
	starOn   []bool
	readFlag []bool
}

func (r *flagRec) Star(ctx context.Context, ids []msg.MessageID, on bool) error {
	r.starOn = append(r.starOn, on)
	return r.opRec.Star(ctx, ids, on)
}

func (r *flagRec) MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error {
	r.readFlag = append(r.readFlag, read)
	return r.opRec.MarkRead(ctx, ids, read)
}

// newFlagRec costruisce lo store registrato sul mock dato.
func newFlagRec(mock *mail.Mock) *flagRec {
	return &flagRec{opRec: opRec{MailStore: mock, mock: mock}}
}

// selectBothRows: X sulla INBOX di convOpsFixture (conversazione 7 + INBOX:4).
func selectBothRows(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = step(m, pressKey("X"))
	if len(m.sel) != 2 {
		t.Fatalf("precondizione: selezione = %v, voglio 2 righe", m.sel)
	}
	return m
}

// Piano 10, Task 3: `a` con selezione archivia tutte le conversazioni marcate,
// con la loro conversazione intera nella INBOX, e la selezione si azzera (D10-S3).
func TestBulkArchiveArchivesAllSelected(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := selectBothRows(t, pagedModel(t, rec))
	m, cmd := step(m, pressKey("a"))
	if len(m.sel) != 0 {
		t.Errorf("dopo a: selezione = %v, voglio vuota", m.sel)
	}
	applyAll(t, m, cmd)
	if len(rec.archived) != 1 || !sameIDs(rec.archived[0], idU1, idU2, idU3, msg.NewID("INBOX", 4)) {
		t.Errorf("Archive ricevuto = %v, voglio le conversazioni selezionate in INBOX", rec.archived)
	}
}

// Piano 10, Task 3: `#` con selezione apre la conferma sulle conversazioni marcate;
// `s` le sposta tutte nel cestino.
func TestBulkTrashAllSelected(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := selectBothRows(t, pagedModel(t, rec))
	m, _ = step(m, pressKey("#"))
	if !m.trashOpen || len(m.trashSel) != 2 {
		t.Fatalf("conferma cestino: aperta=%v, selezionate in conferma=%d, voglio 2", m.trashOpen, len(m.trashSel))
	}
	m, cmd := step(m, pressKey("s"))
	applyAll(t, m, cmd)
	// Piano 10, Task 6 (D10-C2): un Trash per conversazione, così ogni ritorno ha i
	// suoi ID nel cestino e le sue label; l'unione copre le conversazioni selezionate.
	var all []msg.MessageID
	for _, g := range rec.trashed {
		all = append(all, g...)
	}
	if len(rec.trashed) != 2 || !sameIDs(all, idU1, idU2, idU3, msg.NewID("INBOX", 4)) {
		t.Errorf("Trash ricevuto = %v, voglio le conversazioni selezionate, una per chiamata", rec.trashed)
	}
}

// Piano 10, Task 3: nella cartella Cestino il cestino sulla selezione è rifiutato
// con "già nel cestino": nessuna conferma e nessuna op.
func TestBulkTrashRefusedInTrashFolder(t *testing.T) {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mock := &mail.Mock{
		Msgs: []*msg.Message{{Ref: msg.MessageRef{ID: msg.NewID("[Gmail]/Trash", 1), From: "anna@x.dev", Subject: "nel cestino", Date: base}}},
		Mailboxes: []mail.Mailbox{
			{Name: "INBOX", Role: "inbox"},
			{Name: "[Gmail]/Trash", Role: "trash"},
		},
	}
	rec := newFlagRec(mock)
	m := pagedModel(t, rec)
	m, cmd := step(m, pressKey("m"))
	m = applyAll(t, m, cmd)
	m.folderList.Select(1) // [Gmail]/Trash
	m, cmd = step(m, pressKey("enter"))
	m = applyAll(t, m, cmd)
	if m.folder.Name != "[Gmail]/Trash" {
		t.Fatalf("cartella = %q: il cambio non è avvenuto", m.folder.Name)
	}
	m, _ = step(m, pressKey("X"))
	if len(m.sel) == 0 {
		t.Fatalf("precondizione: nessuna riga selezionata")
	}
	m, _ = step(m, pressKey("#"))
	if m.trashOpen {
		t.Errorf("nel cestino la conferma non deve aprirsi")
	}
	if !strings.Contains(toastText(m), "già nel cestino") {
		t.Errorf("toast = %q, voglio \"già nel cestino\"", toastText(m))
	}
	if len(rec.trashed) != 0 {
		t.Errorf("Trash chiamato nel cestino: %v", rec.trashed)
	}
}

// Piano 10, Task 3 (D10-V1): `s` con selezione. Se almeno una conversazione è
// senza stella, la stella va a tutte (on=true); altrimenti la toglie a tutte.
func TestBulkStarRule(t *testing.T) {
	cases := []struct {
		name    string
		starred bool // stella su entrambi i Latest (INBOX:3 e INBOX:4)
		wantOn  bool
	}{
		{"nessuna stellata: tutte stellate", false, true},
		{"tutte stellate: stella tolta a tutte", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := convOpsFixture(t)
			for _, mm := range mock.Msgs {
				if mm.Ref.ID == msg.NewID("INBOX", 3) || mm.Ref.ID == msg.NewID("INBOX", 4) {
					mm.Ref.Starred = c.starred
				}
			}
			rec := newFlagRec(mock)
			m := selectBothRows(t, pagedModel(t, rec))
			m, cmd := step(m, pressKey("s"))
			applyAll(t, m, cmd)
			if len(rec.starOn) != 1 || rec.starOn[0] != c.wantOn {
				t.Fatalf("Star on = %v, voglio una sola chiamata con on=%v", rec.starOn, c.wantOn)
			}
			if len(rec.starred) != 1 || !sameIDs(rec.starred[0], msg.NewID("INBOX", 3), msg.NewID("INBOX", 4)) {
				t.Errorf("Star su %v, voglio il messaggio più recente di ciascuna riga selezionata", rec.starred)
			}
		})
	}
}

// Piano 10, Task 3 (D10-V1): `v` con selezione. Se almeno una conversazione è
// non letta, tutte vanno lette (read=true); altrimenti tutte non lette.
func TestBulkReadRule(t *testing.T) {
	cases := []struct {
		name    string
		allRead bool // true: nessun messaggio non letto nella fixture
		wantRd  bool
	}{
		{"una non letta: tutte lette", false, true},
		{"tutte lette: tutte non lette", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mock := convOpsFixture(t)
			for _, mm := range mock.Msgs {
				if c.allRead {
					mm.Ref.Unread = false
				}
			}
			rec := newFlagRec(mock)
			m := selectBothRows(t, pagedModel(t, rec))
			m, cmd := step(m, pressKey("v"))
			applyAll(t, m, cmd)
			if len(rec.readFlag) != 1 || rec.readFlag[0] != c.wantRd {
				t.Errorf("MarkRead read = %v, voglio una sola chiamata con read=%v", rec.readFlag, c.wantRd)
			}
			idSent := msg.NewID("[Gmail]/Sent", 5)
			if len(rec.read) != 1 || !sameIDs(rec.read[0], idU1, idU2, idU3, idSent, msg.NewID("INBOX", 4)) {
				t.Errorf("MarkRead su %v, voglio entrambe le conversazioni, Inviati incluso", rec.read)
			}
		})
	}
}

// Piano 10, Task 3: il risultato di un'op sulla selezione con listGen superato non
// tocca la lista corrente: nessun refresh (il toast resta).
func TestBulkOpStaleResultDropped(t *testing.T) {
	rec := &opRec{MailStore: convOpsFixture(t)}
	rec.mock = rec.MailStore.(*mail.Mock)
	m := selectBothRows(t, pagedModel(t, rec))
	m, cmd := step(m, pressKey("a"))
	genBefore := m.listGen
	m.listGen++ // cambio di lista mentre l'op è in volo
	m = applyAll(t, m, cmd)
	if m.listGen == genBefore {
		t.Fatalf("precondizione: listGen non cambiato")
	}
	if len(rec.archived) != 1 {
		t.Fatalf("Archive non eseguita: %v", rec.archived)
	}
	if m.refreshing {
		t.Errorf("esito stante ha avviato un refresh sulla lista corrente")
	}
}
