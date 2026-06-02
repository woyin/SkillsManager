// cmd/web.go
package cmd

import (
	"fmt"
	"net/http"
	"os"
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
		reg := registry.New(RegistryDir)

		dbPath := filepath.Join(DataDir, "sm.db")
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