// cmd/browse.go 实现 `sm browse`：浏览/搜索在线 skills.sh 目录。
// 带 token 走 API；否则抓取公开网页。
// cmd/browse.go
package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/woyin/skills-manager/internal/picker"
	"golang.org/x/term"
)

const skillsShBaseURL = "https://skills.sh"
const skillsShAPIBase = "https://skills.sh/api/v1"

var (
	browseTrending bool
	browseHot      bool
	browseTopic    string
	browseAgent    string
	browseOfficial bool
)

// 与 skills.sh API 返回 JSON 形状对应的原始结构。
// rawSkill mirrors the JSON shape returned by the skills.sh API.
type rawSkill struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
	Installs    int64  `json:"installs"`
	URL         string `json:"url"`
}

// 来自 skills.sh 目录的一个技能。
// browseSkill represents a skill from the skills.sh directory.
type browseSkill struct {
	Name        string `json:"name"`
	Source      string `json:"source"` // owner/repo
	Description string `json:"description,omitempty"`
	Installs    int64  `json:"installs,omitempty"`
	URL         string `json:"url,omitempty"`
}

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

	// 交互式选择器模式
	if term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		return browsePicker(skills)
	}

	// 表格模式
	return browseTable(skills)
}
// 读取 SKILLS_SH_TOKEN，回退到 VERCEL_OIDC_TOKEN。

func getSkillsToken() string {
	if token := os.Getenv("SKILLS_SH_TOKEN"); token != "" {
		return token
	}
	return os.Getenv("VERCEL_OIDC_TOKEN")
}
// 按关键词搜索；优先 API（带 token），否则抓取网页。

func searchSkills(query, token string) ([]browseSkill, error) {
	if token != "" {
		return searchSkillsAPI(query, token)
	}
	return scrapeSkillsPage("/search?q=" + url.QueryEscape(query))
}
// 获取排行榜（trending/hot/默认）；带 token 走 API，否则抓取网页。

func fetchLeaderboard(mode, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchLeaderboardAPI(mode, token)
	}
	path := "/"
	switch mode {
	case "trending":
		path = "/trending"
	case "hot":
		path = "/hot"
	}
	return scrapeSkillsPage(path)
}
// 按主题获取技能。

func fetchByTopic(topic, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills?topic="+url.QueryEscape(topic), token)
	}
	return scrapeSkillsPage("/topic/" + url.PathEscape(topic))
}
// 按代理获取技能。

func fetchByAgent(agent, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills?agent="+url.QueryEscape(agent), token)
	}
	return scrapeSkillsPage("/agent/" + url.PathEscape(agent))
}
// 获取官方/精选技能。

func fetchOfficial(token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills/curated", token)
	}
	return scrapeSkillsPage("/official")
}

// 通过 API 按关键词搜索。
// ── API 访问（带鉴权 token）──

func searchSkillsAPI(query, token string) ([]browseSkill, error) {
	endpoint := "/skills/search?q=" + url.QueryEscape(query)
	return fetchSkillsAPI(endpoint, token)
}
// 通过 API 获取排行榜。

func fetchLeaderboardAPI(mode string, token string) ([]browseSkill, error) {
	endpoint := "/skills"
	switch mode {
	case "trending":
		endpoint = "/skills?sort=trending"
	case "hot":
		endpoint = "/skills?sort=hot"
	}
	return fetchSkillsAPI(endpoint, token)
}
// 调用 skills.sh API 的通用获取函数（带 Bearer token）。

func fetchSkillsAPI(endpoint, token string) ([]browseSkill, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", skillsShAPIBase+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
			return nil, fmt.Errorf("API error (%d): %s", resp.StatusCode, apiErr.Message)
		}
		return nil, fmt.Errorf("API error: HTTP %d", resp.StatusCode)
	}

	// API 可能返回裸 JSON 数组，也可能返回分页信封
	// {"skills": [...]}。先尝试数组形式。
	var apiSkills []rawSkill
	if err := json.Unmarshal(body, &apiSkills); err != nil {
		var paginated struct {
			Skills []rawSkill `json:"skills"`
		}
		if err2 := json.Unmarshal(body, &paginated); err2 != nil {
			return nil, fmt.Errorf("parsing API response: %w", err)
		}
		apiSkills = paginated.Skills
	}

	skills := make([]browseSkill, 0, len(apiSkills))
	for _, s := range apiSkills {
		skills = append(skills, browseSkill{
			Name:        s.Name,
			Source:      s.Source,
			Description: s.Description,
			Installs:    s.Installs,
			URL:         s.URL,
		})
	}
	return skills, nil
}

// 无 token 时抓取 skills.sh 页面并解析其中的技能链接。
// ── 抓取兜底（无 token）──

func scrapeSkillsPage(path string) ([]browseSkill, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", skillsShBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sm-cli/1.0")
	req.Header.Set("Accept", "text/html")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	return parseSkillsFromHTML(string(body))
}

