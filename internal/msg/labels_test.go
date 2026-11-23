package msg

import (
	"reflect"
	"testing"
)

// Piano 10, review punto 3 (Task 7): una sola definizione di label utente. Non sono
// label utente l'attributo di sistema (\Inbox, \Sent…), le etichette di sistema di
// Gmail scritte senza backslash, le cartelle [Gmail]/… e INBOX.
func TestIsUserLabel(t *testing.T) {
	cases := map[string]bool{
		"Lavoro":                   true,
		"Lavoro/Clienti":           true,
		"Work Projects":            true,
		"":                         false,
		`\Inbox`:                   false,
		`\Sent`:                    false,
		"Important":                false,
		"Starred":                  false,
		"Chats":                    false,
		"INBOX":                    false,
		"inbox":                    false,
		"[Gmail]/Tutti i messaggi": false,
		"[gmail]/Cestino":          false,
	}
	for name, want := range cases {
		if got := IsUserLabel(name); got != want {
			t.Errorf("IsUserLabel(%q) = %v, voglio %v", name, got, want)
		}
	}
}

// UserLabels filtra con la stessa definizione, nell'ordine dato.
func TestUserLabelsKeepsOrder(t *testing.T) {
	got := UserLabels([]string{`\Inbox`, "Work", "INBOX", "Lavoro", "[Gmail]/Bozze", "Starred"})
	want := []string{"Work", "Lavoro"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("UserLabels = %v, voglio %v", got, want)
	}
	if UserLabels(nil) != nil {
		t.Errorf("UserLabels(nil) = %v, voglio nil", UserLabels(nil))
	}
}
