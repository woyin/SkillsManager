# SkillsManager (sm) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI tool (`sm`) that manages AI agent skills and MCP configurations across projects using symlinks, profiles, and a centralized registry.

**Architecture:** Single Go binary with cobra CLI. Internal packages for registry, profiles, projects, symlinks, installer, and SQLite database. Embedded web dashboard via `embed.FS`. All installed skill locations are symlinks back to the registry originals.

**Tech Stack:** Go 1.23+, cobra (CLI), modernc.org/sqlite (pure-Go SQLite), embed.FS (web assets)

---

## File Structure

```
SkillsManager/
├── main.go                          # Entry point
├── go.mod
├── go.sum
├── .gitignore
├── README.md
├── README.zh-CN.md
├── cmd/
│   ├── root.go                      # Root cobra command + global flags
│   ├── add.go                       # sm add
│   ├── rm.go                        # sm rm
│   ├── install.go                   # sm install
│   ├── update.go                    # sm update
│   ├── check.go                     # sm check
│   ├── list.go                      # sm list
│   └── web.go                       # sm web
├── internal/
│   ├── registry/
│   │   ├── registry.go              # Registry CRUD: add/rm/list skills + MCP
│   │   └── registry_test.go
│   ├── profile/
│   │   ├── profile.go               # Load/resolve profiles
│   │   └── profile_test.go
│   ├── project/
│   │   ├── project.go               # .sm.json read/write
│   │   └── project_test.go
│   ├── symlink/
│   │   ├── symlink.go               # Create/verify/cleanup symlinks
│   │   └── symlink_test.go
│   ├── installer/
│   │   ├── installer.go             # Orchestrates install flow
│   │   └── installer_test.go
│   └── db/
│       ├── db.go                    # SQLite init + CRUD
│       └── db_test.go
├── web/
│   ├── handler.go                   # HTTP API handlers
│   └── static/
│       ├── index.html               # Dashboard SPA
│       ├── style.css
│       └── app.js
├── registry/                        # Skills + MCP registry (user-managed)
│   ├── skills/
│   │   ├── global/
│   │   ├── codex-only/
│   │   └── claude-only/
│   └── mcp/
├── profiles/                        # Profile definitions
│   └── default.json
└── data/                            # SQLite DB (gitignored)
```

---

## Task 1: Project Scaffolding

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `main.go`
- Create: `cmd/root.go`
- Create: `registry/skills/global/.gitkeep`
- Create: `registry/skills/codex-only/.gitkeep`
- Create: `registry/skills/claude-only/.gitkeep`
- Create: `registry/mcp/.gitkeep`
- Create: `profiles/default.json`
- Create: `data/.gitkeep`

- [ ] **Step 1: Initialize Go module**

```bash
cd /Users/breestealth/Documents/DevelopmentRepository/SkillsManager
go mod init github.com/woyin/skills-manager
```

- [ ] **Step 2: Install dependencies**

```bash
go get github.com/spf13/cobra@latest
go get modernc.org/sqlite@latest
```

- [ ] **Step 3: Create directory structure**

```bash
mkdir -p cmd internal/registry internal/profile internal/project internal/symlink internal/installer internal/db web/static registry/skills/global registry/skills/codex-only registry/skills/claude-only registry/mcp profiles data
touch registry/skills/global/.gitkeep registry/skills/codex-only/.gitkeep registry/skills/claude-only/.gitkeep registry/mcp/.gitkeep data/.gitkeep
```

- [ ] **Step 4: Write .gitignore**

```gitignore
# SQLite database (local state, not shared)
data/*.db
data/*.db-wal
data/*.db-shm

# Go build artifacts
sm
*.exe
/bin/

# OS files
.DS_Store
Thumbs.db

# Editor
.idea/
.vscode/
*.swp
*~
```

- [ ] **Step 5: Write main.go**

```go
package main

import (
	"os"
	"github.com/woyin/skills-manager/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Write cmd/root.go**

```go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	registryDir string
	dataDir     string
	profilesDir string
)

var rootCmd = &cobra.Command{
	Use:   "sm",
	Short: "SkillsManager — manage AI agent skills and MCP configurations",
	Long:  "A CLI tool for managing skills (Codex, Claude) and MCP server configurations across projects using symlinks and profiles.",
}

func init() {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, "Documents", "DevelopmentRepository", "SkillsManager")

	rootCmd.PersistentFlags().StringVar(&registryDir, "registry", filepath.Join(base, "registry"), "Registry directory path")
	rootCmd.PersistentFlags().StringVar(&dataDir, "data", filepath.Join(base, "data"), "Data directory path")
	rootCmd.PersistentFlags().StringVar(&profilesDir, "profiles", filepath.Join(base, "profiles"), "Profiles directory path")
}

func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}
```

- [ ] **Step 7: Write profiles/default.json**

```json
{
  "skills": [],
  "mcp": []
}
```

- [ ] **Step 8: Verify build**

```bash
cd /Users/breestealth/Documents/DevelopmentRepository/SkillsManager
go build -o sm .
./sm --help
```

Expected: Shows help with `sm` usage, global flags for `--registry`, `--data`, `--profiles`.

- [ ] **Step 9: Commit**

```bash
git add .
git commit -m "feat: project scaffolding with cobra CLI and directory structure"
```

---

## Task 2: SQLite Database Package

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/db/db_test.go
package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	// Verify tables exist
	var tableCount int
	err = database.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('installations','projects')").Scan(&tableCount)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if tableCount != 2 {
		t.Errorf("Expected 2 tables, got %d", tableCount)
	}
}

func TestRecordInstallation(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	err = database.RecordInstallation("/Users/test/project", "cloudflare", []string{"skill-a", "skill-b"}, []string{"mcp-x"})
	if err != nil {
		t.Fatalf("RecordInstallation failed: %v", err)
	}

	installs, err := database.GetInstallations("/Users/test/project")
	if err != nil {
		t.Fatalf("GetInstallations failed: %v", err)
	}
	if len(installs) != 1 {
		t.Fatalf("Expected 1 installation, got %d", len(installs))
	}
	if installs[0].Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", installs[0].Profile)
	}
}

func TestUpsertProject(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer database.Close()

	err = database.UpsertProject("/Users/test/project", "cloudflare", []string{"extra-skill"}, []string{})
	if err != nil {
		t.Fatalf("UpsertProject failed: %v", err)
	}

	projects, err := database.GetAllProjects()
	if err != nil {
		t.Fatalf("GetAllProjects failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("Expected 1 project, got %d", len(projects))
	}
	if projects[0].Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", projects[0].Profile)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/breestealth/Documents/DevelopmentRepository/SkillsManager
go test ./internal/db/ -v
```

Expected: FAIL — `Open` function and types not defined.

- [ ] **Step 3: Write implementation**

```go
// internal/db/db.go
package db

import (
	"database/sql"
	"encoding/json"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type Installation struct {
	ID          int64
	ProjectPath string
	Profile     string
	Skills      []string
	MCP         []string
	InstalledAt time.Time
}

type Project struct {
	Path          string
	Profile       string
	ExtraSkills   []string
	ExtraMCP      []string
	LastInstalled time.Time
}

func Open(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func initSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS installations (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		project_path TEXT NOT NULL,
		profile      TEXT,
		skills       TEXT,
		mcp          TEXT,
		installed_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS projects (
		path           TEXT PRIMARY KEY,
		profile        TEXT,
		extra_skills   TEXT,
		extra_mcp      TEXT,
		last_installed DATETIME
	);`
	_, err := db.Exec(schema)
	return err
}

func (d *DB) RecordInstallation(projectPath, profile string, skills, mcp []string) error {
	skillsJSON, _ := json.Marshal(skills)
	mcpJSON, _ := json.Marshal(mcp)

	_, err := d.db.Exec(
		"INSERT INTO installations (project_path, profile, skills, mcp) VALUES (?, ?, ?, ?)",
		projectPath, profile, string(skillsJSON), string(mcpJSON),
	)
	return err
}

