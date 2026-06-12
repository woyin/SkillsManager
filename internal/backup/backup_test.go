// internal/backup/backup_test.go
package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupCreatesArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup test environment
	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	// Create test files
	if err := os.WriteFile(filepath.Join(registryDir, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "test.json"), []byte("{}"), 0644); err != nil {
		t.Fatalf("creating test file: %v", err)
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create backup
	archivePath, err := m.Backup("test-backup")
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Verify archive exists
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Fatal("backup archive not created")
	}

	// Verify archive has content
	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("stat archive: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("backup archive is empty")
	}
}

func TestBackupAutoName(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create backup with auto name
	archivePath, err := m.Backup("")
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Verify archive name contains "backup-"
	name := filepath.Base(archivePath)
	if name == ".tar.gz" {
		t.Fatal("backup should have auto-generated name")
	}
}

func TestRestoreFromArchive(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup initial environment
	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	// Create initial content
	initialContent := []byte("initial content")
	if err := os.WriteFile(filepath.Join(registryDir, "skill.txt"), initialContent, 0644); err != nil {
		t.Fatalf("creating initial file: %v", err)
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create backup
	archivePath, err := m.Backup("before-change")
	if err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Modify content
	modifiedContent := []byte("modified content")
	if err := os.WriteFile(filepath.Join(registryDir, "skill.txt"), modifiedContent, 0644); err != nil {
		t.Fatalf("modifying file: %v", err)
	}

	// Restore from backup
	if err := m.Restore(archivePath); err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	// Verify content is restored
	content, err := os.ReadFile(filepath.Join(registryDir, "skill.txt"))
	if err != nil {
		t.Fatalf("reading restored file: %v", err)
	}
	if string(content) != string(initialContent) {
		t.Errorf("expected %q, got %q", initialContent, content)
	}
}

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create multiple backups with explicit names
	names := []string{"backup-1", "backup-2", "backup-3"}
	for _, name := range names {
		if _, err := m.Backup(name); err != nil {
			t.Fatalf("backup %s failed: %v", name, err)
		}
	}

	// List backups
	backups, err := m.List()
	if err != nil {
		t.Fatalf("listing backups: %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("expected 3 backups, got %d", len(backups))
	}
}

func TestRotateKeepsN(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create 5 backups with explicit names
	names := []string{"backup-1", "backup-2", "backup-3", "backup-4", "backup-5"}
	for _, name := range names {
		if _, err := m.Backup(name); err != nil {
			t.Fatalf("backup %s failed: %v", name, err)
		}
	}

	// Rotate to keep 3
	if err := m.Rotate(3); err != nil {
		t.Fatalf("rotate failed: %v", err)
	}

	// Verify only 3 remain
	backups, err := m.List()
	if err != nil {
		t.Fatalf("listing backups: %v", err)
	}

	if len(backups) != 3 {
		t.Errorf("expected 3 backups after rotate, got %d", len(backups))
	}
}

func TestFindByName(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create backup with specific name
	if _, err := m.Backup("my-backup"); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Find by name
	info, err := m.FindByName("my-backup")
	if err != nil {
		t.Fatalf("find by name failed: %v", err)
	}

	if info.Name != "my-backup" {
		t.Errorf("expected name 'my-backup', got %q", info.Name)
	}
}

func TestFindLatest(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	m := New(dataDir, registryDir, profilesDir)

	// Create backups
	if _, err := m.Backup("first"); err != nil {
		t.Fatalf("backup failed: %v", err)
	}
	if _, err := m.Backup("second"); err != nil {
		t.Fatalf("backup failed: %v", err)
	}

	// Find latest
	info, err := m.FindLatest()
	if err != nil {
		t.Fatalf("find latest failed: %v", err)
	}

	if info.Name != "second" {
		t.Errorf("expected latest to be 'second', got %q", info.Name)
	}
}

func TestFindLatestNoBackups(t *testing.T) {
	tmpDir := t.TempDir()

	dataDir := filepath.Join(tmpDir, "data")
	registryDir := filepath.Join(tmpDir, "registry")
	profilesDir := filepath.Join(tmpDir, "profiles")

	for _, dir := range []string{dataDir, registryDir, profilesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("creating directory: %v", err)
		}
	}

	m := New(dataDir, registryDir, profilesDir)

	// Try to find latest with no backups
	_, err := m.FindLatest()
	if err == nil {
		t.Error("expected error when no backups exist")
	}
}
