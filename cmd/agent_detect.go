// cmd/agent_detect.go 提供 cmd 层唯一的 Agent 检测入口。默认实现保持产品契约
// （按宿主 PATH 检测已安装 Agent）；测试通过替换该变量隔离宿主环境，不改变
// 任何调用点的 fallback 行为。
//
// Input: github.com/woyin/skills-manager/internal/tool
// Output: var detectAgents
// Pos: 控制层-Agent 检测接缝
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import "github.com/woyin/skills-manager/internal/tool"

// detectAgents 返回本机已安装的 Agent。生产默认与 tool.DetectInstalled 等价。
var detectAgents = func() []tool.Tool {
	return tool.DetectInstalled(tool.AllTools())
}
