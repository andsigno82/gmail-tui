package mcpserver

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 14, fix review 4: con 20 destinatari e 50 allegati lunghi, la risposta
// resta entro 64 KB anche con max_chars=0; i metadati in eccesso sono omessi e
// dichiarati, non causa di errore. Caratteri a 3 byte per superare il limite.
func TestReadMessageMetadataBudgetNeverFailsWithoutBody(t *testing.T) {
	big := strings.Repeat("€", 1000) // 1000 rune da 3 byte
	m := msgAt(1, "s", "corpo")
	for i := 0; i < 20; i++ {
		m.To = append(m.To, big+"@x.it")
	}
	for i := 0; i < 50; i++ {
		m.Attachments = append(m.Attachments, msg.Attachment{Filename: big, ContentType: big, Size: 1})
	}
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{m}}, Options{Version: "test"})

	res := call(t, cs, "read_message", map[string]any{"id": "INBOX:1", "max_chars": 0})
	if res.IsError {
		t.Fatalf("risposta in errore: %s", resultText(res))
	}
	if n := len(resultText(res)); n > maxResultBytes {
		t.Fatalf("risposta di %d byte, limite %d", n, maxResultBytes)
	}
	_, data := unwrap(t, res)
	if n, _ := data["attachments_omitted"].(float64); n <= 0 {
		t.Errorf("attachments_omitted = %v, atteso > 0", data["attachments_omitted"])
	}
	if data["truncated"] != true {
		t.Errorf("truncated = %v, atteso true", data["truncated"])
	}
}

// Piano 14, fix review 6: il troncamento di un campo è dichiarato con "…[+N]".
func TestFieldTruncationIsDeclared(t *testing.T) {
	m := msgAt(1, strings.Repeat("x", 1000), "")
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{m}}, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "read_message", map[string]any{"id": "INBOX:1"}))
	subj, _ := data["subject"].(string)
	if !strings.HasSuffix(subj, "…[+744]") {
		t.Fatalf("oggetto troncato senza dichiarazione: ...%q", subj[max(0, len(subj)-20):])
	}
}

// Review (budget): il costo JSON di ogni destinatario e allegato include
// l'indentazione di MarshalIndent; con molti elementi piccoli il budget
// sottostimava la risposta e read_message falliva anche per i soli metadati.
func TestReadMessageManySmallRecipientsAndAttachmentsFitBudget(t *testing.T) {
	m := msgAt(1, "s", "corpo")
	for i := 0; i < 9000; i++ {
		m.To = append(m.To, "a@b.c")
	}
	for i := 0; i < 2000; i++ {
		m.Attachments = append(m.Attachments, msg.Attachment{Filename: "a", ContentType: "b", Size: 1})
	}
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{m}}, Options{Version: "test"})

	res := call(t, cs, "read_message", map[string]any{"id": "INBOX:1", "max_chars": 1})
	if res.IsError {
		t.Fatalf("risposta in errore per soli metadati: %s", resultText(res))
	}
	if n := len(resultText(res)); n > maxResultBytes {
		t.Fatalf("risposta di %d byte, limite %d", n, maxResultBytes)
	}
}
