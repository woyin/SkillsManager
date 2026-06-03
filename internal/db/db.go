package db

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type Installation struct {
	ID          int64     `json:"id"`
	ProjectPath string    `json:"project_path"`
	Profile     string    `json:"profile"`
	Skills      []string  `json:"skills"`
	MCP         []string  `json:"mcp"`
	InstalledAt time.Time `json:"installed_at"`
}

type Project struct {
	Path          string    `json:"path"`
	Profile       string    `json:"profile"`
	ExtraSkills   []string  `json:"extra_skills"`
	ExtraMCP      []string  `json:"extra_mcp"`
	LastInstalled time.Time `json:"last_installed"`
}

func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

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
