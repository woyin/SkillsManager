// Package backup 创建、列举、还原 sm 配置的 tar.gz 备份。
//
// 备份内容包括：数据库（sm.db）、注册表、profiles。
// 还原前会自动创建一份 pre-restore 备份，避免误覆盖。
//
// Input: archive/tar, compress/gzip, encoding/json, fmt, io, os, path/filepath, sort, strings, time
// Output: type Manager, type BackupInfo, func New
// Pos: 工具层-备份
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
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

// Manager 创建、列举、还原 sm 的注册表/profiles/数据库的 tar.gz 备份。
type Manager struct {
	dataDir     string
	registryDir string
	profilesDir string
}

// BackupInfo 描述磁盘上一个备份归档。
type BackupInfo struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
	Size      int64     `json:"size"`
	Path      string    `json:"path"`
}

// metadata 是嵌入在每个备份归档中的 JSON 清单。
type metadata struct {
	Name      string    `json:"name"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
}

// New 构造一个指向给定 data/registry/profiles 目录的 Manager。
func New(dataDir, registryDir, profilesDir string) *Manager {
	return &Manager{
		dataDir:     dataDir,
		registryDir: registryDir,
		profilesDir: profilesDir,
	}
}

// backupsDir 返回备份归档所在目录（<dataDir>/backups）。
func (m *Manager) backupsDir() string {
	return filepath.Join(m.dataDir, "backups")
}

// Backup 创建一份新备份；name 为空时按时间戳自动生成。
// 返回归档绝对路径。
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

	// 写入元数据。
	meta := metadata{
		Name:      name,
		Timestamp: time.Now(),
		Version:   "1.0",
	}
	if err := addJSONToTar(tw, "metadata.json", meta); err != nil {
		return "", fmt.Errorf("adding metadata: %w", err)
	}

	// 写入数据库（若存在）。
	dbPath := filepath.Join(m.dataDir, "sm.db")
	if _, err := os.Stat(dbPath); err == nil {
		if err := addFileToTar(tw, "data/sm.db", dbPath); err != nil {
			return "", fmt.Errorf("adding database: %w", err)
		}
	}

	// 写入注册表与 profiles。
	if err := addDirToTar(tw, "registry", m.registryDir); err != nil {
		return "", fmt.Errorf("adding registry: %w", err)
	}
	if err := addDirToTar(tw, "profiles", m.profilesDir); err != nil {
		return "", fmt.Errorf("adding profiles: %w", err)
	}

	return archivePath, nil
}

// Restore 从 archivePath 还原。
// 还原前会自动创建一份 pre-restore 备份，以防误覆盖。
func (m *Manager) Restore(archivePath string) error {
	// 先做一份还原前备份。
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

		// 根据条目名前缀决定落盘位置。
		var targetPath string
		var baseDir string
		switch {
		case header.Name == "metadata.json":
			continue
		case strings.HasPrefix(header.Name, "data/"):
			targetPath = filepath.Join(m.dataDir, strings.TrimPrefix(header.Name, "data/"))
			baseDir = m.dataDir
		case strings.HasPrefix(header.Name, "registry/"):
			targetPath = filepath.Join(m.registryDir, strings.TrimPrefix(header.Name, "registry/"))
			baseDir = m.registryDir
		case strings.HasPrefix(header.Name, "profiles/"):
			targetPath = filepath.Join(m.profilesDir, strings.TrimPrefix(header.Name, "profiles/"))
			baseDir = m.profilesDir
		default:
			continue
		}

		if !isSubPath(targetPath, baseDir) {
			return fmt.Errorf("refusing to extract %q: path escapes target directory", header.Name)
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}

		// 解出文件，保留 tar 头中的权限位。
		if err := extractFile(tr, targetPath, header); err != nil {
			return fmt.Errorf("extracting file %s: %w", header.Name, err)
		}
	}

	return nil
}

// List 返回全部备份，按时间倒序。
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

	// 按时间倒序，最新的在前；同时间戳时按名称倒序作确定性 tiebreaker
	// （自动生成的名称按时间命名，名称倒序即时间倒序；自定义名避免不稳定排序）。
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].Timestamp.Equal(backups[j].Timestamp) {
			return backups[i].Name > backups[j].Name
		}
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})

	return backups, nil
}

// Rotate 仅保留最新的 keepN 个备份，删除其余。
func (m *Manager) Rotate(keepN int) error {
	backups, err := m.List()
	if err != nil {
		return err
	}

	if len(backups) <= keepN {
		return nil
	}

	// 删除较旧的备份。
	toDelete := backups[keepN:]
	for _, b := range toDelete {
		if err := os.Remove(b.Path); err != nil {
			return fmt.Errorf("deleting backup %s: %w", b.Name, err)
		}
	}

	return nil
}

// FindByName 按名查找备份；未找到返回错误。
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

// FindLatest 返回最新备份；无备份时返回错误。
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

// addDirToTar 把 dir 整棵树加入 tar（前缀 prefix）。
// 跳过 .git 与 node_modules 目录；保留隐藏文件（如 .mcp.json、.gitkeep），
// 因为它们承载注册表还原所需的数据。
func addDirToTar(tw *tar.Writer, prefix, dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// 跳过版本控制与依赖目录（整棵子树）。
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}

		if d.IsDir() {
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

// addFileToTar 把单个文件加入 tar。
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

// addJSONToTar 把任意 JSON 可序列化值加入 tar。
func addJSONToTar(tw *tar.Writer, name string, v any) error {
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

// extractFile 把 tr 中的当前条目写到 targetPath，保留 header 的权限位。
func extractFile(tr *tar.Reader, targetPath string, header *tar.Header) error {
	file, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, header.FileInfo().Mode())
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, tr)
	return err
}

func isSubPath(target, base string) bool {
	ct := filepath.Clean(target)
	cb := filepath.Clean(base) + string(filepath.Separator)
	return strings.HasPrefix(ct, cb)
}
