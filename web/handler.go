package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/woyin/skills-manager/internal/aivo"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/symlink"
	"github.com/woyin/skills-manager/internal/tool"
)

//go:embed static/*
var staticFiles embed.FS

type Handler struct {
	registry *registry.Registry
	database *db.DB
}

type checkIssue struct {
	Type string `json:"type"`
	Path string `json:"path"`
}

type checkResponse struct {
	Status string       `json:"status"`
	Issues []checkIssue `json:"issues"`
}

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
	skills, _ := h.registry.ListSkills()
	mcps, _ := h.registry.ListMCP()
	if mcps == nil {
		mcps = []string{}
	}
	skillDetails, _ := h.registry.ListSkillDetails()
	mcpDetails, _ := h.registry.ListMCPDetails()
	if mcpDetails == nil {
		mcpDetails = []registry.ItemDetail{}
	}

	resp := map[string]interface{}{
		"skills":        skills,
		"mcp":           mcps,
		"skill_details": skillDetails,
		"mcp_details":   mcpDetails,
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
	issues := []checkIssue{}

	home, _ := os.UserHomeDir()
	for _, t := range tool.AllTools() {
		dir := filepath.Join(home, t.SkillDir)
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
		Name       string `json:"name"`
		Installed  bool   `json:"installed"`
		HasSkillDir bool  `json:"has_skill_dir"`
	}

	result := make([]toolInfo, len(tools))
	for i, t := range tools {
		result[i] = toolInfo{
			Name:       t.Name,
			Installed:  tool.IsInstalled(t),
			HasSkillDir: tool.HasSkillDir(t),
		}
	}

	writeJSON(w, result)
}

func (h *Handler) handleAivo(w http.ResponseWriter, r *http.Request) {
	info := aivo.Detect()

	resp := map[string]interface{}{
		"installed": info.Installed,
		"version":   info.Version,
		"path":      info.Path,
	}

	if !info.Installed {
		writeJSON(w, resp)
		return
	}

	keys := aivo.ListKeys()
	active := aivo.ActiveKeyFromKeys(keys)
	if active != nil {
		resp["active_key"] = active.Name
		resp["active_model"] = active.BaseURL
	}

	resp["keys_count"] = len(keys)

	healthy, unhealthy := 0, 0
	for _, k := range keys {
		if k.PingOK != nil && *k.PingOK {
			healthy++
		} else {
			unhealthy++
		}
	}
	resp["healthy_keys"] = healthy
	resp["unhealthy_keys"] = unhealthy

	stats := aivo.GetStats()
	if stats != nil {
		resp["total_tokens"] = stats.TotalTokens
		resp["sessions"] = stats.Sessions
		resp["models"] = stats.Models
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
