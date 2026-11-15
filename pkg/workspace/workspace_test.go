package workspace

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecutableOwnsWorkspaceAndSameFolderCannotOpenTwice(t *testing.T) {
	base := t.TempDir()
	cases := []struct{ exe, want string }{
		{filepath.Join(base, "copy-a", "Takealot.exe"), filepath.Join(base, "copy-a", "data")},
		{filepath.Join(base, "copy-b", "Takealot.app", "Contents", "MacOS", "Takealot"), filepath.Join(base, "copy-b", "data")},
	}
	for _, tc := range cases {
		layout, err := Resolve(tc.exe, "")
		if err != nil || layout.Root != tc.want {
			t.Fatalf("resolve: %+v %v", layout, err)
		}
		if err := layout.Open(); err != nil {
			t.Fatal(err)
		}
		other := Layout{Root: layout.Root}
		if err := other.Open(); err == nil {
			other.Close()
			t.Fatal("same workspace opened twice")
		}
		if err := layout.RecordRuntime("127.0.0.1:23456"); err != nil {
			t.Fatal(err)
		}
		layout.Close()
		if _, err := os.Stat(filepath.Join(layout.Root, "runtime.json")); !os.IsNotExist(err) {
			t.Fatal("stale runtime record")
		}
		if err := other.Open(); err != nil {
			t.Fatal("stale lock prevented restart", err)
		}
		other.Close()
	}
	a, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	if a.Addr().String() == b.Addr().String() {
		t.Fatal("instances shared port")
	}
}

func TestMigrationSnapshotsCommittedWALAndDoesNotReimport(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, "takealot.db")
	conn, err := sql.Open("sqlite", legacy)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE owned_data(value TEXT); INSERT INTO owned_data VALUES('from-wal')"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy + "-wal"); err != nil {
		t.Fatal("test requires a live WAL", err)
	}
	layout := Layout{Root: filepath.Join(base, "data"), LegacyRoot: base}
	if err := layout.Open(); err != nil {
		t.Fatal(err)
	}
	defer layout.Close()
	migrated, err := layout.MigrateLegacy()
	if err != nil || !migrated {
		t.Fatal("migration failed", err)
	}
	copied, err := sql.Open("sqlite", layout.Database())
	if err != nil {
		t.Fatal(err)
	}
	defer copied.Close()
	var value string
	if err := copied.QueryRow("SELECT value FROM owned_data").Scan(&value); err != nil || value != "from-wal" {
		t.Fatal("lost WAL content", value, err)
	}
	if _, err := conn.Exec("INSERT INTO owned_data VALUES('later-original')"); err != nil {
		t.Fatal(err)
	}
	migrated, err = layout.MigrateLegacy()
	if err != nil || migrated {
		t.Fatal("existing portable state reimported", err)
	}
	var count int
	if err := copied.QueryRow("SELECT count(*) FROM owned_data").Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy changes leaked into portable state", count, err)
	}
}

func TestFreshWorkspaceNeverImportsCWDOrLegacyConfigOnly(t *testing.T) {
	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "config.json"), []byte(`{"authorization":"unrelated"}`), 0600); err != nil {
		t.Fatal(err)
	}
	layout := Layout{Root: filepath.Join(base, "data"), LegacyRoot: base}
	if err := layout.Open(); err != nil {
		t.Fatal(err)
	}
	defer layout.Close()
	migrated, err := layout.MigrateLegacy()
	if err != nil || migrated {
		t.Fatal(migrated, err)
	}
	if _, err := os.Stat(filepath.Join(layout.Root, "config.json")); !os.IsNotExist(err) {
		t.Fatal("adopted unrelated config")
	}
}

func TestSymlinkUsesSameWorkspaceLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink privileges depend on Windows policy")
	}
	base := t.TempDir()
	original := filepath.Join(base, "original")
	alias := filepath.Join(base, "alias")
	if err := os.Mkdir(original, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, alias); err != nil {
		t.Fatal(err)
	}
	a := Layout{Root: original}
	if err := a.Open(); err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b := Layout{Root: alias}
	if err := b.Open(); err == nil {
		b.Close()
		t.Fatal("alias bypassed directory lock")
	}
}
