package thread

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Piano 8, Task 10: vista conversazione stile Gmail. Quattro messaggi in ordine
// cronologico: A letto, B non letto (in mezzo), C letto, D letto (ultimo).

// convFixture restituisce i quattro messaggi della conversazione di prova, dal
// più vecchio al più recente. Solo B è non letto.
func convFixture() []*msg.Message {
	base := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	mk := func(uid uint32, from, body string, hours int, unread bool) *msg.Message {
		return &msg.Message{
			Ref: msg.MessageRef{
				ID: msg.NewID("INBOX", uid), From: from, Subject: "piano settimana",
				Date: base.Add(time.Duration(hours) * time.Hour), ThreadID: 42, Unread: unread,
			},
			To:       []string{"tu@example.com"},
			BodyText: body,
		}
	}
	return []*msg.Message{
		mk(1, "a@x.dev", "primo corpo", 0, false),
		mk(2, "b@x.dev", "secondo corpo", 1, true),
		mk(3, "c@x.dev", "terzo corpo", 2, false),
		mk(4, "d@x.dev", "quarto corpo", 3, false),
	}
}

// hasBodyLine dice se una riga dello schermo è esattamente il corpo s: il corpo
// espanso compare così, mentre il titolo di un messaggio compresso lo riporta
// solo come anteprima dentro la sua riga.
func hasBodyLine(view, s string) bool {
	for _, ln := range strings.Split(view, "\n") {
		if strings.TrimSpace(ln) == s {
			return true
		}
	}
	return false
}

// TestConversazioneEspandeUltimoENonLetti: i letti in mezzo sono compressi su una
// riga (titolo con mittente e data), l'ultimo e il non letto sono espansi.
func TestConversazioneEspandeUltimoENonLetti(t *testing.T) {
	c := convFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	v := m.View()

	if !hasBodyLine(v, "secondo corpo") {
		t.Errorf("il messaggio non letto deve essere espanso:\n%s", v)
	}
	if !hasBodyLine(v, "quarto corpo") {
		t.Errorf("l'ultimo messaggio deve essere espanso:\n%s", v)
	}
	if hasBodyLine(v, "primo corpo") || hasBodyLine(v, "terzo corpo") {
		t.Errorf("i letti in mezzo devono essere compressi (niente corpo):\n%s", v)
	}
	if !strings.Contains(v, "▸ a@x.dev") || !strings.Contains(v, "▸ c@x.dev") {
		t.Errorf("i letti compressi devono avere la riga di titolo con mittente:\n%s", v)
	}
}

// TestSelezioneDefaultSullUltimo: r/f, o e :save agiscono sul messaggio
// selezionato, che di default è l'ultimo della conversazione.
func TestSelezioneDefaultSullUltimo(t *testing.T) {
	c := convFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	if got := m.Ref().ID; got != c[3].Ref.ID {
		t.Errorf("selezione di default = %s, voglio l'ultimo %s", got, c[3].Ref.ID)
	}
	if m.Msg() != c[3] {
		t.Errorf("Msg() deve essere il messaggio selezionato (l'ultimo)")
	}
}

// TestJKMuoveFraMessaggiEInvioEspandeComprime: j/k selezionano il messaggio
// (non scorrono le righe), l'header segue la selezione, invio alterna
// l'espansione del messaggio selezionato.
func TestJKMuoveFraMessaggiEInvioEspandeComprime(t *testing.T) {
	c := convFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)

	m, _ = m.Update(pressKey("k"))
	m, _ = m.Update(pressKey("k"))
	if got := m.Ref().ID; got != c[1].Ref.ID {
		t.Fatalf("dopo due k la selezione = %s, voglio %s", got, c[1].Ref.ID)
	}
	m, _ = m.Update(pressKey("k"))
	if got := m.Ref().ID; got != c[0].Ref.ID {
		t.Fatalf("dopo tre k la selezione = %s, voglio il primo", got)
	}
	m, _ = m.Update(pressKey("j"))
	m, _ = m.Update(pressKey("j"))
	if got := m.Ref().ID; got != c[2].Ref.ID {
		t.Fatalf("dopo k k j j la selezione = %s, voglio %s", got, c[2].Ref.ID)
	}
	if !strings.Contains(m.View(), "c@x.dev") {
		t.Errorf("l'header deve mostrare il messaggio selezionato:\n%s", m.View())
	}

	// C è compresso: invio lo espande, un secondo invio lo comprime.
	m, _ = m.Update(pressKey("enter"))
	if !hasBodyLine(m.View(), "terzo corpo") {
		t.Errorf("invio deve espandere il messaggio selezionato:\n%s", m.View())
	}
	m, _ = m.Update(pressKey("enter"))
	if hasBodyLine(m.View(), "terzo corpo") {
		t.Errorf("un secondo invio deve comprimere di nuovo il messaggio")
	}
}

// TestClickSuRigaCompressaEspande: il clic sinistro sul titolo di un messaggio
// compresso lo seleziona ed espande; la riga di titolo del primo messaggio è
// alla riga di schermo headerLines (dopo header e riga vuota).
func TestClickSuRigaCompressaEspande(t *testing.T) {
	c := convFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	m.SetSize(80, 40)

	click := tea.MouseClickMsg{X: 0, Y: headerLines, Button: tea.MouseLeft}
	m, _ = m.Update(click)
	if got := m.Ref().ID; got != c[0].Ref.ID {
		t.Fatalf("clic sul primo titolo: selezione = %s, voglio il primo", got)
	}
	if !hasBodyLine(m.View(), "primo corpo") {
		t.Errorf("il clic deve espandere il messaggio compresso:\n%s", m.View())
	}
}

// TestUnreadIDsEMarkReadSuTutti: MarkRead su una conversazione segna letti
// tutti i messaggi (la radice la usa all'apertura e nell'anteprima).
func TestUnreadIDsEMarkReadSuTutti(t *testing.T) {
	c := convFixture()
	m := NewConversation(c[3].Ref, c, testStyles(t))
	if got := m.UnreadIDs(); len(got) != 1 || got[0] != c[1].Ref.ID {
		t.Fatalf("UnreadIDs = %v, voglio [%s]", got, c[1].Ref.ID)
	}
	m.MarkRead()
	if got := m.UnreadIDs(); len(got) != 0 {
		t.Errorf("dopo MarkRead UnreadIDs = %v, voglio vuoto", got)
	}
	if m.AnchorID() != c[3].Ref.ID {
		t.Errorf("AnchorID = %s, voglio l'ID della riga aperta", m.AnchorID())
	}
}

// TestConversazioneSenzaLetturaSingolaRestaInvariata: un solo messaggio (il
// fallback senza X-GM-EXT-1) resta la vista di prima: nessuna selezione multipla.
func TestConversazioneSenzaLetturaSingolaRestaInvariata(t *testing.T) {
	c := convFixture()[:1]
	m := NewConversation(c[0].Ref, c, testStyles(t))
	m.SetSize(80, 40)
	if m.multi() {
		t.Fatalf("un solo messaggio non è una conversazione")
	}
	if !strings.Contains(m.View(), "primo corpo") {
		t.Errorf("il messaggio singolo deve mostrare il corpo:\n%s", m.View())
	}
}
