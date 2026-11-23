package app

// Piano 15, Task 16: clic su un bottone (ButtonClickMsg) o su un link con Warn
// (LinkWarnMsg). Apertura nel browser come i link; con Warn prima la conferma.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/andsigno82/gmail-tui/internal/ui/thread"
	"github.com/andsigno82/gmail-tui/internal/ui/toast"
)

// stubOpenURL sostituisce l'apertura del browser: registra gli URL, non apre mai nulla.
func stubOpenURL(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	orig := openURL
	openURL = func(u string) error { opened = append(opened, u); return nil }
	t.Cleanup(func() { openURL = orig })
	return &opened
}

func TestClicBottoneApreNelBrowserConToast(t *testing.T) {
	opened := stubOpenURL(t)
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{})
	mm, cmd := step(m, thread.ButtonClickMsg{URL: "https://shop.example.com/ordine"})
	if cmd == nil {
		t.Fatal("ButtonClickMsg: nessuna Cmd di apertura")
	}
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	if len(*opened) != 1 || (*opened)[0] != "https://shop.example.com/ordine" {
		t.Errorf("aperti = %q", *opened)
	}
	assertToast(t, mm, toast.Success, "aperto nel browser: shop.example.com")
}

func TestClicBottoneWarnChiedeConfermaNellaBarra(t *testing.T) {
	opened := stubOpenURL(t)
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{})
	mm, cmd := step(m, thread.ButtonClickMsg{URL: "https://evil.example.net/pay", Warn: true, Shown: "paypal.com/paga"})
	if cmd != nil || len(*opened) != 0 {
		t.Fatalf("con Warn nulla si apre prima della conferma (cmd=%v, aperti=%q)", cmd, *opened)
	}
	want := "il link porta a evil.example.net, non a paypal.com: apri? s/esc"
	if bar := plainBar(mm); !strings.Contains(bar, want) {
		t.Errorf("barra = %q, voglio %q", bar, want)
	}
}

func TestConfermaLinkWarnSApreEscAnnulla(t *testing.T) {
	opened := stubOpenURL(t)
	m := newTestModel(t)
	m, _ = step(m, msgsLoaded{})
	msgWarn := thread.LinkWarnMsg{URL: "https://evil.example.net/login", Shown: "www.paypal.com"}

	// esc annulla: nessuna apertura.
	mm, _ := step(m, msgWarn)
	mm, cmd := step(mm, pressKey("esc"))
	if cmd != nil || mm.linkAsk != nil || len(*opened) != 0 {
		t.Fatalf("esc: cmd=%v ask=%v aperti=%q", cmd, mm.linkAsk, *opened)
	}

	// un tasto qualunque e ignorato, la conferma resta.
	mm, _ = step(m, msgWarn)
	mm, cmd = step(mm, pressKey("x"))
	if cmd != nil || mm.linkAsk == nil {
		t.Fatalf("tasto estraneo: cmd=%v ask=%v", cmd, mm.linkAsk)
	}
	// il mouse e ignorato con la conferma aperta.
	mm, cmd = step(mm, tea.MouseClickMsg{X: 3, Y: 3, Button: tea.MouseLeft})
	if cmd != nil || mm.linkAsk == nil {
		t.Fatalf("mouse con conferma aperta: cmd=%v ask=%v", cmd, mm.linkAsk)
	}

	// s apre l'URL vero.
	mm, cmd = step(mm, pressKey("s"))
	if mm.linkAsk != nil || cmd == nil {
		t.Fatalf("s: ask=%v cmd=%v", mm.linkAsk, cmd)
	}
	for _, ms := range runCmd(cmd) {
		mm, _ = step(mm, ms)
	}
	if len(*opened) != 1 || (*opened)[0] != "https://evil.example.net/login" {
		t.Errorf("aperti = %q", *opened)
	}
	assertToast(t, mm, toast.Success, "aperto nel browser: evil.example.net")
}
