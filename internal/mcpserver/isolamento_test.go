package mcpserver

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 15, Task 17: il server MCP resta in testo semplice. Nessuna dipendenza
// del server (nemmeno transitiva) può essere il renderer: internal/msg/richtext
// e internal/ui/*. Il test legge il grafo con go list -deps.
func TestIsolamentoRenderer(t *testing.T) {
	const pref = "github.com/andsigno82/gmail-tui/internal/"
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", "-f", "{{.ImportPath}}",
		pref+"mcpserver").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps: %v\n%s", err, out)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 2 {
		t.Fatalf("go list -deps: elenco sospetto %v", deps)
	}
	for _, dep := range deps {
		if dep == pref+"msg/richtext" || strings.HasPrefix(dep, pref+"msg/richtext/") ||
			strings.HasPrefix(dep, pref+"ui/") {
			t.Errorf("internal/mcpserver dipende da %s: il server deve restare in testo semplice", dep)
		}
	}
}

// Piano 15, Task 17: il Reader resta in sola lettura e senza UnreadCounts: i
// contatori dei non letti non sono esposti al server MCP.
func TestReaderSenzaUnreadCounts(t *testing.T) {
	if _, ok := reflect.TypeOf((*Reader)(nil)).Elem().MethodByName("UnreadCounts"); ok {
		t.Fatal("Reader espone UnreadCounts: il server MCP non deve vedere i contatori")
	}
}

// Piano 15, Task 17: read_message restituisce il testo semplice (BodyText) anche
// per un messaggio con HTML ricco come quelli della demo: nessun markup nel corpo.
func TestReadMessageTestoSempliceConHTMLRicco(t *testing.T) {
	mm := msgAt(1, "Newsletter", "Testo semplice del corpo.\n\n- uno\n- due")
	mm.BodyHTML = `<p><a href="https://example.com/conferma" style="padding:8px">Conferma iscrizione</a></p><table><tr><td>1</td></tr></table>`
	mm.HasHTML = true
	m := &mail.Mock{Msgs: []*msg.Message{mm}}
	cs := connect(t, m, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "read_message", map[string]any{"id": "INBOX:1"}))
	body, _ := data["body"].(string)
	if !strings.Contains(body, "Testo semplice del corpo.") {
		t.Fatalf("corpo = %q, voglio il testo semplice", body)
	}
	if strings.ContainsAny(body, "<>") || strings.Contains(body, "Conferma iscrizione") {
		t.Errorf("corpo = %q: markup o testo dell'HTML nel risultato MCP", body)
	}
}
