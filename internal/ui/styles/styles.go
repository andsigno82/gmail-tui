// Package styles traduce gli slot semantici di internal/theme in stili
// lipgloss v2 per la TUI. Ogni colore deriva ESCLUSIVAMENTE da uno slot del
// tema: mai hex diretti. Ricostruire con New se cambia il tema.
package styles

import (
	"fmt"
	"hash/fnv"
	"image/color"
	"math"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/compat"

	"github.com/andsigno82/gmail-tui/internal/theme"
)

// Styles raccoglie gli stili della TUI per il tema corrente.
type Styles struct {
	ListNormal      lipgloss.Style // riga normale della lista messaggi
	ListUnread      lipgloss.Style // riga non letta (marcatore ● e subject)
	ListStarred     lipgloss.Style // riga con stella (★)
	ListDate        lipgloss.Style // metadati riga: data relativa, mittente
	ListSelected    lipgloss.Style // riga sotto il cursore
	ListCheck       lipgloss.Style // casella di selezione ☐/☑ a inizio riga (Piano 10, Task 2): contrasto WCAG sullo sfondo del tema
	ListCheckSel    lipgloss.Style // casella ☐/☑ sulla riga sotto il cursore: sfondo Selection (Piano 10, Task 2)
	ListTint        lipgloss.Style // riga spuntata (x) non sotto il cursore: testo normale su tinta (richiesta di Andrea, 2026-11-23)
	ListTintUnread  lipgloss.Style // non letto sulla tinta: grassetto, slot unread
	ListTintStarred lipgloss.Style // stella sulla tinta: slot starred
	ListTintDate    lipgloss.Style // data e +N sulla tinta: slot muted
	ListTintCheck   lipgloss.Style // casella ☑ sulla tinta: slot accent, grassetto
	SearchInput     lipgloss.Style // prompt/cursore della ricerca
	StatusErr       lipgloss.Style // barra colorata: conferme modali (invio, scarta, cestino) e errori del wizard
	StatusBar       lipgloss.Style // status bar: stato normale
	HelpView        lipgloss.Style // barra help contestuale / overlay
	ThreadHeader    lipgloss.Style // intestazione del thread
	ThreadHeaderKey lipgloss.Style // etichette dell'header (Da:/A:/Oggetto:)
	Quoted          lipgloss.Style // righe citate attenuate
	Link            lipgloss.Style // link nel corpo del messaggio: slot accent sottolineato (Richiesta di Andrea, 2026-11-23)
	Border          lipgloss.Style // riquadri (viewport, overlay)
	Spinner         lipgloss.Style // spinner durante il caricamento
	Divider         lipgloss.Style // separatore lista/anteprima a due colonne (Piano 7.2, Task 4)
	DividerFocus    lipgloss.Style // separatore con il focus sull'anteprima (slot primary)
	ToastInfo       lipgloss.Style // toast informativo: bordo accent (Piano 7.3, Task 1)
	ToastSuccess    lipgloss.Style // toast di esito positivo: bordo success
	ToastError      lipgloss.Style // toast d'errore: bordo error (resta finché un tasto)
	// Labels è il palette delle etichette utente (Piano 8, Task 9, D8-5): solo
	// slot con contrasto WCAG >= 4.5:1 sul bg del tema, in entrambe le varianti.
	Labels []lipgloss.Style
	// LabelsTint è Labels sulla tinta della riga spuntata, stesso ordine e indice
	// (richiesta di Andrea, 2026-11-23).
	LabelsTint []lipgloss.Style

	// Piano 15, Task 2: campi del contratto per i Task 6-16; valorizzati nel
	// Task 9 con colori derivati dagli slot del tema (vedi New).
	LinkWarn      lipgloss.Style   // link con dominio diverso dal testo mostrato (D15-07)
	LinkDashed    lipgloss.Style   // link sottolineato tratteggiato (D15-07)
	Button        lipgloss.Style   // bottone a riquadro (D15-01)
	ButtonFocus   lipgloss.Style   // bottone con il focus
	ButtonHost    lipgloss.Style   // host di destinazione accanto al bottone
	ChipAttach    lipgloss.Style   // chip allegato (D15-06)
	ChipImage     lipgloss.Style   // chip immagine con testo alternativo (D15-08)
	QuoteBar      []lipgloss.Style // barra della citazione, un elemento per livello (D15-03)
	TableBorder   lipgloss.Style   // bordo delle tabelle (D15-02)
	TableHeader   lipgloss.Style   // intestazione delle tabelle
	TableCell     lipgloss.Style   // celle delle tabelle
	BadgeAvatar   []lipgloss.Style // badge delle iniziali, un colore per indice (D15-05)
	FolderCount   lipgloss.Style   // contatore dei non letti per cartella (D15-10)
	FolderTree    lipgloss.Style   // rami dell'albero cartelle (D15-09)
	OverlayBorder lipgloss.Style   // bordo delle finestre sovrapposte (D15-11)
	OverlayShadow lipgloss.Style   // ombra delle finestre sovrapposte: retino ░, primo piano slot border, nessuno sfondo
	StatusSegment []lipgloss.Style // segmenti della barra di stato (D15-12)
	StatusSep     lipgloss.Style   // separatore dei segmenti
	ErrText       lipgloss.Style   // testo d'errore senza sfondo (splash, Piano 15, Task 16b): slot error, contrasto WCAG sul bg
}

