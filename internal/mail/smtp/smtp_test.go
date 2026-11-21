package smtp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	gosmtp "github.com/emersion/go-smtp"

	"gmail-tui/internal/msg"
)

// capture cattura (from, rcpts, data) del backend SMTP in-process.
type capture struct {
	mu    sync.Mutex
	from  string
	rcpts []string
	data  []byte
}

func (c *capture) snapshot() (string, []string, []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.from, append([]string(nil), c.rcpts...), c.data
}

// capBackend è un smtp.Backend che memorizza tutto ciò che arriva; reject
// elenca gli indirizzi RCPT da rifiutare (per testare la fase `rcpt`).
type capBackend struct {
	cap    *capture
	reject []string
}

func (b *capBackend) NewSession(*gosmtp.Conn) (gosmtp.Session, error) {
	return &capSession{b: b}, nil
}

// capSession implementa go-smtp.Session (+AuthSession per AUTH PLAIN).
type capSession struct {
	b *capBackend
}

func (s *capSession) Reset() {}
func (s *capSession) Logout() error {
	return nil
}

func (s *capSession) AuthMechanisms() []string {
	return []string{sasl.Plain}
}

func (s *capSession) Auth(_ string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, username, password string) error {
		if username != "user" || password != "pass" {
			return errors.New("credenziali errate")
		}
		return nil
	}), nil
}

func (s *capSession) Mail(from string, _ *gosmtp.MailOptions) error {
	s.b.cap.mu.Lock()
	defer s.b.cap.mu.Unlock()
	s.b.cap.from = from
	return nil
}

func (s *capSession) Rcpt(to string, _ *gosmtp.RcptOptions) error {
	s.b.cap.mu.Lock()
	defer s.b.cap.mu.Unlock()
	for _, r := range s.b.reject {
		if r == to {
			return errors.New("destinatario rifiutato")
		}
	}
	s.b.cap.rcpts = append(s.b.cap.rcpts, to)
	return nil
}

func (s *capSession) Data(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.b.cap.mu.Lock()
	defer s.b.cap.mu.Unlock()
	s.b.cap.data = b
	return nil
}

// startSMTPServer avvia un server go-smtp in-process su 127.0.0.1:0
// (nessun TLS: il client di test usa Insecure=true). Chiude con t.Cleanup.
func startSMTPServer(t *testing.T, reject []string) (addr string, cap *capture) {
	t.Helper()
	cap = &capture{}
	srv := gosmtp.NewServer(&capBackend{cap: cap, reject: reject})
	srv.AllowInsecureAuth = true // AUTH PLAIN in chiaro: solo test
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return l.Addr().String(), cap
}

// startTLSTestServer avvia un server go-smtp con STARTTLS abilitato
// (srv.TLSConfig con certificato self-signed generato in-test, solo stdlib).
// Stesso harness cattura di startSMTPServer; il client di test usa
// Options.TLSConfig personalizzata (Insecure=false).
func startTLSTestServer(t *testing.T) (addr string, cap *capture) {
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
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	cap = &capture{}
	srv := gosmtp.NewServer(&capBackend{cap: cap})
	srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{
		{Certificate: [][]byte{der}, PrivateKey: key},
	}}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return l.Addr().String(), cap
}

func testSender(t *testing.T, addr string) *Sender {
	t.Helper()
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	return NewSender(Options{
		Host:     h,
		Port:     mustPort(t, p),
		Username: "user",
		Password: "pass",
		Insecure: true, // solo test: nessun TLS/STARTTLS
	})
}

