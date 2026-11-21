package msg

import (
	"reflect"
	"testing"
)

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Query
	}{
		{"vuota", "", Query{}},
		{"solo spazi", "   ", Query{}},
		{"solo testo", "hello world", Query{Text: "hello world"}},
		{"from", "from:a@b.c", Query{From: "a@b.c"}},
		{"from + testo", "from:a text", Query{From: "a", Text: "text"}},
		{"to", "to:b@d.e", Query{To: "b@d.e"}},
		{"unread", "is:unread", Query{Unread: true}},
		{"starred", "is:starred", Query{Starred: true}},
		{"has attachment", "has:attachment", Query{HasAttachment: true}},
		{"label", "label:Work", Query{Label: "Work"}},
		{"unread + label + testo", "is:unread label:Work x y", Query{Unread: true, Label: "Work", Text: "x y"}},
		{"chiave ignota resta testo", "foo:bar", Query{Text: "foo:bar"}},
		{"is con valore ignoto resta testo", "is:read", Query{Text: "is:read"}},
		{"has con valore ignoto resta testo", "has:pdf", Query{Text: "has:pdf"}},
		{"from ultimo vince", "from:a from:b", Query{From: "b"}},
		{"to ultimo vince", "to:a to:b", Query{To: "b"}},
		{"label ultima vince", "label:A label:B", Query{Label: "B"}},
		{"testo si accumula", "uno due tre", Query{Text: "uno due tre"}},
		{"misto", "is:starred from:x pippo to:y pluto", Query{Starred: true, From: "x", To: "y", Text: "pippo pluto"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseQuery(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseQuery(%q) = %+v, want %+v", tt.in, got, tt.want)
			}
		})
	}
}
