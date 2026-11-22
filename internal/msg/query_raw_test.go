package msg

import "testing"

// Piano 8, Task 5: una query con solo Raw non è vuota.
func TestQueryIsEmptyConsidersRaw(t *testing.T) {
	if (Query{Raw: "older_than:7d"}).IsEmpty() {
		t.Error("Query{Raw} risulta vuota")
	}
	if !(Query{}).IsEmpty() {
		t.Error("Query{} non risulta vuota")
	}
}
