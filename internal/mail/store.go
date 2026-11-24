// Package mail definisce il contratto MailStore (spec §Interfaccia
// MailStore): la UI parla SOLO con questa interfaccia, mai con IMAP.
package mail

import (
	"context"
	"errors"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// ErrNotConnected segnala un'operazione su uno store senza connessione (Piano 14,
// Wave 4): il MCP lo riconosce con errors.Is per azzerare la sessione.
var ErrNotConnected = errors.New("store non connesso")

// ErrMessageNotFound è wrappato quando un messaggio non esiste più (uid assente
// o sparito tra SEARCH e FETCH). Piano 14, Wave 4 review: condiviso con il MCP.
var ErrMessageNotFound = errors.New("messaggio inesistente")

// ErrScanLimit segnala che ListConversations ha raggiunto il tetto di messaggi letti
// prima di riempire la pagina, con altri messaggi da leggere: le righe restituite
// sono valide ma la lista è limitata, non esaurita (Piano 8, review punto 3).
var ErrScanLimit = errors.New("lista limitata: scansione interrotta al tetto di messaggi")

// Page è la paginazione di ListMessages (Limit 0 = default 50).
type Page struct {
	Offset, Limit int
}

// Mailbox è una cartella IMAP o una label Gmail (Piano 7, Task 1).
type Mailbox struct {
	Name string // nome IMAP (es. "[Gmail]/Trash"): è il valore per Query.Label
	// Role viene dagli attributi SPECIAL-USE (RFC 6154), mai dal nome localizzato.
	// Valori: "" (label) | "inbox" | "important" | "sent" | "drafts" | "trash" | "junk" | "all" | "flagged"
	Role string
	// Delimiter è il separatore della gerarchia dal LIST IMAP ("/" per Gmail);
	// vuoto se non noto (Piano 15, Task 2, D15-T9: serve all'albero cartelle).
	Delimiter string
}

// ChangeKind indica cosa è cambiato nel feed Watch.
type ChangeKind int

const (
	ChangeNewMail ChangeKind = iota
	ChangeFlags
	ChangeDisconnected
)

// Change è un evento push (IMAP IDLE nel backend reale).
//
// Refs (Piano 11, Task 2) è valorizzato per ChangeNewMail: intestazioni dei nuovi
// messaggi dal più recente (ENVELOPE, senza \Seen), per la notifica desktop. Vuoto
// per gli altri eventi e se la lettura delle intestazioni fallisce (best-effort).
//
// Unread (Piano 11, review punto 3) è il numero di messaggi NON letti tra gli ID
// nuovi (senza \Seen), per ChangeNewMail. La notifica conta solo questi: se sono
// tutti letti (es. letti da un altro client) non c'è notifica.
type Change struct {
	Kind   ChangeKind
	IDs    []msg.MessageID
	Refs   []msg.MessageRef
	Unread int
}

// Conversation è la riga di lista in vista conversazione (Piano 8, Task 5),
// come Gmail: un thread = una riga. Count, Participants, Unread e Labels
// valgono su All Mail escluso spam e cestino; Latest è il messaggio più recente
// nella mailbox richiesta.
type Conversation struct {
	Latest       msg.MessageRef
	ThreadID     uint64
	Count        int
	Participants []string // mittenti distinti, in ordine cronologico
	Unread       bool     // almeno un messaggio non letto
	Labels       []string // unione delle label utente
}

// SingleConversation è la riga di un solo messaggio (fallback senza X-GM-EXT-1,
// Piano 8, Task 9): Count 1, un partecipante, stato e label del messaggio.
func SingleConversation(r msg.MessageRef) Conversation {
	return Conversation{
		Latest:       r,
		ThreadID:     r.ThreadID,
		Count:        1,
		Participants: []string{r.From},
		Unread:       r.Unread,
		Labels:       append([]string(nil), r.Labels...),
	}
}

// MailStore è l'astrazione di backend posta.
//
// Thread-safety (Piano 3, Task 1): le operazioni sono SERIALIZZATE — UNA OP
// ALLA VOLTA. Il backend IMAP ha una connessione e SEARCH/STORE/EXPUNGE
// agiscono sulla mailbox correntemente selezionata: chiamate concorrenti
// (es. multiple tea.Cmd della TUI) sono corrette — il gate del backend le
// mette in coda, ma non interleaveano i comandi sulla connessione.
type MailStore interface {
	Connect(ctx context.Context) error

	// ListMessages restituisce una pagina di righe di lista secondo q e pg.
	//
	// Semantica IMAP/Gmail:
	//   - q.Label seleziona la mailbox (la label) invece di INBOX;
	//   - q.HasAttachment è best-effort: post-filtro client-side, non una
	//     condizione valutata dal server;
	//   - MessageRef.ThreadLen vale sempre 1: la lunghezza del thread non
	//     è disponibile via IMAP ENVELOPE, il raggruppamento in thread è
	//     responsabilità della UI (Piano 3).
	ListMessages(ctx context.Context, q msg.Query, pg Page) ([]msg.MessageRef, error)

	FetchMessage(ctx context.Context, id msg.MessageID) (*msg.Message, error)

	// Search cerca senza paginazione; valgono la semantica e i limiti
	// documentati su ListMessages (Label seleziona la mailbox,
	// HasAttachment best-effort client-side, ThreadLen = 1).
	Search(ctx context.Context, q msg.Query) ([]msg.MessageRef, error)

	// ListConversations restituisce una riga per conversazione (Piano 8, Task 5),
	// dal più recente. Paginazione su Page come ListMessages (Q1/D8-3: la pagina
	// si riempie leggendo altri messaggi, con tetto).
	// Con q.Raw la query è sintassi Gmail verbatim (Q2/D8-4: All Mail se senza
	// label:/in:). Senza Gmail (server senza X-GM-EXT-1) una riga per messaggio.
	// Se il tetto di scansione è raggiunto prima di riempire la pagina, restituisce
	// le righe trovate con ErrScanLimit: la lista è limitata, non esaurita.
	ListConversations(ctx context.Context, q msg.Query, pg Page) ([]Conversation, error)

	// ConversationMembers restituisce gli ID dei messaggi del thread che stanno
	// nella mailbox indicata (Piano 8, Task 9b): è la conversazione "nella
	// cartella corrente" su cui agiscono le op della riga, come Gmail web. Non
	// usa All Mail. Senza X-GM-EXT-1 o con threadID 0 è un errore: il chiamante
	// usa l'ID della riga.
	ConversationMembers(ctx context.Context, mailbox string, threadID uint64) ([]msg.MessageID, error)

	// ThreadMessages restituisce tutti i messaggi del thread (X-GM-THRID) in
	// ordine cronologico, escluso spam e cestino (Q5/D8-7). Errore per threadID 0.
	ThreadMessages(ctx context.Context, threadID uint64) ([]msg.MessageRef, error)

	MarkRead(ctx context.Context, ids []msg.MessageID, read bool) error

	// Archive archivia i messaggi. Su Gmail/IMAP rimuove il messaggio
	// dalla mailbox corrente: l'expunge equivale alla rimozione della
	// label e il messaggio resta in All Mail.
	Archive(ctx context.Context, ids []msg.MessageID) error

	Star(ctx context.Context, ids []msg.MessageID, on bool) error

	// ApplyLabel gestisce una label Gmail mappata su cartella IMAP:
	// on = copia il messaggio nella cartella-label, off = lo rimuove
	// dalla cartella-label.
	ApplyLabel(ctx context.Context, ids []msg.MessageID, label string, on bool) error

	SaveAttachment(ctx context.Context, id msg.MessageID, part int, path string) error
	Send(ctx context.Context, m msg.Outgoing) error

	// ListMailboxes restituisce cartelle e label con il ruolo SPECIAL-USE.
	// Ordine: INBOX, poi le speciali (important, flagged, sent, drafts, all, junk,
	// trash), poi le label in ordine alfabetico. Le mailbox \Noselect (contenitori come
	// "[Gmail]") sono escluse.
	ListMailboxes(ctx context.Context) ([]Mailbox, error)

	// UnreadCounts restituisce i messaggi non letti per cartella (Piano 15,
	// Task 2, D15-10, D15-T10): chiave = Mailbox.Name, valore = conteggio UNSEEN.
	// Le mailbox assenti dalla mappa non sono state calcolate. Solo lettura: non
	// modifica nulla, non è esposto al server MCP (il Reader resta com'è).
	UnreadCounts(ctx context.Context) (map[string]int, error)

	// Trash sposta i messaggi nel cestino (ruolo "trash"), con semantica Gmail:
	// escono dalla cartella d'origine ed entrano nel cestino. Rifiuta senza
	// agire se il cestino non esiste, se un id è già nel cestino, o se il server
	// non supporta né MOVE né UIDPLUS (mai EXPUNGE generico).
	//
	// Restituisce gli ID dei messaggi nel cestino, per l'annulla (Piano 10,
	// Task 4, D10-T2). Vuoto se il server non riporta i nuovi UID.
	Trash(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error)

	// Restore riporta nella cartella dest i messaggi indicati, che DEVONO stare
	// nel cestino (ruolo "trash"): altrimenti errore prima di ogni azione. È un
	// MOVE di ritorno con UID (mai EXPUNGE generico). Restituisce gli ID nella
	// cartella dest (Piano 10, Task 4, D10-T3). Le label perse col cestino non
	// sono ripristinate qui: il chiamante le riapplica con ModifyLabels.
	Restore(ctx context.Context, ids []msg.MessageID, dest string) ([]msg.MessageID, error)

	// AllMailIDs (solo Gmail, X-GM-EXT-1) restituisce, per ogni ID in INBOX, l'ID
	// dello stesso messaggio (X-GM-MSGID) in "Tutti i messaggi" (ruolo "all").
	// Senza X-GM-EXT-1 è errore. Usato dall'annulla di archivia (Piano 10,
	// Task 4, D10-T4). Non modifica nulla.
	AllMailIDs(ctx context.Context, ids []msg.MessageID) ([]msg.MessageID, error)

	// ModifyLabels aggiunge (add) e rimuove (remove) label Gmail sui messaggi
	// (Piano 10, Task 4, D10-T5).
	//
	// Con X-GM-EXT-1 (STORE ±X-GM-LABELS) le label si cambiano da qualunque
	// cartella, anche sugli ID di "Tutti i messaggi": è sicuro perché si toccano
	// solo le label, mai i messaggi. "\Inbox" è ammesso: è l'annullamento
	// dell'archiviazione (aggiungerlo riporta il messaggio in INBOX; rimuoverlo
	// equivale ad archiviare). Gli altri attributi di sistema (con backslash)
	// non sono label utente e sono rifiutati: la UI non deve esporre \Inbox
	// nell'elenco delle label.
	//
	// Senza X-GM-EXT-1 le label sono cartelle: add copia il messaggio nella
	// cartella-label; remove rimuove il messaggio dalla cartella-label, e ogni
	// ID deve stare in ogni label rimossa (altrimenti errore prima di toccare
	// il server). Un'add e una remove sulla stessa label sono rifiutate.
	ModifyLabels(ctx context.Context, ids []msg.MessageID, add, remove []string) error

	// SaveDraft salva un messaggio RFC 5322 (raw) nella mailbox con ruolo "drafts"
	// (Piano 9, Task 2), con flag \Draft, e restituisce l'ID della bozza. Con
	// replace le bozze indicate sono rimosse DOPO l'APPEND (riscrittura di una
	// bozza). Gli ID in replace devono stare nella cartella Bozze: altrimenti
	// errore prima di qualunque modifica. Non e' esposto al server MCP (D9-J).
	SaveDraft(ctx context.Context, raw []byte, replace []msg.MessageID) (msg.MessageID, error)

	// DeleteDraft rimuove le bozze indicate dalla cartella Bozze (Piano 9, Task 2).
	// Rifiuta senza agire gli ID che non stanno in Bozze.
	DeleteDraft(ctx context.Context, ids []msg.MessageID) error

	// Watch ritorna il canale degli eventi push del feed.
	//
	// Contratto:
	//   - un solo canale per chiamata: ogni chiamata crea il proprio;
	//   - il canale viene chiuso quando ctx è annullato (ctx.Done);
	//   - gli eventi sono best-effort: il canale è bufferizzato, se il
	//     consumer è lento gli eventi si perdono e il consumer deve
	//     re-listare;
	//   - ChangeDisconnected viene emesso prima di una riconnessione;
	//   - la baseline iniziale e la ri-verifica post-riconnessione girano
	//     dentro un round del gate (come le op: una alla volta) — la
	//     chiamata può attendere ops in flight, senza interleave.
	Watch(ctx context.Context) (<-chan Change, error)
}
