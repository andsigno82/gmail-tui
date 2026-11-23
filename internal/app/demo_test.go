package app

import (
	"context"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/net/html"

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
	// Piano 15, Task 17: l'ordine con tabella porta la posta in arrivo a 8 (7 → 8).
	if n := len(inbox); n < 5 || n > 8 {
		t.Fatalf("messaggi seed in posta in arrivo = %d, voglio 5-8", n)
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

// Piano 15, Task 17: il seed mostra le novità del rendering: un bottone (a con
// background e padding), un link il cui testo ha un dominio diverso dall'href,
// un'immagine con alt, una tabella d'ordine, una citazione annidata e elenchi.
func TestDemoStoreContenutoRicco(t *testing.T) {
	mk := DemoStore()
	find := func(part string) *msg.Message {
		for _, mm := range inboxMsgs(mk) {
			if strings.Contains(mm.Ref.Subject, part) {
				return mm
			}
		}
		t.Fatalf("nessun messaggio in posta in arrivo con soggetto %q", part)
		return nil
	}

	// Newsletter: bottone, link con dominio diverso, immagine con alt.
	news := htmlElements(t, find("Go Weekly").BodyHTML)
	var button, warnLink, img bool
	for _, n := range news {
		switch {
		case n.Data == "a" && nodeText(n) == "Conferma iscrizione":
			style := attr(n, "style")
			button = strings.Contains(style, "background") && strings.Contains(style, "padding")
		case n.Data == "a" && strings.HasPrefix(nodeText(n), "example.org"):
			u, err := url.Parse(attr(n, "href"))
			warnLink = err == nil && u.Hostname() == "example.com"
		case n.Data == "img":
			img = img || strings.TrimSpace(attr(n, "alt")) != ""
		}
	}
	if !button {
		t.Error("newsletter: manca il bottone «Conferma iscrizione» (a con background e padding)")
	}
	if !warnLink {
		t.Error("newsletter: manca il link con testo example.org e href su un altro dominio")
	}
	if !img {
		t.Error("newsletter: manca l'immagine con testo alternativo")
	}

	// Conferma d'ordine: tabella con prodotto, quantità, prezzo.
	var table bool
	var headers []string
	for _, n := range htmlElements(t, find("Conferma d'ordine").BodyHTML) {
		if n.Data == "table" {
			table = true
		}
		if n.Data == "th" || n.Data == "td" {
			headers = append(headers, nodeText(n))
		}
	}
	if !table {
		t.Fatal("conferma d'ordine: nessuna tabella")
	}
	for _, want := range []string{"Prodotto", "Quantità", "Prezzo"} {
		if !containsStr(headers, want) {
			t.Errorf("tabella d'ordine: manca la colonna %q (celle %v)", want, headers)
		}
	}

	// Risposta: citazione annidata "> > ", elenco puntato e numerato (testo semplice).
	var nested, bullet, numbered bool
	for _, line := range strings.Split(find("pranzo").BodyText, "\n") {
		nested = nested || strings.HasPrefix(line, "> > ")
		bullet = bullet || strings.HasPrefix(line, "- ")
		numbered = numbered || strings.HasPrefix(line, "1. ")
	}
	if !nested {
		t.Error("risposta: manca la citazione annidata «> > »")
	}
	if !bullet {
		t.Error("risposta: manca un elenco puntato «- »")
	}
	if !numbered {
		t.Error("risposta: manca un elenco numerato «1. »")
	}
}

// Piano 15, Task 17: una label annidata con Delimiter "/" nelle Mailboxes, con
// messaggi (albero cartelle e contatori).
func TestDemoStoreLabelAnnidata(t *testing.T) {
	mk := DemoStore()
	boxes, err := mk.ListMailboxes(t.Context())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	var nested *mail.Mailbox
	for i := range boxes {
		if boxes[i].Name == "Lavoro/Clienti" {
			nested = &boxes[i]
		}
	}
	if nested == nil {
		t.Fatalf("cartelle = %+v, voglio la label annidata Lavoro/Clienti", boxes)
	}
	if nested.Delimiter != "/" {
		t.Errorf("Lavoro/Clienti: Delimiter = %q, voglio \"/\"", nested.Delimiter)
	}
	refs, err := mk.ListMessages(t.Context(), msg.Query{Label: "Lavoro/Clienti"}, mail.Page{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages Lavoro/Clienti: %v", err)
	}
	if len(refs) == 0 {
		t.Error("Lavoro/Clienti: nessun messaggio nel seed")
	}
}

// Piano 15, Task 17: i contatori della demo coincidono con le liste: per ogni
// cartella UnreadCounts = non letti di ListMessages; INBOX e la label annidata
// hanno non letti da mostrare.
func TestDemoStoreContatoriCoerenti(t *testing.T) {
	mk := DemoStore()
	counts, err := mk.UnreadCounts(t.Context())
	if err != nil {
		t.Fatalf("UnreadCounts: %v", err)
	}
	boxes, err := mk.ListMailboxes(t.Context())
	if err != nil {
		t.Fatalf("ListMailboxes: %v", err)
	}
	for _, b := range boxes {
		refs, err := mk.ListMessages(t.Context(), msg.Query{Label: b.Name, Unread: true}, mail.Page{Limit: 1000})
		if err != nil {
			t.Fatalf("ListMessages %s: %v", b.Name, err)
		}
		if counts[b.Name] != len(refs) {
			t.Errorf("%s: UnreadCounts = %d, non letti in lista = %d", b.Name, counts[b.Name], len(refs))
		}
	}
	if counts["INBOX"] == 0 {
		t.Error("INBOX: nessun non letto nei contatori demo")
	}
	if counts["Lavoro/Clienti"] == 0 {
		t.Error("Lavoro/Clienti: nessun non letto nei contatori demo")
	}
}

// htmlElements restituisce gli elementi di s, in ordine di documento.
func htmlElements(t *testing.T, s string) []*html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	var out []*html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			out = append(out, n)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

// nodeText restituisce il testo del nodo con gli spazi normalizzati.
func nodeText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}

// attr restituisce il valore dell'attributo key (vuoto se assente).
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
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
	// Una sola riga per la conversazione di quattro messaggi, con il contatore.
	m, _ = step(m, tea.WindowSizeMsg{Width: 140, Height: 30})
	if n := strings.Count(m.inbox.View(), "Re: design review"); n != 1 {
		t.Fatalf("righe «Re: design review» = %d, voglio una riga per conversazione", n)
	}
	if !strings.Contains(m.inbox.View(), "(4)") {
		t.Errorf("riga della conversazione senza il contatore (4):\n%s", m.inbox.View())
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

// TestDemoBookWritesNoFile (Piano 9, Task 13): in demo la rubrica resta in memoria.
// Con HOME e cache isolate, lista e lettura non devono creare contacts.json.
func TestDemoBookWritesNoFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	mk := DemoStore()
	m, _ := newTestModelMock(t, mk.Msgs...)
	m.SetAddressBook(nil, nil) // come il main in modalità demo
	refs, err := mk.ListMessages(context.Background(), msg.Query{}, mail.Page{Limit: 50})
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	mm, _ := step(m, teaSize())
	mm, _ = step(mm, msgsLoaded{refs: refs})
	_, _ = step(mm, msgLoaded{m: mk.Msgs[0]})
	var found []string
	_ = filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Name() == "contacts.json" {
			found = append(found, p)
		}
		return nil
	})
	if len(found) != 0 {
		t.Errorf("demo ha scritto la rubrica su disco: %v", found)
	}
}

