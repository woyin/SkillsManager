// cmd/browse_fetch.go 实现 `sm browse` 的数据层：从 skills.sh 抓取技能列表。
//
// 抓取分两条路径，由是否提供 token 决定：
//   - 有 token：走官方 JSON API（/api/v1），数据准确、含 installs 等。
//   - 无 token：回退到抓取 HTML 页面并用正则解析（免登录可用，但字段少）。
//
// 所有 API 响应带 10 分钟磁盘缓存（fetchAPIBody），并在远程失败时回退到
// 过期缓存，保证离线/限流场景下 browse 仍可用。
//
// Input: crypto/sha256, encoding/hex, encoding/json, fmt, io, net/http, net/url, os, path/filepath, regexp, time
// Output: type browseSkill, func searchSkills, func fetchLeaderboard, func fetchByTopic, func fetchByAgent, func fetchOfficial, func scrapeSkillsPage
// Pos: 控制层-browse命令数据层（skills.sh API 抓取/HTML 抓取/缓存）
//
// 本注释在文件修改时自动更新，同时触发 FOLDER_INDEX 和 PROJECT_INDEX 更新

package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const skillsShBaseURL = "https://skills.sh"

var skillsShAPIBase = "https://skills.sh/api/v1"

// sharedHTTPClient 复用连接、统一 15s 超时，避免每个请求新建 client。
var sharedHTTPClient = &http.Client{Timeout: 15 * time.Second}

var (
	browseTrending bool
	browseHot      bool
	browseTopic    string
	browseAgent    string
	browseOfficial bool
	browseRefresh  bool
)

// browseSkill 是 skills.sh 上一个技能条目的中性表示，
// 同时作为 API JSON 与 HTML 解析结果的统一输出类型。
type browseSkill struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
	Installs    int64  `json:"installs,omitempty"`
	URL         string `json:"url,omitempty"`
}

// searchSkills 按关键词搜索技能：有 token 走 API，否则 HTML scrape 搜索页。
func searchSkills(query, token string) ([]browseSkill, error) {
	if token != "" {
		return searchSkillsAPI(query, token)
	}
	return scrapeSkillsPage("/search?q=" + url.QueryEscape(query))
}

// fetchLeaderboard 取榜单（trending/hot/默认）：有 token 走 API，否则
// scrape 对应的 HTML 页面。
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

// fetchByTopic 按主题筛选：有 token 走 API（/skills?topic=），否则
// scrape /topic/<topic> 页面。
func fetchByTopic(topic, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills?topic="+url.QueryEscape(topic), token)
	}
	return scrapeSkillsPage("/topic/" + url.PathEscape(topic))
}

// fetchByAgent 按适配的 agent 筛选：有 token 走 API（/skills?agent=），
// 否则 scrape /agent/<agent> 页面。
func fetchByAgent(agent, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills?agent="+url.QueryEscape(agent), token)
	}
	return scrapeSkillsPage("/agent/" + url.PathEscape(agent))
}

// fetchOfficial 取官方精选（curated）技能：有 token 走 API，否则
// scrape /official 页面。
func fetchOfficial(token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills/curated", token)
	}
	return scrapeSkillsPage("/official")
}

func searchSkillsAPI(query, token string) ([]browseSkill, error) {
	return fetchSkillsAPI("/skills/search?q="+url.QueryEscape(query), token)
}

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

// browseCacheTTL 是 API 响应的磁盘缓存有效期：平衡新鲜度与对 skills.sh 的请求频率。
const browseCacheTTL = 10 * time.Minute

// fetchAPIBody 取一个 API endpoint 的响应体，带三级容错：
//  1. TTL 内的缓存命中 → 直接返回（除非 refresh 强制刷新）；
//  2. 远程拉取成功 → 写入缓存并返回；
//  3. 远程失败（离线/限流/5xx）→ 回退到过期缓存，保证 browse 在网络异常时仍可用。
//
// 第 3 级是刻意为之的容错：与其让 browse 整体失败，宁可返回稍旧的数据。
func fetchAPIBody(endpoint, token string, refresh bool) ([]byte, error) {
	key := cacheKey(endpoint)
	cachePath := filepath.Join(DataDir, "cache", "browse", key)

	if !refresh {
		if body, ok := readCache(cachePath, browseCacheTTL); ok {
			return body, nil
		}
	}

	body, err := fetchAPIBodyRemote(endpoint, token)
	if err == nil {
		writeCache(cachePath, body)
		return body, nil
	}
	if cached, ok := readCacheRaw(cachePath); ok {
		return cached, nil
	}
	return nil, err
}

