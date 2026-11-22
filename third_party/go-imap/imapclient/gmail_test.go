package imapclient_test

// Piano 8, Task 2 (patch Gmail X-GM-EXT-1): test di FETCH X-GM-MSGID,
// X-GM-THRID, X-GM-LABELS e dei msg-att sconosciuti. Il server è scriptato
// (net.Pipe): risponde con le forme dei documenti Google IMAP Extensions; il
// memserver upstream non conosce X-GM-*.

import (
	"bufio"
	"fmt"
	"net"
	"reflect"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// newScriptedClient avvia un server finto: a ogni comando ricevuto risponde
// con le righe di respond, poi con "OK" (il tag lo aggiunge lui). Il canale
// restituito riceve i comandi nell'ordine in cui arrivano.
func newScriptedClient(t *testing.T, respond func(cmd string) []string) (*imapclient.Client, <-chan string) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	cmds := make(chan string, 16)
	go func() {
		defer serverConn.Close()
		if _, err := fmt.Fprint(serverConn, "* OK [CAPABILITY IMAP4rev1 X-GM-EXT-1] ready\r\n"); err != nil {
			return
		}
		r := bufio.NewReader(serverConn)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			cmds <- line
			tag, _, _ := strings.Cut(line, " ")
			for _, l := range respond(line) {
				if _, err := fmt.Fprint(serverConn, l+"\r\n"); err != nil {
					return
				}
			}
			if _, err := fmt.Fprintf(serverConn, "%s OK done\r\n", tag); err != nil {
				return
			}
		}
	}()
	client := imapclient.New(clientConn, nil)
	t.Cleanup(func() { client.Close() })
	return client, cmds
}

func TestFetchGmailAttributes(t *testing.T) {
	client, cmds := newScriptedClient(t, func(string) []string {
		return []string{`* 1 FETCH (UID 7 X-GM-MSGID 1278455344230334865 X-GM-THRID 1266894439832287888 X-GM-LABELS (\Inbox \Sent Important "Muy Importante") FLAGS (\Seen))`}
	})
	msgs, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{
		UID:           true,
		Flags:         true,
		GmailMsgID:    true,
		GmailThreadID: true,
		GmailLabels:   true,
	}).Collect()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	m := msgs[0]
	if m.GmailMsgID != 1278455344230334865 {
		t.Errorf("GmailMsgID = %d", m.GmailMsgID)
	}
	if m.GmailThreadID != 1266894439832287888 {
		t.Errorf("GmailThreadID = %d", m.GmailThreadID)
	}
	wantLabels := []string{`\Inbox`, `\Sent`, "Important", "Muy Importante"}
	if !reflect.DeepEqual(m.GmailLabels, wantLabels) {
		t.Errorf("GmailLabels = %q, want %q", m.GmailLabels, wantLabels)
	}
	if m.UID != 7 || len(m.Flags) != 1 {
		t.Errorf("UID/Flags = %d/%v: gli altri item devono restare parsati", m.UID, m.Flags)
	}

	cmd := <-cmds
	for _, want := range []string{"X-GM-MSGID", "X-GM-THRID", "X-GM-LABELS"} {
		if !strings.Contains(cmd, want) {
			t.Errorf("wire %q senza %s", cmd, want)
		}
	}
}

func TestFetchGmailLabelsEmptyAndUTF7(t *testing.T) {
	client, _ := newScriptedClient(t, func(string) []string {
		return []string{`* 1 FETCH (X-GM-LABELS ())`, `* 2 FETCH (X-GM-LABELS ("&ANw-" foo))`}
	})
	msgs, err := client.Fetch(imap.SeqSetNum(1, 2), &imap.FetchOptions{GmailLabels: true}).Collect()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2", len(msgs))
	}
	if len(msgs[0].GmailLabels) != 0 {
		t.Errorf("lista vuota: GmailLabels = %q", msgs[0].GmailLabels)
	}
	if want := []string{"Ü", "foo"}; !reflect.DeepEqual(msgs[1].GmailLabels, want) {
		t.Errorf("UTF-7 non decodificato: GmailLabels = %q, want %q", msgs[1].GmailLabels, want)
	}
}

func TestFetchGmailIDsAboveInt63(t *testing.T) {
	client, _ := newScriptedClient(t, func(string) []string {
		return []string{`* 1 FETCH (X-GM-MSGID 18446744073709551615 X-GM-THRID 9999999999999999999)`}
	})
	msgs, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{GmailMsgID: true, GmailThreadID: true}).Collect()
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if msgs[0].GmailMsgID != 18446744073709551615 {
		t.Errorf("GmailMsgID = %d, want max uint64", msgs[0].GmailMsgID)
	}
	if msgs[0].GmailThreadID != 9999999999999999999 {
		t.Errorf("GmailThreadID = %d", msgs[0].GmailThreadID)
	}
}

func TestFetchGmailIDOverflowIsError(t *testing.T) {
	client, _ := newScriptedClient(t, func(string) []string {
		return []string{`* 1 FETCH (X-GM-MSGID 18446744073709551616)`}
	})
	if _, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{GmailMsgID: true}).Collect(); err == nil {
		t.Fatal("ID sopra uint64: atteso errore, nessuno")
	}
}

func TestFetchUnknownAttributeIsSkipped(t *testing.T) {
	n := 0
	client, _ := newScriptedClient(t, func(string) []string {
		n++
		if n == 1 {
			// attributo sconosciuto con lista, quotato e literal: scartato per intero
			return []string{"* 1 FETCH (X-FUTURE (a \"b\" (c d) {3}\r\nxyz) UID 9 FLAGS (\\Seen))"}
		}
		return []string{"* 1 FETCH (UID 9)"}
	})
	msgs, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		t.Fatalf("msg-att sconosciuto non deve essere errore: %v", err)
	}
	if len(msgs) != 1 || msgs[0].UID != 9 || len(msgs[0].Flags) != 1 {
		t.Fatalf("UID/Flags dopo attributo sconosciuto: %+v", msgs)
	}
	// la connessione resta utilizzabile per il comando successivo
	if _, err := client.Fetch(imap.SeqSetNum(1), &imap.FetchOptions{UID: true}).Collect(); err != nil {
		t.Fatalf("connessione non più utilizzabile: %v", err)
	}
}
