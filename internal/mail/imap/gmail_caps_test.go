package imap

// Piano 8, Task 6: la modalità Gmail dello Store è decisa dalla capability
// X-GM-EXT-1 annunciata al login. Il server è un listener loopback scriptato
// (nessuna rete esterna, nessuna credenziale reale).

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

// fakeIMAPServer risponde a LOGIN e LOGOUT annunciando le capability date.
func fakeIMAPServer(t *testing.T, caps string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		fmt.Fprintf(conn, "* OK [CAPABILITY %s] ready\r\n", caps)
		r := bufio.NewReader(conn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			tag, rest, _ := strings.Cut(strings.TrimSpace(line), " ")
			switch strings.ToUpper(strings.Fields(rest + " x")[0]) {
			case "LOGIN":
				fmt.Fprintf(conn, "* CAPABILITY %s\r\n%s OK [CAPABILITY %s] LOGIN completed\r\n", caps, tag, caps)
			case "LOGOUT":
				fmt.Fprintf(conn, "* BYE bye\r\n%s OK LOGOUT completed\r\n", tag)
				return
			default:
				fmt.Fprintf(conn, "%s BAD non gestito nel test\r\n", tag)
			}
		}
	}()
	return ln.Addr().String()
}

func TestGmailModeFromCapability(t *testing.T) {
	cases := []struct {
		name string
		caps string
		want bool
	}{
		{"gmail annuncia X-GM-EXT-1", "IMAP4rev1 UIDPLUS MOVE X-GM-EXT-1", true},
		{"server senza estensioni Gmail", "IMAP4rev1 UIDPLUS MOVE", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			addr := fakeIMAPServer(t, tc.caps)
			s := NewStore(Options{Addr: addr, Username: "u", Password: "p", Insecure: true})
			if s.gmailOn() {
				t.Fatal("prima di Connect la modalità Gmail deve essere spenta")
			}
			if err := s.Connect(t.Context()); err != nil {
				t.Fatalf("Connect: %v", err)
			}
			defer func() { _ = s.Close() }()
			if got := s.gmailOn(); got != tc.want {
				t.Errorf("gmailOn() = %v, want %v", got, tc.want)
			}
		})
	}
}
