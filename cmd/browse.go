// cmd/browse.go 实现 `sm browse`：浏览/搜索 skills.sh 在线技能目录。
//
// 有 SKILLS_SH_TOKEN/VERCEL_OIDC_TOKEN 时走官方 API，否则回退抓取公开网页
// （抓取逻辑在 browse_fetch.go）。结果在 browse_display.go 里以交互选择器
// 或表格呈现，选中项可直接转交 `sm add` 安装。
//
// Input: fmt, os, strings, github.com/spf13/cobra, golang.org/x/term
// Output: var browseCmd, func runBrowse, func getSkillsToken
// Pos: 控制层-browse命令实现（浏览/搜索 skills.sh 在线技能目录）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var browseCmd = &cobra.Command{
	Use:   "browse [query]",
	Short: "Browse and search the skills.sh directory",
	Long: `Browse and search the online skills.sh directory.

Search for skills from the public agent skills directory at skills.sh.
Selected skills can be registered with 'sm add' and deployed by name with 'sm install <name>'.

Set SKILLS_SH_TOKEN or VERCEL_OIDC_TOKEN environment variable for API access.
Without a token, skill data is scraped from the public website.

Examples:
  # Browse all skills (interactive picker)
  sm browse

  # Search for a specific skill
  sm browse typescript

  # Search within a GitHub owner's repos (requires SKILLS_SH_TOKEN)
  sm browse react --owner vercel-labs

  # Browse trending skills
  sm browse --trending

  # Browse hot skills
  sm browse --hot

  # Browse skills for a specific agent
  sm browse --agent claude-code

  # Browse skills by topic
  sm browse --topic react
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = strings.Join(args, " ")
		}
		return runBrowse(query)
	},
}

func runBrowse(query string) error {
	token := getSkillsToken()

	skills, err := fetchBrowseSkills(query, token)
	if err != nil {
		return fmt.Errorf("fetching skills: %w", err)
	}

	if len(skills) == 0 {
		if query != "" {
			fmt.Printf("No skills found matching %q on skills.sh\n", query)
		} else {
			fmt.Println("No skills found on skills.sh")
		}
		return nil
	}

	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return browsePicker(skills)
	}

	return browseTable(skills)
}

// fetchBrowseSkills 按 flags 决定抓取策略：搜索（可限 owner）、排行榜
// （trending/hot/默认）、topic、agent 或官方精选。
func fetchBrowseSkills(query, token string) ([]browseSkill, error) {
	switch {
	case query != "" && browseOwner != "":
		return fetchByOwner(query, browseOwner, token)
	case query != "":
		return searchSkills(query, token)
	case browseOwner != "":
		return nil, fmt.Errorf("--owner requires a search keyword (e.g. `sm browse react --owner vercel-labs`)")
	case browseTrending:
		return fetchLeaderboard("trending", token)
	case browseHot:
		return fetchLeaderboard("hot", token)
	case browseTopic != "":
		return fetchByTopic(browseTopic, token)
	case browseAgent != "":
		return fetchByAgent(browseAgent, token)
	case browseOfficial:
		return fetchOfficial(token)
	default:
		return fetchLeaderboard("", token)
	}
}

func getSkillsToken() string {
	if token := os.Getenv("SKILLS_SH_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("VERCEL_OIDC_TOKEN")
}

func init() {
	browseCmd.Flags().BoolVar(&browseTrending, "trending", false, "Browse trending skills")
	browseCmd.Flags().BoolVar(&browseHot, "hot", false, "Browse hot skills")
	browseCmd.Flags().StringVar(&browseTopic, "topic", "", "Browse skills by topic")
	browseCmd.Flags().StringVar(&browseAgent, "agent", "", "Browse skills for a specific agent")
	browseCmd.Flags().BoolVar(&browseOfficial, "official", false, "Browse official/curated skills")
	browseCmd.Flags().BoolVar(&browseRefresh, "refresh", false, "Bypass cache and fetch fresh data")
	browseCmd.Flags().StringVar(&browseOwner, "owner", "", "Search within a GitHub owner's repos (requires a keyword, e.g. `sm browse react --owner vercel-labs`)")
	rootCmd.AddCommand(browseCmd)
}