// New costruisce gli stili per il tema dato. La variante dark/light di ogni
// slot è risolta adattivamente al rendering dal background del terminale.
func New(t theme.Theme) Styles {
	tint := mescola(t.Bg, t.Selection, frazioneTinta)
	return Styles{
		ListNormal:      lipgloss.NewStyle().Foreground(adaptive(t.Fg)),
		ListUnread:      lipgloss.NewStyle().Foreground(adaptive(t.Unread)).Bold(true),
		ListStarred:     lipgloss.NewStyle().Foreground(adaptive(t.Starred)),
		ListDate:        lipgloss.NewStyle().Foreground(adaptive(t.Muted)),
		ListSelected:    lipgloss.NewStyle().Foreground(onColor(t.Selection, t.Fg, t.Bg)).Background(adaptive(t.Selection)).Bold(true),
		ListCheck:       lipgloss.NewStyle().Foreground(onColor(t.Bg, t.Accent, t.Fg)),
		ListCheckSel:    lipgloss.NewStyle().Foreground(onColor(t.Selection, t.Accent, t.Fg)).Background(adaptive(t.Selection)).Bold(true),
		ListTint:        lipgloss.NewStyle().Foreground(onColor(tint, t.Fg, t.Bg)).Background(adaptive(tint)),
		ListTintUnread:  lipgloss.NewStyle().Foreground(onColor(tint, t.Unread, t.Fg)).Background(adaptive(tint)).Bold(true),
		ListTintStarred: lipgloss.NewStyle().Foreground(onColor(tint, t.Starred, t.Fg)).Background(adaptive(tint)),
		ListTintDate:    lipgloss.NewStyle().Foreground(onColor(tint, t.Muted, t.Fg)).Background(adaptive(tint)),
		ListTintCheck:   lipgloss.NewStyle().Foreground(onColor(tint, t.Accent, t.Fg)).Background(adaptive(tint)).Bold(true),
		SearchInput:     lipgloss.NewStyle().Foreground(adaptive(t.Accent)),
		StatusErr:       lipgloss.NewStyle().Foreground(onColor(t.Error, t.Fg, t.Bg)).Background(adaptive(t.Error)).Bold(true).Padding(0, 1),
		StatusBar:       lipgloss.NewStyle().Foreground(onColor(t.Primary, t.Fg, t.Bg)).Background(adaptive(t.Primary)).Padding(0, 1),
		HelpView:        lipgloss.NewStyle().Foreground(adaptive(t.Help)),
		ThreadHeader:    lipgloss.NewStyle().Foreground(adaptive(t.Primary)).Bold(true).Padding(0, 1),
		ThreadHeaderKey: lipgloss.NewStyle().Foreground(adaptive(t.Muted)),
		Quoted:          lipgloss.NewStyle().Foreground(adaptive(t.Quoted)).Italic(true),
		Link:            lipgloss.NewStyle().Foreground(adaptive(linkSlot(t))).Underline(true),
		Border:          lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(adaptive(t.Border)),
		Spinner:         lipgloss.NewStyle().Foreground(adaptive(t.Accent)).Bold(true),
		Divider:         lipgloss.NewStyle().Foreground(adaptive(t.Border)),
		DividerFocus:    lipgloss.NewStyle().Foreground(adaptive(t.Primary)),
		ToastInfo:       toastStyle(t, t.Accent),
		ToastSuccess:    toastStyle(t, t.Success),
		ToastError:      toastStyle(t, t.Error),
		Labels:          labelStyles(labelSlots(t)),
		LabelsTint:      labelStylesTint(labelSlots(t), tint, t.Fg),

		// Piano 15, Task 9: valori derivati dagli slot del tema (campi del Task 2).
		// Gli stili con sfondo usano onColor (eccezione U1: nero/bianco); i testi
		// senza sfondo usano onColor sul bg del tema, così il contrasto WCAG >= 4.5:1
		// vale in ogni tema, dark e light. Le sfumature (D15-T12) stanno solo nei
		// bordi e nel titolo, calcolate dai consumatori con Blend1D (overlay: primary da
		// OverlayBorder, accent da Spinner): mai negli stili di testo.
		LinkWarn: lipgloss.NewStyle().
			Foreground(onColorPrima(t.Bg, t.Error, t.Warning, linkSlot(t), t.Fg)).
			UnderlineStyle(lipgloss.UnderlineCurly).UnderlineColor(adaptive(t.Error)),
		LinkDashed: lipgloss.NewStyle().
			Foreground(adaptive(linkSlot(t))).
			UnderlineStyle(lipgloss.UnderlineDashed).UnderlineColor(adaptive(linkSlot(t))),
		Button: lipgloss.NewStyle().
			Foreground(onColorPrima(tint, t.Accent, t.Primary, t.Fg)).Background(adaptive(tint)).
			BorderForeground(adaptive(t.Accent)),
		ButtonFocus: lipgloss.NewStyle().
			Foreground(onColorPrima(t.Primary, t.Bg, t.Fg)).Background(adaptive(t.Primary)).Bold(true).
			BorderForeground(adaptive(t.Primary)),
		ButtonHost:    lipgloss.NewStyle().Foreground(onColorPrima(t.Bg, t.Muted, t.Fg)),
		ChipAttach:    lipgloss.NewStyle().Foreground(onColorPrima(tint, t.Accent, t.Primary, t.Fg)).Background(adaptive(tint)),
		ChipImage:     lipgloss.NewStyle().Foreground(onColorPrima(tint, t.Muted, t.Fg)).Background(adaptive(tint)),
		QuoteBar:      quoteBarStyles(t, numLivelliCitazione),
		TableBorder:   lipgloss.NewStyle().Foreground(adaptive(t.Border)),
		TableHeader:   lipgloss.NewStyle().Foreground(onColorPrima(tint, t.Primary, t.Fg)).Background(adaptive(tint)).Bold(true),
		TableCell:     lipgloss.NewStyle().Foreground(adaptive(t.Fg)),
		BadgeAvatar:   badgeStyles(labelSlots(t), t),
		FolderCount:   lipgloss.NewStyle().Foreground(onColorPrima(t.Unread, t.Bg, t.Fg)).Background(adaptive(t.Unread)).Bold(true),
		FolderTree:    lipgloss.NewStyle().Foreground(onColorPrima(t.Bg, t.Muted, t.Fg)),
		OverlayBorder: lipgloss.NewStyle().Foreground(adaptive(t.Primary)),
		// Ombra a retino ░ nel colore dello slot border, senza sfondo (decisione di Andrea,
		// 2026-11-23): decorazione, niente testo da leggere.
		OverlayShadow: lipgloss.NewStyle().Foreground(adaptive(t.Border)),
		StatusSegment: statusSegmentStyles(t, t.Selection, t.Border),
		StatusSep:     lipgloss.NewStyle().Foreground(adaptive(t.Border)),
		ErrText:       lipgloss.NewStyle().Foreground(onColorPrima(t.Bg, t.Error, t.Warning, t.Fg)),
	}
}

