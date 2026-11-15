package workspace

import "testing"

func TestPreferencesPersistAndRejectInvalidUpdates(t *testing.T) {
	root := t.TempDir()
	s, err := NewPreferencesStore(root)
	if err != nil {
		t.Fatal(err)
	}
	theme, store := "dark", "shop-a"
	if _, err := s.Update(&theme, &store); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewPreferencesStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if value := reopened.Get(); value.Theme != "dark" || value.ActiveStoreID != "shop-a" {
		t.Fatal(value)
	}
	invalid := "invalid"
	if _, err := s.Update(&invalid, nil); err == nil {
		t.Fatal("invalid theme accepted")
	}
	if s.Get().Theme != "dark" {
		t.Fatal("failed write changed live preferences")
	}
	other, err := NewPreferencesStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if other.Get().ActiveStoreID != "" {
		t.Fatal("new folder inherited store selection")
	}
}
