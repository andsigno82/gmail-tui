package imap

// Piano 8, Task 7: conversazioni su server scriptato (nessuna rete, nessuna
// credenziale). Il server risponde per cartella selezionata: INBOX e All Mail.

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

func TestSummarizeThreadAggregates(t *testing.T) {
	d1 := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	refs := []msg.MessageRef{
		{ID: "All Mail:2", From: "Bob", Date: d1.Add(time.Hour), Unread: true, Labels: []string{"Work", "Urgent"}},
		{ID: "All Mail:1", From: "Ann", Date: d1, Labels: []string{"Work"}},
	}
	sum := summarizeThread(refs)
	if sum.count != 2 {
		t.Errorf("count = %d, want 2", sum.count)
	}
	if strings.Join(sum.participants, ",") != "Ann,Bob" {
		t.Errorf("participants = %v, want [Ann Bob] in ordine cronologico", sum.participants)
	}
	if !sum.unread {
		t.Error("unread = false, want true")
	}
	if strings.Join(sum.labels, ",") != "Work,Urgent" {
		t.Errorf("labels = %v", sum.labels)
	}
}

// Piano 10, review punto 3: il filtro IMAP delle label è msg.UserLabels (vedi msg/labels_test.go).
func TestUserLabelsDropsSystemLabels(t *testing.T) {
	got := msg.UserLabels([]string{`\Inbox`, `\Sent`, "Important", "Starred", "Work", "Muy Importante"})
	if strings.Join(got, ",") != "Work,Muy Importante" {
		t.Errorf("msg.UserLabels = %q, want [Work Muy Importante]", got)
	}
}

func TestThreadCriteriaNested(t *testing.T) {
	crit := threadCriteria([]uint64{7, 8, 9})
	// OR(OR(7,8), 9): il livello esterno ha una sola coppia.
	if len(crit.Or) != 1 || len(crit.Or[0][0].Or) != 1 {
		t.Fatalf("catena OR errata: %+v", crit)
	}
	if leaf := crit.Or[0][1].Gmail; leaf == nil || leaf.ThreadID != 9 {
		t.Errorf("ultimo thread nella catena: %+v", leaf)
	}
	if inner := crit.Or[0][0].Or[0][1].Gmail; inner == nil || inner.ThreadID != 8 {
		t.Errorf("thread 8 nella catena interna: %+v", inner)
	}
}

// threadRe estrae i thread richiesti da un SEARCH (X-GM-THRID n).
var threadRe = regexp.MustCompile(`X-GM-THRID (\d+)`)

// gmailFetch descrive un messaggio del server scriptato.
type gmailFetch struct {
	uid    int
	thread int
	login  string // parte locale dell'indirizzo: From = Login <login@x.test>
	date   string // DD-Mon-YYYY
	read   bool
}

