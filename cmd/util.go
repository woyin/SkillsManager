// cmd/util.go 存放命令层共享的小工具函数，供多个 cobra 子命令复用。
//
// Input: none
// Output: func truncate
// Pos: 工具层-命令层共享工具
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

// truncate 把 s 截断到最多 n 个字符（按 rune 计，unicode 安全），超出时
// 末尾以 "..." 表示。n 小于省略号长度时原样返回 s。
//
// 各命令在表格里展示技能描述时应统一走本函数，避免出现阈值不一致
// （此前 find.go 用 50、其余用 60）与按字节切坏中文的问题。
func truncate(s string, n int) string {
	if n <= 3 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}
