package mcpserver

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// writeVerbs sono le parole che un tool di scrittura porterebbe nel nome
// (Piano 14, D14): nessun tool registrato le contiene.
var writeVerbs = []string{"send", "draft", "archiv", "label", "trash", "delet", "mark", "star", "save", "move", "flag", "expunge"}

// Piano 14, Task 6 (D14): nessun nome di tool evoca una scrittura.
func TestToolNamesHaveNoWriteVerbs(t *testing.T) {
	cs := connect(t, &mail.Mock{}, Options{Version: "test"})
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range res.Tools {
		for _, v := range writeVerbs {
			if strings.Contains(strings.ToLower(tool.Name), v) {
				t.Errorf("tool %q contiene il verbo di scrittura %q", tool.Name, v)
			}
		}
	}
}

// Piano 14, Task 6: chiamare tutti i tool, anche con argomenti estremi, non
// cambia lo stato del Mock: niente invii, niente flag, niente spostamenti.
func TestAllToolsLeaveMailboxUntouched(t *testing.T) {
	m := &mail.Mock{Msgs: []*msg.Message{msgAt(1, "uno", "corpo"), msgAt(2, "due", "altro")}}
	m.Msgs[0].Ref.Starred = false
	cs := connect(t, m, Options{Version: "test"})

	call(t, cs, "list_folders", nil)
	call(t, cs, "list_recent", map[string]any{"limit": 100})
	call(t, cs, "search_messages", map[string]any{"query": "is:unread"})
	call(t, cs, "read_message", map[string]any{"id": "INBOX:1"})
	call(t, cs, "read_message", map[string]any{"id": "INBOX:2", "max_chars": 5})

	if len(m.Sent) != 0 {
		t.Errorf("Sent = %d messaggi: nessun invio atteso", len(m.Sent))
	}
	if len(m.Msgs) != 2 {
		t.Errorf("messaggi = %d, attesi 2", len(m.Msgs))
	}
	for _, mm := range m.Msgs {
		if !mm.Ref.Unread || mm.Ref.Starred {
			t.Errorf("%s: stato modificato (unread=%v starred=%v)", mm.Ref.ID, mm.Ref.Unread, mm.Ref.Starred)
		}
	}
}

// Piano 14, Task 6 (M6): la password non compare in nessun risultato, anche quando
// ogni chiamata fallisce con un errore che la contiene.
func TestNoSecretInAnyToolOutput(t *testing.T) {
	const secret = "pw-SEGRETO-9"
	m := &mail.Mock{Err: errors.New("auth fallita con " + secret), Msgs: []*msg.Message{msgAt(1, "s", "c")}}
	cs := connect(t, m, Options{Version: "test", Secrets: []string{secret}})

	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"list_folders", nil},
		{"list_recent", nil},
		{"search_messages", map[string]any{"query": "x"}},
		{"read_message", map[string]any{"id": "INBOX:1"}},
	} {
		res := call(t, cs, tc.name, tc.args)
		if txt := resultText(res); strings.Contains(txt, secret) {
			t.Errorf("%s: password nel risultato: %q", tc.name, txt)
		}
	}
}

// Piano 14, Task 6 (Task 4): il corpo HTML grezzo non esce mai; solo il testo.
func TestReadMessageNeverExposesHTML(t *testing.T) {
	m1 := msgAt(1, "s", "testo")
	m1.BodyHTML = "<p>HTML-SEGRETO-7</p>"
	m1.HasHTML = true
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{m1}}, Options{Version: "test"})

	res := call(t, cs, "read_message", map[string]any{"id": "INBOX:1"})
	if strings.Contains(resultText(res), "HTML-SEGRETO-7") || strings.Contains(resultText(res), "<p>") {
		t.Fatal("HTML grezzo presente nel risultato")
	}
}

// Piano 14, Task 6 (regola 7): il pacchetto del server non scrive su stdout: il
// canale è del protocollo. Controllo statico sui sorgenti non di test.
func TestPackageNeverWritesToStdout(t *testing.T) {
	forbidden := []string{"fmt.Print(", "fmt.Println(", "fmt.Printf(", "os.Stdout", "println("}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range forbidden {
			if strings.Contains(string(b), p) {
				t.Errorf("%s usa %q: stdout è riservato al protocollo MCP", f, p)
			}
		}
	}
}

// Piano 14, Task 6 (regola 1 di AGENTS.md): il server non importa go-imap.
func TestPackageDoesNotReferenceGoIMAP(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "go-imap") || strings.Contains(string(b), "internal/mail/imap") {
			t.Errorf("%s referenzia IMAP: il server parla solo con MailStore", f)
		}
	}
}
