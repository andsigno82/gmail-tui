package mcpserver

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// manyMsgs crea n messaggi nella INBOX.
func manyMsgs(n int) []*msg.Message {
	out := make([]*msg.Message, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, msgAt(uint32(i), "s", ""))
	}
	return out
}

// Piano 14, fix review (1): con più risultati del limit, search_messages dichiara
// has_more e next_offset: l'agente non deve credere di avere tutto.
func TestSearchReportsHasMoreBeyondLimit(t *testing.T) {
	cs := connect(t, &mail.Mock{Msgs: manyMsgs(50)}, Options{Version: "test"})

	_, d1 := unwrap(t, call(t, cs, "search_messages", map[string]any{"query": "", "limit": 20}))
	if items, _ := d1["items"].([]any); len(items) != 20 {
		t.Fatalf("prima pagina: %d righe, attese 20", len(items))
	}
	if d1["has_more"] != true || d1["next_offset"] != float64(20) {
		t.Fatalf("prima pagina: has_more=%v next_offset=%v, attesi true e 20", d1["has_more"], d1["next_offset"])
	}

	_, d2 := unwrap(t, call(t, cs, "search_messages", map[string]any{"query": "", "limit": 20, "offset": 40}))
	if items, _ := d2["items"].([]any); len(items) != 10 {
		t.Fatalf("ultima pagina: %d righe, attese 10", len(items))
	}
	if d2["has_more"] != false {
		t.Fatalf("ultima pagina: has_more=%v, atteso false", d2["has_more"])
	}
}

// Piano 14, fix review (1): list_recent con pagina piena dichiara has_more (euristica:
// pagina piena = possibile seguito); la pagina successiva vuota chiude.
func TestListRecentFullPageHasMore(t *testing.T) {
	cs := connect(t, &mail.Mock{Msgs: manyMsgs(20)}, Options{Version: "test"})

	_, d1 := unwrap(t, call(t, cs, "list_recent", map[string]any{"limit": 20}))
	if d1["has_more"] != true || d1["next_offset"] != float64(20) {
		t.Fatalf("pagina piena: has_more=%v next_offset=%v", d1["has_more"], d1["next_offset"])
	}
	_, d2 := unwrap(t, call(t, cs, "list_recent", map[string]any{"limit": 20, "offset": 20}))
	if items, _ := d2["items"].([]any); len(items) != 0 || d2["has_more"] != false {
		t.Fatalf("pagina vuota: righe=%d has_more=%v", len(items), d2["has_more"])
	}
}
