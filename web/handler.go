// Package web 提供 sm 内嵌的 Web 仪表盘：静态前端 + JSON REST API。
//
// 路由（见 RegisterRoutes）：
//
//	/              内嵌的 index.html
//	/static/*      内嵌的静态资源（CSS/JS）
//	/api/registry  注册表内容（skills + MCP）
//	/api/projects  已记录的项目列表
//	/api/history   安装历史
//	/api/check     完整性检查（失效/孤立符号链接、丢失项目）
//	/api/tools     工具目录及安装状态
//	/api/aivo      aivo 状态（若安装）
package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/aivo"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/home"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

//go:embed static/*
var staticFiles embed.FS

// Handler 提供内嵌仪表盘 UI 与 JSON REST API。
type Handler struct {
	registry *registry.Registry
	database *db.DB
}

// aivoResponse 是 /api/aivo 端点的返回载荷。
type aivoResponse struct {
	Installed     bool   `json:"installed"`
	Version       string `json:"version"`
	Path          string `json:"path"`
	ActiveKey     string `json:"active_key,omitempty"`
	ActiveModel   string `json:"active_model,omitempty"`
	KeysCount     int    `json:"keys_count"`
	HealthyKeys   int    `json:"healthy_keys"`
	UnhealthyKeys int    `json:"unhealthy_keys"`
	TotalTokens   int64  `json:"total_tokens,omitempty"`
	Sessions      int    `json:"sessions,omitempty"`
	Models        int    `json:"models,omitempty"`
}

// registryResponse 是 /api/registry 端点的返回载荷。
type registryResponse struct {
	Skills       map[string][]string              `json:"skills"`
	MCP          []string                         `json:"mcp"`
	SkillDetails map[string][]registry.ItemDetail `json:"skill_details"`
	MCPDetails   []registry.ItemDetail            `json:"mcp_details"`
}

// checkIssue 是 /api/check 报告的一条完整性问题。
type checkIssue struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

// checkResponse 是 /api/check 端点的返回载荷。
type checkResponse struct {
	Status string       `json:"status"`
	Issues []checkIssue `json:"issues"`
}

// NewHandler 返回由指定 registry 与 database 支撑的 Handler。
// database 为 nil 时，project/history 端点返回空数据。
func NewHandler(reg *registry.Registry, database *db.DB) *Handler {
	return &Handler{registry: reg, database: database}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/registry", h.handleRegistry)
	mux.HandleFunc("/api/projects", h.handleProjects)
	mux.HandleFunc("/api/history", h.handleHistory)
	mux.HandleFunc("/api/check", h.handleCheck)
	mux.HandleFunc("/api/tools", h.handleTools)
	mux.HandleFunc("/api/aivo", h.handleAivo)
	mux.HandleFunc("/", h.handleIndex)
	mux.Handle("/static/", http.FileServer(http.FS(staticFiles)))
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write(data)
}

func (h *Handler) handleRegistry(w http.ResponseWriter, r *http.Request) {
	skills, err := h.registry.ListSkills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skills: "+err.Error())
		return
	}
	mcps, err := h.registry.ListMCP()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing mcp: "+err.Error())
		return
	}
	if mcps == nil {
		mcps = []string{}
	}
	skillDetails, err := h.registry.ListSkillDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing skill details: "+err.Error())
		return
	}
	mcpDetails, err := h.registry.ListMCPDetails()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing mcp details: "+err.Error())
		return
	}
	if mcpDetails == nil {
		mcpDetails = []registry.ItemDetail{}
	}

	resp := registryResponse{
		Skills:       skills,
		MCP:          mcps,
		SkillDetails: skillDetails,
		MCPDetails:   mcpDetails,
	}
	writeJSON(w, resp)
}

func (h *Handler) handleProjects(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		writeJSON(w, []any{})
		return
	}
	projects, err := h.database.GetAllProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing projects: "+err.Error())
		return
	}
	writeJSON(w, projects)
}

func (h *Handler) handleHistory(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		writeJSON(w, []any{})
		return
	}
	installs, err := h.database.GetAllInstallations()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "listing installations: "+err.Error())
		return
	}
	writeJSON(w, installs)
}

func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	issues := make([]checkIssue, 0, 8)

	for _, t := range tool.AllTools() {
		dir := filepath.Join(home.Dir(), t.SkillDir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if !symlink.IsSymlink(path) {
				continue
			}
			if !symlink.Verify(path) {
				issues = append(issues, checkIssue{Type: "broken_symlink", Path: path})
				continue
			}
			if !symlink.PointInside(path, h.registry.Dir()) {
				issues = append(issues, checkIssue{Type: "orphaned_symlink", Path: path})
			}
		}
	}

	if h.database != nil {
		projects, err := h.database.GetAllProjects()
		if err == nil {
			for _, project := range projects {
				if _, err := os.Stat(project.Path); os.IsNotExist(err) {
					issues = append(issues, checkIssue{Type: "missing_project", Path: project.Path})
				}
			}
		}
	}

	status := "ok"
	if len(issues) > 0 {
		status = "issues"
	}
	writeJSON(w, checkResponse{Status: status, Issues: issues})
}

func (h *Handler) handleTools(w http.ResponseWriter, r *http.Request) {
	tools := tool.AllTools()
	type toolInfo struct {
		Name        string `json:"name"`
		Installed   bool   `json:"installed"`
		HasSkillDir bool   `json:"has_skill_dir"`
	}

	result := make([]toolInfo, len(tools))
	for i, t := range tools {
		result[i] = toolInfo{
			Name:        t.Name,
			Installed:   tool.IsInstalled(t),
			HasSkillDir: tool.HasSkillDir(t),
		}
	}

	writeJSON(w, result)
}

func (h *Handler) handleAivo(w http.ResponseWriter, r *http.Request) {
	info := aivo.Detect()

	resp := aivoResponse{
		Installed: info.Installed,
		Version:   info.Version,
		Path:      info.Path,
	}

	if !info.Installed {
		writeJSON(w, resp)
		return
	}

	keys := aivo.ListKeys()
	active := aivo.ActiveKeyFromKeys(keys)
	if active != nil {
		resp.ActiveKey = active.Name
		resp.ActiveModel = active.BaseURL
	}

	resp.KeysCount = len(keys)

	healthy, unhealthy := 0, 0
	for _, k := range keys {
		if k.PingOK != nil && *k.PingOK {
			healthy++
		} else {
			unhealthy++
		}
	}
	resp.HealthyKeys = healthy
	resp.UnhealthyKeys = unhealthy

	if stats := aivo.GetStats(); stats != nil {
		resp.TotalTokens = stats.TotalTokens
		resp.Sessions = stats.Sessions
		resp.Models = stats.Models
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
