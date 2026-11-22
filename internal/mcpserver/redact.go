package mcpserver

import (
	"errors"
	"strings"
)

// Redact restituisce un errore con il messaggio ripulito dai segreti (sostituiti
// con ***). Piano 14, Task 1, regola 5: la password non esce mai verso l'agente.
// Il risultato non conserva la catena %w: il messaggio è l'unico dato
// che lascia il server.
func Redact(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	for _, s := range secrets {
		if s == "" {
			continue
		}
		msg = strings.ReplaceAll(msg, s, "***")
	}
	return errors.New(msg)
}
