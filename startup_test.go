package main

import (
	"os"
	"path/filepath"
	"testing"

	"takealot/pkg/db"
)

func TestFreshStartupDoesNotAdoptLegacyStoresOrTargets(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"config.json": `{"authorization":"Key unrelated-legacy-test-key","targets":{"111/500":{"selected":true,"min_price":210}}}`,
		"can.ini":     "legacy\nlegacy\n10\n1\n120\n885\n1",
		"GJDATA":      "111/500/1/210/400",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		database, _, err := openApplicationData(dir)
		if err != nil {
			t.Fatal(err)
		}
		stores, err := database.GetStores()
		if err != nil || len(stores) != 0 {
			t.Fatalf("startup created stores: %+v %v", stores, err)
		}
		targets, err := database.LoadStoreTargets("all")
		if err != nil || len(targets) != 0 {
			t.Fatalf("startup imported legacy monitors: %+v %v", targets, err)
		}
		database.Close()
	}
	for name, want := range files {
		actual, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(actual) != want {
			t.Fatalf("startup changed legacy file %s", name)
		}
	}
}

func TestStartupPreservesExistingStoresAndAllowsReturningToEmpty(t *testing.T) {
	dir := t.TempDir()
	database, _, err := openApplicationData(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.SaveStore(db.Store{ID: "own-shop", Name: "My shop", Authorization: "Key own-test-key"}); err != nil {
		t.Fatal(err)
	}
	database.Close()
	database, _, err = openApplicationData(dir)
	if err != nil {
		t.Fatal(err)
	}
	stores, err := database.GetStores()
	if err != nil || len(stores) != 1 || stores[0].ID != "own-shop" || stores[0].Authorization != "Key own-test-key" {
		t.Fatalf("existing stores changed: %+v %v", stores, err)
	}
	if err := database.DeleteStore("own-shop"); err != nil {
		t.Fatal(err)
	}
	database.Close()
	database, _, err = openApplicationData(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	stores, err = database.GetStores()
	if err != nil || len(stores) != 0 {
		t.Fatalf("restart recreated a deleted store: %+v %v", stores, err)
	}
}
