// cmd/dbutil.go 是命令层共享的数据库连接工具：定位 sm.db 路径并打开
// （应用 WAL 等连接级 pragma，见 internal/db）。供 check/status/cache/import
// 等需要读写持久状态的命令复用。
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
