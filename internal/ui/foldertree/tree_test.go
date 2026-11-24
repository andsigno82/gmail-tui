package foldertree

import (
	"regexp"
	"strings"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/theme"
	"github.com/andsigno82/gmail-tui/internal/ui/styles"
)

// nomi è la funzione di etichetta di app (folderName) ridotta ai casi del test.
func nomi(b mail.Mailbox) string {
	switch b.Role {
	case "inbox":
		return "Posta in arrivo"
	case "important":
		return "Importanti"
	case "flagged":
		return "Speciali"
	case "sent":
		return "Inviati"
	case "drafts":
		return "Bozze"
	}
	return b.Name
}

func testStyles(t *testing.T) styles.Styles {
	t.Helper()
	th, err := theme.Load("default")
	if err != nil {
		t.Fatalf("tema: %v", err)
	}
	return styles.New(th)
}

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// plainLines toglie le sequenze SGR e divide per riga.
func plainLines(s string) []string {
	return strings.Split(sgrRe.ReplaceAllString(s, ""), "\n")
}

func rowNames(rows []Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Name)
	}
	return out
}

// Piano 15, Task 10: label annidate Lavoro/Clienti e Lavoro/Fornitori → un
// nodo Lavoro non selezionabile con due foglie selezionabili.
func TestBuildAnnidamento(t *testing.T) {
	st := testStyles(t)
	boxes := []mail.Mailbox{
		{Name: "Lavoro/Clienti", Delimiter: "/"},
		{Name: "Lavoro/Fornitori", Delimiter: "/"},
	}
	tr := Build(boxes, nomi)
	out, rows := Render(tr, 0, nil, st)

	if got, want := rowNames(rows), []string{"Lavoro/Clienti", "Lavoro/Fornitori"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("righe selezionabili: got %v, want %v", got, want)
	}
	lines := plainLines(out)
	if len(lines) != 3 {
		t.Fatalf("attese 3 righe (Lavoro + 2 foglie), got %d: %q", len(lines), lines)
	}
	if got := strings.TrimSpace(lines[0]); !strings.HasSuffix(got, "Lavoro") || strings.Contains(got, "(") {
		t.Errorf("nodo intermedio senza contatore: got %q", got)
	}
	if rows[0].Line != 1 || !strings.Contains(lines[1], "Clienti") {
		t.Errorf("Clienti: Line=%d, riga=%q", rows[0].Line, lines[1])
	}
	if rows[1].Line != 2 || !strings.Contains(lines[2], "Fornitori") {
		t.Errorf("Fornitori: Line=%d, riga=%q", rows[1].Line, lines[2])
	}
}

// Delimitatore vuoto: nessuna gerarchia, la label resta una foglia piatta.
func TestBuildDelimitatoreVuotoPiatta(t *testing.T) {
	st := testStyles(t)
	boxes := []mail.Mailbox{{Name: "Lavoro/Clienti", Delimiter: ""}}
	out, rows := Render(Build(boxes, nomi), 0, nil, st)
	lines := plainLines(out)
	if len(lines) != 1 || len(rows) != 1 {
		t.Fatalf("attesa una sola foglia piatta: righe=%q rows=%v", lines, rowNames(rows))
	}
	if !strings.Contains(lines[0], "Lavoro/Clienti") {
		t.Errorf("etichetta piatta: got %q", lines[0])
	}
}

// Ordine: posta in arrivo, speciali (in ordine fisso, senza annidarne il nome
// [Gmail]), poi label in ordine alfabetico (case-insensitive) con i rami annidati.
func TestBuildOrdine(t *testing.T) {
	st := testStyles(t)
	boxes := []mail.Mailbox{
		{Name: "Zeta", Delimiter: "/"},
		{Name: "[Gmail]/Drafts", Role: "drafts", Delimiter: "/"},
		{Name: "alfa", Delimiter: "/"},
		{Name: "Beta/x", Delimiter: "/"},
		{Name: "INBOX", Role: "inbox", Delimiter: "/"},
		{Name: "[Gmail]/Sent Mail", Role: "sent", Delimiter: "/"},
		{Name: "[Gmail]/Important", Role: "important", Delimiter: "/"}, // Piano 12a, Task 6
		{Name: "[Gmail]/Starred", Role: "flagged", Delimiter: "/"},     // Piano 12a, Task 6
	}
	out, rows := Render(Build(boxes, nomi), 0, nil, st)

	want := []string{"INBOX", "[Gmail]/Important", "[Gmail]/Starred", "[Gmail]/Sent Mail", "[Gmail]/Drafts", "alfa", "Beta/x", "Zeta"}
	if got := rowNames(rows); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("ordine: got %v, want %v", got, want)
	}
	if strings.Contains(out, "[Gmail]") {
		t.Errorf("le speciali non vanno annidate sotto [Gmail]: %q", out)
	}
}

