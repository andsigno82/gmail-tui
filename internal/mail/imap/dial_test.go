package imap

// Piano 12a, Task 3: Connect (e la sonda di doctor) rispettano il contesto per
// tutto il dial e il login; il dial TLS implicito e provato davvero.

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"

	"github.com/andsigno82/gmail-tui/internal/mail"
	"github.com/andsigno82/gmail-tui/internal/msg"
)

// silentServer e un listener che accetta e non risponde mai al login (con greeting
// vuoto non manda nemmeno quello). Chiude closed quando il client chiude la
// connessione: e la prova che nulla resta aperto.
func silentServer(t *testing.T, greeting string) (addr string, closed <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		if greeting != "" {
			if _, err := fmt.Fprint(conn, greeting); err != nil {
				return
			}
		}
		buf := make([]byte, 512)
		for {
			if _, err := conn.Read(buf); err != nil {
				return // EOF: il client ha chiuso
			}
		}
	}()
	return ln.Addr().String(), done
}

// assertScaduto chiama op con un ctx di 300 ms e verifica che torni con l'errore
// del ctx, presto, e che la connessione sia stata chiusa.
func assertScaduto(t *testing.T, closed <-chan struct{}, op func(ctx context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := op(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, voglio context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("tornato dopo %v: non ha rispettato la scadenza", d)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Error("la connessione e rimasta aperta dopo la scadenza")
	}
}

// TestConnectStopsAtContextDeadline: il ctx scade mentre il greeting non arriva,
// o mentre il LOGIN non ha risposta.
func TestConnectStopsAtContextDeadline(t *testing.T) {
	cases := []struct{ name, greeting string }{
		{"senza greeting", ""},
		{"greeting ma login muto", "* OK ready\r\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			addr, closed := silentServer(t, c.greeting)
			s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
			assertScaduto(t, closed, s.Connect)
			if _, err := s.connected(); !errors.Is(err, mail.ErrNotConnected) {
				t.Errorf("dopo una Connect scaduta connected() = %v, voglio ErrNotConnected", err)
			}
		})
	}
}

// Stessa cosa per la sonda di doctor e del wizard.
func TestCapabilitiesStopsAtContextDeadline(t *testing.T) {
	addr, closed := silentServer(t, "* OK ready\r\n")
	assertScaduto(t, closed, func(ctx context.Context) error {
		_, err := Capabilities(ctx, Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
		return err
	})
}

// TestConnectStopsAtContextCancel: una cancellazione esplicita vale quanto la
// scadenza (il contratto di dialSession dice "scaduto o annullato").
func TestConnectStopsAtContextCancel(t *testing.T) {
	addr, closed := silentServer(t, "* OK ready\r\n")
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	time.AfterFunc(300*time.Millisecond, cancel)
	if err := s.Connect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, voglio context.Canceled", err)
	}
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Error("la connessione e rimasta aperta dopo la cancellazione")
	}
}

// TestConnectSenzaScadenzaHaUnBudget (Piano 12a, revisione finale): le riconnessioni
// di Watch e di UnreadCounts, e gmail-tui fetch, chiamano Connect con un ctx senza
// scadenza. Anche loro devono tornare se il server tace sul login: Connect si dà il
// budget di 90 s dell'avvio (accorciato qui). Per vederlo rosso: togliere il budget
// in Connect. La scadenza che il chiamante fissa resta la sua: non si accorcia e non
// si allunga.
func TestConnectSenzaScadenzaHaUnBudget(t *testing.T) {
	old := connectBudget
	t.Cleanup(func() { connectBudget = old })

	t.Run("senza scadenza vale il budget", func(t *testing.T) {
		connectBudget = 300 * time.Millisecond
		addr, closed := silentServer(t, "* OK ready\r\n")
		s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
		done := make(chan error, 1)
		go func() { done <- s.Connect(t.Context()) }()
		var err error
		select {
		case err = <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("Connect non e tornata entro 3 s: senza scadenza il login resta appeso")
		}
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, voglio context.DeadlineExceeded (il budget di Connect)", err)
		}
		select {
		case <-closed:
		case <-time.After(2 * time.Second):
			t.Error("la connessione e rimasta aperta dopo il budget")
		}
		if _, err := s.connected(); !errors.Is(err, mail.ErrNotConnected) {
			t.Errorf("dopo una Connect scaduta connected() = %v, voglio ErrNotConnected", err)
		}
	})

	t.Run("la scadenza del chiamante vince", func(t *testing.T) {
		connectBudget = time.Nanosecond
		ts := startTestServer(t)
		s := NewStore(Options{Addr: ts.addr(), Username: "tester", Password: "secret", Insecure: true})
		t.Cleanup(func() { _ = s.Close() })
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		if err := s.Connect(ctx); err != nil {
			t.Errorf("Connect con una scadenza sua del chiamante: %v (il budget non deve accorciarla)", err)
		}
	})
}