func (d *DB) GetInstallations(projectPath string) ([]Installation, error) {
	rows, err := d.db.Query(
		"SELECT id, project_path, profile, skills, mcp, installed_at FROM installations WHERE project_path = ? ORDER BY installed_at DESC",
		projectPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Installation
	for rows.Next() {
		var inst Installation
		var skillsStr, mcpStr string
		if err := rows.Scan(&inst.ID, &inst.ProjectPath, &inst.Profile, &skillsStr, &mcpStr, &inst.InstalledAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(skillsStr), &inst.Skills)
		json.Unmarshal([]byte(mcpStr), &inst.MCP)
		results = append(results, inst)
	}
	return results, nil
}

func (d *DB) GetAllInstallations() ([]Installation, error) {
	rows, err := d.db.Query(
		"SELECT id, project_path, profile, skills, mcp, installed_at FROM installations ORDER BY installed_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Installation
	for rows.Next() {
		var inst Installation
		var skillsStr, mcpStr string
		if err := rows.Scan(&inst.ID, &inst.ProjectPath, &inst.Profile, &skillsStr, &mcpStr, &inst.InstalledAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(skillsStr), &inst.Skills)
		json.Unmarshal([]byte(mcpStr), &inst.MCP)
		results = append(results, inst)
	}
	return results, nil
}

func (d *DB) UpsertProject(projectPath, profile string, extraSkills, extraMCP []string) error {
	skillsJSON, _ := json.Marshal(extraSkills)
	mcpJSON, _ := json.Marshal(extraMCP)

	_, err := d.db.Exec(
		`INSERT INTO projects (path, profile, extra_skills, extra_mcp, last_installed)
		 VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(path) DO UPDATE SET
		   profile = excluded.profile,
		   extra_skills = excluded.extra_skills,
		   extra_mcp = excluded.extra_mcp,
		   last_installed = CURRENT_TIMESTAMP`,
		projectPath, profile, string(skillsJSON), string(mcpJSON),
	)
	return err
}

func (d *DB) GetAllProjects() ([]Project, error) {
	rows, err := d.db.Query(
		"SELECT path, profile, extra_skills, extra_mcp, last_installed FROM projects ORDER BY last_installed DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Project
	for rows.Next() {
		var p Project
		var skillsStr, mcpStr string
		if err := rows.Scan(&p.Path, &p.Profile, &skillsStr, &mcpStr, &p.LastInstalled); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(skillsStr), &p.ExtraSkills)
		json.Unmarshal([]byte(mcpStr), &p.ExtraMCP)
		results = append(results, p)
	}
	return results, nil
}

func (d *DB) RemoveProject(projectPath string) error {
	_, err := d.db.Exec("DELETE FROM projects WHERE path = ?", projectPath)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/db/ -v
```

Expected: All 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/db/ go.mod go.sum
git commit -m "feat: add SQLite database package with installations and projects tables"
```

---

## Task 3: Profile Package

**Files:**
- Create: `internal/profile/profile.go`
- Create: `internal/profile/profile_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/profile/profile_test.go
package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProfile(t *testing.T) {
	dir := t.TempDir()
	profileData := Profile{
		Skills: []string{"global", "cloudflare"},
		MCP:    []string{"cloudflare", "github"},
	}
	data, _ := json.Marshal(profileData)
	os.WriteFile(filepath.Join(dir, "cloudflare.json"), data, 0644)

	loader := NewLoader(dir)
	profile, err := loader.Load("cloudflare")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if len(profile.Skills) != 2 {
		t.Errorf("Expected 2 skills, got %d", len(profile.Skills))
	}
	if len(profile.MCP) != 2 {
		t.Errorf("Expected 2 MCP, got %d", len(profile.MCP))
	}
}

func TestLoadNonExistentProfile(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)
	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent profile")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/profile/ -v
```

Expected: FAIL — types not defined.

- [ ] **Step 3: Write implementation**

```go
// internal/profile/profile.go
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Profile struct {
	Skills []string `json:"skills"`
	MCP    []string `json:"mcp"`
}

type Loader struct {
	dir string
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) Load(name string) (*Profile, error) {
	path := filepath.Join(l.dir, name+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("profile %q not found: %w", name, err)
	}

	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid profile %q: %w", name, err)
	}
	return &p, nil
}

func (l *Loader) List() ([]string, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/profile/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/profile/
git commit -m "feat: add profile loader package"
```

---

## Task 4: Project Config Package

**Files:**
- Create: `internal/project/project.go`
- Create: `internal/project/project_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/project/project_test.go
package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	configData := `{"profile":"cloudflare","skills":["extra-skill"],"mcp":["extra-mcp"]}`
	os.WriteFile(filepath.Join(dir, ".sm.json"), []byte(configData), 0644)

	mgr := NewManager(dir)
	config, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Profile != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare', got '%s'", config.Profile)
	}
	if len(config.Skills) != 1 || config.Skills[0] != "extra-skill" {
		t.Errorf("Expected [extra-skill], got %v", config.Skills)
	}
}

func TestWriteProjectConfig(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	config := &Config{
		Profile: "frontend",
		Skills:  []string{"design"},
		MCP:     []string{"github"},
	}
	err := mgr.Save(config)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if loaded.Profile != "frontend" {
		t.Errorf("Expected profile 'frontend', got '%s'", loaded.Profile)
	}
}

func TestNoConfigReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir)

	config, err := mgr.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if config.Profile != "" {
		t.Errorf("Expected empty profile, got '%s'", config.Profile)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/project/ -v
```

Expected: FAIL.

- [ ] **Step 3: Write implementation**

```go
// internal/project/project.go
package project

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const configFileName = ".sm.json"

type Config struct {
	Profile string   `json:"profile,omitempty"`
	Skills  []string `json:"skills,omitempty"`
	MCP     []string `json:"mcp,omitempty"`
}

type Manager struct {
	dir string
}

func NewManager(dir string) *Manager {
	return &Manager{dir: dir}
}

func (m *Manager) configPath() string {
	return filepath.Join(m.dir, configFileName)
}

func (m *Manager) Load() (*Config, error) {
	data, err := os.ReadFile(m.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (m *Manager) Save(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.configPath(), data, 0644)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/project/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/project/
git commit -m "feat: add project config package (.sm.json)"
```

---

## Task 5: Symlink Package

**Files:**
- Create: `internal/symlink/symlink.go`
- Create: `internal/symlink/symlink_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/symlink/symlink_test.go
package symlink

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "registry", "skills", "cloudflare", "my-skill")
	dst := filepath.Join(dir, "codex", "skills", "my-skill")

	os.MkdirAll(src, 0755)
	os.WriteFile(filepath.Join(src, "SKILL.md"), []byte("# skill"), 0644)
	os.MkdirAll(filepath.Dir(dst), 0755)

	err := Create(src, dst)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	target, err := os.Readlink(dst)
	if err != nil {
		t.Fatalf("Readlink failed: %v", err)
	}
	if target != src {
		t.Errorf("Expected symlink to %s, got %s", src, target)
	}
}

func TestIsSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)

	if IsSymlink(dst) {
		t.Error("Expected false for non-existent path")
	}

	Create(src, dst)
	if !IsSymlink(dst) {
		t.Error("Expected true for symlink")
	}
}

func TestVerifySymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)
	Create(src, dst)

	if !Verify(dst) {
		t.Error("Expected valid symlink")
	}

	os.RemoveAll(src)
	if Verify(dst) {
		t.Error("Expected broken symlink to be invalid")
	}
}

func TestCleanupBroken(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "dst")
	os.MkdirAll(src, 0755)
	Create(src, dst)
	os.RemoveAll(src)

	removed, err := RemoveIfBroken(dst)
	if err != nil {
		t.Fatalf("RemoveIfBroken failed: %v", err)
	}
	if !removed {
		t.Error("Expected broken symlink to be removed")
	}
	if _, err := os.Lstat(dst); !os.IsNotExist(err) {
		t.Error("Expected symlink to be gone")
	}
}

func TestFindSymlinksPointingTo(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst1 := filepath.Join(dir, "link1")
	dst2 := filepath.Join(dir, "link2")
	os.MkdirAll(src, 0755)
	Create(src, dst1)
	Create(src, dst2)

	links, err := FindPointingTo(dir, src)
	if err != nil {
		t.Fatalf("FindPointingTo failed: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("Expected 2 symlinks, got %d", len(links))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/symlink/ -v
```

Expected: FAIL.

- [ ] **Step 3: Write implementation**

```go
// internal/symlink/symlink.go
package symlink

import (
	"fmt"
	"os"
	"path/filepath"
)

// Create creates a symlink at dst pointing to src.
// Creates parent directories if needed.
func Create(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("creating parent dir: %w", err)
	}

	// If symlink already exists with correct target, skip
	if target, err := os.Readlink(dst); err == nil {
		if target == src {
			return nil
		}
		return fmt.Errorf("symlink %s already exists pointing to %s (want %s)", dst, target, src)
	}

	return os.Symlink(src, dst)
}

// IsSymlink returns true if path is a symbolic link.
func IsSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeSymlink != 0
}

// Verify checks if a symlink exists and its target exists.
func Verify(path string) bool {
	if !IsSymlink(path) {
		return false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(path), target)
	}
	_, err = os.Stat(target)
	return err == nil
}

// RemoveIfBroken removes a broken symlink. Returns true if removed.
func RemoveIfBroken(path string) (bool, error) {
	if !IsSymlink(path) {
		return false, nil
	}
	if Verify(path) {
		return false, nil
	}
	return true, os.Remove(path)
}

// FindPointingTo finds all symlinks in searchDir that point to target.
func FindPointingTo(searchDir, target string) ([]string, error) {
	var results []string

	err := filepath.Walk(searchDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil
		}
		linkTarget, err := os.Readlink(path)
		if err != nil {
			return nil
		}
		if !filepath.IsAbs(linkTarget) {
			linkTarget = filepath.Join(filepath.Dir(path), linkTarget)
		}
		if linkTarget == target {
			results = append(results, path)
		}
		return nil
	})

	return results, err
}

// RemoveAll removes a symlink or directory.
func RemoveAll(path string) error {
	return os.RemoveAll(path)
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/symlink/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/symlink/
git commit -m "feat: add symlink package for create/verify/cleanup operations"
```

---

## Task 6: Registry Package

**Files:**
- Create: `internal/registry/registry.go`
- Create: `internal/registry/registry_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/registry/registry_test.go
package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func setupTestRegistry(t *testing.T) string {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "skills", "global"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "codex-only"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "claude-only"), 0755)
	os.MkdirAll(filepath.Join(dir, "skills", "cloudflare"), 0755)
	os.MkdirAll(filepath.Join(dir, "mcp"), 0755)
	return dir
}

func TestAddSkillFromLocalPath(t *testing.T) {
	regDir := setupTestRegistry(t)
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "my-skill"), 0755)
	os.WriteFile(filepath.Join(srcDir, "my-skill", "SKILL.md"), []byte("# test"), 0644)

	reg := New(regDir)
	err := reg.AddSkill(filepath.Join(srcDir, "my-skill"), "cloudflare", "")
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}

	dest := filepath.Join(regDir, "skills", "cloudflare", "my-skill")
	if _, err := os.Stat(filepath.Join(dest, "SKILL.md")); err != nil {
		t.Errorf("Skill not copied: %v", err)
	}
}

func TestAddSkillGlobal(t *testing.T) {
	regDir := setupTestRegistry(t)
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "global-skill"), 0755)
	os.WriteFile(filepath.Join(srcDir, "global-skill", "SKILL.md"), []byte("# global"), 0644)

	reg := New(regDir)
	err := reg.AddSkill(filepath.Join(srcDir, "global-skill"), "", "global")
	if err != nil {
		t.Fatalf("AddSkill failed: %v", err)
	}

	dest := filepath.Join(regDir, "skills", "global", "global-skill")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("Global skill not created: %v", err)
	}
}

func TestRemoveSkill(t *testing.T) {
	regDir := setupTestRegistry(t)
	skillDir := filepath.Join(regDir, "skills", "cloudflare", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# test"), 0644)

	reg := New(regDir)
	err := reg.RemoveSkill("test-skill", "cloudflare", "")
	if err != nil {
		t.Fatalf("RemoveSkill failed: %v", err)
	}

	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("Skill directory should be removed")
	}
}

func TestListSkills(t *testing.T) {
	regDir := setupTestRegistry(t)
	os.MkdirAll(filepath.Join(regDir, "skills", "cloudflare", "skill-a"), 0755)
	os.MkdirAll(filepath.Join(regDir, "skills", "cloudflare", "skill-b"), 0755)
	os.MkdirAll(filepath.Join(regDir, "skills", "global", "skill-c"), 0755)

	reg := New(regDir)
	skills, err := reg.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}

	if len(skills["cloudflare"]) != 2 {
		t.Errorf("Expected 2 cloudflare skills, got %d", len(skills["cloudflare"]))
	}
	if len(skills["global"]) != 1 {
		t.Errorf("Expected 1 global skill, got %d", len(skills["global"]))
	}
}

func TestAddMCP(t *testing.T) {
	regDir := setupTestRegistry(t)
	srcDir := t.TempDir()
	mcpJSON := `{"mcpServers":{"test":{"type":"http","url":"https://example.com/mcp"}}}`
	os.WriteFile(filepath.Join(srcDir, "test.json"), []byte(mcpJSON), 0644)

	reg := New(regDir)
	err := reg.AddMCP(filepath.Join(srcDir, "test.json"))
	if err != nil {
		t.Fatalf("AddMCP failed: %v", err)
	}

	dest := filepath.Join(regDir, "mcp", "test.json")
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("MCP file not created: %v", err)
	}
	if string(data) != mcpJSON {
		t.Errorf("MCP content mismatch: got %s", string(data))
	}
}

func TestListMCP(t *testing.T) {
	regDir := setupTestRegistry(t)
	os.WriteFile(filepath.Join(regDir, "mcp", "github.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(regDir, "mcp", "cloudflare.json"), []byte("{}"), 0644)

	reg := New(regDir)
	mcps, err := reg.ListMCP()
	if err != nil {
		t.Fatalf("ListMCP failed: %v", err)
	}
	if len(mcps) != 2 {
		t.Errorf("Expected 2 MCP entries, got %d", len(mcps))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/registry/ -v
```

Expected: FAIL.

- [ ] **Step 3: Write implementation**

```go
// internal/registry/registry.go
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Special directories that have fixed install targets
const (
	Global    = "global"
	CodexOnly = "codex-only"
	ClaudeOnly = "claude-only"
)

var specialDirs = map[string]bool{
	Global:     true,
	CodexOnly:  true,
	ClaudeOnly: true,
}

type Registry struct {
	dir string
}

func New(dir string) *Registry {
	return &Registry{dir: dir}
}

func (r *Registry) skillsDir() string {
	return filepath.Join(r.dir, "skills")
}

func (r *Registry) mcpDir() string {
	return filepath.Join(r.dir, "mcp")
}

// IsSpecialDir returns true if the category is a special directory.
func IsSpecialDir(category string) bool {
	return specialDirs[category]
}

// SkillNameFromPath extracts the name from a path or URL.
func SkillNameFromPath(source string) string {
	// For GitHub URLs like github.com/user/repo/path/to/skill
	// take the last path segment
	source = strings.TrimRight(source, "/")
	parts := strings.Split(source, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return source
}

// IsGitURL returns true if source looks like a git URL.
func IsGitURL(source string) bool {
	return strings.HasPrefix(source, "github.com/") ||
		strings.HasPrefix(source, "https://github.com/") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasSuffix(source, ".git")
}

// normalizeGitURL converts shorthand to full URL.
func normalizeGitURL(source string) string {
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source
	}
	return source
}

// AddSkill adds a skill to the registry. If special is non-empty, it overrides category.
// For GitHub URLs, it clones. For local paths, it copies.
func (r *Registry) AddSkill(source, category, special string) error {
	name := SkillNameFromPath(source)
	if name == "" {
		return fmt.Errorf("cannot determine skill name from source: %s", source)
	}

	var destCategory string
	if special != "" {
		destCategory = special
	} else if category != "" {
		destCategory = category
	} else {
		return fmt.Errorf("must specify category or --global/--codex/--claude")
	}

	dest := filepath.Join(r.skillsDir(), destCategory, name)

	if IsGitURL(source) {
		return r.cloneRepo(normalizeGitURL(source), dest)
	}
	return r.copyDir(source, dest)
}

func (r *Registry) cloneRepo(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	cmd := exec.Command("git", "clone", url, dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (r *Registry) copyDir(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("destination already exists: %s", dest)
	}
	return copyDirRecursive(src, dest)
}

func copyDirRecursive(src, dest string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		if entry.IsDir() {
			if err := copyDirRecursive(srcPath, destPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, destPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	srcInfo, _ := os.Stat(src)
	return os.Chmod(dest, srcInfo.Mode())
}

// RemoveSkill removes a skill from the registry.
func (r *Registry) RemoveSkill(name, category, special string) error {
	var dir string
	if special != "" {
		dir = filepath.Join(r.skillsDir(), special, name)
	} else if category != "" {
		dir = filepath.Join(r.skillsDir(), category, name)
	} else {
		// Search all categories
		found, err := r.findSkillDir(name)
		if err != nil {
			return err
		}
		if found == "" {
			return fmt.Errorf("skill %q not found in registry", name)
		}
		dir = found
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("skill not found: %s", dir)
	}
	return os.RemoveAll(dir)
}

func (r *Registry) findSkillDir(name string) (string, error) {
	skillsDir := r.skillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", err
	}
	for _, cat := range entries {
		if !cat.IsDir() {
			continue
		}
		candidate := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

// ListSkills returns all skills grouped by category.
func (r *Registry) ListSkills() (map[string][]string, error) {
	result := make(map[string][]string)
	skillsDir := r.skillsDir()

	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		skills, err := os.ReadDir(filepath.Join(skillsDir, cat.Name()))
		if err != nil {
			continue
		}
		for _, s := range skills {
			if s.IsDir() && s.Name() != ".gitkeep" {
				result[cat.Name()] = append(result[cat.Name()], s.Name())
			}
		}
	}
	return result, nil
}

// GetSkillPath returns the absolute path to a skill in the registry.
func (r *Registry) GetSkillPath(name, category, special string) (string, error) {
	if special != "" {
		path := filepath.Join(r.skillsDir(), special, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("skill %q not found in %s", name, special)
	}
	if category != "" {
		path := filepath.Join(r.skillsDir(), category, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		return "", fmt.Errorf("skill %q not found in %s", name, category)
	}
	// Search all
	found, err := r.findSkillDir(name)
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("skill %q not found in registry", name)
	}
	return found, nil
}

// AddMCP copies an MCP JSON file into the registry.
func (r *Registry) AddMCP(source string) error {
	name := strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	dest := filepath.Join(r.mcpDir(), name+".json")

	if _, err := os.Stat(dest); err == nil {
		return fmt.Errorf("MCP %q already exists in registry", name)
	}

	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("reading source: %w", err)
	}

	// Validate JSON
	var test map[string]interface{}
	if err := json.Unmarshal(data, &test); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return os.WriteFile(dest, data, 0644)
}

// RemoveMCP removes an MCP definition from the registry.
func (r *Registry) RemoveMCP(name string) error {
	path := filepath.Join(r.mcpDir(), name+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("MCP %q not found", name)
	}
	return os.Remove(path)
}

// ListMCP returns all MCP names in the registry.
func (r *Registry) ListMCP() ([]string, error) {
	entries, err := os.ReadDir(r.mcpDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name()[:len(e.Name())-5])
		}
	}
	return names, nil
}

// GetMCPPath returns the absolute path to an MCP JSON file.
func (r *Registry) GetMCPPath(name string) string {
	return filepath.Join(r.mcpDir(), name+".json")
}

// Dir returns the registry root directory.
func (r *Registry) Dir() string {
	return r.dir
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/registry/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/registry/
git commit -m "feat: add registry package for skill and MCP management"
```

---

## Task 7: Installer Package

**Files:**
- Create: `internal/installer/installer.go`
- Create: `internal/installer/installer_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/installer/installer_test.go
package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestEnv(t *testing.T) (registryDir, profilesDir, codexDir, claudeDir, projectDir string) {
	base := t.TempDir()
	registryDir = filepath.Join(base, "registry")
	profilesDir = filepath.Join(base, "profiles")
	codexDir = filepath.Join(base, ".codex", "skills")
	claudeDir = filepath.Join(base, ".claude", "skills")
	projectDir = filepath.Join(base, "project")

	// Create registry with test skills
	os.MkdirAll(filepath.Join(registryDir, "skills", "global", "global-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "global", "global-skill", "SKILL.md"), []byte("# global"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "cloudflare", "cf-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "cloudflare", "cf-skill", "SKILL.md"), []byte("# cf"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "codex-only", "codex-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "codex-only", "codex-skill", "SKILL.md"), []byte("# codex"), 0644)
	os.MkdirAll(filepath.Join(registryDir, "skills", "claude-only", "claude-skill"), 0755)
	os.WriteFile(filepath.Join(registryDir, "skills", "claude-only", "claude-skill", "SKILL.md"), []byte("# claude"), 0644)

	// Create MCP
	mcpJSON := `{"mcpServers":{"test":{"type":"http","url":"https://example.com/mcp"}}}`
	os.MkdirAll(filepath.Join(registryDir, "mcp"), 0755)
	os.WriteFile(filepath.Join(registryDir, "mcp", "test.json"), []byte(mcpJSON), 0644)

	// Create profile
	profileData := map[string]interface{}{
		"skills": []string{"global", "cloudflare"},
		"mcp":    []string{"test"},
	}
	pData, _ := json.Marshal(profileData)
	os.MkdirAll(profilesDir, 0755)
	os.WriteFile(filepath.Join(profilesDir, "cloudflare.json"), pData, 0644)

	// Create target dirs
	os.MkdirAll(codexDir, 0755)
	os.MkdirAll(claudeDir, 0755)
	os.MkdirAll(projectDir, 0755)

	return
}

func TestInstallWithProfile(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	result, err := inst.Install(projectDir, "cloudflare", nil, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// Check symlinks created
	cfSkillCodex := filepath.Join(codexDir, "cf-skill")
	cfSkillClaude := filepath.Join(claudeDir, "cf-skill")
	globalSkillCodex := filepath.Join(codexDir, "global-skill")
	globalSkillClaude := filepath.Join(claudeDir, "global-skill")

	for _, link := range []string{cfSkillCodex, cfSkillClaude, globalSkillCodex, globalSkillClaude} {
		fi, err := os.Lstat(link)
		if err != nil {
			t.Errorf("Symlink not created: %s (%v)", link, err)
			continue
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("Not a symlink: %s", link)
		}
	}

	// Check .sm.json written
	smPath := filepath.Join(projectDir, ".sm.json")
	data, err := os.ReadFile(smPath)
	if err != nil {
		t.Fatalf(".sm.json not created: %v", err)
	}
	var config map[string]interface{}
	json.Unmarshal(data, &config)
	if config["profile"] != "cloudflare" {
		t.Errorf("Expected profile 'cloudflare' in .sm.json")
	}

	// Check MCP merged into .mcp.json
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		t.Fatalf(".mcp.json not created: %v", err)
	}
	if len(mcpData) == 0 {
		t.Error(".mcp.json is empty")
	}

	// Check result
	if len(result.Skills) != 3 { // global-skill + codex-skill (global) + cf-skill
		t.Errorf("Expected 3 skill links, got %d", len(result.Skills))
	}
}

func TestInstallWithAdHoc(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	result, err := inst.Install(projectDir, "", []string{"cf-skill"}, []string{"test"})
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	// cf-skill should go to both codex and claude
	if _, err := os.Lstat(filepath.Join(codexDir, "cf-skill")); err != nil {
		t.Error("cf-skill not in codex")
	}
	if _, err := os.Lstat(filepath.Join(claudeDir, "cf-skill")); err != nil {
		t.Error("cf-skill not in claude")
	}

	// MCP should be in .mcp.json
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	if _, err := os.Stat(mcpPath); err != nil {
		t.Error(".mcp.json not created")
	}

	if len(result.MCP) != 1 {
		t.Errorf("Expected 1 MCP, got %d", len(result.MCP))
	}
}

func TestInstallCodexOnly(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	// codex-only skill should only appear in codex
	result, err := inst.Install(projectDir, "", []string{"codex-skill"}, nil)
	if err != nil {
		t.Fatalf("Install failed: %v", err)
	}

	if _, err := os.Lstat(filepath.Join(codexDir, "codex-skill")); err != nil {
		t.Error("codex-skill not in codex")
	}
	if _, err := os.Lstat(filepath.Join(claudeDir, "codex-skill")); err == nil {
		t.Error("codex-skill should NOT be in claude")
	}

	if len(result.Skills) != 1 {
		t.Errorf("Expected 1 skill link, got %d", len(result.Skills))
	}
}

func TestInstallNoConfig(t *testing.T) {
	registryDir, profilesDir, codexDir, claudeDir, projectDir := setupTestEnv(t)

	inst, err := New(registryDir, profilesDir, codexDir, claudeDir)
	if err != nil {
		t.Fatalf("New installer failed: %v", err)
	}

	_, err = inst.Install(projectDir, "", nil, nil)
	if err == nil {
		t.Error("Expected error when no profile and no ad-hoc items")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/installer/ -v
```

Expected: FAIL.

- [ ] **Step 3: Write implementation**

```go
// internal/installer/installer.go
package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/project"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
)

type Installer struct {
	registry   *registry.Registry
	profiles   *profile.Loader
	codexDir   string
	claudeDir  string
}

type InstallResult struct {
	Skills []string
	MCP    []string
}

func New(registryDir, profilesDir, codexDir, claudeDir string) (*Installer, error) {
	return &Installer{
		registry:  registry.New(registryDir),
		profiles:  profile.NewLoader(profilesDir),
		codexDir:  codexDir,
		claudeDir: claudeDir,
	}, nil
}

// Install installs skills and MCP into a project directory.
// profileName: optional profile to apply.
// extraSkills, extraMCP: ad-hoc additions.
func (inst *Installer) Install(projectDir, profileName string, extraSkills, extraMCP []string) (*InstallResult, error) {
	if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
		return nil, fmt.Errorf("nothing to install: specify --profile, or add skills/mcp to .sm.json")
	}

	var allSkills []string
	var allMCP []string

	// Resolve profile
	if profileName != "" {
		p, err := inst.profiles.Load(profileName)
		if err != nil {
			return nil, fmt.Errorf("loading profile %q: %w", profileName, err)
		}
		allSkills = append(allSkills, p.Skills...)
		allMCP = append(allMCP, p.MCP...)
	}

	// Merge ad-hoc
	allSkills = append(allSkills, extraSkills...)
	allMCP = append(allMCP, extraMCP...)

	// Deduplicate
	allSkills = deduplicate(allSkills)
	allMCP = deduplicate(allMCP)

	result := &InstallResult{}

	// Install skills as symlinks
	for _, skillName := range allSkills {
		links, err := inst.installSkill(skillName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping skill %q: %v\n", skillName, err)
			continue
		}
		result.Skills = append(result.Skills, links...)
	}

	// Install MCP
	for _, mcpName := range allMCP {
		if err := inst.installMCP(projectDir, mcpName); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping MCP %q: %v\n", mcpName, err)
			continue
		}
		result.MCP = append(result.MCP, mcpName)
	}

	// Write .sm.json
	pm := project.NewManager(projectDir)
	config := &project.Config{
		Profile: profileName,
		Skills:  extraSkills,
		MCP:     extraMCP,
	}
	if err := pm.Save(config); err != nil {
		return result, fmt.Errorf("writing .sm.json: %w", err)
	}

	return result, nil
}

func (inst *Installer) installSkill(name string) ([]string, error) {
	// Find the skill in registry
	skillPath, category, err := inst.findSkill(name)
	if err != nil {
		return nil, err
	}

	var links []string

	// Determine install targets based on category
	if category == registry.CodexOnly {
		link := filepath.Join(inst.codexDir, name)
		if err := symlink.Create(skillPath, link); err == nil {
			links = append(links, link)
		}
	} else if category == registry.ClaudeOnly {
		link := filepath.Join(inst.claudeDir, name)
		if err := symlink.Create(skillPath, link); err == nil {
			links = append(links, link)
		}
	} else {
		// global, or any other category → both tools
		linkCodex := filepath.Join(inst.codexDir, name)
		linkClaude := filepath.Join(inst.claudeDir, name)
		if err := symlink.Create(skillPath, linkCodex); err == nil {
			links = append(links, linkCodex)
		}
		if err := symlink.Create(skillPath, linkClaude); err == nil {
			links = append(links, linkClaude)
		}
	}

	return links, nil
}

func (inst *Installer) findSkill(name string) (string, string, error) {
	skillsDir := filepath.Join(inst.registry.Dir(), "skills")
	categories, err := os.ReadDir(skillsDir)
	if err != nil {
		return "", "", err
	}

	for _, cat := range categories {
		if !cat.IsDir() {
			continue
		}
		path := filepath.Join(skillsDir, cat.Name(), name)
		if _, err := os.Stat(path); err == nil {
			return path, cat.Name(), nil
		}
	}
	return "", "", fmt.Errorf("skill %q not found in registry", name)
}

func (inst *Installer) installMCP(projectDir, mcpName string) error {
	mcpPath := inst.registry.GetMCPPath(mcpName)
	mcpData, err := os.ReadFile(mcpPath)
	if err != nil {
		return fmt.Errorf("MCP %q not found in registry", mcpName)
	}

	var newMCP map[string]interface{}
	if err := json.Unmarshal(mcpData, &newMCP); err != nil {
		return fmt.Errorf("invalid MCP JSON: %w", err)
	}

	// Read existing .mcp.json or create new
	mcpFilePath := filepath.Join(projectDir, ".mcp.json")
	var existing map[string]interface{}

	if data, err := os.ReadFile(mcpFilePath); err == nil {
		json.Unmarshal(data, &existing)
	}
	if existing == nil {
		existing = map[string]interface{}{"mcpServers": map[string]interface{}{}}
	}

	// Merge mcpServers
	existingServers, _ := existing["mcpServers"].(map[string]interface{})
	if existingServers == nil {
		existingServers = make(map[string]interface{})
	}

	newServers, _ := newMCP["mcpServers"].(map[string]interface{})
	for k, v := range newServers {
		existingServers[k] = v
	}
	existing["mcpServers"] = existingServers

	merged, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(mcpFilePath, merged, 0644)
}

func deduplicate(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/installer/ -v
```

Expected: All 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/installer/
git commit -m "feat: add installer package with symlink-based install flow"
```

---

## Task 8: CLI Commands — add, rm, list

**Files:**
- Create: `cmd/add.go`
- Create: `cmd/rm.go`
- Create: `cmd/list.go`

- [ ] **Step 1: Write cmd/add.go**

```go
// cmd/add.go
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	addGlobal  bool
	addCodex   bool
	addClaude  bool
	addIsMCP   bool
)

var addCmd = &cobra.Command{
	Use:   "add <source> [category]",
	Short: "Add a skill or MCP to the registry",
	Long: `Add a skill or MCP server definition to the registry.
Source can be a GitHub URL or local path.
Category is the directory name under registry/skills/ or registry/mcp/.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		source := args[0]
		reg := registry.New(registryDir)

		if addIsMCP {
			if err := reg.AddMCP(source); err != nil {
				return fmt.Errorf("adding MCP: %w", err)
			}
			fmt.Printf("✓ Added MCP from %s\n", source)
			return nil
		}

		var special string
		if addGlobal {
			special = registry.Global
		} else if addCodex {
			special = registry.CodexOnly
		} else if addClaude {
			special = registry.ClaudeOnly
		}

		category := ""
		if len(args) > 1 {
			category = args[1]
		}

		if err := reg.AddSkill(source, category, special); err != nil {
			return fmt.Errorf("adding skill: %w", err)
		}

		dest := special
		if dest == "" {
			dest = category
		}
		name := registry.SkillNameFromPath(source)
		fmt.Printf("✓ Added skill %q to %s\n", name, dest)
		return nil
	},
}

func init() {
	addCmd.Flags().BoolVar(&addGlobal, "global", false, "Add to global directory (all tools)")
	addCmd.Flags().BoolVar(&addCodex, "codex", false, "Add to codex-only directory")
	addCmd.Flags().BoolVar(&addClaude, "claude", false, "Add to claude-only directory")
	addCmd.Flags().BoolVar(&addIsMCP, "mcp", false, "Add as MCP server definition")

	rootCmd.AddCommand(addCmd)
}
```

- [ ] **Step 2: Write cmd/rm.go**

```go
// cmd/rm.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
)

var (
	rmGlobal  bool
	rmCodex   bool
	rmClaude  bool
	rmIsMCP   bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <name> [category]",
	Short: "Remove a skill or MCP from the registry",
	Long: `Remove a skill or MCP server definition from the registry.
Also cleans up symlinks in installed locations.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		reg := registry.New(registryDir)

		if rmIsMCP {
			if err := reg.RemoveMCP(name); err != nil {
				return fmt.Errorf("removing MCP: %w", err)
			}
			fmt.Printf("✓ Removed MCP %q\n", name)
			return nil
		}

		var special string
		if rmGlobal {
			special = registry.Global
		} else if rmCodex {
			special = registry.CodexOnly
		} else if rmClaude {
			special = registry.ClaudeOnly
		}

		category := ""
		if len(args) > 1 {
			category = args[1]
		}

		// Find the skill path before removal (for symlink cleanup)
		skillPath, _, _ := reg.GetSkillPath(name, category, special)

		if err := reg.RemoveSkill(name, category, special); err != nil {
			return fmt.Errorf("removing skill: %w", err)
		}

		// Clean up symlinks in installed locations
		if skillPath != "" {
			home, _ := os.UserHomeDir()
			for _, dir := range []string{
				filepath.Join(home, ".codex", "skills"),
				filepath.Join(home, ".claude", "skills"),
			} {
				links, _ := symlink.FindPointingTo(dir, skillPath)
				for _, link := range links {
					os.Remove(link)
					fmt.Printf("  Removed symlink: %s\n", link)
				}
			}
		}

		fmt.Printf("✓ Removed skill %q\n", name)
		return nil
	},
}

func init() {
	rmCmd.Flags().BoolVar(&rmGlobal, "global", false, "Remove from global directory")
	rmCmd.Flags().BoolVar(&rmCodex, "codex", false, "Remove from codex-only directory")
	rmCmd.Flags().BoolVar(&rmClaude, "claude", false, "Remove from claude-only directory")
	rmCmd.Flags().BoolVar(&rmIsMCP, "mcp", false, "Remove MCP server definition")

	rootCmd.AddCommand(rmCmd)
}
```



- [ ] **Step 3: Write cmd/list.go**

```go
// cmd/list.go
package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all skills and MCP in the registry",
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := registry.New(registryDir)

		skills, err := reg.ListSkills()
		if err != nil {
			return err
		}

		mcps, err := reg.ListMCP()
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

		fmt.Fprintln(w, "SKILLS:")
		fmt.Fprintln(w, "  CATEGORY\tNAME")
		fmt.Fprintln(w, "  --------\t----")
		for cat, names := range skills {
			for _, name := range names {
				special := ""
				if registry.IsSpecialDir(cat) {
					special = " *"
				}
				fmt.Fprintf(w, "  %s\t%s%s\n", cat, name, special)
			}
		}

		fmt.Fprintln(w)
		fmt.Fprintln(w, "MCP:")
		fmt.Fprintln(w, "  NAME")
		fmt.Fprintln(w, "  ----")
		for _, name := range mcps {
			fmt.Fprintf(w, "  %s\n", name)
		}

		fmt.Fprintf(w, "\nTotal: %d skills, %d MCP\n", countSkills(skills), len(mcps))
		fmt.Fprintln(w, "  (* = special directory with fixed install target)")

		return w.Flush()
	},
}

