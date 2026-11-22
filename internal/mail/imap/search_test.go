package imap

// Test di Search (piano 2, Task 6): ricerca server-side senza paginazione,
// con post-filtro client-side per HasAttachment. Riusa la harness di
// imap_test.go (startTestServer, connectStore, subjects) e gli helper
// multipart di fetch_test.go (seedMultipartMail, pdfContent,
// connectStoreCache).

import (
	"fmt"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestSearchFrom: from:x → solo i messaggi del mittente x, dal più nuovo;
// mittente inesistente → zero risultati senza errore.
func TestSearchFrom(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	ts.seedMail("Bob <bob@example.org>", "Another Bob", "altro corpo", true, false)
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{From: "bob@example.org"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"Another Bob", "Second"} // dal più nuovo al più vecchio
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
	}
	for i, s := range want {
		if refs[i].Subject != s {
			t.Errorf("refs[%d].Subject = %q, voglio %q", i, refs[i].Subject, s)
		}
	}

	refs, err = store.Search(t.Context(), msg.Query{From: "nobody@example.org"})
	if err != nil {
		t.Fatalf("Search mittente inesistente: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("refs = %v, voglio 0 risultati per mittente inesistente", subjects(refs))
	}
}

// TestSearchTextInSubject: TEXT matcha la subject lato server (e il corpo).
func TestSearchTextInSubject(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{Text: "Fourth"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if want := []string{"Fourth"}; len(refs) != 1 || refs[0].Subject != want[0] {
		t.Errorf("refs = %v, voglio %v (TEXT matcha la subject)", subjects(refs), want)
	}

	refs, err = store.Search(t.Context(), msg.Query{Text: "quinto corpo"})
	if err != nil {
		t.Fatalf("Search testo nel corpo: %v", err)
	}
	if want := []string{"Fifth"}; len(refs) != 1 || refs[0].Subject != want[0] {
		t.Errorf("refs = %v, voglio %v (TEXT matcha anche il corpo)", subjects(refs), want)
	}
}

// TestSearchUnread: unread → solo le unseen (Fifth, Fourth, Second).
func TestSearchUnread(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{Unread: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
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

// TestSearchStarred: starred → solo le \Flagged (Fourth, Third).
func TestSearchStarred(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{Starred: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"Fourth", "Third"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
	}
	for i := range refs {
		if !refs[i].Starred {
			t.Errorf("refs[%d] (%q): Starred = false, voglio true", i, refs[i].Subject)
		}
	}
}

// TestSearchHasAttachment: has:attachment → solo la multipart con allegato
// (SENZA cache: il post-filtro scarica i corpi via rete). La combinazione
// con un criterio server-side (from) è un AND: la mail di Alice non ha
// allegati e viene scartata dal post-filtro.
func TestSearchHasAttachment(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, false)
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{HasAttachment: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if want := []string{"With attachment"}; len(refs) != 1 || refs[0].Subject != want[0] {
		t.Fatalf("refs = %v, voglio %v (solo la multipart con allegato)", subjects(refs), want)
	}

	// AND server-side + post-filtro: il mittente con allegato passa, Alice no.
	refs, err = store.Search(t.Context(), msg.Query{From: "mia@example.org", HasAttachment: true})
	if err != nil {
		t.Fatalf("Search from+has:attachment: %v", err)
	}
	if want := []string{"With attachment"}; len(refs) != 1 || refs[0].Subject != want[0] {
		t.Errorf("refs = %v, voglio %v", subjects(refs), want)
	}
	refs, err = store.Search(t.Context(), msg.Query{From: "alice@example.org", HasAttachment: true})
	if err != nil {
		t.Fatalf("Search from senza allegati: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("refs = %v, voglio 0 (Alice non ha allegati)", subjects(refs))
	}
}

// TestSearchHasAttachmentWithCache: con la cache il post-filtro è
// cache-first: la prima Search popola la cache via FetchMessage, la seconda
// riusa le entry (zero download dei corpi) con lo stesso risultato.
func TestSearchHasAttachmentWithCache(t *testing.T) {
	ts := startTestServer(t)
	ts.seedFive(t)
	ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", "With attachment",
		"Hello from the text part.", "doc.pdf", pdfContent, false, false)
	store, _ := connectStoreCache(t, ts)

	for range 2 {
		refs, err := store.Search(t.Context(), msg.Query{HasAttachment: true})
		if err != nil {
			t.Fatalf("Search con cache: %v", err)
		}
		if want := []string{"With attachment"}; len(refs) != 1 || refs[0].Subject != want[0] {
			t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
		}
	}
}

// TestSearchLabelSelectsMailbox: Query.Label seleziona la mailbox su cui
// cercare (semantica label = cartella), come per ListMessages.
func TestSearchLabelSelectsMailbox(t *testing.T) {
	ts := startTestServer(t)
	ts.createMailbox("Archive")
	ts.seedInMailbox("Archive", "Arch1 <a@example.org>", "Archived one", "corpo a", true, false)
	ts.seedInMailbox("Archive", "Arch2 <a@example.org>", "Archived two", "corpo b", false, true)
	ts.seedMail("Inbox <i@example.org>", "Inbox only", "corpo inbox", false, false)
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{Label: "Archive"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	want := []string{"Archived two", "Archived one"}
	if len(refs) != len(want) {
		t.Fatalf("refs = %v, voglio %v", subjects(refs), want)
	}
	if refs[0].ID != msg.NewID("Archive", 2) || refs[1].ID != msg.NewID("Archive", 1) {
		t.Errorf("IDs = [%s %s], voglio gli uid della mailbox Archive", refs[0].ID, refs[1].ID)
	}
}

// TestSearchNoPagination: Search non pagina: 60 match → 60 ref (il default
// di ListMessages sarebbe 50), ordine dal più nuovo.
func TestSearchNoPagination(t *testing.T) {
	ts := startTestServer(t)
	for i := 1; i <= 60; i++ {
		ts.seedMail("Bulk <bulk@example.org>", fmt.Sprintf("Bulk %d", i), "corpo", false, false)
	}
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(refs) != 60 {
		t.Fatalf("refs = %d, voglio 60 (Search senza paginazione)", len(refs))
	}
	if refs[0].Subject != "Bulk 60" || refs[59].Subject != "Bulk 1" {
		t.Errorf("ordine = [%s ... %s], voglio dal più nuovo (Bulk 60 ... Bulk 1)", refs[0].Subject, refs[59].Subject)
	}
}

// TestSearchAttachmentPostFilterCap: il post-filtro HasAttachment esamina al
// più maxAttachmentCandidates ref (i più recenti): con 205 match tutti con
// allegato, la Search ne restituisce esattamente 200.
func TestSearchAttachmentPostFilterCap(t *testing.T) {
	ts := startTestServer(t)
	total := maxAttachmentCandidates + 5
	for i := 1; i <= total; i++ {
		ts.seedMultipartMail("INBOX", "Mia <mia@example.org>", fmt.Sprintf("Att %d", i),
			"corpo", "doc.pdf", pdfContent, false, false)
	}
	store := connectStore(t, ts)

	refs, err := store.Search(t.Context(), msg.Query{HasAttachment: true})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(refs) != maxAttachmentCandidates {
		t.Fatalf("refs = %d, voglio %d (cap del post-filtro HasAttachment)", len(refs), maxAttachmentCandidates)
	}
	if refs[0].Subject != "Att 205" || refs[len(refs)-1].Subject != "Att 6" {
		t.Errorf("finestra = [%s ... %s], voglio i %d più recenti (Att 205 ... Att 6)",
			refs[0].Subject, refs[len(refs)-1].Subject, maxAttachmentCandidates)
	}
}
