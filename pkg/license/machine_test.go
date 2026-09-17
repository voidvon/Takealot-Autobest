package license

import (
	"testing"
)

func TestGetMachineID(t *testing.T) {
	id1 := GetMachineID()
	id2 := GetMachineID()
	if id1 == "" {
		t.Fatal("MachineID is empty")
	}
	if id1 != id2 {
		t.Fatalf("MachineID inconsistent: %s vs %s", id1, id2)
	}
	t.Logf("Generated Machine ID: %s", id1)
}