// TestConnectStopsDuringTLSHandshake: il ctx scade mentre l'handshake TLS non
// procede (il server accetta la connessione e non risponde).
func TestConnectStopsDuringTLSHandshake(t *testing.T) {
	addr, closed := silentServer(t, "")
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p"})
	assertScaduto(t, closed, s.Connect)
}

// TestConnectSurvivesCallerContextCancel (test di guardia): il ctx vale fino alla
// fine della chiamata. Una volta connessi, la sua cancellazione NON chiude la
// connessione dello Store. Per vederlo rosso: togliere stop() in dialSession.
func TestConnectSurvivesCallerContextCancel(t *testing.T) {
	ts := startTestServer(t)
	ctx, cancel := context.WithCancel(t.Context())
	s := NewStore(Options{Addr: ts.addr(), Username: "tester", Password: "secret", Insecure: true})
	if err := s.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	cancel()
	if _, err := s.ListMessages(context.Background(), msg.Query{}, mail.Page{}); err != nil {
		t.Errorf("lettura dopo la cancellazione del ctx di Connect: %v", err)
	}
}

// selfSignedCert genera un certificato per 127.0.0.1 (solo stdlib), come nei test
// del package smtp.
func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

// tlsScriptedServer avvia il server a copione dietro TLS implicito e ne restituisce
// l'indirizzo e il certificato servito.
func tlsScriptedServer(t *testing.T) (addr string, cert tls.Certificate) {
	t.Helper()
	cert = selfSignedCert(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	scriptedIMAPOn(tls.NewListener(ln, &tls.Config{Certificates: []tls.Certificate{cert}}), "IMAP4rev1", func(string) []string { return nil })
	return ln.Addr().String(), cert
}

// TestConnectOverTLS: il dial implicito fa l'handshake e il login, e la connessione
// resta dello Store. Per vederlo rosso: togliere in dialConn la riga che riempie
// cfg.ServerName (il dial fallisce con "either ServerName or InsecureSkipVerify").
func TestConnectOverTLS(t *testing.T) {
	addr, cert := tlsScriptedServer(t)
	pool := x509.NewCertPool()
	pool.AddCert(cert.Leaf)
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p", TLSConfig: &tls.Config{RootCAs: pool}})
	if err := s.Connect(t.Context()); err != nil {
		t.Fatalf("Connect su TLS: %v", err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.connected(); err != nil {
		t.Errorf("dopo Connect su TLS connected() = %v", err)
	}
}

// TestConnectTLSRejectsUntrustedCertificate: il certificato si verifica davvero;
// senza la CA del test il dial fallisce e non si connette.
func TestConnectTLSRejectsUntrustedCertificate(t *testing.T) {
	addr, _ := tlsScriptedServer(t)
	s := NewStore(Options{Addr: addr, Username: "u", Password: "p"})
	err := s.Connect(t.Context())
	var nonFidato x509.UnknownAuthorityError
	if !errors.As(err, &nonFidato) {
		_ = s.Close()
		t.Fatalf("err = %v, voglio un certificato non fidato (x509.UnknownAuthorityError)", err)
	}
	if _, err := s.connected(); !errors.Is(err, mail.ErrNotConnected) {
		t.Errorf("connected() = %v, voglio ErrNotConnected", err)
	}
}

// ctxCheScadeAlControllo e un contesto il cui Err() riesce le prime "passaggi" volte;
// alla chiamata successiva annulla il contesto e risponde comunque nil al controllo che
// l'ha chiamato; da li in poi risponde con l'errore del contesto. Riproduce cosi il
// caso in cui il ctx finisce fra il controllo di dialSession e il distacco della
// chiusura automatica (Piano 12a, revisione del Task 3).
// Non serve aspettare la chiusura: l'annullamento di un ctx nato da WithCancel arriva
// in modo sincrono alla chiusura registrata con context.AfterFunc, quindi subito dopo
// stop() torna false, qualunque sia l'ordine dei goroutine.
type ctxCheScadeAlControllo struct {
	context.Context
	cancel    context.CancelFunc
	passaggi  int // chiamate a Err() che passano prima di quella che annulla
	controlli int // chiamate a Err() viste finora: dialSession e sincrona, le vede un solo goroutine
}

func (c *ctxCheScadeAlControllo) Err() error {
	c.controlli++
	switch {
	case c.controlli <= c.passaggi:
		return nil
	case c.controlli == c.passaggi+1:
		c.cancel()
		return nil
	default:
		return context.Canceled
	}
}

// TestDialSessionNonRestituisceUnClientChiuso: se il contesto chiude la connessione
// fra il controllo e il distacco della chiusura automatica, il client non si
// restituisce come buono. Per vederlo rosso: ignorare il valore di ritorno di stop().
// Il conteggio finale delle chiamate a Err() fissa dove cade l'annullamento: se
// dialSession o Connect cominciano a controllare il ctx in un altro punto, il test lo
// segnala invece di passare per un altro percorso.
func TestDialSessionNonRestituisceUnClientChiuso(t *testing.T) {
	casi := []struct {
		nome       string
		attraverso bool // true: si passa da Store.Connect
		controlli  int  // chiamate a Err() attese alla fine
	}{
		{"dialSession diretto", false, 2},
		{"attraverso Connect", true, 3},
	}
	for _, c := range casi {
		t.Run(c.nome, func(t *testing.T) {
			ts := startTestServer(t)
			o := Options{Addr: ts.addr(), Username: "tester", Password: "secret", Insecure: true}
			// Scadenza lontana: Connect lascia suo questo ctx. Senza, lo ricava dal suo
			// budget, e i controlli di Err() di dialSession non passano piu da qui.
			base, cancel := context.WithTimeout(t.Context(), time.Hour)
			defer cancel()
			// passaggi: Connect controlla il ctx all'ingresso, dialSession no. La chiamata
			// che annulla e il controllo dopo il login.
			passaggi := 0
			if c.attraverso {
				passaggi = 1
			}
			ctx := &ctxCheScadeAlControllo{Context: base, cancel: cancel, passaggi: passaggi}
			if !c.attraverso {
				client, err := dialSession(ctx, o, "Connect", &imapclient.Options{})
				if !errors.Is(err, context.Canceled) || client != nil {
					t.Errorf("dialSession = (client presente: %v, %v), voglio (nil, context.Canceled)", client != nil, err)
				}
			} else {
				s := NewStore(o)
				if err := s.Connect(ctx); !errors.Is(err, context.Canceled) {
					t.Errorf("Connect = %v, voglio context.Canceled", err)
				}
				if _, err := s.connected(); !errors.Is(err, mail.ErrNotConnected) {
					t.Errorf("connected() = %v, voglio ErrNotConnected", err)
				}
				_ = s.Close()
			}
			if ctx.controlli != c.controlli {
				t.Errorf("chiamate a Err() = %d, attese %d: l'annullamento non cade piu fra il controllo e stop()",
					ctx.controlli, c.controlli)
			}
		})
	}
}