// numLivelliCitazione è il numero di livelli di annidamento distinti delle
// citazioni (Piano 15, Task 9, D15-03); oltre, i consumatori ciclano con %.
const numLivelliCitazione = 4

// quoteBarStyles costruisce le barre delle citazioni: n colori sfumati da quoted
// (livello 0) a primary (ultimo) con Blend1D, calcolati per variante dark e light
// separatamente (nessuna dipendenza dallo stato di compat).
func quoteBarStyles(t theme.Theme, n int) []lipgloss.Style {
	qD, qL := t.Quoted.Resolve()
	pD, pL := t.Primary.Resolve()
	dark := lipgloss.Blend1D(n, lipgloss.Color(qD), lipgloss.Color(pD))
	light := lipgloss.Blend1D(n, lipgloss.Color(qL), lipgloss.Color(pL))
	out := make([]lipgloss.Style, n)
	for i := range out {
		out[i] = lipgloss.NewStyle().Foreground(adaptive(theme.Slot{Dark: hexColore(dark[i]), Light: hexColore(light[i])}))
	}
	return out
}

// badgeStyles costruisce i badge delle iniziali: sfondo = slot del palette delle
// label (stesso ordine, quindi stesso indice di labelIndex), testo onColor in
// grassetto (Piano 15, Task 9, D15-05).
func badgeStyles(slots []theme.Slot, t theme.Theme) []lipgloss.Style {
	out := make([]lipgloss.Style, len(slots))
	for i, s := range slots {
		out[i] = lipgloss.NewStyle().Foreground(onColorPrima(s, t.Bg, t.Fg)).Background(adaptive(s)).Bold(true)
	}
	return out
}