func countSkills(skills map[string][]string) int {
	count := 0
	for _, names := range skills {
		count += len(names)
	}
	return count
}

func init() {
	rootCmd.AddCommand(listCmd)
}
```

- [ ] **Step 4: Build and test**

```bash
go build -o sm .
./sm add --help
./sm rm --help
./sm list --help
```

Expected: Help output for each command.

- [ ] **Step 5: Commit**

```bash
git add cmd/add.go cmd/rm.go cmd/list.go
git commit -m "feat: add CLI commands — add, rm, list"
```

---

## Task 9: CLI Commands — install, update, check

**Files:**
- Create: `cmd/install.go`
- Create: `cmd/update.go`
- Create: `cmd/check.go`

- [ ] **Step 1: Write cmd/install.go**

```go
// cmd/install.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/installer"
	"github.com/woyin/skills-manager/internal/project"
)

var (
	installProfile string
	installDir     string
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install skills and MCP into the current project",
	Long: `Install skills and MCP configurations into a project directory.
Reads .sm.json if present, or uses --profile flag.
Creates symlinks in ~/.codex/skills/ and ~/.claude/skills/.
Writes .mcp.json for MCP server configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := installDir
		if projectDir == "" {
			var err error
			projectDir, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("getting working directory: %w", err)
			}
		}

		// Load existing project config
		pm := project.NewManager(projectDir)
		config, err := pm.Load()
		if err != nil {
			return fmt.Errorf("loading project config: %w", err)
		}

		profileName := installProfile
		if profileName == "" {
			profileName = config.Profile
		}

		extraSkills := config.Skills
		extraMCP := config.MCP

		if profileName == "" && len(extraSkills) == 0 && len(extraMCP) == 0 {
			return fmt.Errorf("nothing to install: create .sm.json with a profile, or use --profile flag")
		}

		// Detect install targets
		home, _ := os.UserHomeDir()
		codexDir := filepath.Join(home, ".codex", "skills")
		claudeDir := filepath.Join(home, ".claude", "skills")

		inst, err := installer.New(registryDir, profilesDir, codexDir, claudeDir)
		if err != nil {
			return fmt.Errorf("creating installer: %w", err)
		}

		result, err := inst.Install(projectDir, profileName, extraSkills, extraMCP)
		if err != nil {
			return fmt.Errorf("install failed: %w", err)
		}

		// Record in database
		dbPath := filepath.Join(dataDir, "sm.db")
		database, err := db.Open(dbPath)
		if err != nil {
			return fmt.Errorf("opening database: %w", err)
		}
		defer database.Close()

		if err := database.RecordInstallation(projectDir, profileName, result.Skills, result.MCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to record installation: %v\n", err)
		}

		if err := database.UpsertProject(projectDir, profileName, extraSkills, extraMCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update project record: %v\n", err)
		}

		// Summary
		fmt.Printf("✓ Installed to %s\n", projectDir)
		if profileName != "" {
			fmt.Printf("  Profile: %s\n", profileName)
		}
		if len(result.Skills) > 0 {
			fmt.Printf("  Skills: %d symlinks created\n", len(result.Skills))
			for _, s := range result.Skills {
				fmt.Printf("    → %s\n", s)
			}
		}
		if len(result.MCP) > 0 {
			fmt.Printf("  MCP: %v\n", result.MCP)
		}

		return nil
	},
}

func init() {
	installCmd.Flags().StringVar(&installProfile, "profile", "", "Profile name to install")
	installCmd.Flags().StringVar(&installDir, "dir", "", "Project directory (default: current dir)")

	rootCmd.AddCommand(installCmd)
}
```

- [ ] **Step 2: Write cmd/update.go**

```go
// cmd/update.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update all git-managed registry entries to latest",
	Long: `Walk the registry directory, find all entries with .git,
and run git pull --ff-only on each.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		skillsDir := filepath.Join(registryDir, "skills")
		updated := 0
		skipped := 0
		errors := 0

		err := filepath.Walk(skillsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() && info.Name() == ".git" {
				repoDir := filepath.Dir(path)
				fmt.Printf("Updating %s ... ", repoDir)

				pullCmd := exec.Command("git", "-C", repoDir, "pull", "--ff-only")
				output, err := pullCmd.CombinedOutput()
				if err != nil {
					fmt.Printf("ERROR: %v\n%s\n", err, string(output))
					errors++
				} else {
					fmt.Println("OK")
					updated++
				}
				return filepath.SkipDir
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("walking registry: %w", err)
		}

		fmt.Printf("\nSummary: %d updated, %d skipped, %d errors\n", updated, skipped, errors)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
```

- [ ] **Step 3: Write cmd/check.go**

```go
// cmd/check.go
package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/symlink"
)

var checkFix bool

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installation integrity",
	Long: `Scan installed symlinks and project records.
Report broken symlinks, missing projects, and orphaned entries.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		issues := 0

		// Check symlinks in codex and claude dirs
		for _, dir := range []string{
			filepath.Join(home, ".codex", "skills"),
			filepath.Join(home, ".claude", "skills"),
		} {
			entries, err := os.ReadDir(dir)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return err
			}

			for _, entry := range entries {
				linkPath := filepath.Join(dir, entry.Name())
				if !symlink.IsSymlink(linkPath) {
					continue
				}
				if !symlink.Verify(linkPath) {
					issues++
					fmt.Printf("⚠ Broken symlink: %s\n", linkPath)
					if checkFix {
						os.Remove(linkPath)
						fmt.Printf("  → Removed\n")
					}
				}
			}
		}

		// Check projects in database
		dbPath := filepath.Join(dataDir, "sm.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Printf("Note: No database found at %s\n", dbPath)
		} else {
			defer database.Close()

			projects, err := database.GetAllProjects()
			if err != nil {
				return err
			}

			for _, p := range projects {
				if _, err := os.Stat(p.Path); os.IsNotExist(err) {
					issues++
					fmt.Printf("⚠ Project directory missing: %s\n", p.Path)
					if checkFix {
						database.RemoveProject(p.Path)
						fmt.Printf("  → Removed from database\n")
					}
				}
			}
		}

		if issues == 0 {
			fmt.Println("✓ All installations healthy")
		} else {
			fmt.Printf("\nFound %d issue(s)\n", issues)
			if !checkFix {
				fmt.Println("Run with --fix to auto-repair")
			}
		}

		return nil
	},
}

func init() {
	checkCmd.Flags().BoolVar(&checkFix, "fix", false, "Auto-fix broken symlinks and stale records")

	rootCmd.AddCommand(checkCmd)
}
```

- [ ] **Step 4: Build and verify**

```bash
go build -o sm .
./sm install --help
./sm update --help
./sm check --help
```

Expected: Help output for each command.

- [ ] **Step 5: Commit**

```bash
git add cmd/install.go cmd/update.go cmd/check.go
git commit -m "feat: add CLI commands — install, update, check"
```

---

## Task 10: Web Dashboard — Backend

**Files:**
- Create: `web/handler.go`
- Create: `web/static/index.html`
- Create: `web/static/style.css`
- Create: `web/static/app.js`
- Modify: `cmd/web.go`

- [ ] **Step 1: Write web/handler.go**

```go
// web/handler.go
package web

import (
	"embed"
	"encoding/json"
	"net/http"

	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/registry"
)

//go:embed static/*
var staticFiles embed.FS

type Handler struct {
	registry *registry.Registry
	database *db.DB
}

func NewHandler(reg *registry.Registry, database *db.DB) *Handler {
	return &Handler{registry: reg, database: database}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/registry", h.handleRegistry)
	mux.HandleFunc("/api/projects", h.handleProjects)
	mux.HandleFunc("/api/history", h.handleHistory)
	mux.HandleFunc("/api/check", h.handleCheck)
	mux.Handle("/", http.FileServer(http.FS(staticFiles)))
}

func (h *Handler) handleRegistry(w http.ResponseWriter, r *http.Request) {
	skills, _ := h.registry.ListSkills()
	mcps, _ := h.registry.ListMCP()

	resp := map[string]interface{}{
		"skills": skills,
		"mcp":    mcps,
	}
	writeJSON(w, resp)
}

func (h *Handler) handleProjects(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		writeJSON(w, []interface{}{})
		return
	}
	projects, _ := h.database.GetAllProjects()
	writeJSON(w, projects)
}

func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		writeJSON(w, []interface{}{})
		return
	}
	installs, _ := h.database.GetAllInstallations()
	writeJSON(w, installs)
}

func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	// Simplified check for web view
	resp := map[string]interface{}{
		"status": "ok",
	}
	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
```

- [ ] **Step 2: Write web/static/index.html**

```html
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>SkillsManager Dashboard</title>
    <link rel="stylesheet" href="/static/style.css">
</head>
<body>
    <nav class="sidebar">
        <h1>sm</h1>
        <ul>
            <li><a href="#" data-view="overview" class="active">Overview</a></li>
            <li><a href="#" data-view="registry">Registry</a></li>
            <li><a href="#" data-view="projects">Projects</a></li>
            <li><a href="#" data-view="history">History</a></li>
        </ul>
    </nav>
    <main id="content">
        <div id="overview" class="view active"></div>
        <div id="registry" class="view"></div>
        <div id="projects" class="view"></div>
        <div id="history" class="view"></div>
    </main>
    <script src="/static/app.js"></script>
</body>
</html>
```

- [ ] **Step 3: Write web/static/style.css**

```css
* { margin: 0; padding: 0; box-sizing: border-box; }

body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
    display: flex;
    min-height: 100vh;
    background: #f5f5f5;
    color: #333;
}

.sidebar {
    width: 200px;
    background: #1a1a2e;
    color: #eee;
    padding: 20px 0;
}

.sidebar h1 {
    padding: 0 20px 20px;
    font-size: 24px;
    border-bottom: 1px solid #333;
}

.sidebar ul { list-style: none; padding: 10px 0; }

.sidebar a {
    display: block;
    padding: 10px 20px;
    color: #aaa;
    text-decoration: none;
    transition: all 0.2s;
}

.sidebar a:hover, .sidebar a.active {
    color: #fff;
    background: #16213e;
}

main {
    flex: 1;
    padding: 30px;
}

.view { display: none; }
.view.active { display: block; }

.stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 20px;
    margin-bottom: 30px;
}

.stat-card {
    background: white;
    border-radius: 8px;
    padding: 20px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}

.stat-card h3 { font-size: 14px; color: #666; text-transform: uppercase; }
.stat-card .value { font-size: 36px; font-weight: bold; color: #1a1a2e; }

table {
    width: 100%;
    background: white;
    border-radius: 8px;
    overflow: hidden;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
    border-collapse: collapse;
}

th, td { padding: 12px 16px; text-align: left; }
th { background: #1a1a2e; color: white; }
tr:nth-child(even) { background: #f9f9f9; }

.category-group { margin-bottom: 24px; }
.category-group h3 {
    font-size: 16px;
    padding: 8px 0;
    border-bottom: 2px solid #1a1a2e;
    margin-bottom: 8px;
}

.badge {
    display: inline-block;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 12px;
    background: #e0e0e0;
}

.badge.special { background: #ffd700; }
```

- [ ] **Step 4: Write web/static/app.js**

```javascript
document.addEventListener('DOMContentLoaded', () => {
    const nav = document.querySelectorAll('.sidebar a');
    const views = document.querySelectorAll('.view');

    nav.forEach(link => {
        link.addEventListener('click', e => {
            e.preventDefault();
            const viewId = link.dataset.view;

            nav.forEach(l => l.classList.remove('active'));
            link.classList.add('active');

            views.forEach(v => v.classList.remove('active'));
            document.getElementById(viewId).classList.add('active');

            loadView(viewId);
        });
    });

    loadView('overview');
});

async function fetchJSON(url) {
    const res = await fetch(url);
    return res.json();
}

async function loadView(view) {
    switch(view) {
        case 'overview': return loadOverview();
        case 'registry': return loadRegistry();
        case 'projects': return loadProjects();
        case 'history': return loadHistory();
    }
}

async function loadOverview() {
    const [reg, projects, history] = await Promise.all([
        fetchJSON('/api/registry'),
        fetchJSON('/api/projects'),
        fetchJSON('/api/history'),
    ]);

    const skillCount = Object.values(reg.skills || {}).flat().length;
    const mcpCount = (reg.mcp || []).length;
    const projectCount = (projects || []).length;

    document.getElementById('overview').innerHTML = `
        <h2>Overview</h2>
        <div class="stats">
            <div class="stat-card"><h3>Skills</h3><div class="value">${skillCount}</div></div>
            <div class="stat-card"><h3>MCP Servers</h3><div class="value">${mcpCount}</div></div>
            <div class="stat-card"><h3>Projects</h3><div class="value">${projectCount}</div></div>
            <div class="stat-card"><h3>Total Installs</h3><div class="value">${(history || []).length}</div></div>
        </div>
        <h3>Recent Installs</h3>
        ${renderHistoryTable((history || []).slice(0, 10))}
    `;
}

async function loadRegistry() {
    const reg = await fetchJSON('/api/registry');
    let html = '<h2>Registry</h2>';

    const skills = reg.skills || {};
    const specialDirs = ['global', 'codex-only', 'claude-only'];

    for (const [cat, names] of Object.entries(skills)) {
        const isSpecial = specialDirs.includes(cat);
        html += `<div class="category-group">
            <h3>${cat} ${isSpecial ? '<span class="badge special">special</span>' : ''}</h3>
            <table><tr><th>Skill Name</th></tr>`;
        names.forEach(n => html += `<tr><td>${n}</td></tr>`);
        html += '</table></div>';
    }

    html += '<h3>MCP Servers</h3><table><tr><th>Name</th></tr>';
    (reg.mcp || []).forEach(n => html += `<tr><td>${n}</td></tr>`);
    html += '</table>';

    document.getElementById('registry').innerHTML = html;
}

async function loadProjects() {
    const projects = await fetchJSON('/api/projects');
    let html = '<h2>Projects</h2>';

    if (!projects || projects.length === 0) {
        html += '<p>No projects installed yet.</p>';
    } else {
        html += '<table><tr><th>Path</th><th>Profile</th><th>Extra Skills</th><th>Extra MCP</th><th>Last Installed</th></tr>';
        projects.forEach(p => {
            html += `<tr>
                <td>${p.path}</td>
                <td>${p.profile || '-'}</td>
                <td>${(p.extra_skills || []).join(', ') || '-'}</td>
                <td>${(p.extra_mcp || []).join(', ') || '-'}</td>
                <td>${p.last_installed || '-'}</td>
            </tr>`;
        });
        html += '</table>';
    }

    document.getElementById('projects').innerHTML = html;
}

async function loadHistory() {
    const history = await fetchJSON('/api/history');
    document.getElementById('history').innerHTML = `
        <h2>Install History</h2>
        ${renderHistoryTable(history || [])}
    `;
}

function renderHistoryTable(items) {
    if (!items || items.length === 0) return '<p>No installations recorded.</p>';
    let html = '<table><tr><th>Time</th><th>Project</th><th>Profile</th><th>Skills</th><th>MCP</th></tr>';
    items.forEach(i => {
        html += `<tr>
            <td>${i.installed_at}</td>
            <td>${i.project_path}</td>
            <td>${i.profile || '-'}</td>
            <td>${(i.skills || []).join(', ') || '-'}</td>
            <td>${(i.mcp || []).join(', ') || '-'}</td>
        </tr>`;
    });
    html += '</table>';
    return html;
}
```

- [ ] **Step 5: Write cmd/web.go**

```go
// cmd/web.go
package cmd

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/web"
)

var webPort int

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Start web dashboard",
	Long:  `Start an embedded HTTP server to browse skills, MCP, projects, and install history.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		reg := registry.New(registryDir)

		dbPath := filepath.Join(dataDir, "sm.db")
		database, err := db.Open(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not open database: %v\n", err)
		}
		defer func() {
			if database != nil {
				database.Close()
			}
		}()

		handler := web.NewHandler(reg, database)
		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)

		addr := fmt.Sprintf(":%d", webPort)
		fmt.Printf("SkillsManager dashboard running at http://localhost%s\n", addr)
		return http.ListenAndServe(addr, mux)
	},
}

