package wellknown

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchAllV1(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-skills/index.json":
			fmt.Fprint(w, `{"skills":[{"name":"demo","description":"Demo skill","files":["SKILL.md","assets/example.txt"]}]}`)
		case "/.well-known/agent-skills/demo/SKILL.md":
			fmt.Fprint(w, "---\nname: demo\ndescription: Demo skill\n---\n# Demo\n")
		case "/.well-known/agent-skills/demo/assets/example.txt":
			fmt.Fprint(w, "example")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	skills, err := FetchAll(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("got %d skills, want 1", len(skills))
	}
	got := skills[0]
	if got.Name != "demo" || got.InstallName != "demo" || string(got.Files["assets/example.txt"]) != "example" {
		t.Fatalf("unexpected v1 skill: %+v", got)
	}
}

func TestFetchAllV2SkillMDVerifiesDigest(t *testing.T) {
	content := []byte("---\nname: demo\ndescription: Demo skill\n---\n# Demo\n")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-skills/index.json":
			fmt.Fprintf(w, `{"$schema":%q,"skills":[{"name":"demo","description":"Demo skill","type":"skill-md","url":"/artifacts/demo.md","digest":"sha256:%s"}]}`,
				DiscoverySchemaV2, hex.EncodeToString(digest[:]))
		case "/artifacts/demo.md":
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	skills, err := FetchAll(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(skills) != 1 || string(skills[0].Files["SKILL.md"]) != string(content) {
		t.Fatalf("unexpected v2 skills: %+v", skills)
	}
}

func TestFetchAllV2Archive(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	file, err := writer.Create("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("---\nname: Archive Demo\ndescription: Archive skill\n---\n# Demo\n"))
	asset, err := writer.Create("assets/example.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = asset.Write([]byte("example"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	content := archive.Bytes()
	digest := sha256.Sum256(content)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/agent-skills/index.json":
			fmt.Fprintf(w, `{"$schema":%q,"skills":[{"name":"archive-demo","description":"Archive skill","type":"archive","url":"/artifacts/demo.zip","digest":"sha256:%s"}]}`,
				DiscoverySchemaV2, hex.EncodeToString(digest[:]))
		case "/artifacts/demo.zip":
			_, _ = w.Write(content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	skills, err := FetchAll(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(skills) != 1 || string(skills[0].Files["assets/example.txt"]) != "example" {
		t.Fatalf("unexpected archive skills: %+v", skills)
	}
}

func TestFetchAllRejectsUnsafeV1Paths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/agent-skills/index.json" {
			fmt.Fprint(w, `{"skills":[{"name":"demo","description":"Demo skill","files":["SKILL.md","../escape"]}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	skills, err := FetchAll(context.Background(), server.URL)
	if err == nil || len(skills) != 0 {
		t.Fatalf("FetchAll returned (%+v, %v), want no skills and an error", skills, err)
	}
}

func TestSelectorFromDirectSkillURL(t *testing.T) {
	if got := Selector("https://example.test/product/.well-known/agent-skills/demo"); got != "demo" {
		t.Fatalf("Selector = %q, want demo", got)
	}
	if got := Selector("https://example.test/.well-known/skills/index.json"); got != "" {
		t.Fatalf("Selector from index = %q, want empty", got)
	}
}
