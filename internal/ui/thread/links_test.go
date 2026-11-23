package thread

import (
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// Richiesta di Andrea (2026-11-23): i link del corpo si aprono con il clic
// (browser di sistema) e sono wrappati in OSC 8 per il cmd+clic nativo.

// ansiLinkRE toglie SGR (colori) e OSC 8 (link), cosi l'indice di colonna e quello del testo visibile.
var ansiLinkRE = regexp.MustCompile(`\x1b\]8;[^\x1b\x07]*(?:\x1b\\|\x07)|\x1b\[[0-9;:]*m`)
var osc8OpenRE = regexp.MustCompile("\x1b\\]8;([^;\x1b\x07]*);([^\x1b\x07]*)\x1b\\\\")

func stripANSI(s string) string { return ansiLinkRE.ReplaceAllString(s, "") }

// testBodyModel: vista singola con il corpo dato, dimensionata a w x h.
func testBodyModel(t *testing.T, body string, w, h int) Model {
	t.Helper()
	full := testMsg()
	full.BodyText = body
	m := newModel(t, full)
	m.SetSize(w, h)
	return m
}

// errTest è l'errore iniettato nel comando di apertura.
var errTest = errors.New("apertura fallita")

// TestURLSpezzatoSuDueRigheRestaUnLink: un URL più lungo della riga va a capo
// ma resta un unico link: stesso URL e stesso id OSC 8 su entrambe le parti.
func TestURLSpezzatoSuDueRigheRestaUnLink(t *testing.T) {
	const url = "https://example.com/un/percorso/molto/lungo/davvero"
	m := testBodyModel(t, "x "+url+" fine", 20, 20)

	var opens []string
	for _, line := range strings.Split(m.vp.View(), "\n") {
		for _, mt := range osc8OpenRE.FindAllStringSubmatch(line, -1) {
			if mt[2] != "" {
				opens = append(opens, mt[1]+"|"+mt[2])
			}
		}
	}
	if len(opens) < 2 {
		t.Fatalf("aperture OSC 8 = %d, voglio almeno 2 (una per riga)\n%q", len(opens), m.vp.View())
	}
	for _, o := range opens {
		if o != opens[0] {
			t.Errorf("aperture diverse: %q e %q", opens[0], o)
		}
	}
	if !strings.HasSuffix(opens[0], "|"+url) {
		t.Errorf("URL dell'OSC 8 = %q, voglio %q", opens[0], url)
	}

	// Il testo visibile dei link (cella per cella) ricompone l'URL intero.
	var joined strings.Builder
	for r := 0; r < len(m.linkRows); r++ {
		y := headerLines + r
		plain := stripANSI(strings.Split(m.vp.View(), "\n")[r])
		for x := 0; x < 20; x++ {
			if u, ok := m.LinkAt(x, y); ok {
				if u != url {
					t.Errorf("link inatteso %q", u)
				}
				joined.WriteRune([]rune(plain)[x])
			}
		}
	}
	if joined.String() != url {
		t.Errorf("testo dei link spezzati = %q, voglio %q", joined.String(), url)
	}
}

// TestClickMappaURLConScorrimento: il clic su una riga scorsa (YOffset > 0)
// trova il link alla riga giusta; la riga sopra non è un link.
func TestClickMappaURLConScorrimento(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("riga " + strings.Repeat("x", 3) + "\n")
	}
	b.WriteString("vai su https://example.com/target ora\n")
	for i := 0; i < 20; i++ {
		b.WriteString("coda\n")
	}
	// Altezza 16: scheda 7 righe + due allegati (3) fuori dal viewport, restano 6.
	m := testBodyModel(t, b.String(), 60, 16)
	m.vp.SetYOffset(35)
	row := 40 // riga del link nel contenuto
	y := headerLines + row - m.vp.YOffset()
	visible := strings.Split(m.vp.View(), "\n")
	if i := row - m.vp.YOffset(); i < 0 || i >= len(visible) {
		t.Fatalf("riga %d fuori dal viewport (offset %d, righe visibili %d)", row, m.vp.YOffset(), len(visible))
	}
	plain := stripANSI(visible[row-m.vp.YOffset()])
	x := strings.Index(plain, "https")
	if x < 0 {
		t.Fatalf("URL non visibile nella riga %d: %q", row, plain)
	}
	if u, ok := m.LinkAt(x, y); !ok || u != "https://example.com/target" {
		t.Errorf("LinkAt(%d,%d) = (%q,%v), voglio il link", x, y, u, ok)
	}
	if _, ok := m.LinkAt(x, y-1); ok {
		t.Errorf("riga sopra al link classificata come link")
	}
	if _, ok := m.LinkAt(0, y); ok {
		t.Errorf("colonna 0 (testo \"vai su\") classificata come link")
	}
}

