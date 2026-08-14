// cmd/import.go 实现 `sm import`：从 JSON 文件导入配置
// （skills、MCP、profiles、prompts、projects），支持 merge/replace。
//
// Input: encoding/json, fmt, io, os, path/filepath, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/profile, github.com/woyin/skills-manager/internal/prompt, github.com/woyin/skills-manager/internal/registry
// Output: var importCmd, func printImportPreview, func performImport, func importSkills, func importMCP, func importProfiles, func importPrompts, func importProjects
// Pos: 控制层-import命令实现（从 JSON 文件导入配置）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/profile"
	"github.com/woyin/skills-manager/internal/prompt"
	"github.com/woyin/skills-manager/internal/registry"
)

var (
	importDryRun  bool
	importMerge   bool
	importReplace bool
)

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Import configuration from JSON",
	Long: `Import SkillsManager configuration from a JSON file.
Supports merge (default) or replace mode.
Use --replace to clear existing data before importing, or --merge (default) to add alongside existing entries.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath := args[0]

		if importReplace && cmd.Flags().Changed("merge") {
			return fmt.Errorf("--replace and --merge are mutually exclusive")
		}
		if importReplace {
			importMerge = false
		}

		// 读取文件
		var reader io.Reader
		if filePath == "-" {
			reader = os.Stdin
		} else {
			file, err := os.Open(filePath)
			if err != nil {
				return fmt.Errorf("opening file: %w", err)
			}
			defer file.Close()
			reader = file
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		var exportData ExportData
		if err := json.Unmarshal(data, &exportData); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}

		if importDryRun {
			return printImportPreview(&exportData)
		}

		return performImport(&exportData)
	},
}

// dry-run 模式：打印将要导入的内容而不实际写入。

func printImportPreview(data *ExportData) error {
	fmt.Println("Import Preview")
	fmt.Println("==============")

	if len(data.Skills) > 0 {
		fmt.Printf("\nSkills:\n")
		for category, skills := range data.Skills {
			for _, s := range skills {
				fmt.Printf("  - %s/%s\n", category, s.Name)
			}
		}
	}

	if len(data.MCP) > 0 {
		fmt.Printf("\nMCP Servers:\n")
		for _, m := range data.MCP {
			fmt.Printf("  - %s\n", m.Name)
		}
	}

	if len(data.Profiles) > 0 {
		fmt.Printf("\nProfiles:\n")
		for name := range data.Profiles {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(data.Prompts) > 0 {
		fmt.Printf("\nPrompt Sets:\n")
		for name := range data.Prompts {
			fmt.Printf("  - %s\n", name)
		}
	}

	if len(data.Projects) > 0 {
		fmt.Printf("\nProjects:\n")
		for _, p := range data.Projects {
			fmt.Printf("  - %s (profile: %s)\n", p.Path, p.Profile)
		}
	}

	fmt.Println("\nRun without --dry-run to apply changes.")
	return nil
}

// performImport 把导出数据依次写入本地五个类目。
// 各类目独立处理：replace=true 时先清空再导入；否则与现有数据合并；
// 单项失败按 merge/strict 语义警告或终止（见 warnOrFail）。
func performImport(data *ExportData) error {
	steps := []func(*ExportData, bool) error{
		importSkills,
		importMCP,
		importProfiles,
		importPrompts,
		importProjects,
	}
	for _, step := range steps {
		if err := step(data, importReplace); err != nil {
			return err
		}
	}

	fmt.Println("✓ Import completed successfully")
	return nil
}

// warnOrFail 处理单项导入失败：strict（非 --merge）模式返回错误终止导入，
// merge 模式打印警告后继续。verb 为错误描述动词（adding/saving 等），
// kind 为类目名（skill/MCP/profile 等），label 为该项的标识。
func warnOrFail(verb, kind, label string, err error) error {
	if !importMerge {
		return fmt.Errorf("%s %s %s: %w", verb, kind, label, err)
	}
	fmt.Fprintf(os.Stderr, "warning: skipping %s %s: %v\n", kind, label, err)
	return nil
}

// importSkills 导入 skills 类目：replace 时先移除已有 skills。
func importSkills(data *ExportData, replace bool) error {
	if len(data.Skills) == 0 {
		return nil
	}
	reg := registry.New(RegistryDir)

	if replace {
		existing, _ := reg.ListSkillDetails()
		for category, skills := range existing {
			for _, s := range skills {
				if err := reg.RemoveSkill(s.Name, category, ""); err != nil {
					fmt.Fprintf(os.Stderr, "warning: removing skill %s/%s: %v\n", category, s.Name, err)
				}
			}
		}
	}

	for category, skills := range data.Skills {
		for _, s := range skills {
			if s.SourceURL == "" {
				continue // 跳过没有 source URL 的技能
			}
			if err := reg.AddSkill(s.SourceURL, category, ""); err != nil {
				if ferr := warnOrFail("adding", "skill", category+"/"+s.Name, err); ferr != nil {
					return ferr
				}
			}
		}
	}
	return nil
}

// importMCP 导入 MCP 类目：replace 时先移除已有 MCP。
func importMCP(data *ExportData, replace bool) error {
	if len(data.MCP) == 0 {
		return nil
	}
	reg := registry.New(RegistryDir)

	if replace {
		existing, _ := reg.ListMCPDetails()
		for _, m := range existing {
			if err := reg.RemoveMCP(m.Name); err != nil {
				fmt.Fprintf(os.Stderr, "warning: removing MCP %s: %v\n", m.Name, err)
			}
		}
	}

	for _, m := range data.MCP {
		if m.SourceURL == "" {
			continue // Skip MCP without source URL
		}
		if err := reg.AddMCP(m.SourceURL); err != nil {
			if ferr := warnOrFail("adding", "MCP", m.Name, err); ferr != nil {
				return ferr
			}
		}
	}
	return nil
}

// importProfiles 导入 profiles 类目：replace 时先删除已有 profiles。
func importProfiles(data *ExportData, replace bool) error {
	if len(data.Profiles) == 0 {
		return nil
	}
	loader := profile.NewLoader(ProfilesDir)

	if replace {
		existing, _ := loader.List()
		for _, name := range existing {
			if err := loader.Delete(name); err != nil {
				fmt.Fprintf(os.Stderr, "warning: removing profile %s: %v\n", name, err)
			}
		}
	}

	for name, config := range data.Profiles {
		if err := loader.Save(name, config); err != nil {
			if ferr := warnOrFail("saving", "profile", name, err); ferr != nil {
				return ferr
			}
		}
	}
	return nil
}

// importPrompts 导入 prompt sets 类目：replace 时先删除已有 prompt sets。
func importPrompts(data *ExportData, replace bool) error {
	if len(data.Prompts) == 0 {
		return nil
	}
	manager := prompt.NewManager(filepath.Join(RegistryDir, "prompts"))

	if replace {
		existing, _ := manager.List()
		for _, name := range existing {
			if err := manager.Delete(name); err != nil {
				fmt.Fprintf(os.Stderr, "warning: removing prompt set %s: %v\n", name, err)
			}
		}
	}

	for name, ps := range data.Prompts {
		if ps == nil {
			continue
		}
		if ps.Name == "" {
			ps.Name = name
		}
		if err := manager.Save(ps); err != nil {
			if ferr := warnOrFail("saving", "prompt set", name, err); ferr != nil {
				return ferr
			}
		}
	}
	return nil
}

// importProjects 导入 projects 类目：replace 时先移除已有 projects。
func importProjects(data *ExportData, replace bool) error {
	if len(data.Projects) == 0 {
		return nil
	}
	database, err := openDB()
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()

	if replace {
		existing, _ := database.GetAllProjects()
		for _, p := range existing {
			if err := database.RemoveProject(p.Path); err != nil {
				fmt.Fprintf(os.Stderr, "warning: removing project %s: %v\n", p.Path, err)
			}
		}
	}

	for _, p := range data.Projects {
		if err := database.UpsertProject(p.Path, p.Profile, p.ExtraSkills, p.ExtraMCP); err != nil {
			if ferr := warnOrFail("importing", "project", p.Path, err); ferr != nil {
				return ferr
			}
		}
	}
	return nil
}

func init() {
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "Show what would be imported without making changes")
	importCmd.Flags().BoolVar(&importMerge, "merge", true, "Merge with existing data (default)")
	importCmd.Flags().BoolVar(&importReplace, "replace", false, "Replace existing data (clear everything first)")

	rootCmd.AddCommand(importCmd)
}