// Piano 10, review punto 6: in demo l'annulla dell'archivia funziona. Il seed ha le
// copie in "Tutti i messaggi" con lo stesso GmailID: l'archivia le usa per ripristinare
// la INBOX, e `u` riporta la conversazione in Posta in arrivo.
func TestDemoArchiveUndoRestoresInbox(t *testing.T) {
	mk := DemoStore()
	before := inboxGmailIDs(mk)
	m := pagedModel(t, mk)
	m, cmd := step(m, pressKey("a"))
	m = applyAll(t, m, cmd)
	if len(inboxGmailIDs(mk)) >= len(before) {
		t.Fatal("archivia in demo: nessun messaggio uscito dalla INBOX")
	}
	if !toastHasUndo(m) {
		t.Fatalf("toast = %v, voglio l'offerta di u in demo", m.toast.Texts())
	}
	m, cmd = step(m, pressKey("u"))
	applyAll(t, m, cmd)
	if got := inboxGmailIDs(mk); !sameGmailIDs(got, before) {
		t.Errorf("dopo u la INBOX = %v, voglio %v", got, before)
	}
}

// inboxGmailIDs sono gli ID Gmail dei messaggi in posta in arrivo.
func inboxGmailIDs(mk *mail.Mock) []uint64 {
	var out []uint64
	for _, mm := range inboxMsgs(mk) {
		out = append(out, mm.Ref.GmailID)
	}
	return out
}

// sameGmailIDs confronta due insiemi di ID Gmail senza ordine.
func sameGmailIDs(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[uint64]int, len(a))
	for _, id := range a {
		seen[id]++
	}
	for _, id := range b {
		if seen[id] == 0 {
			return false
		}
		seen[id]--
	}
	return true
}
