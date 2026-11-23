package render

import (
	"strings"
	"testing"
)

// Piano 15, Task 14 (lavoro B): la sanificazione e unica (cleanText, safeHref di
// link.go): stesse regole per bottoni e link, anche per i caratteri di formato
// Unicode che le vecchie copie locali non coprivano.
func TestButtonSanificazioneUnica(t *testing.T) {
	// U+00AD (soft hyphen) e U+2060 (word joiner) sono Cf: non devono comparire.
	shy, wj := string(rune(0xAD)), string(rune(0x2060))
	o := Button(btn("Acqu"+shy+"ista"+wj+" ora", "https://a.example/"+wj+"x", "", false), testCtx(t, 40))
	need(t, o, 3)
	if got := plain(o.Lines[1]); !strings.Contains(got, "Acquista ora") {
		t.Errorf("etichetta non sanificata: %q", got)
	}
	if u := o.Zones[0][0].URL; u != "https://a.example/x" {
		t.Errorf("url = %q", u)
	}
	// Host vuoto (solo porta): rifiutato come da safeHref.
	o = Button(btn("x", "https://:8080/", "", false), testCtx(t, 40))
	need(t, o, 3)
	if len(o.Zones[0]) != 0 {
		t.Errorf("host vuoto accettato: %+v", o.Zones[0])
	}
}
