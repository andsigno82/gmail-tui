package mcpserver

import "testing"

// Piano 14, fix review 3: i Unicode Tags (ASCII smuggling) non sopravvivono a Sanitize.
func TestSanitizeRemovesTagCharacters(t *testing.T) {
	in := "a\U000E0041\U000E0042b" // lettere tag A e B, testo nascosto
	if got := Sanitize(in); got != "ab" {
		t.Fatalf("Sanitize = %q, atteso %q", got, "ab")
	}
}

// Piano 14, fix review 3: marcature bidi (LRM, RLM, ALM), selettori di variante
// e intervallo 'Variation Selectors Supplement' vengono rimossi.
func TestSanitizeRemovesBidiMarksAndVariationSelectors(t *testing.T) {
	in := "x\u200Ey\u200Fz\u061Cw\uFE0Fv\U000E0100u"
	if got := Sanitize(in); got != "xyzwvu" {
		t.Fatalf("Sanitize = %q, atteso %q", got, "xyzwvu")
	}
}

// Piano 14, fix review 3: ZWJ e ZWNJ restano, perché servono a sequenze emoji
// (famiglia, ZWJ U+200D) e ad alcuni script (ZWNJ U+200C).
func TestSanitizeKeepsZWJAndZWNJ(t *testing.T) {
	in := "\U0001F468\u200d\U0001F469 a\u200cb"
	if got := Sanitize(in); got != in {
		t.Fatalf("Sanitize = %q, atteso invariato %q", got, in)
	}
}

// Piano 14, fix review 9: un OSC senza terminatore non cancella il testo che
// segue; ESC viene rimosso come controllo, il resto resta.
func TestSanitizeUnterminatedOSCKeepsText(t *testing.T) {
	in := "a\x1b]0;titolo\nb"
	const want = "a]0;titolo\nb"
	if got := Sanitize(in); got != want {
		t.Fatalf("Sanitize = %q, atteso %q", got, want)
	}
}
