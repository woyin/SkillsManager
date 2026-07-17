package cmd

import (
	"path/filepath"

	"github.com/woyin/skills-manager/internal/db"
)

func openDB() (*db.DB, error) {
	return db.Open(filepath.Join(DataDir, "sm.db"))
}

func dbPath() string {
	return filepath.Join(DataDir, "sm.db")
}