// statusSegmentStyles costruisce i segmenti della barra di stato: sfondi in
// alternanza, testo onColor (Piano 15, Task 9, D15-12).
func statusSegmentStyles(t theme.Theme, bgs ...theme.Slot) []lipgloss.Style {
	out := make([]lipgloss.Style, len(bgs))
	for i, bg := range bgs {
		out[i] = lipgloss.NewStyle().Foreground(onColorPrima(bg, t.Fg, t.Bg)).Background(adaptive(bg))
	}
	return out
}

// LabelStyle restituisce il colore di una etichetta utente: hash deterministico
// del nome sul palette (Piano 8, Task 9, D8-5). Senza palette ricade su ListNormal.
func (s Styles) LabelStyle(name string) lipgloss.Style {
	if len(s.Labels) == 0 {
		return s.ListNormal
	}
	return s.Labels[labelIndex(name, len(s.Labels))]
}

// LabelStyleTint è LabelStyle per una riga spuntata (sfondo tinta, richiesta di
// Andrea, 2026-11-23): stesso indice sul palette LabelsTint. Senza palette ricade
// su ListTint.
func (s Styles) LabelStyleTint(name string) lipgloss.Style {
	if len(s.LabelsTint) == 0 {
		return s.ListTint
	}
	return s.LabelsTint[labelIndex(name, len(s.LabelsTint))]
}

// labelIndex sceglie l'indice del palette per il nome: hash deterministico, n > 0.
func labelIndex(name string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name)) // fnv non fallisce mai su Write
	return int(h.Sum32() % uint32(n))
}

