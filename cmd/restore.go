// cmd/restore.go 实现 `sm restore`：从备份还原（还原前自动创建 pre-restore 备份）。
//
// Input: fmt, github.com/spf13/cobra, github.com/woyin/skills-manager/internal/backup
// Output: var restoreCmd, var restoreLatest
// Pos: 控制层-restore命令实现
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/backup"
)

var restoreLatest bool

var restoreCmd = &cobra.Command{
	Use:   "restore [name]",
	Short: "Restore from a backup",
	Long: `Restore SkillsManager configuration from a backup.
Creates an automatic pre-restore backup before restoring.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m := backup.New(DataDir, RegistryDir, ProfilesDir)

		var info *backup.BackupInfo
		var err error

		if restoreLatest {
			info, err = m.FindLatest()
			if err != nil {
				return fmt.Errorf("finding latest backup: %w", err)
			}
		} else if len(args) > 0 {
			info, err = m.FindByName(args[0])
			if err != nil {
				return fmt.Errorf("finding backup: %w", err)
			}
		} else {
			return fmt.Errorf("specify backup name or use --latest")
		}

		fmt.Printf("Restoring from: %s\n", info.Name)

		if err := m.Restore(info.Path); err != nil {
			return fmt.Errorf("restore failed: %w", err)
		}

		fmt.Println("✓ Configuration restored successfully")
		fmt.Println("  (A pre-restore backup was created automatically)")

		return nil
	},
}

func init() {
	restoreCmd.Flags().BoolVar(&restoreLatest, "latest", false, "Restore from the most recent backup")

	rootCmd.AddCommand(restoreCmd)
}
