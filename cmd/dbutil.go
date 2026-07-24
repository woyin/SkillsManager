//
// Input: path/filepath, github.com/woyin/skills-manager/internal/db
// Output: func openDB, func dbPath
// Pos: 工具层-数据库连接工具（打开 sm.db/获取数据库路径）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"path/filepath"

	"github.com/woyin/skills-manager/internal/db"
)

func openDB() (*db.DB, error) {
	return db.Open(filepath.Join(DataDir, "sm.db"))
}

func dbPath() string {
	return filepath.Join(DataDir, "sm.db")
}
