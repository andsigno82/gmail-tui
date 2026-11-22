package mcpserver

import (
	"reflect"
	"sort"
	"testing"

	"github.com/andsigno82/gmail-tui/internal/mail"
)

// Piano 14, Task 2: MailStore e Mock soddisfano Reader (controllo a compile time).
var (
	_ Reader = mail.MailStore(nil)
	_ Reader = (*mail.Mock)(nil)
)

// Piano 14, Task 2: Reader non espone metodi di scrittura. L'insieme dei
// metodi è quello atteso, per nome.
func TestReaderHasOnlyReadMethods(t *testing.T) {
	var got []string
	methodType := reflect.TypeOf((*Reader)(nil)).Elem()
	for i := 0; i < methodType.NumMethod(); i++ {
		got = append(got, methodType.Method(i).Name)
	}
	sort.Strings(got)
	want := []string{"Connect", "FetchMessage", "ListMailboxes", "ListMessages", "Search"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metodi di Reader = %v, attesi %v", got, want)
	}
}

// Piano 14, Task 2: offset/limit diventano mail.Page con i default e i tetti
// di M5 (default 20, massimo 100); valori negativi rifiutati.
func TestNormalizePage(t *testing.T) {
	cases := []struct {
		name          string
		offset, limit int
		want          mail.Page
		wantErr       bool
	}{
		{"default", 0, 0, mail.Page{Offset: 0, Limit: 20}, false},
		{"tetto", 0, 500, mail.Page{Offset: 0, Limit: 100}, false},
		{"valore valido", 40, 15, mail.Page{Offset: 40, Limit: 15}, false},
		{"limit negativo", 0, -1, mail.Page{}, true},
		{"offset negativo", -3, 10, mail.Page{}, true},
	}
	for _, c := range cases {
		got, err := normalizePage(c.offset, c.limit)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: errore = %v, atteso errore %v", c.name, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("%s: Page = %+v, atteso %+v", c.name, got, c.want)
		}
	}
}
