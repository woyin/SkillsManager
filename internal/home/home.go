// Package home 集中管理用户主目录的解析，避免全项目散落的
// os.UserHomeDir() 调用各自静默忽略错误。
//
// 用法：
//   - 包加载时尽力解析一次，供 flag 默认值等 init 期代码使用；
//   - 在程序入口（cmd.Execute）调用 home.Init() 做严格校验；
//   - 之后所有包通过 home.Dir() 读取已验证的主目录路径。
//
// Input: fmt, os, sync
// Output: func Init, func Dir, func ResetForTest
// Pos: 工具层-用户home目录
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package home

import (
	"fmt"
	"os"
	"sync"
)

var (
	mu  sync.RWMutex
	dir string
)

func init() {
	// 尽力解析：flag 默认值在 cobra init 阶段就会读 Dir()，
	// 此时还不能要求 Init() 已成功。失败时 dir 为空串。
	dir, _ = os.UserHomeDir()
}

// Init 重新解析并校验主目录。成功后 Dir() 返回可靠路径；
// 失败时返回错误，调用方应中止启动。
//
// 注意：Init 不使用 sync.Once——包级 init 已做过一次尽力解析，
// 若共享 Once，Init 永远无法真正执行校验。
func Init() error {
	d, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	if d == "" {
		return fmt.Errorf("cannot determine home directory")
	}
	mu.Lock()
	dir = d
	mu.Unlock()
	return nil
}

// Dir 返回当前缓存的用户主目录。在 Init 成功前可能是空串
// （仅当 os.UserHomeDir 在包加载时已失败）。
func Dir() string {
	mu.RLock()
	defer mu.RUnlock()
	return dir
}

// ResetForTest 重新从环境解析主目录，供测试在改 HOME 后刷新缓存。
// 生产代码不应调用。
func ResetForTest() {
	d, _ := os.UserHomeDir()
	mu.Lock()
	dir = d
	mu.Unlock()
}
