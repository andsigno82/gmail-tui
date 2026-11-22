package msg

import (
	"strings"
	"testing"
)

// TestValidateRecipients (Piano 7, Task 2): indirizzi validi passano (anche
// con nome visualizzato), malformati o assenti falliscono citando campo e
// indirizzo.
func TestValidateRecipients(t *testing.T) {
	cases := []struct {
		name    string
		o       Outgoing
		wantErr string // "" = nessun errore
	}{
		{"solo to", Outgoing{To: []string{"a@b.it"}}, ""},
		{"nome visualizzato", Outgoing{To: []string{"Mario Rossi <m@x.it>"}}, ""},
		{"dominio senza punto", Outgoing{To: []string{"a@b"}}, ""},
		{"solo bcc", Outgoing{Bcc: []string{"c@d.it"}}, ""},
		{"nessuno", Outgoing{}, "nessun destinatario"},
		{"solo vuoti", Outgoing{To: []string{"  "}}, "nessun destinatario"},
		{"to malformato", Outgoing{To: []string{"abc"}}, `A: indirizzo non valido "abc"`},
		{"cc senza dominio", Outgoing{To: []string{"a@b.it"}, Cc: []string{"x@"}}, `Cc: indirizzo non valido "x@"`},
		{"bcc malformato", Outgoing{To: []string{"a@b.it"}, Bcc: []string{"<>"}}, `Ccn: indirizzo non valido "<>"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRecipients(tc.o)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("errore inatteso: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want contenente %q", err, tc.wantErr)
			}
		})
	}
}

// TestValidateAddressesIgnoresEmptyLists: ValidateAddresses controlla solo il
// formato (Piano 7, Task 6, composer): liste vuote passano, la presenza di un
// destinatario resta di ValidateRecipients; un indirizzo malformato è sempre un errore.
func TestValidateAddressesIgnoresEmptyLists(t *testing.T) {
	if err := ValidateAddresses(Outgoing{Subject: "bozza senza destinatari"}); err != nil {
		t.Errorf("liste vuote: err = %v, voglio nil", err)
	}
	if err := ValidateAddresses(Outgoing{To: []string{"a@b.it"}, Cc: []string{"Mario <m@x.it>"}}); err != nil {
		t.Errorf("indirizzi validi: err = %v, voglio nil", err)
	}
	if err := ValidateAddresses(Outgoing{To: []string{"a@b.it"}, Bcc: []string{"abc"}}); err == nil || !strings.Contains(err.Error(), "Ccn") {
		t.Errorf("Ccn malformato: err = %v, voglio errore che cita Ccn", err)
	}
}
