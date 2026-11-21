package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"gmail-tui/internal/mail"
	"gmail-tui/internal/msg"
)

func fetchRef(id msg.MessageID, from, subject string, date time.Time, unread, starred bool) msg.MessageRef {
	return msg.MessageRef{
		ID:      id,
		From:    from,
		Subject: subject,
		Date:    date,
		Unread:  unread,
		Starred: starred,
	}
}

// TestFetchRows verifica righe esatte (uid\tfrom\tsubject\tdate\tflags),
// ordine data desc del mock, flag "U"/"*" concatenati ed exit 0.
func TestFetchRows(t *testing.T) {
	d1 := time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC)
	d2 := time.Date(2026, 11, 21, 9, 30, 0, 0, time.UTC)
	m := &mail.Mock{Msgs: []*msg.Message{
		{Ref: fetchRef("INBOX:2", "Bob <bob@x>", "Ciao", d2, true, true)},
		{Ref: fetchRef("INBOX:1", "alice@x", "Piano", d1, false, false)},
	}}
	var out bytes.Buffer
	if code := Fetch(&out, m, msg.Query{}, 0); code != 0 {
		t.Fatalf("Fetch exit = %d, want 0", code)
	}
	want := strings.Join([]string{
		"INBOX:2\tBob <bob@x>\tCiao\t2026-11-21T09:30:00Z\tU*",
		"INBOX:1\talice@x\tPiano\t2026-11-20T10:00:00Z\t",
		"",
	}, "\n")
	if out.String() != want {
		t.Errorf("output:\n%q\nwant:\n%q", out.String(), want)
	}
}

// TestFetchFlags singoli: solo "U", solo "*", nessuno.
func TestFetchFlags(t *testing.T) {
	d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &mail.Mock{Msgs: []*msg.Message{
		{Ref: fetchRef("INBOX:3", "u@x", "solo unread", d, true, false)},
		{Ref: fetchRef("INBOX:2", "s@x", "solo starred", d, false, true)},
		{Ref: fetchRef("INBOX:1", "n@x", "nessuno", d, false, false)},
	}}
	var out bytes.Buffer
	if code := Fetch(&out, m, msg.Query{}, 0); code != 0 {
		t.Fatalf("Fetch exit = %d, want 0", code)
	}
	flags := []string{}
	for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
		flags = append(flags, strings.Split(line, "\t")[4])
	}
	want := []string{"U", "*", ""}
	for i, w := range want {
		if flags[i] != w {
			t.Errorf("flags[%d] = %q, want %q", i, flags[i], w)
		}
	}
}

// TestFetchLimit: limit esplicito taglia; limit <= 0 ⇒ default 20.
func TestFetchLimit(t *testing.T) {
	m := &mail.Mock{}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 25; i++ {
		m.Msgs = append(m.Msgs, &msg.Message{Ref: fetchRef(
			msg.NewID("INBOX", uint32(i+1)), "f@x", "s", base.Add(time.Duration(i)*time.Hour), false, false)})
	}

	countLines := func(out bytes.Buffer) int {
		return len(strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n"))
	}

	var out bytes.Buffer
	if code := Fetch(&out, m, msg.Query{}, 3); code != 0 {
		t.Fatalf("Fetch(limit=3) exit = %d, want 0", code)
	}
	if n := countLines(out); n != 3 {
		t.Errorf("limit=3: %d righe, want 3", n)
	}

	var outDef bytes.Buffer
	if code := Fetch(&outDef, m, msg.Query{}, 0); code != 0 {
		t.Fatalf("Fetch(limit=0) exit = %d, want 0", code)
	}
	if n := countLines(outDef); n != 20 {
		t.Errorf("limit=0 (default): %d righe, want 20", n)
	}
}

// TestFetchEmpty: nessun risultato ⇒ exit 0, output vuoto.
func TestFetchEmpty(t *testing.T) {
	m := &mail.Mock{}
	var out bytes.Buffer
	if code := Fetch(&out, m, msg.Query{}, 0); code != 0 {
		t.Fatalf("Fetch exit = %d, want 0", code)
	}
	if out.Len() != 0 {
		t.Errorf("output = %q, want vuoto", out.String())
	}
}

// TestFetchError: store in errore ⇒ exit 1.
func TestFetchError(t *testing.T) {
	m := &mail.Mock{Err: errors.New("rete giù")}
	var out bytes.Buffer
	if code := Fetch(&out, m, msg.Query{}, 0); code != 1 {
		t.Errorf("Fetch exit = %d, want 1", code)
	}
}