func init() {
	webCmd.Flags().IntVarP(&webPort, "port", "p", 3721, "Port to listen on")

	rootCmd.AddCommand(webCmd)
}
```

- [ ] **Step 6: Build and test**

```bash
go build -o sm .
./sm web -p 8080 &
curl -s http://localhost:8080/api/registry
curl -s http://localhost:8080/
kill %1
```

Expected: JSON response from API, HTML from dashboard.

- [ ] **Step 7: Commit**

```bash
git add web/ cmd/web.go
git commit -m "feat: add web dashboard with embedded static files and REST API"
```

---

## Task 11: Profile Definitions

**Files:**
- Modify: `profiles/default.json`
- Create: `profiles/cloudflare.json`
- Create: `profiles/frontend.json`
- Create: `profiles/security-audit.json`

- [ ] **Step 1: Write profile files**

```bash
cat > profiles/default.json << 'EOF'
{
  "skills": ["global"],
  "mcp": []
}
EOF

cat > profiles/cloudflare.json << 'EOF'
{
  "skills": ["global", "cloudflare"],
  "mcp": ["cloudflare", "github"]
}
EOF

cat > profiles/frontend.json << 'EOF'
{
  "skills": ["global", "nextjs", "design"],
  "mcp": ["github"]
}
EOF