// fetchAPIBodyRemote 向 skills.sh API 发起一次带 Bearer token 的请求，
// 非 200 时尝试解析 JSON 错误体并返回带状态码的错误。
func fetchAPIBodyRemote(endpoint, token string) ([]byte, error) {
	req, err := http.NewRequest("GET", skillsShAPIBase+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	resp, err := sharedHTTPClient.Do(req)
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
	return body, nil
}

// cacheKey 把 endpoint 映射为稳定的缓存文件名（endpoint 的 sha256 十六进制），
// 避免特殊字符出现在文件名里。
func cacheKey(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

// readCache 读取缓存并在文件存在且未超过 ttl 时返回其内容。
func readCache(path string, ttl time.Duration) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if time.Since(info.ModTime()) > ttl {
		return nil, false
	}
	return readCacheRaw(path)
}

// readCacheRaw 读取缓存文件而不校验时效（供远程失败时的过期回退使用）。
func readCacheRaw(path string) ([]byte, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return body, true
}

// writeCache 原子性较差地写入缓存（先建目录再 WriteFile）。
// 写失败被静默忽略：缓存只是优化，写不进去不影响功能正确性。
func writeCache(path string, body []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0644)
}

// fetchSkillsAPI 取一个 skills 列表 endpoint 并解析为 []browseSkill。
// API 的列表端点返回两种 JSON 形状——裸数组 [skill, ...] 或分页对象
// {skills: [...]}——这里先按裸数组尝试，失败再回退到分页形状。
func fetchSkillsAPI(endpoint, token string) ([]browseSkill, error) {
	body, err := fetchAPIBody(endpoint, token, browseRefresh)
	if err != nil {
		return nil, err
	}

	var rawSkills []browseSkill
	if err := json.Unmarshal(body, &rawSkills); err != nil {
		var paginated struct {
			Skills []browseSkill `json:"skills"`
		}
		if err2 := json.Unmarshal(body, &paginated); err2 != nil {
			return nil, fmt.Errorf("parsing API response: %w", err)
		}
		rawSkills = paginated.Skills
	}
	return rawSkills, nil
}

// scrapeSkillsPage 抓取 skills.sh 的一个 HTML 页面并解析出其中的技能链接。
// 这是无 token 时的回退路径：免登录可用，但拿不到 installs 等动态字段。
func scrapeSkillsPage(path string) ([]browseSkill, error) {
	req, err := http.NewRequest("GET", skillsShBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sm-cli/1.0")
	req.Header.Set("Accept", "text/html")

	resp, err := sharedHTTPClient.Do(req)
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

// nonSkillPathPrefixes 是技能链接 owner 段的黑名单：skills.sh 页面里也会
// 出现 /docs、/topic、/api、/_next 等非技能路径，它们同样匹配三段链接正则，
// 必须按 owner 前缀排除，否则会被误当成技能收录。
var nonSkillPathPrefixes = map[string]bool{
	"api": true, "docs": true, "topic": true, "agent": true,
	"_next": true, "search": true, "site": true, "about": true,
	"contact": true, "privacy": true, "terms": true, "official": true,
	"trending": true, "hot": true, "audits": true,
}

// skillLinkRe 匹配 skills.sh 技能链接 /<owner>/<repo>/<skill>。
// 三段均为 [a-zA-Z0-9_.-]，覆盖 GitHub owner/repo 与技能名的常见字符集。
var skillLinkRe = regexp.MustCompile(
	`href="/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)"`,
)

// parseSkillsFromHTML 从 skills.sh 页面 HTML 中抽取技能链接。
// 用正则（而非 HTML parser）是因为链接结构高度规整且我们只需 href；
// 配合 nonSkillPathPrefixes 黑名单过滤掉非技能路径，按 owner/repo/skill 去重。
func parseSkillsFromHTML(html string) ([]browseSkill, error) {
	matches := skillLinkRe.FindAllStringSubmatch(html, -1)

	seen := make(map[string]bool)
	var skills []browseSkill

	for _, m := range matches {
		owner := m[1]
		repo := m[2]
		skillName := m[3]

		if nonSkillPathPrefixes[owner] {
			continue
		}

		key := owner + "/" + repo + "/" + skillName
		if seen[key] {
			continue
		}
		seen[key] = true

		skills = append(skills, browseSkill{
			Name:   skillName,
			Source: owner + "/" + repo,
			URL:    skillsShBaseURL + "/" + key,
		})
	}

	return skills, nil
}
