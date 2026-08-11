package cmd

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func setupBrowseTestState(t *testing.T) {
	t.Helper()
	oldAPIBase, oldHTMLBase, oldDataDir := skillsShAPIBase, skillsShBaseURL, DataDir
	oldTrending, oldHot := browseTrending, browseHot
	oldTopic, oldAgent, oldOfficial, oldRefresh, oldOwner := browseTopic, browseAgent, browseOfficial, browseRefresh, browseOwner
	DataDir = t.TempDir()
	browseTrending, browseHot = false, false
	browseTopic, browseAgent, browseOfficial, browseRefresh, browseOwner = "", "", false, false, ""
	t.Cleanup(func() {
		skillsShAPIBase, skillsShBaseURL, DataDir = oldAPIBase, oldHTMLBase, oldDataDir
		browseTrending, browseHot = oldTrending, oldHot
		browseTopic, browseAgent, browseOfficial, browseRefresh, browseOwner = oldTopic, oldAgent, oldOfficial, oldRefresh, oldOwner
	})
}

func TestBrowseAPIRoutesUseExpectedEndpoints(t *testing.T) {
	setupBrowseTestState(t)
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
		requests = append(requests, r.URL.RequestURI())
		fmt.Fprint(w, `[{"name":"demo","source":"acme/repo","description":"demo"}]`)
	}))
	defer server.Close()
	skillsShAPIBase = server.URL

	for _, call := range []func() ([]browseSkill, error){
		func() ([]browseSkill, error) { return searchSkills("go skill", "token") },
		func() ([]browseSkill, error) { return fetchLeaderboard("", "token") },
		func() ([]browseSkill, error) { return fetchLeaderboard("trending", "token") },
		func() ([]browseSkill, error) { return fetchLeaderboard("hot", "token") },
		func() ([]browseSkill, error) { return fetchByTopic("web tools", "token") },
		func() ([]browseSkill, error) { return fetchByAgent("claude code", "token") },
		func() ([]browseSkill, error) { return fetchOfficial("token") },
		func() ([]browseSkill, error) { return fetchByOwner("go", "acme org", "token") },
	} {
		skills, err := call()
		if err != nil || len(skills) != 1 || skills[0].Name != "demo" {
			t.Fatalf("API route = %#v, %v", skills, err)
		}
	}
	joined := strings.Join(requests, "\n")
	for _, want := range []string{
		"/skills/search?q=go+skill",
		"/skills",
		"/skills?sort=trending",
		"/skills?sort=hot",
		"/skills?topic=web+tools",
		"/skills?agent=claude+code",
		"/skills/curated",
		"/skills/search?q=go&owner=acme+org",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("requests %q do not contain %q", joined, want)
		}
	}
}

func TestBrowseHTMLRoutesAndAPIErrors(t *testing.T) {
	setupBrowseTestState(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/rejected") {
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"token rejected"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/failure") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "unavailable")
			return
		}
		fmt.Fprint(w, `<a href="/acme/repo/demo">Demo</a>`)
	}))
	defer server.Close()
	skillsShBaseURL, skillsShAPIBase = server.URL, server.URL

	for _, call := range []func() ([]browseSkill, error){
		func() ([]browseSkill, error) { return searchSkills("demo", "") },
		func() ([]browseSkill, error) { return fetchLeaderboard("", "") },
		func() ([]browseSkill, error) { return fetchLeaderboard("trending", "") },
		func() ([]browseSkill, error) { return fetchLeaderboard("hot", "") },
		func() ([]browseSkill, error) { return fetchByTopic("web", "") },
		func() ([]browseSkill, error) { return fetchByAgent("codex", "") },
		func() ([]browseSkill, error) { return fetchOfficial("") },
	} {
		skills, err := call()
		if err != nil || len(skills) != 1 || skills[0].Source != "acme/repo" {
			t.Fatalf("HTML route = %#v, %v", skills, err)
		}
	}
	if _, err := fetchByOwner("demo", "acme", ""); err == nil {
		t.Fatal("owner search without a token should fail")
	}
	if _, err := fetchAPIBodyRemote("/rejected", "token"); err == nil || !strings.Contains(err.Error(), "token rejected") {
		t.Fatalf("rejected API error = %v", err)
	}
	if _, err := fetchAPIBodyRemote("/failure", "token"); err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("generic API error = %v", err)
	}
}

func TestRunBrowseRendersNonInteractiveResults(t *testing.T) {
	setupBrowseTestState(t)
	t.Setenv("SKILLS_SH_TOKEN", "token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"demo","source":"acme/repo","installs":1200,"description":"example"}]`)
	}))
	defer server.Close()
	skillsShAPIBase = server.URL

	output := captureBrowseStdout(t, func() error { return runBrowse("demo") })
	if !strings.Contains(output, "demo") || !strings.Contains(output, "1.2K") {
		t.Fatalf("browse output = %q", output)
	}
	browseOwner = "acme"
	if err := runBrowse(""); err == nil || !strings.Contains(err.Error(), "--owner requires") {
		t.Fatalf("owner without query error = %v", err)
	}
}

func TestFetchSkillsAPIHandlesPaginatedAndInvalidResponses(t *testing.T) {
	setupBrowseTestState(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/paginated":
			fmt.Fprint(w, `{"skills":[{"name":"paged","source":"acme/repo"}]}`)
		default:
			fmt.Fprint(w, "not json")
		}
	}))
	defer server.Close()
	skillsShAPIBase = server.URL
	if skills, err := fetchSkillsAPI("/paginated", "token"); err != nil || len(skills) != 1 || skills[0].Name != "paged" {
		t.Fatalf("paginated response = %#v, %v", skills, err)
	}
	if _, err := fetchSkillsAPI("/invalid", "token"); err == nil || !strings.Contains(err.Error(), "parsing API response") {
		t.Fatalf("invalid response error = %v", err)
	}
}

func TestRunBrowseRoutesEveryNonSearchFilter(t *testing.T) {
	setupBrowseTestState(t)
	t.Setenv("SKILLS_SH_TOKEN", "token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"name":"demo","source":"acme/repo"}]`)
	}))
	defer server.Close()
	skillsShAPIBase = server.URL

	routes := []struct {
		name  string
		setup func()
	}{
		{name: "default", setup: func() {}},
		{name: "trending", setup: func() { browseTrending = true }},
		{name: "hot", setup: func() { browseHot = true }},
		{name: "topic", setup: func() { browseTopic = "web" }},
		{name: "agent", setup: func() { browseAgent = "codex" }},
		{name: "official", setup: func() { browseOfficial = true }},
	}
	for _, route := range routes {
		t.Run(route.name, func(t *testing.T) {
			browseTrending, browseHot = false, false
			browseTopic, browseAgent, browseOfficial = "", "", false
			route.setup()
			output := captureBrowseStdout(t, func() error { return runBrowse("") })
			if !strings.Contains(output, "demo") {
				t.Fatalf("route output = %q", output)
			}
		})
	}
}

func captureBrowseStdout(t *testing.T, call func() error) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = writer
	err = call()
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	os.Stdout = old
	if err != nil {
		t.Fatal(err)
	}
	data, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data)
}
