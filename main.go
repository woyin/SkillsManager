// main.go 是 sm（SkillsManager）CLI 的程序入口。
//
// 职责极简：把控制权交给 cmd.Execute()（见 cmd/root.go），
// 后者负责解析命令行参数并派发到具体子命令。出错时以非零状态码退出。
//
// Input: os, github.com/woyin/skills-manager/cmd
// Output: func main
// Pos: 控制层-程序入口
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新
package main

import (
	"os"

	"github.com/woyin/skills-manager/cmd"
)

// main 是程序入口。任何由子命令返回的错误都会被 cmd.Execute 打印到
// stderr；此处仅在出错时把进程退出码置为 1，以便脚本/CI 据此判定成败。
func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