// scriptedGmailServer risponde come Gmail: LIST, SELECT, UID SEARCH, UID FETCH
// per la cartella selezionata. Il SEARCH con X-GM-THRID ritorna tutti i UID di
// All Mail, il FETCH tutti i messaggi della cartella (il client filtra gli UID).
func scriptedGmailServer(t *testing.T, boxes map[string][]gmailFetch) string {
	t.Helper()
	box := ""
	respond := func(cmd string) []string {
		switch {
		case strings.HasPrefix(cmd, "LIST"):
			return []string{`* LIST (\HasNoChildren) "/" INBOX`, `* LIST (\HasNoChildren \All) "/" "[Gmail]/All Mail"`}
		case strings.HasPrefix(cmd, "SELECT"):
			box = strings.Trim(strings.TrimPrefix(cmd, "SELECT "), `"`)
			return []string{fmt.Sprintf("* %d EXISTS", len(boxes[box]))}
		case strings.HasPrefix(cmd, "UID SEARCH"):
			// come Gmail: i criteri X-GM-THRID nel comando filtrano i thread
			wanted := map[string]bool{}
			for _, m := range threadRe.FindAllStringSubmatch(cmd, -1) {
				wanted[m[1]] = true
			}
			var uids []string
			for _, m := range boxes[box] {
				if len(wanted) > 0 && !wanted[fmt.Sprint(m.thread)] {
					continue
				}
				// X-GM-RAW "is:unread" verbatim: solo i messaggi non letti
				if strings.Contains(cmd, `X-GM-RAW "is:unread"`) && m.read {
					continue
				}
				uids = append(uids, fmt.Sprint(m.uid))
			}
			return []string{"* SEARCH " + strings.Join(uids, " ")}
		case strings.HasPrefix(cmd, "UID FETCH"):
			// come il server reale: solo gli UID del set richiesto
			asked := fetchSet(cmd)
			if strings.Contains(cmd, "BODY.PEEK[]") {
				// corpo (fetchRaw): il literal {n} porta il testo, con X-GM-* nella stessa risposta
				var lines []string
				for _, m := range boxes[box] {
					if asked != nil && !asked[m.uid] {
						continue
					}
					body := "Subject: s\r\n\r\nciao"
					flags := ""
					if m.read {
						flags = `\Seen`
					}
					lines = append(lines, fmt.Sprintf(
						`* %d FETCH (UID %d X-GM-MSGID %d X-GM-THRID %d X-GM-LABELS () FLAGS (%s) BODY[] {%d}`,
						m.uid, m.uid, 1000+m.uid, m.thread, flags, len(body)), body+")")
				}
				return lines
			}
			var lines []string
			for _, m := range boxes[box] {
				if asked != nil && !asked[m.uid] {
					continue
				}
				flags := ""
				if m.read {
					flags = `\Seen`
				}
				lines = append(lines, fmt.Sprintf(
					`* %d FETCH (UID %d X-GM-MSGID %d X-GM-THRID %d ENVELOPE (NIL "s%d" (("%s" NIL "%s" "x.test")) NIL NIL NIL NIL NIL NIL "<%d@x>") FLAGS (%s) INTERNALDATE "%s 10:00:00 +0000")`,
					m.uid, m.uid, 1000+m.uid, m.thread, m.uid, upperFirst(m.login), m.login, m.uid, flags, m.date))
			}
			return lines
		}
		return nil
	}
	return scriptedIMAP(t, "IMAP4rev1 X-GM-EXT-1 UIDPLUS", respond)
}

func TestListConversationsGmailOneRowPerThread(t *testing.T) {
	inbox := []gmailFetch{
		{uid: 1, thread: 200, login: "cy", date: "01-Nov-2026", read: true},
		{uid: 2, thread: 100, login: "ann", date: "02-Nov-2026", read: true},
		{uid: 3, thread: 100, login: "bob", date: "03-Nov-2026", read: false},
	}
	allMail := append([]gmailFetch{}, inbox...)
	allMail = append(allMail, gmailFetch{uid: 4, thread: 100, login: "me", date: "04-Nov-2026", read: true})
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": inbox, "[Gmail]/All Mail": allMail})

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	convs, err := s.ListConversations(ctx, msg.Query{}, mail.Page{})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("len = %d, want 2 (una riga per thread): %+v", len(convs), convs)
	}
	c := convs[0]
	if c.ThreadID != 100 || c.Latest.ID != "INBOX:3" {
		t.Errorf("prima riga: thread %d latest %s, want 100 INBOX:3", c.ThreadID, c.Latest.ID)
	}
	if c.Count != 3 {
		t.Errorf("Count = %d, want 3 (All Mail: ann, bob, me)", c.Count)
	}
	wantP := "Ann <ann@x.test>,Bob <bob@x.test>,Me <me@x.test>"
	if strings.Join(c.Participants, ",") != wantP {
		t.Errorf("Participants = %v, want %s", c.Participants, wantP)
	}
	if !c.Unread {
		t.Error("Unread = false: bob non ha letto il messaggio")
	}
	if d := convs[1]; d.ThreadID != 200 || d.Count != 1 || d.Unread {
		t.Errorf("seconda riga: %+v", d)
	}

	// ThreadMessages: tutti i messaggi del thread 100 su All Mail, cronologici.
	refs, err := s.ThreadMessages(ctx, 100)
	if err != nil {
		t.Fatalf("ThreadMessages: %v", err)
	}
	if len(refs) != 3 || refs[0].Date.After(refs[2].Date) {
		t.Errorf("ThreadMessages = %d messaggi, ordine non cronologico", len(refs))
	}
}

