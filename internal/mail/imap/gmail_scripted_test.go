package imap

// Server IMAP scriptato per i test di Piano 8: risponde a LOGIN e LOGOUT, e per
// ogni altro comando chiama respond (senza tag, es. "UID FETCH 1:3 (...)") e
// chiude il comando con OK. Loopback, nessuna rete esterna.

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
)

func scriptedIMAP(t *testing.T, caps string, respond func(cmd string) []string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	serveScripted(ln, caps, respond)
	return ln.Addr().String()
}

// scriptedIMAPOn serve il protocollo scriptato su un listener già pronto: TLS
// implicito nei test del dial (Piano 12a, Task 3).
func scriptedIMAPOn(ln net.Listener, caps string, respond func(cmd string) []string) {
	serveScripted(ln, caps, respond)
}

// serveScripted accetta UNA connessione su ln: risponde a LOGIN e LOGOUT, e per
// ogni altro comando chiama respond (senza tag) e chiude il comando con OK.
func serveScripted(ln net.Listener, caps string, respond func(cmd string) []string) {
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
			verb, _, _ := strings.Cut(rest, " ")
			switch strings.ToUpper(verb) {
			case "LOGIN":
				fmt.Fprintf(conn, "%s OK [CAPABILITY %s] LOGIN completed\r\n", tag, caps)
			case "LOGOUT":
				fmt.Fprintf(conn, "* BYE bye\r\n%s OK LOGOUT completed\r\n", tag)
				return
			default:
				status := "OK done"
				for _, l := range respond(rest) {
					// Una riga "NO motivo" è l'esito taggato NO del comando (Piano 10, review punto 4).
					if reason, ok := strings.CutPrefix(l, "NO "); ok {
						status = "NO " + reason
						continue
					}
					fmt.Fprintf(conn, "%s\r\n", l)
				}
				fmt.Fprintf(conn, "%s %s\r\n", tag, status)
			}
		}
	}()
}
