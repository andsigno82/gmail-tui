package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// gmailReader annuncia la modalità Gmail (come imap.Store con X-GM-EXT-1) e
// registra le query di ListConversations.
type gmailReader struct {
	Reader
	qs []msg.Query
}

func (g *gmailReader) GmailExt() bool { return true }

func (g *gmailReader) ListConversations(ctx context.Context, q msg.Query, pg mail.Page) ([]mail.Conversation, error) {
	g.qs = append(g.qs, q)
	return g.Reader.ListConversations(ctx, q, pg)
}

// Piano 14, Wave 4 (punto 1): in modalità Gmail la query arriva verbatim come Raw
// (X-GM-RAW), senza il parsing locale.
func TestSearchGmailSendsRawQuery(t *testing.T) {
	g := &gmailReader{Reader: &mail.Mock{Msgs: manyMsgs(3)}}
	cs := connect(t, g, Options{Version: "test"})

	call(t, cs, "search_messages", map[string]any{"query": "older_than:7d -label:x"})
	if len(g.qs) != 1 {
		t.Fatalf("ListConversations chiamate = %d, attese 1", len(g.qs))
	}
	if g.qs[0].Raw != "older_than:7d -label:x" {
		t.Fatalf("Raw = %q, atteso la query verbatim", g.qs[0].Raw)
	}
}

// Piano 14, Wave 4 (punto 1): query con caratteri di controllo o troppo lunghe
// sono rifiutate prima di qualunque chiamata allo store.
func TestSearchRejectsControlCharsAndLongQuery(t *testing.T) {
	g := &gmailReader{Reader: &mail.Mock{Msgs: manyMsgs(3)}}
	cs := connect(t, g, Options{Version: "test"})

	for _, q := range []string{"from:a\x00b", "x\r\nA1 NOOP", strings.Repeat("a", maxQueryRunes+1)} {
		if res := call(t, cs, "search_messages", map[string]any{"query": q}); !res.IsError {
			t.Errorf("query %q: atteso IsError", q[:min(len(q), 20)])
		}
	}
	if len(g.qs) != 0 {
		t.Fatalf("lo store è stato interrogato per query non valide: %d", len(g.qs))
	}
}

// threadMsg crea un messaggio del thread tid, con data crescente per uid.
func threadMsg(uid uint32, tid uint64, body string) *msg.Message {
	m := msgAt(uid, "s"+string(rune('a'+uid%26)), body)
	m.Ref.ThreadID = tid
	m.Ref.Date = time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC).Add(time.Duration(uid) * time.Minute)
	return m
}

// Piano 14, Wave 4 (punto 2): read_thread restituisce i messaggi del thread in
// ordine cronologico, dentro il marcatore non fidato, con il corpo.
func TestReadThreadChronologicalWrapped(t *testing.T) {
	m := &mail.Mock{Msgs: []*msg.Message{threadMsg(2, 77, "secondo"), threadMsg(1, 77, "primo"), threadMsg(3, 88, "altro thread")}}
	cs := connect(t, m, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "read_thread", map[string]any{"thread_id": "77"}))
	if data["thread_id"] != "77" {
		t.Fatalf("thread_id = %v, atteso 77", data["thread_id"])
	}
	msgs, _ := data["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("messaggi = %d, attesi 2", len(msgs))
	}
	first, _ := msgs[0].(map[string]any)
	if first["id"] != "INBOX:1" || first["body"] != "primo" {
		t.Fatalf("primo messaggio = %v, atteso INBOX:1 con corpo primo", first)
	}
}

// Piano 14, Wave 4 (punto 2): leggere un thread non segna i messaggi come letti
// (fetch dei corpi con BODY.PEEK nel backend; il Mock non deve ricevere MarkRead).
func TestReadThreadDoesNotMarkRead(t *testing.T) {
	m := &mail.Mock{Msgs: []*msg.Message{threadMsg(1, 77, "a")}}
	cs := connect(t, m, Options{Version: "test"})
	unwrap(t, call(t, cs, "read_thread", map[string]any{"thread_id": "77"}))
	if !m.Msgs[0].Ref.Unread {
		t.Fatal("read_thread ha segnato il messaggio come letto")
	}
}

// Piano 14, Wave 4 (punto 2): thread_id deve essere un decimale > 0.
func TestReadThreadRejectsBadID(t *testing.T) {
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{threadMsg(1, 77, "a")}}, Options{Version: "test"})
	for _, id := range []string{"abc", "0", "-1", "", "77x"} {
		if res := call(t, cs, "read_thread", map[string]any{"thread_id": id}); !res.IsError {
			t.Errorf("thread_id %q: atteso IsError", id)
		}
	}
}

// Piano 14, Wave 4 (punto 2): oltre maxThreadMessages si restituiscono i più recenti,
// e gli omessi sono dichiarati.
func TestReadThreadLimitsMessageCount(t *testing.T) {
	var msgs []*msg.Message
	for i := 1; i <= 25; i++ {
		msgs = append(msgs, threadMsg(uint32(i), 77, "c"))
	}
	cs := connect(t, &mail.Mock{Msgs: msgs}, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "read_thread", map[string]any{"thread_id": "77"}))
	got, _ := data["messages"].([]any)
	if len(got) != maxThreadMessages {
		t.Fatalf("messaggi = %d, attesi %d", len(got), maxThreadMessages)
	}
	if data["messages_omitted"] != float64(25-maxThreadMessages) || data["truncated"] != true {
		t.Fatalf("omessi = %v, truncated = %v", data["messages_omitted"], data["truncated"])
	}
	last, _ := got[len(got)-1].(map[string]any)
	if last["id"] != "INBOX:25" {
		t.Fatalf("ultimo messaggio = %v, atteso INBOX:25 (il più recente)", last["id"])
	}
}

// Piano 14, Wave 4 (punto 2): il corpo di ogni messaggio del thread è limitato.
func TestReadThreadBodyPerMessageLimited(t *testing.T) {
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{threadMsg(1, 77, strings.Repeat("b", 20000))}}, Options{Version: "test"})
	_, data := unwrap(t, call(t, cs, "read_thread", map[string]any{"thread_id": "77"}))
	msgs, _ := data["messages"].([]any)
	first, _ := msgs[0].(map[string]any)
	if b, _ := first["body"].(string); len(b) != defaultBodyLen || first["body_truncated"] != true {
		t.Fatalf("corpo di %d byte, troncato=%v; atteso %d e true", len(b), first["body_truncated"], defaultBodyLen)
	}
}

// Piano 14, Wave 4 (punto 3): label utente e ID Gmail (decimali) nei risultati di
// lista e in read_message, dentro il marcatore.
func TestGmailFieldsInListAndRead(t *testing.T) {
	m := msgAt(1, "s", "corpo")
	m.Ref.GmailID = 1234
	m.Ref.ThreadID = 99
	m.Ref.Labels = []string{"Lavoro", "Fattura"}
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{m}}, Options{Version: "test"})

	_, dl := unwrap(t, call(t, cs, "list_recent", nil))
	row, _ := dl["items"].([]any)[0].(map[string]any)
	if row["gmail_id"] != "1234" || row["thread_id"] != "99" || len(row["labels"].([]any)) != 2 {
		t.Fatalf("riga lista = %v", row)
	}

	_, dr := unwrap(t, call(t, cs, "read_message", map[string]any{"id": "INBOX:1"}))
	if dr["gmail_id"] != "1234" || dr["thread_id"] != "99" || len(dr["labels"].([]any)) != 2 {
		t.Fatalf("read_message = %v", dr)
	}
}
