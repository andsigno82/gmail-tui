package thread

import "testing"

// TestCleanHeaderTogliCaratteriDiFormato (revisione Piano 15, punto 4): la scheda
// usa la stessa sanificazione del corpo, quindi anche bidi e zero-width spariscono
// (un From non puo mascherare il mittente con U+202E).
func TestCleanHeaderTogliCaratteriDiFormato(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"bidi", "fattura\u202Efdp.exe", "fatturafdp.exe"},
		{"zero-width", "pay\u200Bpal\uFEFF.com", "paypal.com"},
		{"soft hyphen", "a\u00ADb", "ab"},
		{"controlli", "a\x1b[31mb\x07", "a[31mb"},
		{"a capo e tab in spazio", "Ciao\r\n  mondo\t!", "Ciao mondo !"},
		{"accenti intatti", "perch\u00e9 \u2764", "perch\u00e9 \u2764"},
	}
	for _, tt := range tests {
		if got := cleanHeader(tt.in); got != tt.want {
			t.Errorf("%s: cleanHeader(%q) = %q, atteso %q", tt.name, tt.in, got, tt.want)
		}
	}
}
