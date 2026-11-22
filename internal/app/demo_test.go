package app

import (
	"strings"
	"testing"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
)

// TestDemoStoreRealisticSeed (Piano 4, Task 5): la factory della demo produce
// un Mock con 5-6 messaggi realistici — mix di unread e starred, un soggetto
// di benvenuto, un Re: — così chi clona il repo vede l'app viva.
func TestDemoStoreRealisticSeed(t *testing.T) {
	mk := DemoStore()
	if mk == nil {
		t.Fatal("DemoStore = nil")
	}
	if n := len(mk.Msgs); n < 5 || n > 6 {
		t.Fatalf("messaggi seed = %d, voglio 5-6", n)
	}
	var unread, starred int
	var subjects []string
	for _, mm := range mk.Msgs {
		if mm.Ref.Unread {
			unread++
		}
		if mm.Ref.Starred {
			starred++
		}
		subjects = append(subjects, mm.Ref.Subject)
		if mm.Ref.From == "" || mm.Ref.ID == "" || mm.Ref.Date.IsZero() {
			t.Errorf("seed %q: ref incompleto (From/ID/Date)", mm.Ref.Subject)
		}
		if strings.TrimSpace(mm.BodyText) == "" {
			t.Errorf("seed %q: corpo vuoto", mm.Ref.Subject)
		}
	}
	if unread == 0 || unread == len(mk.Msgs) {
		t.Errorf("unread = %d su %d: voglio un mix", unread, len(mk.Msgs))
	}
	if starred == 0 {
		t.Error("nessun messaggio starred nel seed")
	}
	var hasWelcome, hasRe bool
	for _, s := range subjects {
		if strings.Contains(s, "Benvenuto") {
			hasWelcome = true
		}
		if strings.HasPrefix(s, "Re:") {
			hasRe = true
		}
	}
	if !hasWelcome || !hasRe {
		t.Errorf("soggetti seed = %v, voglio un benvenuto e un Re:", subjects)
	}
}

// TestDemoStoreAttachmentSavable: un messaggio del seed porta un allegato
// con Raw popolato: `:save 1` su quel messaggio scrive il file (il Mock salva
// i contenuti da Raw).
func TestDemoStoreAttachmentSavable(t *testing.T) {
	mk := DemoStore()
	var withAtt *msg.Message
	for _, mm := range mk.Msgs {
		if len(mm.Attachments) > 0 {
			withAtt = mm
			break
		}
	}
	if withAtt == nil {
		t.Fatal("nessun messaggio con allegato nel seed")
	}
	if len(mk.Raw[withAtt.Ref.ID]) == 0 {
		t.Errorf("Raw[%s] vuoto: :save fallirebbe in demo", withAtt.Ref.ID)
	}
	for _, a := range withAtt.Attachments {
		if _, ok := mk.Raw[withAtt.Ref.ID][a.PartIndex]; !ok {
			t.Errorf("allegato %q (PartIndex %d) senza Raw", a.Filename, a.PartIndex)
		}
	}
}

// TestDemoStoreListable: il seed si lista come una casella vera (data desc) e
// il mock fa anche da sender (demo: un solo oggetto — il Send registra).
func TestDemoStoreListable(t *testing.T) {
	mk := DemoStore()
	refs, err := mk.ListMessages(t.Context(), msg.Query{}, mail.Page{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(refs) != len(mk.Msgs) {
		t.Fatalf("refs = %d, voglio %d", len(refs), len(mk.Msgs))
	}
	for i := 1; i < len(refs); i++ {
		if refs[i].Date.After(refs[i-1].Date) {
			t.Errorf("ordine data non desc a posizione %d", i)
		}
	}
}