cat > profiles/security-audit.json << 'EOF'
{
  "skills": ["global", "security"],
  "mcp": ["github"]
}
EOF
```

- [ ] **Step 2: Verify profiles load**

```bash
go build -o sm .
# Add a test skill so list has content
mkdir -p registry/skills/global/test-skill
echo "# test" > registry/skills/global/test-skill/SKILL.md
./sm list
```

Expected: Shows test-skill under global.

- [ ] **Step 3: Commit**

```bash
git add profiles/
git commit -m "feat: add default profile definitions (cloudflare, frontend, security-audit)"
```

---

## Task 12: README Files

**Files:**
- Create: `README.md`
- Create: `README.zh-CN.md`

- [ ] **Step 1: Write README.md (English)**

```markdown
# SkillsManager (sm)

A CLI tool for managing AI agent skills (Codex, Claude) and MCP server configurations across multiple projects.

## Design Philosophy

### One Original, All Symlinks

The registry holds the original files. All installed locations (`~/.codex/skills/`, `~/.claude/skills/`) are symlinks pointing back to the registry. This means:

- **No duplication** — disk space is minimal
- **Instant updates** — update the registry, all installations reflect the change
- **Easy cleanup** — remove the registry entry, all symlinks break visibly

### Profiles as Presets

A profile bundles a set of skills and MCP configurations for a scenario (e.g., "cloudflare development", "frontend", "security audit"). Projects reference a profile as a base, then layer ad-hoc additions on top.