// fetchSet legge il set di UID di un UID FETCH (numeri e intervalli a:b separati
// da virgole). Nil se il set non è leggibile: il server risponde a tutti.
func fetchSet(cmd string) map[int]bool {
	f := strings.Fields(cmd)
	if len(f) < 3 {
		return nil
	}
	set := map[int]bool{}
	for _, part := range strings.Split(f[2], ",") {
		lo, hi, ok := strings.Cut(part, ":")
		var a, b int
		if _, err := fmt.Sscan(lo, &a); err != nil {
			return nil
		}
		b = a
		if ok {
			if _, err := fmt.Sscan(hi, &b); err != nil {
				return nil
			}
		}
		for u := a; u <= b; u++ {
			set[u] = true
		}
	}
	return set
}

// upperFirst maiuscola il primo carattere (nomi di test ASCII).
func upperFirst(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

// Piano 8, review (punto 6): RFC 3501 non garantisce l'ordine di UID SEARCH. Con
// più blocchi di scansione (chunkScansione) i messaggi più recenti devono essere
// scelti per UID, non per posizione nella risposta del server.
func TestListConversationsOrdineUIDNonGarantito(t *testing.T) {
	var box []gmailFetch
	for uid := 150; uid >= 1; uid-- { // risposta del server in ordine decrescente
		box = append(box, gmailFetch{uid: uid, thread: uid, login: "ann", date: "01-Nov-2026", read: true})
	}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": box, "[Gmail]/All Mail": box})

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	convs, err := s.ListConversations(ctx, msg.Query{}, mail.Page{Limit: 10})
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(convs) != 10 || convs[0].Latest.ID != "INBOX:150" {
		t.Fatalf("prima riga %v (len %d), voglio INBOX:150 e 10 righe: i più recenti per UID", convs[0].Latest.ID, len(convs))
	}
}

// Piano 8, review (punto 3): quando il tetto di scansione (maxMessaggiScansionati)
// interrompe la lettura con messaggi ancora da leggere e la pagina non è piena, la
// lista NON è esaurita: il backend lo dichiara con mail.ErrScanLimit, così la UI non
// la tratta come fine della cartella.
func TestListConversationsTettoDiScansioneSegnalato(t *testing.T) {
	var box []gmailFetch
	for uid := 1; uid <= 600; uid++ { // 600 messaggi in soli 3 thread
		box = append(box, gmailFetch{uid: uid, thread: uid%3 + 1, login: "ann", date: "01-Nov-2026", read: true})
	}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": box, "[Gmail]/All Mail": box})

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	convs, err := s.ListConversations(ctx, msg.Query{}, mail.Page{Limit: 50})
	if len(convs) != 3 {
		t.Fatalf("righe = %d, voglio 3 (i thread trovati prima del tetto)", len(convs))
	}
	if !errors.Is(err, mail.ErrScanLimit) {
		t.Fatalf("err = %v, voglio mail.ErrScanLimit: la lista è limitata, non esaurita", err)
	}
}

// Piano 8, review (punto 1): FetchMessage porta gli ID Gmail (X-GM-MSGID, X-GM-THRID)
// nel Ref, così la cache e la deduplica per GmailID hanno una chiave reale.
func TestFetchMessageCarriesGmailIDs(t *testing.T) {
	box := []gmailFetch{{uid: 4, thread: 7, login: "ann", date: "01-Nov-2026", read: true}}
	addr := scriptedGmailServer(t, map[string][]gmailFetch{"INBOX": box, "[Gmail]/All Mail": box})

	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx := context.Background()
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = s.Close() }()

	m, err := s.FetchMessage(ctx, msg.NewID("INBOX", 4))
	if err != nil {
		t.Fatalf("FetchMessage: %v", err)
	}
	if m.Ref.GmailID != 1004 || m.Ref.ThreadID != 7 {
		t.Errorf("GmailID = %d, ThreadID = %d; voglio 1004 e 7 (X-GM-MSGID/THRID del server)", m.Ref.GmailID, m.Ref.ThreadID)
	}
}
