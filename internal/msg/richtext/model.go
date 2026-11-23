// Package richtext definisce il modello a blocchi del corpo dei messaggi
// (Piano 15, Task 1, D15-T1): paragrafi, citazioni, elenchi, tabelle, bottoni,
// immagini. Lo costruiscono FromHTML (Task 4) e FromText (Task 5); i renderer
// di internal/ui/render lo consumano. Il modello non si salva in cache: si
// ricava da BodyHTML a render time (D15-T2). Nessuna logica in questo file.
package richtext

// Limiti di input (Piano 15, Task 1, D15-T4): email grandi non devono bloccare
// la UI. Valori da tarare con misure reali (Task 19).
const (
	// MaxInputBytes: oltre, si parsano solo i primi MaxInputBytes e FromHTML
	// segnala il troncamento.
	MaxInputBytes = 512 << 10
	// MaxNodes: numero massimo di nodi HTML visitati.
	MaxNodes = 5000
	// MaxDepth: annidamento massimo dei blocchi.
	MaxDepth = 16
	// MaxTableRows e MaxTableCols: oltre, la tabella ricade su testo.
	MaxTableRows = 200
	MaxTableCols = 20
)

// Kind è il tipo di un Block. Il valore zero è KindParagraph: invariante su
// cui i parser contano (Block{} è un paragrafo vuoto).
type Kind uint8

const (
	// KindParagraph: testo corrente (Runs).
	KindParagraph Kind = iota
	// KindQuote: citazione; il contenuto sta in Children, Depth = livello.
	KindQuote
	// KindList: elenco; ogni elemento è un Block in Children, Ordered e Start
	// scelgono la numerazione.
	KindList
	// KindTable: tabella; le celle sono in Rows.
	KindTable
	// KindButton: link reso come bottone; testo e href in Runs, Host è il
	// dominio di destinazione.
	KindButton
	// KindImage: immagine come chip con Alt; nessuna richiesta di rete.
	KindImage
	// KindRule: linea orizzontale (<hr>), nessun campo usato.
	KindRule
	// KindPre: testo preformattato (Runs, a capo conservati).
	KindPre
)

// Run è un tratto di testo. Href == "" = testo semplice; altrimenti è un link
// con testo e href sanificato (D15-T13).
type Run struct {
	Text, Href string
}

// Cell è una cella di tabella (usata solo da KindTable).
type Cell struct {
	Runs   []Run
	Header bool // true per <th>
}

// Block è un blocco del corpo. I campi usati solo da certi Kind sono
// indicati accanto a ciascuno; gli altri restano al valore zero.
type Block struct {
	Kind Kind
	// Runs: KindParagraph, KindPre, KindButton (testo e href del bottone).
	Runs []Run
	// Children: KindQuote (contenuto), KindList (un Block per elemento) e, per un
	// elemento di elenco (KindParagraph), i blocchi annidati sotto il suo testo
	// (elenchi, citazioni; Piano 15, Task 4).
	Children []Block
	// Ordered e Start: solo KindList (<ol start=N>).
	Ordered bool
	Start   int
	// Rows: solo KindTable.
	Rows [][]Cell
	// Host: solo KindButton, dominio di destinazione (es. "amazon.it").
	Host string
	// Alt: solo KindImage, testo alternativo sanificato.
	Alt string
	// Depth: solo KindQuote, livello di annidamento (0 = primo).
	Depth int
	// Warn: link/bottone il cui testo mostra un dominio diverso dal reale
	// (D15-07, D15-T6); impostato dal parser.
	Warn bool
}

// FromText costruisce i blocchi dal corpo solo testo (citazioni ">", elenchi
// "-"/"1."). Stub (Piano 15, Task 1): lo sostituisce il Task 5.
func FromText(text string) []Block {
	return nil
}