// Contatori: solo su INBOX e label (Q15-4), solo se N > 0; un nodo intermedio
// che è anche label (Lavoro) mostra il suo conteggio.
func TestRenderContatori(t *testing.T) {
	st := testStyles(t)
	boxes := []mail.Mailbox{
		{Name: "INBOX", Role: "inbox", Delimiter: "/"},
		{Name: "[Gmail]/Drafts", Role: "drafts", Delimiter: "/"},
		{Name: "Lavoro", Delimiter: "/"},
		{Name: "Lavoro/Clienti", Delimiter: "/"},
		{Name: "Zeta", Delimiter: "/"},
	}
	counts := map[string]int{"INBOX": 3, "Lavoro": 2, "Lavoro/Clienti": 0, "Zeta": 5, "[Gmail]/Drafts": 9}
	out, _ := Render(Build(boxes, nomi), 0, counts, st)
	lines := plainLines(out)

	find := func(sub string) string {
		for _, l := range lines {
			if strings.Contains(l, sub) {
				return l
			}
		}
		t.Fatalf("riga con %q non trovata in %q", sub, lines)
		return ""
	}
	if l := find("Posta in arrivo"); !strings.Contains(l, "(3)") {
		t.Errorf("INBOX senza (3): %q", l)
	}
	if l := find("Lavoro"); !strings.Contains(l, "(2)") {
		t.Errorf("label Lavoro senza (2): %q", l)
	}
	if l := find("Zeta"); !strings.Contains(l, "(5)") {
		t.Errorf("label Zeta senza (5): %q", l)
	}
	if l := find("Bozze"); strings.Contains(l, "(9)") {
		t.Errorf("Bozze non è contabile (Q15-4): %q", l)
	}
	if strings.Contains(out, "(0)") {
		t.Errorf("nessun (0) atteso: %q", out)
	}
}

// Cursore: evidenziata la riga Rows[cursor], con lo stile di selezione del tema.
func TestRenderCursore(t *testing.T) {
	st := testStyles(t)
	boxes := []mail.Mailbox{
		{Name: "Lavoro/Clienti", Delimiter: "/"},
		{Name: "Lavoro/Fornitori", Delimiter: "/"},
	}
	tr := Build(boxes, nomi)

	out, _ := Render(tr, 1, nil, st)
	if !strings.Contains(out, st.ListSelected.Render("Fornitori")) {
		t.Errorf("cursore 1: Fornitori non evidenziata")
	}
	if strings.Contains(out, st.ListSelected.Render("Clienti")) {
		t.Errorf("cursore 1: Clienti evidenziata per errore")
	}

	out, _ = Render(tr, 9, nil, st)
	if strings.Contains(out, st.ListSelected.Render("Fornitori")) || strings.Contains(out, st.ListSelected.Render("Clienti")) {
		t.Errorf("cursore fuori range: nessuna riga evidenziata attesa")
	}
}

// Nomi lunghi: troncati a 40 colonne con "…", niente a capo né overflow.
func TestRenderNomiLunghiTroncati(t *testing.T) {
	st := testStyles(t)
	long := strings.Repeat("a", 60)
	out, rows := Render(Build([]mail.Mailbox{{Name: long, Delimiter: ""}}, nomi), 0, nil, st)
	lines := plainLines(out)
	if len(lines) != 1 || len(rows) != 1 {
		t.Fatalf("attesa una riga: %q", lines)
	}
	if !strings.HasSuffix(lines[0], strings.Repeat("a", 39)+"…") || strings.Contains(lines[0], strings.Repeat("a", 40)) {
		t.Errorf("troncamento: got %q", lines[0])
	}
}

// Rows e Render devono descrivere le stesse righe, con Line che punta alla riga giusta.
func TestRowsCoincidonoConRender(t *testing.T) {
	st := testStyles(t)
	boxes := []mail.Mailbox{
		{Name: "INBOX", Role: "inbox", Delimiter: "/"},
		{Name: "Lavoro/Clienti", Delimiter: "/"},
		{Name: "Lavoro/Fornitori", Delimiter: "/"},
		{Name: "Zeta", Delimiter: "/"},
	}
	tr := Build(boxes, nomi)
	out, rows := Render(tr, 0, nil, st)
	if got := Rows(tr); strings.Join(rowNames(got), "|") != strings.Join(rowNames(rows), "|") {
		t.Fatalf("Rows e Render discordano: %v vs %v", rowNames(got), rowNames(rows))
	}
	lines := plainLines(out)
	for _, r := range rows {
		if r.Line < 0 || r.Line >= len(lines) {
			t.Fatalf("Line fuori range per %q: %d", r.Name, r.Line)
		}
		if last := r.Name[strings.LastIndex(r.Name, "/")+1:]; !strings.Contains(lines[r.Line], last) && r.Name != "INBOX" {
			t.Errorf("Line di %q punta a %q", r.Name, lines[r.Line])
		}
	}
}

// Albero vuoto: nessuna riga, nessuna cartella selezionabile.
func TestRenderVuoto(t *testing.T) {
	st := testStyles(t)
	out, rows := Render(Build(nil, nomi), 0, nil, st)
	if out != "" || len(rows) != 0 {
		t.Errorf("albero vuoto: out=%q rows=%v", out, rowNames(rows))
	}
}