// TestSanificazioneOSC8: un corpo con sequenze ESC/BEL non può chiudere l'OSC 8
// né iniettarne altre: nella riga resta una sola coppia di sequenze per il link.
func TestSanificazioneOSC8(t *testing.T) {
	m := testBodyModel(t, "guarda https://a.com/\x1b]8;;https://evil.example\x07 fine", 80, 20)
	view := m.vp.View()
	if n := strings.Count(view, "\x1b]8;"); n != 2 {
		t.Errorf("sequenze OSC 8 = %d, voglio 2 (apertura e chiusura)", n)
	}
	for _, mt := range osc8OpenRE.FindAllStringSubmatch(view, -1) {
		if strings.ContainsAny(mt[2], "\x1b\x07") {
			t.Errorf("URL dell'OSC 8 con caratteri di controllo: %q", mt[2])
		}
	}
	if strings.Contains(view, "\x1b]8;;https") {
		t.Errorf("sequenza OSC 8 iniettata dal corpo")
	}
}

// TestClickFuoriDaiLinkNonCambiaNulla: un clic su testo semplice non è un link
// e non tocca la vista (scorrimento invariato, nessuna Cmd).
func TestClickFuoriDaiLinkNonCambiaNulla(t *testing.T) {
	m := testBodyModel(t, "testo semplice\nsecondo rigo https://example.com/x", 80, 20)
	if _, ok := m.LinkAt(2, headerLines); ok {
		t.Fatal("testo semplice classificato come link")
	}
	mm, cmd := m.Update(tea.MouseClickMsg{X: 2, Y: headerLines, Button: tea.MouseLeft})
	if cmd != nil {
		t.Errorf("clic su testo semplice: Cmd inattesa")
	}
	if mm.YOffset() != m.YOffset() {
		t.Errorf("scorrimento cambiato dal clic")
	}
}

// TestClickSuLinkChiamaApertura: il clic su un link produce una Cmd che apre
// l'URL con l'iniettore e riporta LinkOpenedMsg con l'host come destinazione.
func TestClickSuLinkChiamaApertura(t *testing.T) {
	var opened []string
	orig := openLink
	openLink = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openLink = orig })

	m := testBodyModel(t, "vai su https://example.com/target ora", 80, 20)
	x := strings.Index("vai su https://example.com/target ora", "https")
	mm, _ := m.Update(tea.MouseClickMsg{X: x, Y: headerLines, Button: tea.MouseLeft})
	_ = mm
	cmd, ok := m.LinkClick(x, headerLines)
	if !ok || cmd == nil {
		t.Fatalf("LinkClick = (%v, %v), voglio Cmd sul link", cmd, ok)
	}
	res := cmd()
	lo, isLO := res.(LinkOpenedMsg)
	if !isLO {
		t.Fatalf("esito = %T, voglio LinkOpenedMsg", res)
	}
	if len(opened) != 1 || opened[0] != "https://example.com/target" {
		t.Errorf("aperti = %q, voglio l'URL del link", opened)
	}
	if lo.Target != "example.com" || lo.Err != nil {
		t.Errorf("LinkOpenedMsg = %+v, voglio Target example.com senza errore", lo)
	}
}

// TestErroreApertura: un errore del comando diventa l'esito d'errore.
func TestErroreApertura(t *testing.T) {
	orig := openLink
	openLink = func(string) error { return errTest }
	t.Cleanup(func() { openLink = orig })
	m := testBodyModel(t, "vai su https://example.com/t", 80, 20)
	cmd, ok := m.LinkClick(len("vai su "), headerLines)
	if !ok {
		t.Fatal("link non riconosciuto")
	}
	lo, _ := cmd().(LinkOpenedMsg)
	if lo.Err == nil {
		t.Errorf("errore del comando non riportato")
	}
}

// TestConversazioneLinkENonRompeClickRow: nella conversazione il link del corpo
// si apre e il clic sul titolo continua ad alternare l'espansione.
func TestConversazioneLinkENonRompeClickRow(t *testing.T) {
	full := testMsg()
	full.BodyText = "qui https://example.com/conv"
	other := testMsg()
	other.Ref.ID = msg.NewID("INBOX", 8)
	other.Ref.Date = time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	m := NewConversation(full.Ref, []*msg.Message{other, full}, testStyles(t))
	m.SetSize(80, 30)

	// Titolo del primo messaggio (compresso): ClickRow lo espande.
	if ok, _ := m.ClickRow(headerLines); !ok {
		t.Fatal("ClickRow sul titolo non riconosciuto")
	}
	if _, ok := m.LinkAt(0, headerLines); ok {
		t.Errorf("titolo classificato come link")
	}
}

// TestStileLinkSottolineato: il link è sottolineato e usa lo slot accent.
func TestStileLinkSottolineato(t *testing.T) {
	st := testStyles(t)
	if !st.Link.GetUnderline() {
		t.Error("stile link senza sottolineatura")
	}
}
