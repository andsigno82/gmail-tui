package imap

// Test delle mutazioni (piano 2, Task 5): MarkRead, Star, Archive,
// ApplyLabel + coerenza cache dei flag dopo le mutazioni. Riusa la harness
// di imap_test.go e gli helper di fetch_test.go (connectStoreCache).

import (
	"strings"
	"testing"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
)

// TestMarkReadMarksSeen: MarkRead(true) su due unread → re-list: non più
// tra le unread, Unread=false nel ref (piano: "unread→read, re-list
// conferma seen").
func TestMarkReadMarksSeen(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	if err := store.MarkRead(t.Context(), []msg.MessageID{
		msg.NewID("INBOX", 2), // Second (unseen)
		msg.NewID("INBOX", 5), // Fifth (unseen)
	}, true); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	unread, err := store.ListMessages(t.Context(), msg.Query{Unread: true}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages unread: %v", err)
	}
	if got := subjects(unread); strings.Join(got, ",") != "Fourth" {
		t.Errorf("unread dopo MarkRead = %v, voglio solo [Fourth]", got)
	}

	all, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	for _, r := range all {
		if (r.Subject == "Second" || r.Subject == "Fifth") && r.Unread {
			t.Errorf("%q ancora Unread=true dopo MarkRead", r.Subject)
		}
	}
}

// TestMarkReadUnread: MarkRead(false) rimuove \Seen da una mail letta →
// re-list la mostra tra le unread.
func TestMarkReadUnread(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	if err := store.MarkRead(t.Context(), []msg.MessageID{msg.NewID("INBOX", 1)}, false); err != nil {
		t.Fatalf("MarkRead(false): %v", err)
	}

	unread, err := store.ListMessages(t.Context(), msg.Query{Unread: true}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages unread: %v", err)
	}
	found := false
	for _, r := range unread {
		if r.Subject == "First" {
			found = true
		}
	}
	if !found {
		t.Errorf("First non è tra le unread (%v) dopo MarkRead(false)", subjects(unread))
	}
}

