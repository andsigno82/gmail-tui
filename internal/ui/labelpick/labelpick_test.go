package labelpick

import (
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// Piano 10, Task 7: ogni riga mostra la sua casella; con più righe di maxRows la
// finestra segue il cursore, che resta visibile.
func TestRenderShowsBoxesAndKeepsCursorVisible(t *testing.T) {
	th, err := theme.Load("default")
	if err != nil {
		t.Fatal(err)
	}
	st := styles.New(th)
	rows := []Row{{"☑", "Work"}, {"◪", "Lavoro"}, {"☐", "Newsletter"}, {"☐", "Viaggi"}}
	out := Render(st, "etichette su 2", "filtro: ", rows, 3, 2, "vuoto")
	for _, want := range []string{"☐", "Viaggi"} {
		if !strings.Contains(out, want) {
			t.Errorf("il pannello non mostra %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Work") {
		t.Errorf("la finestra di 2 righe mostra anche Work:\n%s", out)
	}
}

// Piano 10, Task 7: senza righe si mostra il messaggio vuoto, non l'elenco.
func TestRenderEmptyMessage(t *testing.T) {
	th, _ := theme.Load("default")
	out := Render(styles.New(th), "etichette su 1", "filtro: ", nil, 0, 5, "nessuna etichetta")
	if !strings.Contains(out, "nessuna etichetta") {
		t.Errorf("messaggio vuoto assente:\n%s", out)
	}
}
