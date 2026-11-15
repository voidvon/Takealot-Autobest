package account

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPortableCredentialCopiesWithoutMachineOrPathBinding(t *testing.T) {
	base := t.TempDir()
	a := filepath.Join(base, "copy-a")
	b := filepath.Join(base, "copy-b")
	store := NewFileStore(a, "https://member.example/", "vip")
	if err := store.Save("portable-token"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b, filepath.Base(store.path)), data, 0600); err != nil {
		t.Fatal(err)
	}
	copied := NewFileStore(b, "https://member.example", "vip")
	if token, err := copied.Load(); err != nil || token != "portable-token" {
		t.Fatal("copied token not restored", err)
	}
	if err := copied.Save("copy-b-token"); err != nil {
		t.Fatal(err)
	}
	if token, err := store.Load(); err != nil || token != "portable-token" {
		t.Fatal("second copy overwrote first", err)
	}
	if err := copied.Delete(); err != nil {
		t.Fatal(err)
	}
	if token, err := store.Load(); err != nil || token != "portable-token" {
		t.Fatal("second logout deleted first file", err)
	}
	for _, other := range []*FileStore{NewFileStore(a, "https://other.example", "vip"), NewFileStore(a, "https://member.example", "other-group")} {
		if _, err := other.Load(); !os.IsNotExist(err) {
			t.Fatal("different service adopted token", err)
		}
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(store.path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatal("credential permissions", err)
		}
	}
}