// TestMarkReadBatchesAcrossMailboxes: id di mailbox diverse in una sola
// chiamata (batching per mailbox) → entrambe marcate lette.
func TestMarkReadBatchesAcrossMailboxes(t *testing.T) {
	ts := startTestServer(t)
	ts.createMailbox("Work")
	uidInbox := ts.seedMail("In <i@example.org>", "In inbox", "corpo", false, false)
	uidWork := ts.seedInMailbox("Work", "W <w@example.org>", "In work", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.MarkRead(t.Context(), []msg.MessageID{
		msg.NewID("INBOX", uidInbox),
		msg.NewID("Work", uidWork),
	}, true); err != nil {
		t.Fatalf("MarkRead multi-mailbox: %v", err)
	}

	for _, tc := range []struct{ label, subject string }{
		{"", "In inbox"},
		{"Work", "In work"},
	} {
		refs, err := store.ListMessages(t.Context(), msg.Query{Label: tc.label}, mail.Page{})
		if err != nil {
			t.Fatalf("ListMessages %q: %v", tc.label, err)
		}
		if len(refs) != 1 || refs[0].Subject != tc.subject || refs[0].Unread {
			t.Errorf("mailbox %q: refs = %+v, voglio %q letta", tc.label, refs, tc.subject)
		}
	}
}

// TestStarOnOff: Star(true) → re-list starred; Star(false) → non più.
func TestStarOnOff(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)
	id := msg.NewID("INBOX", 5) // Fifth, non flagged

	if err := store.Star(t.Context(), []msg.MessageID{id}, true); err != nil {
		t.Fatalf("Star(true): %v", err)
	}
	starred, err := store.ListMessages(t.Context(), msg.Query{Starred: true}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages starred: %v", err)
	}
	// seedFive ha già Third e Fourth starred: dopo Star(true) anche Fifth.
	if got := subjects(starred); strings.Join(got, ",") != "Fifth,Fourth,Third" {
		t.Errorf("starred dopo Star(true) = %v, voglio [Fifth Fourth Third]", got)
	}

	if err := store.Star(t.Context(), []msg.MessageID{id}, false); err != nil {
		t.Fatalf("Star(false): %v", err)
	}
	starred, err = store.ListMessages(t.Context(), msg.Query{Starred: true}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages starred: %v", err)
	}
	if contains(subjects(starred), "Fifth") {
		t.Errorf("Fifth ancora starred dopo Star(false): %v", subjects(starred))
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestArchiveRemovesFromMailbox: Archive su due id di mailbox diverse
// (batching) → re-list: le mail non sono più nelle rispettive mailbox.
// Semantica Gmail: expunge dalla cartella = rimozione label = archiviazione
// (nel memserver l'expunge rimuove fisicamente: il test verifica il
// comportamento IMAP).
func TestArchiveRemovesFromMailbox(t *testing.T) {
	ts := startTestServer(t)
	ts.createMailbox("Work")
	uidInbox := ts.seedMail("In <i@example.org>", "To archive", "corpo", false, false)
	uidWork := ts.seedInMailbox("Work", "W <w@example.org>", "Work to archive", "corpo", false, false)
	ts.seedMail("Keep <k@example.org>", "Keep me", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.Archive(t.Context(), []msg.MessageID{
		msg.NewID("INBOX", uidInbox),
		msg.NewID("Work", uidWork),
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	inbox, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages INBOX: %v", err)
	}
	if got := subjects(inbox); strings.Join(got, ",") != "Keep me" {
		t.Errorf("INBOX dopo Archive = %v, voglio solo [Keep me] (expunge selettivo)", got)
	}
	work, err := store.ListMessages(t.Context(), msg.Query{Label: "Work"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Work: %v", err)
	}
	if len(work) != 0 {
		t.Errorf("Work dopo Archive = %v, voglio vuota", subjects(work))
	}
}

// TestApplyLabelOnCreatesMailboxAndCopies: ApplyLabel(true) con label che
// NON esiste → la cartella viene creata (Gmail: creare la cartella crea la
// label) e la mail appare nella mailbox-label; su una label già esistente
// idem.
func TestApplyLabelOnCreatesMailboxAndCopies(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMail("L <l@example.org>", "Label me", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.ApplyLabel(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "Projects", true); err != nil {
		t.Fatalf("ApplyLabel on (mailbox nuova): %v", err)
	}
	refs, err := store.ListMessages(t.Context(), msg.Query{Label: "Projects"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Projects: %v", err)
	}
	if len(refs) != 1 || refs[0].Subject != "Label me" {
		t.Fatalf("Projects = %v, voglio [Label me]", subjects(refs))
	}

	// Seconda ApplyLabel su una label GIÀ esistente: nessun errore dal
	// CREATE-if-missing (path già creata).
	if err := store.ApplyLabel(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "Projects", true); err != nil {
		t.Fatalf("ApplyLabel on (label esistente): %v", err)
	}
	refs, err = store.ListMessages(t.Context(), msg.Query{Label: "Projects"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Projects: %v", err)
	}
	if len(refs) != 2 {
		t.Errorf("Projects dopo doppia ApplyLabel = %d mail, voglio 2 (due COPY)", len(refs))
	}
}

// TestApplyLabelOffRemovesFromLabelMailbox: ApplyLabel(false) → STORE
// \Deleted + UID EXPUNGE nella mailbox-label: la mail scompare dalla
// mailbox-label (re-list lì). Gli id passati referenziano la mailbox-label
// (il uid nella label-mailbox è quello assegnato dal COPY).
func TestApplyLabelOffRemovesFromLabelMailbox(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMail("L <l@example.org>", "Unlabel me", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.ApplyLabel(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "Projects", true); err != nil {
		t.Fatalf("ApplyLabel on: %v", err)
	}
	labelRefs, err := store.ListMessages(t.Context(), msg.Query{Label: "Projects"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Projects: %v", err)
	}
	if len(labelRefs) != 1 {
		t.Fatalf("Projects = %d ref, voglio 1", len(labelRefs))
	}

	if err := store.ApplyLabel(t.Context(), []msg.MessageID{labelRefs[0].ID}, "Projects", false); err != nil {
		t.Fatalf("ApplyLabel off: %v", err)
	}
	labelRefs, err = store.ListMessages(t.Context(), msg.Query{Label: "Projects"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Projects dopo off: %v", err)
	}
	if len(labelRefs) != 0 {
		t.Errorf("Projects dopo ApplyLabel off = %v, voglio vuota", subjects(labelRefs))
	}

	// L'originale in INBOX resta al suo posto (rimozione dalla label non
	// tocca le altre mailbox).
	inbox, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages INBOX: %v", err)
	}
	if len(inbox) != 1 || inbox[0].Subject != "Unlabel me" {
		t.Errorf("INBOX dopo ApplyLabel off = %v, voglio [Unlabel me] intatta", subjects(inbox))
	}
}

// TestApplyLabelOffWrongMailboxGuard (fix review): ApplyLabel off con id che
// NON referenziano la mailbox-label (vengono da INBOX, label = "Work") ⇒
// errore che menziona entrambe le mailbox e NESSUNA operazione lato server:
// mail intatta in INBOX e copia intatta in Work (guard anti data-loss).
func TestApplyLabelOffWrongMailboxGuard(t *testing.T) {
	ts := startTestServer(t)
	uid := ts.seedMail("G <g@example.org>", "Guard me", "corpo", false, false)
	store := connectStore(t, ts)

	if err := store.ApplyLabel(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "Work", true); err != nil {
		t.Fatalf("ApplyLabel on: %v", err)
	}

	err := store.ApplyLabel(t.Context(), []msg.MessageID{msg.NewID("INBOX", uid)}, "Work", false)
	if err == nil {
		t.Fatal("want errore guard (id non nella mailbox-label), got nil")
	}
	for _, want := range []string{"INBOX", "Work"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("errore senza %q: %v", want, err)
		}
	}

	// Stato server invariato: originale in INBOX e copia in Work al loro posto.
	inbox, err := store.ListMessages(t.Context(), msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages INBOX: %v", err)
	}
	if len(inbox) != 1 || inbox[0].Subject != "Guard me" {
		t.Errorf("INBOX dopo guard = %v, voglio [Guard me] intatta", subjects(inbox))
	}
	work, err := store.ListMessages(t.Context(), msg.Query{Label: "Work"}, mail.Page{})
	if err != nil {
		t.Fatalf("ListMessages Work: %v", err)
	}
	if len(work) != 1 || work[0].Subject != "Guard me" {
		t.Errorf("Work dopo guard = %v, voglio [Guard me] (copia intatta)", subjects(work))
	}
}

// TestMutationsKeepCacheCoherent (FIX cache stale flags): fetch riempie la
// cache, MarkRead/Star mutano i flag, kill del server: un nuovo fetch deve
// riuscire DALLA CACHE con i flag aggiornati (Unread=false, Starred=true).
func TestMutationsKeepCacheCoherent(t *testing.T) {
	ts := startTestServer(t)
	uidRead := ts.seedMail("A <a@example.org>", "To read", "corpo", false, false)
	uidStar := ts.seedMail("B <b@example.org>", "To star", "corpo", false, false)
	store, _ := connectStoreCache(t, ts)

	idRead := msg.NewID("INBOX", uidRead)
	idStar := msg.NewID("INBOX", uidStar)
	for _, id := range []msg.MessageID{idRead, idStar} {
		if _, err := store.FetchMessage(t.Context(), id); err != nil {
			t.Fatalf("FetchMessage (riempie cache) %s: %v", id, err)
		}
	}

	if err := store.MarkRead(t.Context(), []msg.MessageID{idRead}, true); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := store.Star(t.Context(), []msg.MessageID{idStar}, true); err != nil {
		t.Fatalf("Star: %v", err)
	}

	// Kill del server: i fetch seguenti possono riuscire solo dalla cache.
	if err := ts.srv.Close(); err != nil {
		t.Fatalf("close server: %v", err)
	}

	m, err := store.FetchMessage(t.Context(), idRead)
	if err != nil {
		t.Fatalf("FetchMessage %s dalla cache senza rete: %v", idRead, err)
	}
	if m.Ref.Unread {
		t.Error("cache incoerente: Ref.Unread = true dopo MarkRead, voglio false")
	}
	m, err = store.FetchMessage(t.Context(), idStar)
	if err != nil {
		t.Fatalf("FetchMessage %s dalla cache senza rete: %v", idStar, err)
	}
	if !m.Ref.Starred {
		t.Error("cache incoerente: Ref.Starred = false dopo Star, voglio true")
	}
}
