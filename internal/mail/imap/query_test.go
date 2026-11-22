package imap

import (
	"reflect"
	"testing"

	"github.com/emersion/go-imap/v2"

	"github.com/andsigno82/gmail-tui/internal/msg"
)

// TestSearchCriteria verifica la tabella di traduzione Query → SEARCH:
// la SEMANTICA è il contratto, i nomi campo sono quelli dell'API pinnata
// (go-imap/v2 v2.0.0-beta.9). reflect.DeepEqual sull'intero criterio
// assicura che nessun campo extra venga impostato.
func TestSearchCriteria(t *testing.T) {
	tests := []struct {
		name string
		q    msg.Query
		want *imap.SearchCriteria
	}{
		{
			name: "query vuota: criteri che matchano tutto",
			q:    msg.Query{},
			want: &imap.SearchCriteria{},
		},
		{
			name: "from: header From",
			q:    msg.Query{From: "alice@example.org"},
			want: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{
					{Key: "From", Value: "alice@example.org"},
				},
			},
		},
		{
			name: "to: header To",
			q:    msg.Query{To: "bob@example.org"},
			want: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{
					{Key: "To", Value: "bob@example.org"},
				},
			},
		},
		{
			name: "text: text search (corpo+subject lato server)",
			q:    msg.Query{Text: "fattura"},
			want: &imap.SearchCriteria{
				Text: []string{"fattura"},
			},
		},
		{
			name: "unread: senza flag \\Seen",
			q:    msg.Query{Unread: true},
			want: &imap.SearchCriteria{
				NotFlag: []imap.Flag{imap.FlagSeen},
			},
		},
		{
			name: "starred: con flag \\Flagged",
			q:    msg.Query{Starred: true},
			want: &imap.SearchCriteria{
				Flag: []imap.Flag{imap.FlagFlagged},
			},
		},
		{
			name: "combinazione from+unread+text: intersezione (AND)",
			q:    msg.Query{From: "carol@example.org", Unread: true, Text: "report"},
			want: &imap.SearchCriteria{
				Header: []imap.SearchCriteriaHeaderField{
					{Key: "From", Value: "carol@example.org"},
				},
				Text:    []string{"report"},
				NotFlag: []imap.Flag{imap.FlagSeen},
			},
		},
		{
			name: "label e has:attachment: ignorate (gestite altrove)",
			q:    msg.Query{Label: "Work", HasAttachment: true},
			want: &imap.SearchCriteria{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := searchCriteria(tt.q)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("searchCriteria(%+v)\n  got  %+v\n  want %+v", tt.q, got, tt.want)
			}
		})
	}
}
