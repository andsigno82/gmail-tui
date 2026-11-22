// Package app contiene il Model radice della TUI (Bubble Tea v2): routing
// viste, status bar globale, keymap centrale. La UI parla SOLO con
// mail.MailStore — ogni chiamata di rete gira in un tea.Cmd, mai nel loop UI.
package app

import (
	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
)

// Tipi Msg di risultato delle Cmd: uno per operazione, con errore incorporato.
// Contratto condiviso da tutte le viste (fonte: piano 3, "Contratto messaggi").

// msgsLoaded è il risultato di ListMessages/Search (list/search). query è la
// stringa digitata dall'utente quando il risultato viene da una ricerca
// (badge "N risultati per …" in status bar); vuota per il load/re-list della
// lista. silent contraddistingue il refresh automatico dopo una mutazione
// (Task 7): aggiorna lista e cursore ma NON tocca lo status (il feedback
// dell'op deve restare visibile; il badge/clear sono solo per azioni
// esplicite dell'utente).
type msgsLoaded struct {
	refs   []msg.MessageRef
	err    error
	query  string
	silent bool
}

// msgLoaded è il risultato di FetchMessage. requested è l'ID richiesto dalla
// Cmd di fetch (fix merge-gate anti-race): i fetch non hanno ordine garantito
// (A lenta, B veloce) e il risultato di A può arrivare DOPO che B è già stato
// aperto (o dopo il back all'inbox) — il handler confronta requested col ref
// in attesa e scarta i risultati stanti (niente rebuild, niente auto MarkRead).
type msgLoaded struct {
	m         *msg.Message
	err       error
	requested msg.MessageID
}

// opDone è il risultato di una mutazione (feedback in status bar). refresh
// chiede il re-list della lista corrente al risultato (le op dell'utente,
// Task 7); le op automatiche — es. auto MarkRead al fetch (Task 5) — non
// rilistano (lo status resta l'unico feedback, la lista si aggiorna via
// Watch/nuova ricerca). closeComposer (Piano 4, Task 4) è dell'invio riuscito:
// il composer si chiude e si torna all'inbox prima del re-list. send (fix
// doppio invio) identifica l'opDone dell'invio: la radice lo usa per
// riabilitare un nuovo invio (m.sending) al suo arrivo, ok o errore.
type opDone struct {
	label         string
	err           error
	refresh       bool
	closeComposer bool
	send          bool
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

// statusMsg è un messaggio effimero per la status bar: resta finché il
// prossimo evento non lo sostituisce. isOK (Piano 3 polish, use-or-drop —
// adottato al Piano 4 Task 5) distingue il feedback positivo ("inviato",
// "salvato", "tema applicato", setup completato) che viene reso col stile
// StatusOK anziché il normale StatusBar.
type statusMsg struct {
	text  string
	isErr bool
	isOK  bool
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
