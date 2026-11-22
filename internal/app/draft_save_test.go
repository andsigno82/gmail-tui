package app

import (
	"context"
	"errors"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/composer"
)

// replaceFailStore simula il backend quando la bozza è salvata ma la precedente
// non si rimuove: SaveDraft torna l'ID nuovo con errore (contratto IMAP, Piano 9 Task 3).
type replaceFailStore struct{ *mail.Mock }

func (s replaceFailStore) SaveDraft(ctx context.Context, raw []byte, replace []msg.MessageID) (msg.MessageID, error) {
	id, err := s.Mock.SaveDraft(ctx, raw, nil)
	if err != nil {
		return "", err
	}
	return id, errors.New("bozza salvata ma la precedente non rimossa")
}

// TestSaveDraftKeepsNewIDWhenOldRemovalFails (Piano 9, review F): con errore di
// rimozione la copia nuova resta la bozza del composer (draftID aggiornato), così
// il prossimo esc sostituisce quella e non ne crea una terza.
func TestSaveDraftKeepsNewIDWhenOldRemovalFails(t *testing.T) {
	mk := &mail.Mock{Msgs: []*msg.Message{draftMsg(7, []string{"a@x.dev"}, "vecchia", "corpo")}}
	th, err := theme.Load("default")
	if err != nil {
		t.Fatal(err)
	}
	m := New(replaceFailStore{mk}, nil, th)
	mm, _ := step(m, teaSize())
	mm = draftComposer(mm, composer.ComposerConfig{To: "anna@example.dev", Subject: "ciao", Body: "testo"})
	mm.draftID = msg.NewID("[Gmail]/Drafts", 7)
	mm, cmd := step(mm, pressKey("esc"))
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	if mm.draftID == msg.NewID("[Gmail]/Drafts", 7) || mm.draftID == "" {
		t.Errorf("draftID = %q dopo errore di rimozione, voglio l'ID della copia nuova", mm.draftID)
	}
	if mm.currentView() != viewComposer {
		t.Error("il composer deve restare aperto con l'errore")
	}
}
