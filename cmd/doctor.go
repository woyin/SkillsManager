// cmd/doctor.go 实现 `sm doctor`：对 CLI 工具、目录、数据库、
// 环境变量做健康检查并汇总。
// cmd/doctor.go
package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/aivo"
	"github.com/woyin/skills-manager/internal/db"
	"github.com/woyin/skills-manager/internal/tool"
)

type checkResult struct {
	Name    string
	Status  string // "pass", "warn", "fail"
	Message string
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check environment and dependencies",
	Long: `Check the health of your SkillsManager environment.
Verifies CLI tools, directories, database, and environment variables.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		results := runDoctor()
		return printDoctorResults(results)
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}

func runDoctor() []checkResult {
	var results []checkResult
	results = append(results, checkCLITools()...)
	results = append(results, checkAivo()...)
	results = append(results, checkDirectories()...)
	results = append(results, checkDatabase()...)
	results = append(results, checkEnvironment()...)
	return results
}

func checkCLITools() []checkResult {
	var results []checkResult

	results = append(results, binaryCheck("Git", "git"))

	for _, t := range tool.AllTools() {
		results = append(results, binaryCheck(t.Name, t.Binary))
	}

	results = append(results, binaryCheck("Go", "go"))

	return results
}

func checkAivo() []checkResult {
	var results []checkResult

	info := aivo.Detect()
	if !info.Installed {
		results = append(results, checkResult{
			Name:    "aivo",
			Status:  "warn",
			Message: "not found (optional: API key management & model routing)",
		})
		return results
	}

	results = append(results, checkResult{
		Name:    "aivo",
		Status:  "pass",
		Message: fmt.Sprintf("%s (%s)", info.Path, info.Version),
	})

	keys := aivo.ListKeys()
	if len(keys) == 0 {
		results = append(results, checkResult{
			Name:    "aivo keys",
			Status:  "warn",
			Message: "no API keys configured",
		})
		return results
	}

	healthy, failed := 0, 0
	for _, k := range keys {
		if k.PingOK != nil && *k.PingOK {
			healthy++
		} else {
			failed++
		}
	}

	active := aivo.ActiveKeyFromKeys(keys)
	activeMsg := "none"
	if active != nil {
		activeMsg = active.Name
	}

	msg := fmt.Sprintf("%d key(s), active: %s", len(keys), activeMsg)
	if failed > 0 {
		msg += fmt.Sprintf(" (%d unhealthy)", failed)
	}
	status := "pass"
	if failed > 0 {
		status = "warn"
	}

	results = append(results, checkResult{
		Name:    "aivo keys",
		Status:  status,
		Message: msg,
	})

	return results
}

func binaryCheck(label, binary string) checkResult {
	path, err := exec.LookPath(binary)
	if err != nil {
		return checkResult{Name: label, Status: "warn", Message: "not found in PATH"}
	}
	return checkResult{Name: label, Status: "pass", Message: fmt.Sprintf("found at %s", path)}
}

func checkDirectories() []checkResult {
	var results []checkResult
	home, _ := os.UserHomeDir()

	dirs := []struct {
		name string
		path string
	}{
		{"Registry (skills)", filepath.Join(RegistryDir, "skills")},
		{"Registry (mcp)", filepath.Join(RegistryDir, "mcp")},
		{"Profiles", ProfilesDir},
		{"Data", DataDir},
	}

	for _, t := range tool.AllTools() {
		dirs = append(dirs, struct {
			name string
			path string
		}{t.Name + " skills", filepath.Join(home, t.SkillDir)})
	}

	for _, dir := range dirs {
		info, err := os.Stat(dir.path)
		if os.IsNotExist(err) {
			results = append(results, checkResult{Name: dir.name, Status: "warn", Message: fmt.Sprintf("directory not found: %s", dir.path)})
			continue
		}
		if err != nil {
			results = append(results, checkResult{Name: dir.name, Status: "fail", Message: fmt.Sprintf("error accessing: %v", err)})
			continue
		}
		if !info.IsDir() {
			results = append(results, checkResult{Name: dir.name, Status: "fail", Message: fmt.Sprintf("not a directory: %s", dir.path)})
			continue
		}

		testFile := filepath.Join(dir.path, ".write-test")
		if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
			results = append(results, checkResult{Name: dir.name, Status: "warn", Message: fmt.Sprintf("directory not writable: %v", err)})
		} else {
			os.Remove(testFile)
			results = append(results, checkResult{Name: dir.name, Status: "pass", Message: "exists and writable"})
		}
	}

	return results
}

func checkDatabase() []checkResult {
	var results []checkResult

	dbPath := filepath.Join(DataDir, "sm.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		results = append(results, checkResult{Name: "Database", Status: "warn", Message: fmt.Sprintf("not found at %s (will be created on first use)", dbPath)})
		return results
	}

	database, err := db.Open(dbPath)
	if err != nil {
		results = append(results, checkResult{Name: "Database", Status: "fail", Message: fmt.Sprintf("cannot open: %v", err)})
		return results
	}
	defer database.Close()

	// db.Open applies pragmas (WAL, busy_timeout, foreign_keys) and creates
	// the schema, so a clean open plus table presence check is a full health check.
	for _, table := range []string{"installations", "projects"} {
		if !database.HasTable(table) {
			results = append(results, checkResult{Name: "Database", Status: "warn", Message: fmt.Sprintf("table '%s' not found", table)})
		}
	}

	results = append(results, checkResult{Name: "Database", Status: "pass", Message: "healthy"})
	return results
}

func checkEnvironment() []checkResult {
	var results []checkResult

	envVars := []struct {
		name    string
		checkFn func() (string, string)
	}{
		{"HOME", func() (string, string) {
			if os.Getenv("HOME") == "" {
				return "fail", "HOME environment variable not set"
			}
			return "pass", "set"
		}},
		{"PATH", func() (string, string) {
			if os.Getenv("PATH") == "" {
				return "fail", "PATH environment variable not set"
			}
			return "pass", "set"
		}},
	}

	for _, envVar := range envVars {
		status, message := envVar.checkFn()
		results = append(results, checkResult{Name: envVar.name, Status: status, Message: message})
	}

	if runtime.GOOS == "darwin" {
		results = append(results, checkResult{Name: "OS", Status: "pass", Message: "macOS detected"})
	} else if runtime.GOOS == "linux" {
		results = append(results, checkResult{Name: "OS", Status: "pass", Message: "Linux detected"})
	}

	return results
}

func printDoctorResults(results []checkResult) error {
	fmt.Println("SkillsManager Environment Check")
	fmt.Println("===============================")
	fmt.Println()

	passCount, warnCount, failCount := 0, 0, 0

	for _, r := range results {
		var icon string
		switch r.Status {
		case "pass":
			icon = "✓"
			passCount++
		case "warn":
			icon = "⚠"
			warnCount++
		case "fail":
			icon = "✗"
			failCount++
		}
		fmt.Printf("%s %-20s %s\n", icon, r.Name+":", r.Message)
	}

	fmt.Println()
	fmt.Printf("Summary: %d passed, %d warnings, %d failed\n", passCount, warnCount, failCount)

	if failCount > 0 {
		return fmt.Errorf("environment check failed with %d error(s)", failCount)
	}
	return nil
}