// labelCandidates sono gli slot che il colore delle etichette utente può usare
// (Piano 8, Task 9, D8-5). Il testo dell'etichetta è disegnato sullo slot bg.
func labelCandidates(t theme.Theme) []theme.Slot {
	return []theme.Slot{t.Primary, t.Accent, t.Success, t.Warning, t.Error, t.Starred, t.Quoted, t.Help, t.Unread}
}

// labelSlots tiene i candidati con contrasto >= 4.5:1 sul bg del tema.
func labelSlots(t theme.Theme) []theme.Slot {
	var out []theme.Slot
	for _, s := range labelCandidates(t) {
		if slotContrasta(s, t.Bg) {
			out = append(out, s)
		}
	}
	return out
}

// linkSlot sceglie lo slot del testo dei link (Richiesta di Andrea, 2026-11-23):
// l'accent, o il primary se l'accent non ha contrasto WCAG >= 4.5:1 sul bg del
// tema in entrambe le varianti; ultimo ripiego il testo (fg).
func linkSlot(t theme.Theme) theme.Slot {
	for _, s := range []theme.Slot{t.Accent, t.Primary} {
		if slotContrasta(s, t.Bg) {
			return s
		}
	}
	return t.Fg
}

// slotContrasta dice se lo slot fg, come testo, ha contrasto WCAG >= 4.5:1 su bg
// in entrambe le varianti dark e light.
func slotContrasta(fg, bg theme.Slot) bool {
	fD, fL := fg.Resolve()
	bD, bL := bg.Resolve()
	return contrastRatio(lipgloss.Color(fD), lipgloss.Color(bD)) >= minContrast &&
		contrastRatio(lipgloss.Color(fL), lipgloss.Color(bL)) >= minContrast
}

// labelStyles costruisce uno stile di solo colore per ogni slot del palette.
func labelStyles(slots []theme.Slot) []lipgloss.Style {
	out := make([]lipgloss.Style, len(slots))
	for i, s := range slots {
		out[i] = lipgloss.NewStyle().Foreground(adaptive(s))
	}
	return out
}

// labelStylesTint come labelStyles, ma sulla tinta della riga spuntata: il testo
// di ogni etichetta usa onColor sulla tinta, così vale il test di contrasto WCAG.
func labelStylesTint(slots []theme.Slot, bg, fg theme.Slot) []lipgloss.Style {
	out := make([]lipgloss.Style, len(slots))
	for i, s := range slots {
		out[i] = lipgloss.NewStyle().Foreground(onColor(bg, s, fg)).Background(adaptive(bg))
	}
	return out
}

// toastStyle è il riquadro di una notifica (Piano 7.3, Task 1): superficie
// dello slot bg, bordo arrotondato nello slot edge del tipo. Il testo usa
// onColor sulla superficie, così vale il test di contrasto WCAG.
func toastStyle(t theme.Theme, edge theme.Slot) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(onColor(t.Bg, t.Fg, t.Accent)).
		Background(adaptive(t.Bg)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(adaptive(edge)).
		BorderBackground(adaptive(t.Bg)).
		Padding(0, 1)
}

// adaptive converte uno Slot in un colore lipgloss: la coppia dark/light
// dello slot diventa adattiva. Nota v2: lipgloss.AdaptiveColor è stato
// rimosso; compat.AdaptiveColor (UPGRADE_GUIDE_V2.md) è il sostituto drop-in
// ufficiale con le stesse semantiche, e risolve la variante al rendering.
// In Bubble Tea il background arriva da tea.BackgroundColorMsg; compat verrà
// aggiornato in main (Task 8/9) al primo messaggio ricevuto.
func adaptive(s theme.Slot) color.Color {
	dark, light := s.Resolve()
	return compat.AdaptiveColor{
		Dark:  lipgloss.Color(dark),
		Light: lipgloss.Color(light),
	}
}

