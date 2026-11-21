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

// msgLoaded è il risultato di FetchMessage.
type msgLoaded struct {
	m   *msg.Message
	err error
}

// opDone è il risultato di una mutazione (feedback in status bar). refresh
// chiede il re-list della lista corrente al risultato (le op dell'utente,
// Task 7); le op automatiche — es. auto MarkRead al fetch (Task 5) — non
// rilistano (lo status resta l'unico feedback, la lista si aggiorna via
// Watch/nuova ricerca).
type opDone struct {
	label   string
	err     error
	refresh bool
}

// changeEvt è un evento push del feed Watch.
type changeEvt struct {
	c mail.Change
}

// statusMsg è un messaggio effimero per la status bar: resta finché il
// prossimo evento non lo sostituisce.
type statusMsg struct {
	text  string
	isErr bool
}
