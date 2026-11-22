package app

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestDemoStoreRealisticSeed (Piano 4, Task 5): la factory della demo produce
// un Mock con 5-7 messaggi realistici — mix di unread e starred, un soggetto
// di benvenuto, un Re: — così chi clona il repo vede l'app viva.
func TestDemoStoreRealisticSeed(t *testing.T) {
	mk := DemoStore()
	if mk == nil {
		t.Fatal("DemoStore = nil")
	}
	inbox := inboxMsgs(mk)
	// Piano 8, Task 12: la conversazione con risposte in Inviati e un non letto
	// richiede un messaggio in più in posta in arrivo (6 → 7).
	if n := len(inbox); n < 5 || n > 7 {
		t.Fatalf("messaggi seed in posta in arrivo = %d, voglio 5-7", n)
	}
	var unread, starred int
	var subjects []string
	for _, mm := range inbox {
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
	if unread == 0 || unread == len(inbox) {
		t.Errorf("unread = %d su %d: voglio un mix", unread, len(inbox))
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
	if want := len(inboxMsgs(mk)); len(refs) != want {
		t.Fatalf("refs = %d, voglio %d (posta in arrivo)", len(refs), want)
	}
	for i := 1; i < len(refs); i++ {
		if refs[i].Date.After(refs[i-1].Date) {
			t.Errorf("ordine data non desc a posizione %d", i)
		}
	}
}

// inboxMsgs restituisce i messaggi del seed nella posta in arrivo (INBOX).
func inboxMsgs(mk *mail.Mock) []*msg.Message {
	var out []*msg.Message
	for _, mm := range mk.Msgs {
		if box, _, err := mm.Ref.ID.Split(); err == nil && box == "INBOX" {
			out = append(out, mm)
		}
	}
	return out
}

// TestDemoStoreFolders (Piano 7, fix review 8): il seed ha una cartella Inviati
// (ruolo sent) e una label Work, entrambe con messaggi, così `m` in demo mostra
// la navigazione fra cartelle e label.
func TestDemoStoreFolders(t *testing.T) {
	mk := DemoStore()
	boxes, err := mk.ListMailboxes(t.Context())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	roles := map[string]string{}
	for _, b := range boxes {
		roles[b.Name] = b.Role
	}
	if roles["[Gmail]/Inviati"] != "sent" {
		t.Errorf("cartelle = %+v, voglio [Gmail]/Inviati con ruolo sent", boxes)
	}
	if _, ok := roles["Work"]; !ok {
		t.Errorf("cartelle = %+v, voglio la label Work", boxes)
	}
	for _, box := range []string{"[Gmail]/Inviati", "Work"} {
		refs, err := mk.ListMessages(t.Context(), msg.Query{Label: box}, mail.Page{Limit: 50})
		if err != nil {
			t.Fatalf("ListMessages %s: %v", box, err)
		}
		if len(refs) < 2 {
			t.Errorf("%s: %d messaggi, voglio almeno 2", box, len(refs))
		}
	}
}

// Piano 8, Task 12: il seed ha conversazioni Gmail (X-GM-THRID). La conversazione
// "Re: design review" ha quattro messaggi: due in posta in arrivo, due risposte
// dell'utente in Inviati, uno dei due in arrivo non letto.
func TestDemoStoreConversazioniSimulate(t *testing.T) {
	mk := DemoStore()
	convs, err := mk.ListConversations(t.Context(), msg.Query{}, mail.Page{Limit: 50})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	var design, lunch *mail.Conversation
	var labelled bool
	for i := range convs {
		switch convs[i].ThreadID {
		case demoThreadDesign:
			design = &convs[i]
		case demoThreadLunch:
			lunch = &convs[i]
		}
		if len(convs[i].Labels) > 0 {
			labelled = true
		}
	}
	if design == nil {
		t.Fatal("nessuna conversazione «Re: design review» in posta in arrivo")
	}
	if design.Count != 4 {
		t.Errorf("design review: Count = %d, voglio 4 (incluse le risposte in Inviati)", design.Count)
	}
	if !design.Unread {
		t.Error("design review: deve avere almeno un messaggio non letto")
	}
	if !containsStr(design.Participants, "tu@example.com") {
		t.Errorf("partecipanti = %v, voglio la risposta dell'utente", design.Participants)
	}
	if lunch == nil || lunch.Count != 1 {
		t.Errorf("pranzo: conversazione singola attesa, got %+v", lunch)
	}
	if !labelled {
		t.Error("nessuna conversazione con label utente nel seed")
	}
}

// Piano 8, Task 12: la conversazione apre tutti i suoi messaggi in ordine
// cronologico, comprese le risposte in Inviati.
func TestDemoStoreThreadMessagesCronologici(t *testing.T) {
	mk := DemoStore()
	refs, err := mk.ThreadMessages(t.Context(), demoThreadDesign)
	if err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	if len(refs) != 4 {
		t.Fatalf("messaggi del thread = %d, voglio 4", len(refs))
	}
	for i := 1; i < len(refs); i++ {
		if refs[i].Date.Before(refs[i-1].Date) {
			t.Errorf("ordine non cronologico a posizione %d", i)
		}
	}
	if !containsStr(senders(refs), "tu@example.com") {
		t.Errorf("nel thread manca la risposta inviata dall'utente")
	}
}

// Piano 8, Task 12, dalla demo: la lista mostra la riga della conversazione con
// il contatore, e aprirla mostra la risposta dell'utente nella vista thread.
func TestDemoConversazioneAperta(t *testing.T) {
	m := pagedModel(t, DemoStore())
	// Una sola riga per la conversazione di quattro messaggi. Il contatore (4) è
	// coperto da TestDemoStoreConversazioniSimulate: nella riga si perde nel
	// troncamento dei partecipanti (Piano 8, Task 9), segnalato a parte.
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	if n := strings.Count(m.inbox.View(), "Re: design review"); n != 1 {
		t.Fatalf("righe «Re: design review» = %d, voglio una riga per conversazione", n)
	}
	var row msg.MessageRef
	for _, r := range m.refs {
		if r.ThreadID == demoThreadDesign {
			row = r
		}
	}
	if row.ID == "" {
		t.Fatal("riga della conversazione non trovata in lista")
	}
	m, cmd := step(m, inboxSelected(row))
	for _, ms := range runCmd(cmd) {
		if _, ok := ms.(msgLoaded); ok {
			m, _ = step(m, ms)
		}
	}
	if !strings.Contains(m.thread.View(), "▸ tu@example.com") {
		t.Errorf("la vista thread deve mostrare la risposta dell'utente compressa:\n%s", m.thread.View())
	}
}

// containsStr dice se s contiene v.
func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// senders estrae i mittenti dei ref.
func senders(refs []msg.MessageRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.From)
	}
	return out
}
