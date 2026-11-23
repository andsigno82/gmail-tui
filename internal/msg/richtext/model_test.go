package richtext

import "testing"

// TestBlockZeroValue (Piano 15, Task 1): il valore zero di Block è un
// paragrafo senza run né figli; i parser (Task 4 e 5) contano su questa
// invariante per costruire i blocchi campo per campo.
func TestBlockZeroValue(t *testing.T) {
	var b Block
	if b.Kind != KindParagraph {
		t.Errorf("Kind zero = %d, atteso KindParagraph", b.Kind)
	}
	if len(b.Runs) != 0 || len(b.Children) != 0 || len(b.Rows) != 0 {
		t.Errorf("valore zero con contenuto: %+v", b)
	}
	if b.Ordered || b.Warn || b.Start != 0 || b.Depth != 0 || b.Host != "" || b.Alt != "" {
		t.Errorf("valore zero con campi valorizzati: %+v", b)
	}
}

// TestKindDistinti (Piano 15, Task 1): ogni Kind ha un valore distinto e
// KindParagraph è 0 (invariante del valore zero).
func TestKindDistinti(t *testing.T) {
	kinds := []Kind{
		KindParagraph, KindQuote, KindList, KindTable,
		KindButton, KindImage, KindRule, KindPre,
	}
	seen := map[Kind]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Errorf("Kind duplicato: %d", k)
		}
		seen[k] = true
	}
	if KindParagraph != 0 {
		t.Errorf("KindParagraph = %d, atteso 0", KindParagraph)
	}
}

// TestLimiti (Piano 15, Task 1): fissa i valori di D15-T4; cambiarli è una
// decisione di piano (da tarare nel Task 19), non un refuso.
func TestLimiti(t *testing.T) {
	if MaxInputBytes != 512<<10 {
		t.Errorf("MaxInputBytes = %d, atteso 512 KiB", MaxInputBytes)
	}
	if MaxNodes != 5000 {
		t.Errorf("MaxNodes = %d, atteso 5000", MaxNodes)
	}
	if MaxDepth != 16 {
		t.Errorf("MaxDepth = %d, atteso 16", MaxDepth)
	}
	if MaxTableRows != 200 || MaxTableCols != 20 {
		t.Errorf("tabelle = %dx%d, atteso 200x20", MaxTableRows, MaxTableCols)
	}
}
