package main

import (
	"path/filepath"

	"takealot/pkg/config"
	"takealot/pkg/db"
)

// Startup opens local state only. It must not create a store, adopt a legacy
// seller credential or import monitor targets without an explicit user action.
func openApplicationData(baseDir string) (*db.DB, *config.Manager, error) {
	database, err := db.New(filepath.Join(baseDir, "takealot.db"))
	if err != nil {
		return nil, nil, err
	}
	return database, config.NewManager(baseDir), nil
}
