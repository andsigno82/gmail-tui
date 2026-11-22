package mcpserver

import "testing"

import "github.com/andsigno82/gmail-tui/internal/mail"

// Piano 14, fix review (3): nomi cartella con NUL, CR o LF (o altri controlli)
// sono rifiutati con un errore del tool, sia dal parametro sia dalla label nella query.
func TestFolderWithControlCharsRejected(t *testing.T) {
	cs := connect(t, &mail.Mock{Msgs: manyMsgs(3)}, Options{Version: "test"})

	for _, name := range []string{"INBOX\r\nA001 NOOP", "IN\x00BOX", "a\tb"} {
		if res := call(t, cs, "list_recent", map[string]any{"folder": name}); !res.IsError {
			t.Errorf("list_recent folder %q: atteso IsError", name)
		}
		if res := call(t, cs, "search_messages", map[string]any{"query": "x", "folder": name}); !res.IsError {
			t.Errorf("search folder %q: atteso IsError", name)
		}
	}
	if res := call(t, cs, "search_messages", map[string]any{"query": "label:a\x00b"}); !res.IsError {
		t.Error("label nella query con NUL: atteso IsError")
	}
}
