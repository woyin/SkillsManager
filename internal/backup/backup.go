// internal/backup/backup.go
package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manager creates, lists, and restores tar.gz backups of sm's registry,
// profiles, and database state.
type Manager struct {
	dataDir     string
	registryDir string
	profilesDir string
}

// BackupInfo describes one backup archive on disk.
type BackupInfo struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
}

// metadata is the JSON manifest embedded inside each backup archive.
type metadata struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// New returns a backup Manager pointing at the given data, registry, and
// profiles directories.
func New(dataDir, registryDir, profilesDir string) *Manager {
	return &Manager{
		dataDir:     dataDir,
		registryDir: registryDir,
		profilesDir: profilesDir,
	}
}

func (m *Manager) backupsDir() string {
	return filepath.Join(m.dataDir, "backups")
}

func (m *Manager) Backup(name string) (string, error) {
	if err := os.MkdirAll(m.backupsDir(), 0755); err != nil {
		return "", fmt.Errorf("creating backups directory: %w", err)
	}

	if name == "" {
		name = fmt.Sprintf("backup-%s", time.Now().Format("20060102-150405"))
	}

	archivePath := filepath.Join(m.backupsDir(), name+".tar.gz")

	file, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("creating archive file: %w", err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// Add metadata
	meta := metadata{
		Name:      name,
		Timestamp: time.Now(),
		Version:   "1.0",
	}
	if err := addJSONToTar(tw, "metadata.json", meta); err != nil {
		return "", fmt.Errorf("adding metadata: %w", err)
	}

	// Add database
	dbPath := filepath.Join(m.dataDir, "sm.db")
	if _, err := os.Stat(dbPath); err == nil {
		if err := addFileToTar(tw, "data/sm.db", dbPath); err != nil {
			return "", fmt.Errorf("adding database: %w", err)
		}
	}

	// Add registry
	if err := addDirToTar(tw, "registry", m.registryDir); err != nil {
		return "", fmt.Errorf("adding registry: %w", err)
	}

	// Add profiles
	if err := addDirToTar(tw, "profiles", m.profilesDir); err != nil {
		return "", fmt.Errorf("adding profiles: %w", err)
	}

	return archivePath, nil
}

func (m *Manager) Restore(archivePath string) error {
	// Create pre-restore backup
	preRestoreName := fmt.Sprintf("pre-restore-%s", time.Now().Format("20060102-150405"))
	if _, err := m.Backup(preRestoreName); err != nil {
		return fmt.Errorf("creating pre-restore backup: %w", err)
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening archive: %w", err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("creating gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// Determine target path
		var targetPath string
		switch {
		case header.Name == "metadata.json":
			continue // Skip metadata during restore
		case strings.HasPrefix(header.Name, "data/"):
			targetPath = filepath.Join(m.dataDir, strings.TrimPrefix(header.Name, "data/"))
		case strings.HasPrefix(header.Name, "registry/"):
			targetPath = filepath.Join(m.registryDir, strings.TrimPrefix(header.Name, "registry/"))
		case strings.HasPrefix(header.Name, "profiles/"):
			targetPath = filepath.Join(m.profilesDir, strings.TrimPrefix(header.Name, "profiles/"))
		default:
			continue // Skip unknown files
		}

		// Create directory if needed
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		// Extract file, preserving original permissions from tar header
		if err := extractFile(tr, targetPath, header); err != nil {
			return fmt.Errorf("extracting file %s: %w", header.Name, err)
		}
	}

	return nil
}

func (m *Manager) List() ([]BackupInfo, error) {
	backupsDir := m.backupsDir()
	if _, err := os.Stat(backupsDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		return nil, fmt.Errorf("reading backups directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".tar.gz")
		backups = append(backups, BackupInfo{
			Name:      name,
			Timestamp: info.ModTime(),
			Size:      info.Size(),
			Path:      filepath.Join(backupsDir, entry.Name()),
		})
	}

	// Sort by timestamp, newest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

func (m *Manager) Rotate(keepN int) error {
	backups, err := m.List()
	if err != nil {
		return err
	}

	if len(backups) <= keepN {
		return nil
	}

	// Delete oldest backups
	toDelete := backups[keepN:]
	for _, b := range toDelete {
		if err := os.Remove(b.Path); err != nil {
			return fmt.Errorf("deleting backup %s: %w", b.Name, err)
		}
	}

	return nil
}

func (m *Manager) FindByName(name string) (*BackupInfo, error) {
	backups, err := m.List()
	if err != nil {
		return nil, err
	}

	for _, b := range backups {
		if b.Name == name {
			return &b, nil
		}
	}

	return nil, fmt.Errorf("backup not found: %s", name)
}

func (m *Manager) FindLatest() (*BackupInfo, error) {
	backups, err := m.List()
	if err != nil {
		return nil, err
	}

	if len(backups) == 0 {
		return nil, fmt.Errorf("no backups found")
	}

	return &backups[0], nil
}

func addDirToTar(tw *tar.Writer, prefix, dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directories
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Skip hidden files
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		name := filepath.Join(prefix, relPath)
		return addFileToTar(tw, name, path)
	})
}

func addFileToTar(tw *tar.Writer, name, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name:    name,
		Size:    info.Size(),
		Mode:    int64(info.Mode()),
		ModTime: info.ModTime(),
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = io.Copy(tw, file)
	return err
}

func addJSONToTar(tw *tar.Writer, name string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}

	header := &tar.Header{
		Name: name,
		Size: int64(len(data)),
		Mode: 0644,
	}

	if err := tw.WriteHeader(header); err != nil {
		return err
	}

	_, err = tw.Write(data)
	return err
}

func extractFile(tr *tar.Reader, targetPath string, header *tar.Header) error {
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, tr)
	return err
}
