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

var sharedHTTPClient = &http.Client{Timeout: 15 * time.Second}

var (
	browseTrending bool
	browseHot      bool
	browseTopic    string
	browseAgent    string
	browseOfficial bool
	browseRefresh  bool
)

type browseSkill struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description,omitempty"`
	Installs    int64  `json:"installs,omitempty"`
	URL         string `json:"url,omitempty"`
}

func searchSkills(query, token string) ([]browseSkill, error) {
	if token != "" {
		return searchSkillsAPI(query, token)
	}
	return scrapeSkillsPage("/search?q=" + url.QueryEscape(query))
}

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

func fetchByTopic(topic, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills?topic="+url.QueryEscape(topic), token)
	}
	return scrapeSkillsPage("/topic/" + url.PathEscape(topic))
}

func fetchByAgent(agent, token string) ([]browseSkill, error) {
	if token != "" {
		return fetchSkillsAPI("/skills?agent="+url.QueryEscape(agent), token)
	}
	return scrapeSkillsPage("/agent/" + url.PathEscape(agent))
}

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

const browseCacheTTL = 10 * time.Minute

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

func cacheKey(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

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

func readCacheRaw(path string) ([]byte, bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	return body, true
}

func writeCache(path string, body []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	_ = os.WriteFile(path, body, 0644)
}

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

var nonSkillPathPrefixes = map[string]bool{
	"api": true, "docs": true, "topic": true, "agent": true,
	"_next": true, "search": true, "site": true, "about": true,
	"contact": true, "privacy": true, "terms": true, "official": true,
	"trending": true, "hot": true, "audits": true,
}

var skillLinkRe = regexp.MustCompile(
	`href="/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)/([a-zA-Z0-9_.-]+)"`,
)

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
