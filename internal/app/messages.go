// Package app contiene il Model radice della TUI (Bubble Tea v2): routing
// viste, status bar globale, keymap centrale. La UI parla SOLO con
// mail.MailStore — ogni chiamata di rete gira in un tea.Cmd, mai nel loop UI.
package app

import (
	"errors"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// scanLimited separa il segnale di lista limitata (mail.ErrScanLimit, Piano 8, review
// punto 3) dagli errori veri: le righe che lo accompagnano restano valide.
func scanLimited(err error) (bool, error) {
	if errors.Is(err, mail.ErrScanLimit) {
		return true, nil
	}
	return false, err
}

// limitedStatus è l'avviso di barra per una lista limitata dal tetto di scansione:
// stato persistente e neutro, con la via d'uscita (la ricerca va più indietro).
const limitedStatus = "lista limitata ai messaggi più recenti — cerca con / per andare più indietro"

// latestRefs estrae il messaggio più recente di ogni conversazione (Piano 8,
// Task 9): è la lista di ref su cui girano selezione, anteprima e op.
func latestRefs(convs []mail.Conversation) []msg.MessageRef {
	refs := make([]msg.MessageRef, len(convs))
	for i, c := range convs {
		refs[i] = c.Latest
	}
	return refs
}

// Tipi Msg di risultato delle Cmd: uno per operazione, con errore incorporato.
// Contratto condiviso da tutte le viste (fonte: piano 3, "Contratto messaggi").

// msgsLoaded è il risultato di ListMessages/Search (list/search). query è la
// stringa digitata dall'utente quando il risultato viene da una ricerca
// (badge "N risultati per …" in status bar); vuota per il load/re-list della
// lista. silent contraddistingue il refresh automatico dopo una mutazione
// (Task 7): aggiorna lista e cursore ma NON tocca lo status (il feedback
// dell'op deve restare visibile; il badge/clear sono solo per azioni
// esplicite dell'utente).
//
// limit è il Limit della pagina richiesta (Piano 7, Task 3): 0 = risultato non
// paginato (ricerca o risultato costruito a mano), nessuna pagina successiva.
// Con silent e pagina piena la coda già caricata oltre la prima pagina non si
// perde (il refresh di prima pagina, D5, non tronca la lista).
type msgsLoaded struct {
	refs    []msg.MessageRef
	convs   []mail.Conversation // righe-conversazione (Piano 8, Task 9); nil = righe di un solo messaggio da refs
	err     error
	query   string
	silent  bool
	limit   int
	limited bool   // tetto di scansione raggiunto: lista limitata, non esaurita (Piano 8, review punto 3)
	mailbox string // cartella richiesta ("" = INBOX): risultato stante se non corrisponde (Piano 7, Task 4)
	gen     int    // generazione della lista al momento della richiesta: stante se non più la corrente (Piano 7, fix review 2)
}

// moreLoaded è il risultato di una pagina successiva (scroll in fondo, Piano 7,
// Task 3). query è la stringa di ricerca al momento della richiesta: se la
// lista è cambiata nel frattempo la pagina è STANTE e viene scartata.
type moreLoaded struct {
	refs    []msg.MessageRef
	convs   []mail.Conversation // come msgsLoaded.convs
	limited bool                // come msgsLoaded.limited (Piano 8, review punto 3)
	err     error
	query   string
	mailbox string // cartella della richiesta (Piano 7, Task 4)
	gen     int    // generazione della lista della richiesta (Piano 7, fix review 2)
}

// msgLoaded è il risultato di FetchMessage. requested è l'ID richiesto dalla
// Cmd di fetch (fix merge-gate anti-race): i fetch non hanno ordine garantito
// (A lenta, B veloce) e il risultato di A può arrivare DOPO che B è già stato
// aperto (o dopo il back all'inbox) — il handler confronta requested col ref
// in attesa e scarta i risultati stanti (niente rebuild, niente auto MarkRead).
type msgLoaded struct {
	m         *msg.Message                   // messaggio aperto (solo per i chiamanti a messaggio singolo)
	refs      []msg.MessageRef               // righe della conversazione in ordine cronologico (Piano 8, Task 13)
	bodies    map[msg.MessageID]*msg.Message // corpi già scaricati: gli espansi all'apertura
	err       error
	requested msg.MessageID
	cached    bool // Piano 16, Task 10: da istantanea (provvisorio), non dal server
}

// opDone è il risultato di una mutazione (feedback in status bar). refresh
// chiede il re-list della lista corrente al risultato (le op dell'utente,
// Task 7); le op automatiche — es. auto MarkRead al fetch (Task 5) — non
// rilistano (lo status resta l'unico feedback, la lista si aggiorna via
// Watch/nuova ricerca). send (fix
// doppio invio) identifica l'opDone dell'invio: la radice lo usa per
// riabilitare un nuovo invio (m.sending) al suo arrivo, ok o errore.
type opDone struct {
	label     string
	err       error
	refresh   bool
	deep      bool // refresh a profondità piena (D5, Piano 7 Task 3): solo Archive/Trash
	send      bool
	draftErr  error           // Piano 9, Task 7: rimozione dalla Bozze fallita dopo un invio riuscito
	observed  []string        // Piano 9, Task 13: destinatari dell'invio riuscito, per la rubrica
	draftKept bool            // ripresa con allegati non scaricati: la bozza originale resta in Bozze (fix review)
	retry     *pendingSend    // Piano 9, Task 9: invio fallito dopo la finestra → il composer si riapre con la bozza
	gone      []msg.MessageID // ID usciti dalla cartella con l'op (Archive/Trash): la coda del refresh profondo non li riaccoda (fix review 1)
	// gen/checkGen (Piano 10, Task 3): listGen di partenza di un'op sulla selezione.
	// Se la lista è cambiata nel frattempo l'esito è stante: niente gone né refresh.
	gen      int
	checkGen bool
	undo     *bulkUndo // Piano 10, Task 6: ritorno di archivia/cestino riuscito (`u` per 10 s)
	// localID/localRead: MarkRead riuscito su questo messaggio; lo stato di lettura
	// della riga in lista si aggiorna senza refresh (Piano 7.2, fix review).
	localID   msg.MessageID
	localRead bool
	// convSet/convUnread (Piano 8, Task 9b): stato di lettura dell'intera
	// conversazione dopo `v`; convSet dice se è noto.
	convSet    bool
	convUnread bool
}

// changeEvt è un evento push del feed Watch.
type changeEvt struct {
	c mail.Change
}

// watchStarted è il risultato di startWatch (Task 8): il canale del feed
// Watch, o l'errore se l'apertura è fallita. Al messaggio la radice custodisce
// il canale e arma il loop waitForChange; con errore mostra lo status e NON
// arma (nessun push, la TUI resta usabile con i re-list espliciti).
type watchStarted struct {
	ch  <-chan mail.Change
	err error
}

// statusMsg è lo stato persistente della status bar (cartella, badge di
// ricerca, ⟳ riconnessione, banner): resta finché il prossimo evento non lo
// sostituisce. Gli esiti delle azioni (inviato, archiviata, errori…) non vi
// passano: sono toast (internal/ui/toast, Piano 7.3).
type statusMsg struct {
	text string
}

// attachmentSaved è il risultato di store.SaveAttachment (Piano 4, Task 2:
// :save/:open del thread). open è true per `:open` — il salvataggio è la
// preparazione del tmpfile e la radice risponde con thread.OpenCmd (exec del
// browser, stesso meccanismo del tasto `o`); il feedback viene da OpenedMsg.
type attachmentSaved struct {
	path string
	open bool
	err  error
}

// themePersisted è il risultato della persistenza del tema nella config dopo
// un thread.ThemeChanged. Errore → statusMsg non fatale: il tema runtime
// resta applicato (il salvataggio ripartirà al prossimo cambio tema).
type themePersisted struct {
	err error
}

// connResult è l'esito del connectCmd d'avvio (bugfix 2026-11-22): il
// contratto MailStore richiede Connect prima delle letture, e con lo store
// IMAP reale il load senza connessione fallisce ("non connesso"). Il Mock
// la mascherava perché Connect è un no-op.
type connResult struct {
	err error
}

// settingsSaved è l'esito del salvataggio del menu impostazioni `,` (Piano 7.2,
// Task 3). Errore → status d'errore; i valori restano applicati a runtime.
type settingsSaved struct {
	err error
}

// Anteprima a due colonne (Piano 7.2, Task 4). previewDue scade dopo 150 ms di
// cursore fermo; previewLoaded è il fetch dell'anteprima; previewReadDue scade
// dopo 2 s per il segna-come-letto ritardato (U5). Il token (prevTok) scarta i
// risultati e i timer di un messaggio che non è più sotto il cursore.
type previewDue struct {
	tok int
}

type previewLoaded struct {
	tok    int
	id     msg.MessageID
	m      *msg.Message     // messaggio singolo (solo per i chiamanti a messaggio singolo)
	refs   []msg.MessageRef // righe della conversazione (Piano 8, Task 13)
	bodies map[msg.MessageID]*msg.Message
	err    error
	cached bool // Piano 16, Task 10: da istantanea (provvisorio), non dal server
}

// bodyLoaded è il corpo di un messaggio della conversazione richiesto alla selezione
// o all'espansione (Piano 8, Task 13). gen e anchor scartano i risultati stantii.
type bodyLoaded struct {
	gen    int
	anchor msg.MessageID
	id     msg.MessageID
	m      *msg.Message
	err    error
}

type previewReadDue struct {
	tok int
	id  msg.MessageID
}
