package imap

// Apertura della sessione (dial, TLS, login) col contesto rispettato (Piano 12a,
// Task 3). Le funzioni di dial di imapclient (beta.9) non prendono un contesto: una
// Connect che scadeva lasciava girare dial e login dietro le spalle dell'app, fino
// ai timeout di lettura del client, o anche senza fine: dopo il greeting il loop di
// lettura del client aspetta senza scadenza (Client.read chiama Decoder.EOF prima
// che readResponse rimetta i 30 s). Qui TCP e TLS partono col contesto e, alla sua
// scadenza, la connessione si CHIUDE: anche l'attesa del greeting e il LOGIN in
// corso tornano subito. Lo usa anche la sonda di doctor (Capabilities).

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/trace"
)

// dialTimeout e il tempo massimo di TCP e dell'handshake TLS: lo stesso default di
// imapclient (30 s). Il ctx del chiamante puo essere piu corto.
const dialTimeout = 30 * time.Second

// dialSession apre la connessione (TLS, o in chiaro se o.Insecure) e fa il login.
// A ctx scaduto o annullato la connessione e chiusa e l'errore e quello del ctx
// (riconoscibile con errors.Is): non resta una connessione aperta dietro.
// op e il nome dell'operazione per la traccia dei passi (Piano 12a, Task 5).
func dialSession(ctx context.Context, o Options, op string, options *imapclient.Options) (*imapclient.Client, error) {
	ctx = trace.WithOp(ctx, op)
	conn, err := dialConn(ctx, o)
	if err != nil {
		return nil, err
	}
	// Alla scadenza del ctx la connessione si chiude: i comandi in corso falliscono
	// subito, senza aspettare i timeout di lettura del client.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })

	c := imapclient.New(conn, options)
	step := trace.BeginStep(ctx, trace.StepLogin) // Piano 12a, Task 5
	err = c.Login(o.Username, o.Password).Wait()
	cerr := ctx.Err() // scaduto durante il login: vale il suo errore, non quello di rete
	if cerr != nil {
		err = cerr
	}
	step.End("", 0, 0, err)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("imap login %s: %w", o.Username, err)
	}
	stop() // da qui la connessione vive con lo Store: il ctx del chiamante non la chiude
	return c, nil
}

// dialConn apre TCP e, se non e Insecure, fa l'handshake TLS: nome del server
// ricavato dall'indirizzo e ALPN "imap", come faceva DialTLS di imapclient.
// o.TLSConfig, se dato, parte al posto della configurazione di default (nei test
// porta la CA del certificato autofirmato).
func dialConn(ctx context.Context, o Options) (net.Conn, error) {
	step := trace.BeginStep(ctx, trace.StepTCP) // Piano 12a, Task 5: DNS e TCP
	raw, err := (&net.Dialer{Timeout: dialTimeout}).DialContext(ctx, "tcp", o.Addr)
	if err != nil {
		step.End("", 0, 0, err)
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	step.End("", 0, 0, nil)
	if o.Insecure {
		return raw, nil
	}
	host, _, err := net.SplitHostPort(o.Addr)
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	cfg := &tls.Config{}
	if o.TLSConfig != nil {
		cfg = o.TLSConfig.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = host
	}
	if cfg.NextProtos == nil {
		cfg.NextProtos = []string{"imap"}
	}
	hsCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	step = trace.BeginStep(ctx, trace.StepTLS)
	tc := tls.Client(raw, cfg)
	if err := tc.HandshakeContext(hsCtx); err != nil {
		step.End("", 0, 0, err)
		_ = raw.Close()
		return nil, fmt.Errorf("imap dial %s: %w", o.Addr, err)
	}
	step.End("", 0, 0, nil)
	return tc, nil
}
