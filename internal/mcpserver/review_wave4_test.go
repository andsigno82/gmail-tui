package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// realisticBody produce n caratteri di prosa con accenti, virgolette e a capo:
// il caso reale che fa crescere il JSON (escape) più dei caratteri ASCII ripetuti.
func realisticBody(n int) string {
	const para = "Ciao Marco, ti scrivo per confermare l'appuntamento di giovedì alle 15:30. " +
		"\"Importante\": porta il contratto firmato & la ricevuta.\n\n"
	var b strings.Builder
	for b.Len() < n*2 {
		b.WriteString(para)
	}
	return string([]rune(b.String())[:n])
}

// Piano 14, Wave 4 review (punto 1): 20 messaggi con corpi realistici da 8000
// caratteri restano entro 64 KB: nessun errore, e messaggi + omessi = 20.
func TestReadThreadTwentyRealisticBodiesFitLimit(t *testing.T) {
	var msgs []*msg.Message
	for i := 1; i <= 20; i++ {
		m := threadMsg(uint32(i), 77, realisticBody(8000))
		m.Ref.Labels = []string{"Lavoro"}
		m.To = []string{"marco@example.com", "altro@example.com"}
		msgs = append(msgs, m)
	}
	cs := connect(t, &mail.Mock{Msgs: msgs}, Options{Version: "test"})

	res := call(t, cs, "read_thread", map[string]any{"thread_id": "77"})
	if res.IsError {
		t.Fatalf("read_thread su thread lungo in errore: %s", resultText(res))
	}
	if n := len(resultText(res)); n > maxResultBytes {
		t.Fatalf("risposta di %d byte, limite %d", n, maxResultBytes)
	}
	_, data := unwrap(t, res)
	got, _ := data["messages"].([]any)
	omitted, _ := data["messages_omitted"].(float64)
	if float64(len(got))+omitted != 20 {
		t.Fatalf("messaggi %d + omessi %v != 20", len(got), omitted)
	}
	if data["truncated"] != true {
		t.Errorf("truncated = %v, atteso true (corpi tagliati o messaggi omessi)", data["truncated"])
	}
}

// Piano 14, Wave 4 review (punto 3): le label sono limitate a maxLabels ovunque e
// il numero omesso è dichiarato (labels_omitted), in lista e in read_message.
func TestLabelsCappedAndDeclaredEverywhere(t *testing.T) {
	m := msgAt(1, "s", "corpo")
	for i := 0; i < 25; i++ {
		m.Ref.Labels = append(m.Ref.Labels, fmt.Sprintf("etichetta%02d", i))
	}
	cs := connect(t, &mail.Mock{Msgs: []*msg.Message{m}}, Options{Version: "test"})

	_, dl := unwrap(t, call(t, cs, "list_recent", nil))
	row, _ := dl["items"].([]any)[0].(map[string]any)
	if len(row["labels"].([]any)) != maxLabels || row["labels_omitted"] != float64(25-maxLabels) {
		t.Fatalf("lista: labels=%d labels_omitted=%v", len(row["labels"].([]any)), row["labels_omitted"])
	}

	_, dr := unwrap(t, call(t, cs, "read_message", map[string]any{"id": "INBOX:1"}))
	if len(dr["labels"].([]any)) != maxLabels || dr["labels_omitted"] != float64(25-maxLabels) {
		t.Fatalf("read_message: labels=%d labels_omitted=%v", len(dr["labels"].([]any)), dr["labels_omitted"])
	}
}

// vanishReader simula un messaggio sparito tra ThreadMessages e FetchMessage.
type vanishReader struct {
	Reader
	gone msg.MessageID
}

func (v *vanishReader) FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error) {
	if id == v.gone {
		return nil, fmt.Errorf("uid %s sparito: %w", id, mail.ErrMessageNotFound)
	}
	return v.Reader.FetchMessage(ctx, id)
}

// Piano 14, Wave 4 review (punto 5): un messaggio sparito non fa fallire il thread:
// è saltato e contato in messages_omitted.
func TestReadThreadSkipsVanishedMessage(t *testing.T) {
	v := &vanishReader{Reader: &mail.Mock{Msgs: []*msg.Message{
		threadMsg(1, 77, "uno"), threadMsg(2, 77, "due"), threadMsg(3, 77, "tre"),
	}}, gone: msg.NewID("INBOX", 2)}
	cs := connect(t, v, Options{Version: "test"})

	_, data := unwrap(t, call(t, cs, "read_thread", map[string]any{"thread_id": "77"}))
	got, _ := data["messages"].([]any)
	if len(got) != 2 || data["messages_omitted"] != float64(1) || data["truncated"] != true {
		t.Fatalf("messaggi=%d omessi=%v truncated=%v; attesi 2, 1, true", len(got), data["messages_omitted"], data["truncated"])
	}
}
