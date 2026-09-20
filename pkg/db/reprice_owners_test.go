package db

import (
	"path/filepath"
	"sync"
	"testing"
)

func TestRepriceOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owners.db")
	d, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { d.Close() }()
	for _, id := range []string{"A", "B"} {
		if err := d.SaveStore(Store{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	claim := func(store, tsin, plid, want string) {
		t.Helper()
		owner, err := d.ClaimRepriceProduct(store, tsin, plid)
		if err != nil || owner != want {
			t.Fatalf("claim(%s, %s, %s) = %q, %v; want %s", store, tsin, plid, owner, err, want)
		}
	}
	claim("A", "100", "200", "A")
	claim("B", "100", "", "A")
	claim("B", "101", "200", "A")
	claim("B", "101", "201", "B") // Rejected claim must not reserve TSIN 101.
	claim("A", "00100", "200", "A")
	claim("B", "100", "201", "A") // Conflicting aliases cannot bypass protection.
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	d, err = New(path)
	if err != nil {
		t.Fatal(err)
	}
	claim("B", "100", "200", "A")
	claim("A", "100", "200", "A")
	if err := d.DeleteStore("A"); err != nil {
		t.Fatal(err)
	}
	claim("B", "100", "200", "B")
	if _, err := d.ClaimRepriceProduct("A", "999", "999"); err == nil {
		t.Fatal("deleted store claimed product")
	}
	if _, err := d.ClaimRepriceProduct("B", "", "0"); err == nil {
		t.Fatal("invalid identifiers accepted")
	}
}

func TestConcurrentRepriceOwnership(t *testing.T) {
	d, err := New(filepath.Join(t.TempDir(), "owners.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	for _, id := range []string{"A", "B"} {
		if err := d.SaveStore(Store{ID: id, Name: id}); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	owners := make(chan string, 20)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			owner, err := d.ClaimRepriceProduct([]string{"A", "B"}[i%2], "100", "200")
			if err != nil {
				t.Error(err)
				return
			}
			owners <- owner
		}(i)
	}
	close(start)
	wg.Wait()
	close(owners)
	winner := ""
	for owner := range owners {
		if winner == "" {
			winner = owner
		}
		if owner != winner {
			t.Fatalf("multiple owners: %s and %s", winner, owner)
		}
	}
}