### Minimal Global, Maximal Local

Only truly cross-tool skills go into `global/`. Domain-specific skills are installed per-project via profiles. This keeps each project's AI environment focused and lightweight.

### Three Special Directories

| Directory | Behavior |
|-----------|----------|
| `global/` | Installs to both Codex and Claude |
| `codex-only/` | Installs to Codex only |
| `claude-only/` | Installs to Claude only |

All other directories are user-defined categories. Skills in category directories install to both tools by default.

## Installation

```bash
go install github.com/woyin/skills-manager@latest
```

Or build from source:

```bash
git clone https://github.com/woyin/skills-manager.git
cd skills-manager
go build -o sm .
# Move to PATH
mv sm /usr/local/bin/
```

## Commands

### `sm add <source> [category]`

Add a skill or MCP to the registry.

```bash
# Add from GitHub
sm add github.com/user/repo/path cloudflare

# Add from local path, globally
sm add ./my-skill --global

# Add MCP definition
sm add github.com/user/mcp-server --mcp
```

**Flags:**
- `--global` — Add to `global/` directory
- `--codex` — Add to `codex-only/` directory
- `--claude` — Add to `claude-only/` directory
- `--mcp` — Treat as MCP server definition

### `sm rm <name> [category]`

Remove a skill or MCP from the registry. Also cleans up symlinks in installed locations.

