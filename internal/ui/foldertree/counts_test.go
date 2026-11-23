package foldertree

import (
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// Piano 15, Task 10: il contatore è "(N)" (Q15-4).
func TestCountLabel(t *testing.T) {
	for n, want := range map[int]string{3: "(3)", 12: "(12)"} {
		if got := countLabel(n); got != want {
			t.Errorf("countLabel(%d) = %q, want %q", n, got, want)
		}
	}
}

// Q15-4: contatori su INBOX e label, non su speciali.
func TestIsCounted(t *testing.T) {
	cases := []struct {
		box  mail.Mailbox
		want bool
	}{
		{mail.Mailbox{Name: "INBOX", Role: "inbox"}, true},
		{mail.Mailbox{Name: "Lavoro"}, true},
		{mail.Mailbox{Name: "[Gmail]/Sent Mail", Role: "sent"}, false},
		{mail.Mailbox{Name: "[Gmail]/Drafts", Role: "drafts"}, false},
		{mail.Mailbox{Name: "[Gmail]/Spam", Role: "junk"}, false},
		{mail.Mailbox{Name: "[Gmail]/Trash", Role: "trash"}, false},
		{mail.Mailbox{Name: "[Gmail]/All Mail", Role: "all"}, false},
	}
	for _, c := range cases {
		if got := isCounted(c.box); got != c.want {
			t.Errorf("isCounted(%q, role %q) = %v, want %v", c.box.Name, c.box.Role, got, c.want)
		}
	}
}
