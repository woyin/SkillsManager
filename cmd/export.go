// cmd/export.go 实现 `sm export`：把配置导出为 JSON。
// 含注册表、profiles、prompts、projects；可用 --include 选择性导出。
package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/prompt"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	exportOutput  string
	exportInclude string
)

// ExportData 是 sm 配置导出 JSON 的磁盘形状：注册表 skills/MCP、profiles、prompts、projects。
type ExportData struct {
	Version    string                           `json:"version"`
	ExportedAt time.Time                        `json:"exported_at"`
	Skills     map[string][]registry.ItemDetail `json:"skills,omitempty"`
	MCP        []registry.ItemDetail            `json:"mcp,omitempty"`
	Profiles   map[string]*profile.Config       `json:"profiles,omitempty"`
	Prompts    map[string]*prompt.PromptSet     `json:"prompts,omitempty"`
	Projects   []db.Project                     `json:"projects,omitempty"`
}

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export configuration to JSON",
	Long: `Export SkillsManager configuration to a JSON file.
Includes registry contents, profiles, and project records.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		include := parseIncludeFlags(exportInclude)
		data, err := buildExportData(include)
		if err != nil {
			return err
		}

		// 序列化为 JSON
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}

		// 输出
		if exportOutput == "" || exportOutput == "-" {
			fmt.Println(string(jsonData))
		} else {
			if err := os.WriteFile(exportOutput, jsonData, 0644); err != nil {
				return fmt.Errorf("writing file: %w", err)
			}
			fmt.Printf("✓ Exported to %s\n", exportOutput)
		}

		return nil
	},
}

// 按 include 标志构建导出数据。

func buildExportData(include map[string]bool) (*ExportData, error) {
	data := &ExportData{
		Version:    "1.0",
		ExportedAt: time.Now(),
	}

	// 导出注册表
	if include["registry"] {
		reg := registry.New(RegistryDir)

		skills, err := reg.ListSkillDetails()
		if err != nil {
			return nil, fmt.Errorf("listing skills: %w", err)
		}
		data.Skills = skills

		mcp, err := reg.ListMCPDetails()
		if err != nil {
			return nil, fmt.Errorf("listing MCP: %w", err)
		}
		data.MCP = mcp
	}

	// 导出 profiles
	if include["profiles"] {
		loader := profile.NewLoader(ProfilesDir)
		names, err := loader.List()
		if err != nil {
			return nil, fmt.Errorf("listing profiles: %w", err)
		}

		data.Profiles = make(map[string]*profile.Config)
		for _, name := range names {
			p, err := loader.Load(name)
			if err != nil {
				continue
			}
			data.Profiles[name] = p
		}
	}

	// 导出 prompt sets
	if include["prompts"] {
		manager := prompt.NewManager(filepath.Join(RegistryDir, "prompts"))
		names, err := manager.List()
		if err != nil {
			return nil, fmt.Errorf("listing prompt sets: %w", err)
		}

		data.Prompts = make(map[string]*prompt.PromptSet)
		for _, name := range names {
			ps, err := manager.Load(name)
			if err != nil {
				continue
			}
			data.Prompts[name] = ps
		}
	}

	// 导出 projects
	if include["projects"] {
		if _, err := os.Stat(dbPath()); err == nil {
			database, err := openDB()
			if err == nil {
				defer database.Close()
				projects, err := database.GetAllProjects()
				if err == nil {
					data.Projects = projects
				}
			}
		}
	}

	return data, nil
}

// 解析 --include 逗号分隔列表，返回各分项是否包含的 map。

func parseIncludeFlags(include string) map[string]bool {
	result := map[string]bool{
		"registry": true,
		"profiles": true,
		"prompts":  true,
		"projects": true,
	}

	if include == "" {
		return result
	}

	// 全部重置为 false
	for k := range result {
		result[k] = false
	}

	// 把指定的项置为 true
	for _, name := range strings.Split(include, ",") {
		name = strings.TrimSpace(name)
		if _, ok := result[name]; ok {
			result[name] = true
		}
	}

	return result
}

func init() {
	exportCmd.Flags().StringVarP(&exportOutput, "output", "o", "", "Output file path (default: stdout)")
	exportCmd.Flags().StringVar(&exportInclude, "include", "", "Comma-separated list of items to export: registry,profiles,prompts,projects (default: all)")

	rootCmd.AddCommand(exportCmd)
}
