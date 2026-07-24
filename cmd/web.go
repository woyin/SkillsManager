// cmd/web.go 实现 `sm web`：启动内嵌的 HTTP 仪表盘服务。
//
// Input: fmt, net/http, os, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/web
// Output: var webCmd, var webPort
// Pos: 控制层-web仪表盘命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"
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

		database, err := openDB()
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
