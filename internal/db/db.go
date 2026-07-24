// Package db 封装 sm 的本地 SQLite 状态库。
//
// 该数据库记录两类信息：
//   - installations：每次 `sm install` 的历史（项目、profile、技能、MCP）；
//   - projects：已安装项目的持久化配置（profile + 额外技能/MCP）。
//
// 连接参数（见 Open）针对"读多写少 + 偶发写"的负载做了优化：
// WAL 日志模式 + NORMAL 同步级别 + busy_timeout，兼顾吞吐与并发。
//
// Input: database/sql, encoding/json, fmt, os, path/filepath, time, modernc.org/sqlite
// Output: type DB, type Installation, type Project, func Open
// Pos: 数据层-SQLite状态库
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // 纯 Go 的 SQLite 驱动，注册为 "sqlite"
)

// DB 包装一个 *sql.DB 连接，提供 sm 特定的查询方法。
type DB struct {
	db *sql.DB
}

// Installation 记录一次 `sm install` 调用：哪个项目、用了哪个 profile、
// 安装了哪些技能与 MCP。
type Installation struct {
	ID          int64     `json:"id"`
	ProjectPath string    `json:"project_path"`
	Profile     string    `json:"profile"`
	Skills      []string  `json:"skills"`
	MCP         []string  `json:"mcp"`
	InstalledAt time.Time `json:"installed_at"`
}

// Project 是某个被 sm 安装过的项目的持久化配置。
type Project struct {
	Path          string    `json:"path"`
	Profile       string    `json:"profile"`
	ExtraSkills   []string  `json:"extra_skills"`
	ExtraMCP      []string  `json:"extra_mcp"`
	LastInstalled time.Time `json:"last_installed"`
}

// Open 打开（必要时创建）位于 dbPath 的 SQLite 数据库，应用 schema 与
// 连接 pragma。父目录缺失时会被创建。
func Open(dbPath string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	// modernc.org/sqlite 是纯 Go 驱动，注册名 "sqlite"。
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// SQLite 单写者模型；保留小规模连接池，使并发读者（如 Web 仪表盘）
	// 不必串行在单一连接上。写入仍由 busy_timeout 保护。
	db.SetMaxOpenConns(4)

	// 针对工作负载的 pragma 调优：
	//   WAL          —— 写前日志不阻塞读者；
	//   NORMAL       —— 在 WAL 下用少量崩溃耐久度换取大幅写入加速；
	//   busy_timeout —— 让并发写者等待而非立即报错。
	if err := applyPragmas(db); err != nil {
		db.Close()
		return nil, err
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

// applyPragmas 配置连接的并发与写入性能。
// journal_mode=WAL 会持久化到数据库文件（只需设一次）；其余为每连接级，
// 重复应用无副作用。
func applyPragmas(db *sql.DB) error {
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			return err
		}
	}
	return nil
}

// Close 关闭底层数据库连接。
func (d *DB) Close() error {
	return d.db.Close()
}

// initSchema 创建所需的表（已存在则跳过）。
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

// RecordInstallation 记录一次安装事件。
// skills / mcp 以 JSON 字符串形式持久化。
func (d *DB) RecordInstallation(projectPath, profile string, skills, mcp []string) error {
	skillsJSON, err := json.Marshal(skills)
	if err != nil {
		return fmt.Errorf("marshaling skills: %w", err)
	}
	mcpJSON, err := json.Marshal(mcp)
	if err != nil {
		return fmt.Errorf("marshaling mcp: %w", err)
	}

	_, err = d.db.Exec(
		"INSERT INTO installations (project_path, profile, skills, mcp) VALUES (?, ?, ?, ?)",
		projectPath, profile, string(skillsJSON), string(mcpJSON),
	)
	return err
}

// GetInstallations 返回某项目的全部安装历史，按时间倒序。
func (d *DB) GetInstallations(projectPath string) ([]Installation, error) {
	rows, err := d.db.Query(
		"SELECT id, project_path, profile, skills, mcp, installed_at FROM installations WHERE project_path = ? ORDER BY installed_at DESC",
		projectPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstallations(rows)
}

// GetAllInstallations 返回全部安装历史，按时间倒序。
func (d *DB) GetAllInstallations() ([]Installation, error) {
	rows, err := d.db.Query(
		"SELECT id, project_path, profile, skills, mcp, installed_at FROM installations ORDER BY installed_at DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanInstallations(rows)
}

// scanInstallations 从 rows 中扫描安装记录，消除 GetInstallations 与
// GetAllInstallations 的行扫描重复。反序列化失败时仅告警，不阻断 ——
// 单条坏数据不应让整个查询失败。
func scanInstallations(rows *sql.Rows) ([]Installation, error) {
	var results []Installation
	for rows.Next() {
		var inst Installation
		var skillsStr, mcpStr string
		if err := rows.Scan(&inst.ID, &inst.ProjectPath, &inst.Profile, &skillsStr, &mcpStr, &inst.InstalledAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(skillsStr), &inst.Skills); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unmarshal skills for installation %d: %v\n", inst.ID, err)
		}
		if err := json.Unmarshal([]byte(mcpStr), &inst.MCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unmarshal mcp for installation %d: %v\n", inst.ID, err)
		}
		results = append(results, inst)
	}
	return results, nil
}

// UpsertProject 插入或更新（按 path 主键）项目记录，并刷新 last_installed。
func (d *DB) UpsertProject(projectPath, profile string, extraSkills, extraMCP []string) error {
	skillsJSON, err := json.Marshal(extraSkills)
	if err != nil {
		return fmt.Errorf("marshaling extra_skills: %w", err)
	}
	mcpJSON, err := json.Marshal(extraMCP)
	if err != nil {
		return fmt.Errorf("marshaling extra_mcp: %w", err)
	}

	_, err = d.db.Exec(
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

// GetAllProjects 返回全部项目记录，按 last_installed 倒序。
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
		if err := json.Unmarshal([]byte(skillsStr), &p.ExtraSkills); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unmarshal extra_skills for project %q: %v\n", p.Path, err)
		}
		if err := json.Unmarshal([]byte(mcpStr), &p.ExtraMCP); err != nil {
			fmt.Fprintf(os.Stderr, "warning: unmarshal extra_mcp for project %q: %v\n", p.Path, err)
		}
		results = append(results, p)
	}
	return results, nil
}

// RemoveProject 删除一条项目记录。
func (d *DB) RemoveProject(projectPath string) error {
	_, err := d.db.Exec("DELETE FROM projects WHERE path = ?", projectPath)
	return err
}

// HasTable 判断数据库 schema 中是否存在某张表。
// `sm doctor` 用它作为健康检查的一部分。
func (d *DB) HasTable(name string) bool {
	const q = "SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?"
	var count int
	if err := d.db.QueryRow(q, name).Scan(&count); err != nil {
		return false
	}
	return count > 0
}
