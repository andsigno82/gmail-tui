package richtext

import (
	"fmt"
	"strings"
	"testing"
)

// wantUnsafe fissa l'insieme dei caratteri tolti dal filtro unico (revisione
// Piano 15, punto 4): controlli Cc e formato Cf, come da Unicode del toolchain
// pinnato. Se un aggiornamento di Go cambia l'insieme, il test lo segnala e va
// riletto (nuovi Cf sono nuovi caratteri invisibili).
const wantUnsafe = "0000-001F 007F-009F 00AD-00AD 0600-0605 061C-061C 06DD-06DD 070F-070F " +
	"0890-0891 08E2-08E2 180E-180E 200B-200F 202A-202E 2060-2064 2066-206F FEFF-FEFF " +
	"FFF9-FFFB 110BD-110BD 110CD-110CD 13430-1343F 1BCA0-1BCA3 1D173-1D17A E0001-E0001 E0020-E007F "

func TestUnsafeInsiemeFisso(t *testing.T) {
	var b strings.Builder
	start := rune(-1)
	for r := rune(0); r <= 0x110000; r++ {
		u := r <= 0x10FFFF && Unsafe(r)
		if u && start < 0 {
			start = r
		}
		if !u && start >= 0 {
			fmt.Fprintf(&b, "%04X-%04X ", start, r-1)
			start = -1
		}
	}
	if got := b.String(); got != wantUnsafe {
		t.Errorf("insieme Unsafe cambiato:\n got  %s\n want %s", got, wantUnsafe)
	}
	// Lo spazio, le lettere accentate e i simboli restano.
	for _, r := range []rune{' ', 'e', 0xE9, 0x2764} {
		if Unsafe(r) {
			t.Errorf("Unsafe(%U) = true", r)
		}
	}
}

// TestCleanLineEHref: i due filtri derivati da Unsafe.
func TestCleanLineEHref(t *testing.T) {
	rlo, zw, esc, bel := string(rune(0x202E)), string(rune(0x200B)), string(rune(0x1B)), string(rune(7))
	if got := CleanLine(" a" + rlo + "\tb \n" + zw + "c" + esc + " "); got != "a b c" {
		t.Errorf("CleanLine = %q", got)
	}
	if got := CleanHref("https://a.example/" + zw + "x y" + bel); got != "https://a.example/xy" {
		t.Errorf("CleanHref = %q", got)
	}
}