func mustPort(t *testing.T, port string) int {
	t.Helper()
	n, err := strconv.Atoi(port)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func sampleOutgoing() msg.Outgoing {
	return msg.Outgoing{
		From:     "me@example.com",
		To:       []string{"a@b.com", "dup@b.com"},
		Cc:       []string{"dup@b.com", "c@d.com"},
		Bcc:      []string{"dup@b.com", "e@f.com"},
		Subject:  "Ciao",
		BodyText: "riga1\nriga2",
	}
}

// TestSendCapturesFromRcptsData: Send consegna from, RCPT To+Cc+Bcc
// deduplicati, e data che contiene To/Cc/From/Subject ma MAI Bcc.
func TestSendCapturesFromRcptsData(t *testing.T) {
	addr, cap := startSMTPServer(t, nil)
	s := testSender(t, addr)

	m := sampleOutgoing()
	if err := s.Send(t.Context(), m); err != nil {
		t.Fatalf("Send: %v", err)
	}

	from, rcpts, data := cap.snapshot()
	if from != "me@example.com" {
		t.Errorf("from = %q, want me@example.com", from)
	}
	wantRcpts := []string{"a@b.com", "dup@b.com", "c@d.com", "e@f.com"}
	if strings.Join(rcpts, ",") != strings.Join(wantRcpts, ",") {
		t.Errorf("rcpts = %v, want %v (dedup, ordine To+Cc+Bcc)", rcpts, wantRcpts)
	}

	d := string(data)
	for _, want := range []string{
		"From: me@example.com\r\n",
		"To: a@b.com, dup@b.com\r\n",
		"Cc: dup@b.com, c@d.com\r\n",
		"Subject: Ciao\r\n",
	} {
		if !strings.Contains(d, want) {
			t.Errorf("data privo di %q:\n%s", want, d)
		}
	}
	if strings.Contains(d, "Bcc") {
		t.Errorf("data contiene Bcc (proibito):\n%s", d)
	}
}

// TestSendMultipartWithAttachment: con allegato il DATA è multipart/mixed.
func TestSendMultipartWithAttachment(t *testing.T) {
	addr, cap := startSMTPServer(t, nil)
	s := testSender(t, addr)

	dir := t.TempDir()
	path := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(path, []byte("allegato di test"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := sampleOutgoing()
	m.Attachments = []string{path}

	if err := s.Send(t.Context(), m); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, _, data := cap.snapshot()
	d := string(data)
	if !strings.Contains(d, "Content-Type: multipart/mixed; boundary=") {
		t.Errorf("data non multipart/mixed:\n%s", d)
	}
	if !strings.Contains(d, `filename="report.txt"`) {
		t.Errorf("data privo di filename allegato:\n%s", d)
	}
}

// TestSendAuthError: credenziali sbagliate → errore fase `auth`.
func TestSendAuthError(t *testing.T) {
	addr, _ := startSMTPServer(t, nil)
	h, p, _ := net.SplitHostPort(addr)
	s := NewSender(Options{
		Host:     h,
		Port:     mustPort(t, p),
		Username: "user",
		Password: "WRONG",
		Insecure: true,
	})
	err := s.Send(t.Context(), sampleOutgoing())
	if err == nil {
		t.Fatal("want errore auth, got nil")
	}
	if !strings.Contains(err.Error(), "auth") {
		t.Errorf("errore senza fase auth: %v", err)
	}
}

// TestSendRcptError: destinatario rifiutato dal server → errore fase
// `rcpt <addr>` (il primo in ordine To+Cc+Bcc dedup).
func TestSendRcptError(t *testing.T) {
	addr, _ := startSMTPServer(t, []string{"a@b.com"})
	s := testSender(t, addr)
	err := s.Send(t.Context(), sampleOutgoing())
	if err == nil {
		t.Fatal("want errore rcpt, got nil")
	}
	if !strings.Contains(err.Error(), "rcpt a@b.com") {
		t.Errorf("errore senza fase `rcpt a@b.com`: %v", err)
	}
}

// TestSendDialError: server non raggiungibile → errore fase `dial`.
func TestSendDialError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	h, p, _ := net.SplitHostPort(addr)
	s := NewSender(Options{Host: h, Port: mustPort(t, p), Insecure: true})
	err = s.Send(t.Context(), sampleOutgoing())
	if err == nil {
		t.Fatal("want errore dial, got nil")
	}
	if !strings.Contains(err.Error(), "dial") {
		t.Errorf("errore senza fase dial: %v", err)
	}
}

// TestSendStartTLS: STARTTLS end-to-end — server go-smtp con TLSConfig
// (self-signed in-test), client Options{Insecure:false, TLSConfig} di test:
// handshake ok, AUTH PLAIN sul canale cifrato, backend cattura il messaggio.
func TestSendStartTLS(t *testing.T) {
	addr, cap := startTLSTestServer(t)
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSender(Options{
		Host:      h,
		Port:      mustPort(t, p),
		Username:  "user",
		Password:  "pass",
		TLSConfig: &tls.Config{ServerName: "localhost", InsecureSkipVerify: true}, // solo test
	})

	if err := s.Send(t.Context(), sampleOutgoing()); err != nil {
		t.Fatalf("Send (STARTTLS): %v", err)
	}
	from, rcpts, data := cap.snapshot()
	if from != "me@example.com" {
		t.Errorf("from = %q, want me@example.com", from)
	}
	if len(rcpts) == 0 || rcpts[0] != "a@b.com" {
		t.Errorf("rcpts = %v, voglio primo a@b.com", rcpts)
	}
	if !strings.Contains(string(data), "Subject: Ciao\r\n") {
		t.Errorf("data privo di Subject: %s", data)
	}
}

// TestSendStartTLSWrongServerName (failure path): ServerName che non
// corrisponde al certificato (verifica hostname attiva) → handshake TLS
// fallisce → errore wrapped con fase `starttls`.
func TestSendStartTLSWrongServerName(t *testing.T) {
	addr, _ := startTLSTestServer(t)
	h, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSender(Options{
		Host:      h,
		Port:      mustPort(t, p),
		TLSConfig: &tls.Config{ServerName: "wrong.example.org"}, // mismatch hostname
	})
	if err := s.Send(t.Context(), sampleOutgoing()); err == nil {
		t.Fatal("want errore starttls (hostname mismatch), got nil")
	} else if !strings.Contains(err.Error(), "starttls") {
		t.Errorf("errore senza fase starttls: %v", err)
	}
}
