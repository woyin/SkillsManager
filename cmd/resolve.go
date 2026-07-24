// cmd/resolve.go 定义 specialFlags：把 --global/--codex/--claude 等
// 布尔标志解析为注册表特殊目录名。
//
// 单一来源：单工具标志（--codex/--claude/...）在运行时从 tool.SpecialFlagSpecs()
// 派生，新增 first-class 工具只需在 catalog 加 specialDir 字段，此处无需改动。
// --global 不属任何单工具，单独保留。
//
// Input: github.com/spf13/cobra, github.com/woyin/skills-manager/internal/registry, github.com/woyin/skills-manager/internal/tool
// Output: type specialFlags, func newSpecialFlags, func (specialFlags) Bind, func (specialFlags) Resolve
// Pos: 控制层-特殊目录标志解析
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/registry"
	"github.com/woyin/skills-manager/internal/tool"
)

// specialFlags 把 --global/--codex/--claude 等布尔标志解析为注册表特殊目录名。
// 被 add 与 rm 命令共享。
type specialFlags struct {
	global bool                   // --global：目标为全部工具，不属任何单工具
	vals   map[string]*bool       // key = SpecialFlagSpec.Flag（如 "codex"）
	specs  []tool.SpecialFlagSpec // 绑定时快照，保证 Resolve 顺序与 Bind 一致
}

// newSpecialFlags 从 tool catalog 派生单工具标志集合。
func newSpecialFlags() *specialFlags {
	specs := tool.SpecialFlagSpecs()
	vals := make(map[string]*bool, len(specs))
	for _, s := range specs {
		b := false
		vals[s.Flag] = &b
	}
	return &specialFlags{vals: vals, specs: specs}
}

// Bind 注册 --global 与全部 --<agent> 标志。
// verb 用于拼接帮助文案（如 "Add to"/"Remove from"）。
func (f *specialFlags) Bind(c *cobra.Command, verb string) {
	c.Flags().BoolVar(&f.global, "global", false, verb+" global (all tools)")
	for _, s := range f.specs {
		s := s
		c.Flags().BoolVar(f.vals[s.Flag], s.Flag, false, verb+" "+s.SpecialDir)
	}
}

// Resolve 返回第一个被置位的标志对应的特殊目录名；均未置位返回 ""。
// 保持 add/rm 历史的 first-match 行为：--global 优先，随后按 catalog 顺序。
func (f *specialFlags) Resolve() string {
	if f.global {
		return registry.Global
	}
	for _, s := range f.specs {
		if *f.vals[s.Flag] {
			return s.SpecialDir
		}
	}
	return ""
}
