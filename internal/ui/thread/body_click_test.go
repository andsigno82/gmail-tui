package thread

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
	"github.com/andsigno82/gmail-tui/internal/ui/render"
)

// Piano 15, Task 14: zone cliccabili del corpo a blocchi (link, bottone) e dei chip
// allegati; il thread produce messaggi per l'app, l'apertura vera e del Task 16.

func TestClickSuBottoneProduceButtonClickMsg(t *testing.T) {
	m := htmlModel(t, 80, 40)
	row, col := rowOf(t, m, "Vedi ordine")
	y := headerLines + row

	z, ok := m.ZoneAt(col, y)
	if !ok || z.Kind != render.ZoneButton || z.URL != "https://shop.example.com/ordine" {
		t.Fatalf("ZoneAt sul bottone = %+v,%v", z, ok)
	}
	if u, ok := m.LinkAt(col, y); !ok || u != z.URL {
		t.Errorf("LinkAt sul bottone = (%q,%v)", u, ok)
	}
	// Il clic passa da Update e produce il messaggio per l'app, senza aprire nulla.
	opened := false
	orig := openLink
	openLink = func(string) error { opened = true; return nil }
	t.Cleanup(func() { openLink = orig })

	_, cmd := m.Update(tea.MouseClickMsg{X: col, Y: y, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("clic sul bottone: nessuna Cmd")
	}
	got, ok := cmd().(ButtonClickMsg)
	if !ok || got.URL != "https://shop.example.com/ordine" {
		t.Errorf("messaggio = %#v, voglio ButtonClickMsg con l'URL", got)
	}
	if opened {
		t.Errorf("il thread non deve aprire il bottone (e del Task 16)")
	}
}

func TestClickSuChipAllegato(t *testing.T) {
	m := htmlModel(t, 80, 40)
	top := headerLines + m.bodyHeight() + 1 // dopo il corpo e la riga vuota

	for i, name := range []string{"report.pdf", "dati.csv"} {
		z, ok := m.ZoneAt(2, top+i)
		if !ok || z.Kind != render.ZoneAttachment || z.Index != i {
			t.Fatalf("chip %s: ZoneAt = %+v,%v", name, z, ok)
		}
		if _, ok := m.LinkAt(2, top+i); ok {
			t.Errorf("il chip %s non ha URL: LinkAt deve dire no", name)
		}
		cmd, ok := m.LinkClick(2, top+i)
		if !ok {
			t.Fatalf("LinkClick sul chip %s", name)
		}
		if got, _ := cmd().(AttachmentClickMsg); got.Index != i || got.ID != m.Ref().ID {
			t.Errorf("messaggio = %+v, voglio Index %d e ID del messaggio", got, i)
		}
	}
	if _, ok := m.ZoneAt(2, top+2); ok {
		t.Errorf("riga dopo l'ultimo chip classificata come zona")
	}
	if _, ok := m.ZoneAt(2, top-1); ok {
		t.Errorf("la riga vuota prima dei chip e una zona")
	}
}

// Con la command line aperta la riga extra sta dopo i chip: le coordinate dei chip
// non cambiano, il viewport si accorcia.
func TestChipConCommandLineAperta(t *testing.T) {
	m := htmlModel(t, 80, 40)
	m, _ = m.Update(pressKey(":"))
	top := headerLines + m.bodyHeight() + 1
	if z, ok := m.ZoneAt(2, top); !ok || z.Kind != render.ZoneAttachment {
		t.Errorf("chip non trovato con la command line aperta: %+v,%v", z, ok)
	}
}

// Il clic sul titolo di una conversazione continua ad alternare l'espansione con la
// scheda nuova; il clic su un link del corpo HTML dentro la conversazione si apre.
func TestConversazioneConCorpoHTMLClickRowELink(t *testing.T) {
	full := testMsg()
	full.BodyHTML = htmlBody
	other := testMsg()
	other.Ref.ID = msg.NewID("INBOX", 8)
	other.Ref.Date = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	m := NewConversation(full.Ref, []*msg.Message{other, full}, testStyles(t))
	m.SetSize(80, 40)

	// Titolo del primo messaggio (compresso, riga 0): lo seleziona ed espande.
	if ok, _ := m.ClickRow(headerLines); !ok {
		t.Fatal("ClickRow sul titolo non riconosciuto")
	}
	if _, ok := m.LinkAt(0, headerLines); ok {
		t.Errorf("titolo classificato come zona")
	}
	// Il link del secondo messaggio e raggiungibile (selezione tornata sull'ultimo).
	m.sel = 1
	m.refresh()
	row, col := rowOf(t, m, "example.com ora")
	if u, ok := m.LinkAt(col, headerLines+row); !ok || u != "https://www.example.com/x" {
		t.Errorf("link HTML nella conversazione: (%q,%v)", u, ok)
	}
}

// Con lo scorrimento il bottone resta cliccabile alla sua riga reale.
func TestZonaBottoneConScorrimento(t *testing.T) {
	m := htmlModel(t, 80, 14) // basso: il bottone sta in fondo al contenuto
	m.vp.GotoBottom()
	row, col := rowOf(t, m, "Vedi ordine")
	if u, ok := m.LinkAt(col, headerLines+row); !ok || u != "https://shop.example.com/ordine" {
		t.Errorf("bottone dopo lo scorrimento: (%q,%v) alla riga %d", u, ok, row)
	}
}
