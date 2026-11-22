package mcpserver

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 14, fix review (2): con troncamento per dimensione, paginando con
// next_offset nessuna riga viene saltata né duplicata.
func TestPagingWithNextOffsetSkipsNothing(t *testing.T) {
	msgs := make([]*msg.Message, 0, 100)
	for i := 1; i <= 100; i++ {
		// '<' si scrive < nel JSON (6 byte): le righe superano 64 KB e il troncamento scatta.
		msgs = append(msgs, msgAt(uint32(i), strings.Repeat("<", 3000), ""))
	}
	cs := connect(t, &mail.Mock{Msgs: msgs}, Options{Version: "test"})

	var ids []string
	offset, pages := 0, 0
	for {
		pages++
		if pages > 100 {
			t.Fatal("la paginazione non termina")
		}
		_, data := unwrap(t, call(t, cs, "list_recent", map[string]any{"limit": 100, "offset": offset}))
		items, _ := data["items"].([]any)
		for _, it := range items {
			id, _ := it.(map[string]any)["id"].(string)
			ids = append(ids, id)
		}
		if data["returned"] != float64(len(items)) {
			t.Fatalf("returned = %v, righe = %d", data["returned"], len(items))
		}
		if data["has_more"] != true {
			break
		}
		next, ok := data["next_offset"].(float64)
		if !ok {
			t.Fatal("truncated senza next_offset")
		}
		if int(next) != offset+len(items) {
			t.Fatalf("next_offset = %v, atteso %d", next, offset+len(items))
		}
		offset = int(next)
	}
	if pages < 2 {
		t.Fatal("il test non ha esercitato il troncamento")
	}
	if len(ids) != 100 {
		t.Fatalf("righe totali = %d, attese 100", len(ids))
	}
	for i, id := range ids {
		if want := string(msg.NewID("INBOX", uint32(i+1))); id != want {
			t.Fatalf("riga %d = %s, attesa %s", i, id, want)
		}
	}
}