// 从 skills.sh HTML / RSC 负载中抽取技能条目。
// 匹配形如 /{owner}/{repo}/{skill} 的链接，并跳过非技能路径（api/docs/topic 等）。
// parseSkillsFromHTML extracts skill entries from skills.sh HTML/RSC payload.
// Looks for links matching /{owner}/{repo}/{skill} pattern.
func parseSkillsFromHTML(html string) ([]browseSkill, error) {
	// 模式：排行榜中的 href="/owner/repo/skill-name"
	// 同时匹配 RSC 数据中的技能引用
	re := regexp.MustCompile(`href="/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)"`)
	matches := re.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var skills []browseSkill

	for _, m := range matches {
		owner := m[1]
		repo := m[2]
		skillName := m[3]

		// 跳过非技能路径
		if owner == "api" || owner == "docs" || owner == "topic" ||
			owner == "agent" || owner == "_next" || owner == "search" ||
			owner == "site" || owner == "about" || owner == "contact" ||
			owner == "privacy" || owner == "terms" || owner == "official" ||
			owner == "trending" || owner == "hot" || owner == "audits" {
			continue
		}

		key := owner + "/" + repo + "/" + skillName
		if seen[key] {
			continue
		}
		seen[key] = true

		source := owner + "/" + repo
		skills = append(skills, browseSkill{
			Name:   skillName,
			Source: source,
			URL:    skillsShBaseURL + "/" + owner + "/" + repo + "/" + skillName,
		})
	}

	return skills, nil
}

// 交互式选择器：选中后可立即安装。
// ── 展示 ──

func browsePicker(skills []browseSkill) error {
	items := make([]picker.Item, len(skills))
	for i, s := range skills {
		detail := s.Source
		if s.Description != "" {
			detail = s.Description
		}
		if s.Installs > 0 {
			detail = fmt.Sprintf("%s (%s installs)", detail, formatInstalls(s.Installs))
		}
		items[i] = picker.Item{
			Label:  s.Name,
			Detail: detail,
			Value:  fmt.Sprintf("%d", i),
		}
	}

	title := "Browse skills.sh"
	if browseTrending {
		title = "Trending on skills.sh"
	} else if browseHot {
		title = "Hot on skills.sh"
	}

	selected, err := picker.Pick(title+" (enter to install, esc to quit)", items)
	if err != nil {
		return nil // cancelled
	}

	var idx int
	fmt.Sscanf(selected, "%d", &idx)
	if idx >= 0 && idx < len(skills) {
		skill := skills[idx]
		fmt.Printf("\nSelected: %s (%s)\n", skill.Name, skill.Source)
		if skill.Description != "" {
			fmt.Printf("Description: %s\n", skill.Description)
		}
		if skill.URL != "" {
			fmt.Printf("URL: %s\n", skill.URL)
		}
		fmt.Printf("\nInstall with: sm add %s --skill %s\n", skill.Source, skill.Name)

		// 询问是否立即安装
		if term.IsTerminal(int(os.Stdin.Fd())) {
			fmt.Print("\nInstall now? [y/N] ")
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(answer) == "y" || strings.ToLower(answer) == "yes" {
				return runAddFromBrowse(skill.Source, skill.Name)
			}
		}
	}
	return nil
}
// 以表格形式列出技能。

func browseTable(skills []browseSkill) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tSKILL\tSOURCE\tINSTALLS\tDESCRIPTION")
	fmt.Fprintln(w, "--\t-----\t------\t--------\t-----------")
	for i, s := range skills {
		desc := s.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		installs := ""
		if s.Installs > 0 {
			installs = formatInstalls(s.Installs)
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", i+1, s.Name, s.Source, installs, desc)
	}
	w.Flush()
	fmt.Printf("\n%d skill(s) found on skills.sh\n", len(skills))
	fmt.Println("Install with: sm install <source> --skill <name> --agent <agent>")
	return nil
}
// 把安装数格式化为易读的 K/M 形式。

func formatInstalls(n int64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

// 从 browse 选择器触发的安装：复用 install 命令逻辑。
// runAddFromBrowse triggers an install from the browse picker.
func runAddFromBrowse(source, skillName string) error {
	// 复用 install 命令逻辑
	installCmd.SetArgs([]string{source, "--skill", skillName, "--yes"})
	return installCmd.Execute()
}

func init() {
	browseCmd.Flags().BoolVar(&browseTrending, "trending", false, "Browse trending skills")
	browseCmd.Flags().BoolVar(&browseHot, "hot", false, "Browse hot skills")
	browseCmd.Flags().StringVar(&browseTopic, "topic", "", "Browse skills by topic")
	browseCmd.Flags().StringVar(&browseAgent, "agent", "", "Browse skills for a specific agent")
	browseCmd.Flags().BoolVar(&browseOfficial, "official", false, "Browse official/curated skills")
	rootCmd.AddCommand(browseCmd)
}