```bash
sm rm my-skill
sm rm my-skill --global
sm rm cloudflare --mcp
```

### `sm install`

Install skills and MCP into a project directory.

```bash
# In project directory
cd ~/my-project
sm install --profile cloudflare

# Or specify directory
sm install --profile frontend --dir ~/my-project
```

Reads `.sm.json` if present. Creates symlinks in `~/.codex/skills/` and `~/.claude/skills/`. Writes `.mcp.json` for MCP configurations. Records installation in SQLite database.

### `sm update`

Update all git-managed registry entries.

```bash
sm update
```

Walks `registry/`, finds directories with `.git`, runs `git pull --ff-only` on each.

### `sm check`

Verify installation integrity.

```bash
sm check
sm check --fix  # Auto-repair broken symlinks
```

### `sm list`

List all registry contents.

```bash
sm list
```

### `sm web`

Start the web dashboard.

```bash
sm web           # Default port 3721
sm web -p 8080   # Custom port
```

## Project Config (.sm.json)

Projects store their configuration in `.sm.json`:

```json
{
  "profile": "cloudflare",
  "skills": ["extra-skill"],
  "mcp": ["extra-mcp"]
}
```

The profile is the base. `skills` and `mcp` arrays are ad-hoc additions.

## Directory Structure

```
SkillsManager/
├── registry/
│   ├── skills/
│   │   ├── global/          ← Special: all tools
│   │   ├── codex-only/      ← Special: Codex only
│   │   ├── claude-only/     ← Special: Claude only
│   │   ├── cloudflare/      ← User-defined category
│   │   └── ...
│   └── mcp/
│       ├── cloudflare.json
│       └── ...
├── profiles/
│   ├── cloudflare.json
│   └── ...
├── data/
│   └── sm.db                ← SQLite (local, gitignored)
└── ...
```

