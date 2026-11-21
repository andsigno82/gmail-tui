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

// msgsLoaded è il risultato di ListMessages/Search (list/search).
type msgsLoaded struct {
	refs []msg.MessageRef
	err  error
}

// msgLoaded è il risultato di FetchMessage.
type msgLoaded struct {
	m   *msg.Message
	err error
}

// opDone è il risultato di una mutazione (feedback in status bar).
type opDone struct {
	label string
	err   error
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
