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
Selected skills can be installed directly with 'sm add'.

Set SKILLS_SH_TOKEN or VERCEL_OIDC_TOKEN environment variable for API access.
Without a token, skill data is scraped from the public website.

Examples:
  # Browse all skills (interactive picker)
  sm browse

  # Search for a specific skill
  sm browse typescript

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
	var skills []browseSkill
	var err error

	token := getSkillsToken()

	if query != "" {
		skills, err = searchSkills(query, token)
	} else if browseTrending {
		skills, err = fetchLeaderboard("trending", token)
	} else if browseHot {
		skills, err = fetchLeaderboard("hot", token)
	} else if browseTopic != "" {
		skills, err = fetchByTopic(browseTopic, token)
	} else if browseAgent != "" {
		skills, err = fetchByAgent(browseAgent, token)
	} else if browseOfficial {
		skills, err = fetchOfficial(token)
	} else {
		skills, err = fetchLeaderboard("", token)
	}

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
	rootCmd.AddCommand(browseCmd)
}