## License

MIT
```

- [ ] **Step 2: Write README.zh-CN.md (Chinese)**

```markdown
# SkillsManager (sm)

一个用于管理 AI 代理技能（Codex、Claude）和 MCP 服务器配置的 CLI 工具，支持跨项目管理。

## 设计哲学

### 一份原件，全部软链接

注册表持有原始文件。所有安装位置（`~/.codex/skills/`、`~/.claude/skills/`）都是指向注册表的软链接。这意味着：

- **无重复** — 磁盘占用极小
- **即时更新** — 更新注册表后，所有安装自动生效
- **轻松清理** — 删除注册表条目，所有软链接立即可见失效

### 预设配置即 Profile

Profile 将一组技能和 MCP 配置打包为一个场景（例如 "cloudflare 开发"、"前端"、"安全审计"）。项目以 Profile 为基础，在其上叠加自定义配置。

### 最小化全局，最大化本地

只有真正跨工具的技能才放入 `global/`。领域特定的技能通过 Profile 按项目安装。这让每个项目的 AI 环境保持专注和轻量。

### 三个特殊目录

| 目录 | 行为 |
|------|------|
| `global/` | 安装到 Codex 和 Claude |
| `codex-only/` | 仅安装到 Codex |
| `claude-only/` | 仅安装到 Claude |

其他所有目录均为用户自定义类别。类别目录中的技能默认安装到两个工具。

## 安装

```bash
go install github.com/woyin/skills-manager@latest
```

或从源码构建：

```bash
git clone https://github.com/woyin/skills-manager.git
cd skills-manager
go build -o sm .
mv sm /usr/local/bin/
```

## 命令

### `sm add <source> [category]`

向注册表添加技能或 MCP。

```bash
# 从 GitHub 添加
sm add github.com/user/repo/path cloudflare

# 从本地路径添加，设为全局
sm add ./my-skill --global

# 添加 MCP 定义
sm add github.com/user/mcp-server --mcp
```

**参数：**
- `--global` — 添加到 `global/` 目录
- `--codex` — 添加到 `codex-only/` 目录
- `--claude` — 添加到 `claude-only/` 目录
- `--mcp` — 作为 MCP 服务器定义添加

### `sm rm <name> [category]`

从注册表删除技能或 MCP。同时清理已安装位置的软链接。

```bash
sm rm my-skill
sm rm my-skill --global
sm rm cloudflare --mcp
```

### `sm install`

将技能和 MCP 安装到项目目录。

```bash
# 在项目目录内
cd ~/my-project
sm install --profile cloudflare

# 或指定目录
sm install --profile frontend --dir ~/my-project
```

读取 `.sm.json`（如存在）。在 `~/.codex/skills/` 和 `~/.claude/skills/` 中创建软链接。为 MCP 配置写入 `.mcp.json`。在 SQLite 数据库中记录安装详情。

### `sm update`

更新所有 Git 管理的注册表条目。

```bash
sm update
```

遍历 `registry/`，找到含 `.git` 的目录，对每个执行 `git pull --ff-only`。

### `sm check`

检查安装完整性。

```bash
sm check
sm check --fix  # 自动修复损坏的软链接
```

### `sm list`

列出所有注册表内容。

```bash
sm list
```

### `sm web`

启动 Web 仪表板。

```bash
sm web           # 默认端口 3721
sm web -p 8080   # 自定义端口
```

## 项目配置 (.sm.json)

项目在 `.sm.json` 中存储配置：

```json
{
  "profile": "cloudflare",
  "skills": ["extra-skill"],
  "mcp": ["extra-mcp"]
}
```

Profile 是基础配置，`skills` 和 `mcp` 数组是额外叠加的配置。

## 目录结构

```
SkillsManager/
├── registry/
│   ├── skills/
│   │   ├── global/          ← 特殊目录：所有工具
│   │   ├── codex-only/      ← 特殊目录：仅 Codex
│   │   ├── claude-only/     ← 特殊目录：仅 Claude
│   │   ├── cloudflare/      ← 用户自定义类别
│   │   └── ...
│   └── mcp/
│       ├── cloudflare.json
│       └── ...
├── profiles/
│   ├── cloudflare.json
│   └── ...
├── data/
│   └── sm.db                ← SQLite（本地，gitignore）
└── ...
```

## 许可证

MIT
```

- [ ] **Step 3: Commit**

```bash
git add README.md README.zh-CN.md
git commit -m "docs: add bilingual README (English + Chinese)"
```

---

## Final Verification

- [ ] **Step 1: Full build**

```bash
cd /Users/breestealth/Documents/DevelopmentRepository/SkillsManager
go build -o sm .
```

- [ ] **Step 2: Run all tests**

```bash
go test ./... -v
```

Expected: All tests pass.

- [ ] **Step 3: Smoke test**

```bash
./sm --help
./sm list
./sm add --help
./sm install --help
```

Expected: All commands show correct help.

- [ ] **Step 4: Final commit**

```bash
git add .
git commit -m "chore: final cleanup and verification"
```
