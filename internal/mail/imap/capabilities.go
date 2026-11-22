package imap

// Probe di capability per `gmail-tui doctor` (Piano 7.1, Task 4): connessione
// di sola diagnostica (dial, login, Caps, logout), senza SELECT né cache. I
// tipi go-imap restano qui: il chiamante riceve nomi di capability in stringhe.

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/emersion/go-imap/v2/imapclient"
)

// Capabilities apre una connessione con le Options (TLS, o in chiaro se
// Insecure), esegue il login e restituisce le capability annunciate dal server
// in ordine alfabetico. Il ctx limita l'attesa: allo scadere torna ctx.Err() e
// la connessione abbandonata si chiude da sola quando il dial o il login
// finiscono (il dial ha il timeout di default di imapclient).
func Capabilities(ctx context.Context, o Options) ([]string, error) {
	type result struct {
		caps []string
		err  error
	}
	ch := make(chan result, 1) // buffer: la goroutine non resta bloccata se il ctx è scaduto
	go func() {
		caps, err := probeCapabilities(o)
		ch <- result{caps, err}
	}()
	select {
	case r := <-ch:
		return r.caps, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// probeCapabilities è il lavoro bloccante di Capabilities: dial, login, Caps,
// chiusura. Gli errori non includono la password.
func probeCapabilities(o Options) ([]string, error) {
	dial := imapclient.DialTLS
	if o.Insecure {
		dial = imapclient.DialInsecure
	}
	c, err := dial(o.Addr, &imapclient.Options{})
	if err != nil {
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	defer func() { _ = c.Close() }()

	if err := c.Login(o.Username, o.Password).Wait(); err != nil {
		return nil, fmt.Errorf("imap login %s: %w", o.Username, err)
	}
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