// frazioneTinta è la quota di selection mescolata nel bg per la riga spuntata con x
// (richiesta di Andrea, 2026-11-23): 50% verso selection (il 20% iniziale era invisibile sul fondo del terminale), sempre più tenue del cursore.
const frazioneTinta = 0.50

// mescola mescola linearmente in sRGB lo slot a verso lo slot b di frac (0 dà a, 1
// dà b), separatamente per la variante dark e light. Nessun colore fisso: la tinta
// deriva dal tema (richiesta di Andrea, 2026-11-23).
func mescola(a, b theme.Slot, frac float64) theme.Slot {
	aD, aL := a.Resolve()
	bD, bL := b.Resolve()
	return theme.Slot{Dark: mescolaHex(aD, bD, frac), Light: mescolaHex(aL, bL, frac)}
}

// mescolaHex mescola due colori esadecimali (#rgb o #rrggbb) canale per canale, con
// arrotondamento a 8 bit.
func mescolaHex(a, b string, frac float64) string {
	ar, ag, ab, _ := lipgloss.Color(a).RGBA() // canali a 16 bit: >>8 riporta a 8 bit
	br, bg, bb, _ := lipgloss.Color(b).RGBA()
	canale := func(x, y uint32) uint8 {
		v := float64(x>>8) + (float64(y>>8)-float64(x>>8))*frac
		return uint8(math.Round(v))
	}
	return fmt.Sprintf("#%02x%02x%02x", canale(ar, br), canale(ag, bg), canale(ab, bb))
}

// minContrast è la soglia WCAG AA per il testo normale. Piano 7.2, Task 1.
const minContrast = 4.5

// onColor calcola il colore del testo su una barra colorata con sfondo bg.
// Piano 7.2, Task 1 (decisione U1): per la variante dark e light separatamente
// sceglie il primo tra fg e base (lo slot bg del tema) con contrasto WCAG >=
// 4.5 rispetto a bg; altrimenti nero o bianco, quello con contrasto maggiore.
func onColor(bg, fg, base theme.Slot) color.Color {
	return onColorPrima(bg, fg, base)
}

// onColorPrima generalizza onColor a una catena di candidati (Piano 15, Task 9):
// per ogni variante sceglie il primo slot con contrasto >= 4.5 su bg, altrimenti
// nero o bianco (eccezione U1, unico colore non derivato dal tema).
func onColorPrima(bg theme.Slot, cands ...theme.Slot) color.Color {
	bgD, bgL := bg.Resolve()
	var dark, light []color.Color
	for _, c := range cands {
		d, l := c.Resolve()
		dark = append(dark, lipgloss.Color(d))
		light = append(light, lipgloss.Color(l))
	}
	return compat.AdaptiveColor{
		Dark:  textOn(lipgloss.Color(bgD), dark...),
		Light: textOn(lipgloss.Color(bgL), light...),
	}
}

// textOn applica la regola di onColor a un singolo lato (dark o light).
func textOn(bg color.Color, cands ...color.Color) color.Color {
	for _, c := range cands {
		if contrastRatio(c, bg) >= minContrast {
			return c
		}
	}
	black, white := lipgloss.Color("#000000"), lipgloss.Color("#ffffff")
	if contrastRatio(black, bg) >= contrastRatio(white, bg) {
		return black
	}
	return white
}

// hexColore formatta un colore come #rrggbb (8 bit per canale).
func hexColore(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// contrastRatio è il rapporto di contrasto WCAG 2.x tra due colori (da 1 a 21).
func contrastRatio(a, b color.Color) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// relativeLuminance è la luminanza relativa WCAG 2.x (canali linearizzati).
func relativeLuminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA() // canali a 16 bit
	return 0.2126*linearChannel(r) + 0.7152*linearChannel(g) + 0.0722*linearChannel(b)
}

func linearChannel(v uint32) float64 {
	c := float64(v) / 65535
	if c <= 0.03928 {
		return c / 12.92
	}
	return math.Pow((c+0.055)/1.055, 2.4)
}
