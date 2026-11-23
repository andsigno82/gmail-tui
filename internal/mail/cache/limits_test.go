package cache

import "testing"

// Piano 16, Task 2: i limiti sono costanti (D16-A3), non campi di config.
func TestLimits(t *testing.T) {
	if MaxBodies != 200 || MaxThreadSnapshots != 200 || MaxListSnapshots != 50 || SnapshotRows != 50 {
		t.Fatalf("limiti = %d/%d/%d/%d, voglio 200/200/50/50",
			MaxBodies, MaxThreadSnapshots, MaxListSnapshots, SnapshotRows)
	}
}
