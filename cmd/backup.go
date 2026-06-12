// cmd/backup.go
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/backup"
)

var (
	backupName  string
	backupRotate int
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a configuration backup",
	Long: `Create a backup of your SkillsManager configuration.
Backups include the database, registry, and profiles.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		m := backup.New(DataDir, RegistryDir, ProfilesDir)

		path, err := m.Backup(backupName)
		if err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}

		fmt.Printf("✓ Backup created: %s\n", path)

		// Rotate if requested
		if backupRotate > 0 {
			if err := m.Rotate(backupRotate); err != nil {
				return fmt.Errorf("rotation failed: %w", err)
			}
			fmt.Printf("✓ Rotated to %d backups\n", backupRotate)
		}

		return nil
	},
}

func init() {
	backupCmd.Flags().StringVar(&backupName, "name", "", "Custom backup name (default: auto-generated)")
	backupCmd.Flags().IntVar(&backupRotate, "rotate", 0, "Keep only N most recent backups (0 = no rotation)")

	rootCmd.AddCommand(backupCmd)
}
