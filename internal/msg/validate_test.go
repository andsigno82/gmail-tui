package msg

import (
	"reflect"
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
		{"dominio senza punto", Outgoing{To: []string{"a@b"}}, `A: indirizzo non valido "a@b"`},
		{"local part non ASCII", Outgoing{To: []string{"josé@x.it"}}, `A: indirizzo non valido "josé@x.it"`},
		{"dominio IDN", Outgoing{To: []string{"x@café.it"}}, ""},
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

// TestParseRecipientField (Piano 7.1, fix code-review): un campo è una lista RFC
// 5322 (la virgola nel nome quotato non spezza); un campo non parsabile resta
// come elemento unico, così la validazione lo rifiuta citandolo.
func TestParseRecipientField(t *testing.T) {
	cases := []struct {
		name  string
		field string
		want  []string
	}{
		{"campo vuoto", "  ", nil},
		{"lista con virgola finale", " a@x.it , b@y.it , ", []string{"a@x.it", "b@y.it"}},
		{"nome con virgola quotata", `"Rossi, Mario" <m@x.it>`, []string{`"Rossi, Mario" <m@x.it>`}},
		{"campo non parsabile", "a@x.it, b@@y", []string{"a@x.it, b@@y"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseRecipientField(tc.field); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseRecipientField(%q) = %q, want %q", tc.field, got, tc.want)
			}
		})
	}
}

// TestRecipientFieldInvalidCitesField (Piano 7.1, fix code-review): un campo non
// parsabile non perde indirizzi silenziosamente: l'errore cita il campo.
func TestRecipientFieldInvalidCitesField(t *testing.T) {
	err := ValidateAddresses(Outgoing{To: ParseRecipientField("a@x.it, b@@y")})
	if want := `A: indirizzo non valido "a@x.it, b@@y"`; err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %q", err, want)
	}
}

// TestNormalizeQuotedLocalPart (Piano 7.1, fix code-review): una local part
// quotata con spazio resta quotata nella forma canonica (RCPT e header validi).
func TestNormalizeQuotedLocalPart(t *testing.T) {
	got, err := NormalizeRecipients(Outgoing{To: []string{`"a b"@x.it`}})
	if err != nil {
		t.Fatalf("errore inatteso: %v", err)
	}
	if want := []string{`"a b"@x.it`}; !reflect.DeepEqual(got.To, want) {
		t.Errorf("To = %q, want %q", got.To, want)
	}
}

// TestNormalizeRecipients (Piano 7.1, Task 1): forma canonica di ogni indirizzo
// (nome RFC 2047 se non ASCII, dominio in punycode), errori come ValidateRecipients.
func TestNormalizeRecipients(t *testing.T) {
	cases := []struct {
		name    string
		in      Outgoing
		want    Outgoing
		wantErr string
	}{
		{
			"nome visualizzato quotato",
			Outgoing{To: []string{"Mario Rossi <m@x.it>"}},
			Outgoing{To: []string{`"Mario Rossi" <m@x.it>`}},
			"",
		},
		{
			"dominio IDN in punycode",
			Outgoing{To: []string{"x@café.it"}},
			Outgoing{To: []string{"x@xn--caf-dma.it"}},
			"",
		},
		{
			"elementi vuoti scartati, ordine e duplicati preservati",
			Outgoing{To: []string{" b@x.it ", "", "a@x.it", "b@x.it"}, Bcc: []string{"  "}},
			Outgoing{To: []string{"b@x.it", "a@x.it", "b@x.it"}},
			"",
		},
		{
			"dominio senza punto",
			Outgoing{To: []string{"a@b"}},
			Outgoing{},
			`A: indirizzo non valido "a@b"`,
		},
		{
			"local part non ASCII",
			Outgoing{To: []string{"x@x.it"}, Cc: []string{"josé@x.it"}},
			Outgoing{},
			`Cc: indirizzo non valido "josé@x.it"`,
		},
		{
			"nessun destinatario",
			Outgoing{Subject: "s"},
			Outgoing{},
			"nessun destinatario",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeRecipients(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want contenente %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("errore inatteso: %v", err)
			}
			if !reflect.DeepEqual(got.To, tc.want.To) || !reflect.DeepEqual(got.Cc, tc.want.Cc) || !reflect.DeepEqual(got.Bcc, tc.want.Bcc) {
				t.Errorf("To/Cc/Bcc = %q/%q/%q, want %q/%q/%q", got.To, got.Cc, got.Bcc, tc.want.To, tc.want.Cc, tc.want.Bcc)
			}
		})
	}
}
