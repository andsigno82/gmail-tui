package imap

// Probe di capability per `gmail-tui doctor` (Piano 7.1, Task 4): connessione
// di sola diagnostica (dial, login, Caps, logout), senza SELECT né cache. I
// tipi go-imap restano qui: il chiamante riceve nomi di capability in stringhe.

import (
	"context"
	"errors"
	"slices"

	"github.com/emersion/go-imap/v2/imapclient"
)

// Capabilities apre una connessione con le Options (TLS, o in chiaro se
// Insecure), esegue il login e restituisce le capability annunciate dal server
// in ordine alfabetico. Il ctx limita l'attesa: alla scadenza la connessione si
// chiude (dialSession, Piano 12a, Task 3) e torna l'errore del ctx.
func Capabilities(ctx context.Context, o Options) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c, err := dialSession(ctx, o, &imapclient.Options{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Close() }()
	set := c.Caps()
	if set == nil {
		return nil, errors.New("imap: capability non lette dal server")
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, string(name))
	}
	slices.Sort(out)
	return out, nil
}
