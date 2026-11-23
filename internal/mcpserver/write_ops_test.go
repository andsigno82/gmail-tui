package mcpserver

import (
	"reflect"
	"sort"
	"testing"
)

// Piano 10, Task 8 (D10-C4): Restore, AllMailIDs e ModifyLabels sono scritture e restano
// fuori dal server MCP. Il Reader è read-only: il test fissa il method set con una lista
// chiusa, così ogni metodo nuovo (di lettura o di scrittura) va dichiarato qui.
func TestReaderMethodSetIsReadOnly(t *testing.T) {
	want := []string{"Connect", "FetchMessage", "ListConversations", "ListMailboxes", "ListMessages", "ThreadMessages"}
	rt := reflect.TypeOf((*Reader)(nil)).Elem()
	var got []string
	for i := 0; i < rt.NumMethod(); i++ {
		got = append(got, rt.Method(i).Name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("metodi del Reader = %v, attesi %v: un nuovo metodo va verificato come sola lettura", got, want)
	}
	for _, w := range []string{"Restore", "AllMailIDs", "ModifyLabels", "Trash", "Archive", "ApplyLabel"} {
		if _, ok := rt.MethodByName(w); ok {
			t.Errorf("il Reader espone la scrittura %s", w)
		}
	}
}
