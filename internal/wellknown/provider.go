// Package wellknown fetches Skills Discovery Protocol endpoints.
package wellknown

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
)

const (
	// DiscoverySchemaV2 is the Skills Discovery Protocol v0.2 schema URI.
	DiscoverySchemaV2 = "https://schemas.agentskills.io/discovery/0.2.0/schema.json"
	maxArchiveBytes   = 50 * 1024 * 1024
	maxArchiveFiles   = 1000
)

var errNoSkills = errors.New("no well-known skills found")

// Skill is a fully materialized Well-Known Source skill.
type Skill struct {
	Name        string
	Description string
	InstallName string
	SourceURL   string
	Files       map[string][]byte
}

type indexCandidate struct {
	baseURL       string
	wellKnownPath string
	indexURL      string
	index         discoveryIndex
}

type discoveryIndex struct {
	Schema string            `json:"$schema"`
	Skills []json.RawMessage `json:"skills"`
}

type v1Entry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Files       []string `json:"files"`
}

type v2Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Digest      string `json:"digest"`
}

// IsSource reports whether input is a non-git HTTP(S) source that npx skills
// resolves through the Well-Known Skills Discovery Protocol.
func IsSource(input string) bool {
	parsed, err := url.Parse(input)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || strings.HasSuffix(strings.ToLower(parsed.Path), ".git") {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "github.com", "gitlab.com", "raw.githubusercontent.com":
		return false
	}
	return true
}

// Selector returns the skill name embedded in a direct Well-Known Source
// skill URL, if present. An index URL has no embedded selector.
func Selector(input string) string {
	parsed, err := url.Parse(input)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for i := 0; i+2 < len(parts); i++ {
		if parts[i] == ".well-known" && (parts[i+1] == "agent-skills" || parts[i+1] == "skills") && parts[i+2] != "index.json" {
			return parts[i+2]
		}
	}
	return ""
}

// FetchAll returns the first Well-Known Source index candidate that yields one
// or more valid skills. It supports both discovery protocol v1 and v2.
func FetchAll(ctx context.Context, source string) ([]Skill, error) {
	candidates, err := fetchIndexCandidates(ctx, source)
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		var skills []Skill
		if candidate.index.Schema == DiscoverySchemaV2 {
			skills = fetchV2Skills(ctx, candidate)
		} else if candidate.index.Schema == "" {
			skills = fetchV1Skills(ctx, candidate)
		}
		if len(skills) > 0 {
			return skills, nil
		}
	}
	return nil, errNoSkills
}

func fetchIndexCandidates(ctx context.Context, source string) ([]indexCandidate, error) {
	parsed, err := url.Parse(source)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid Well-Known Source URL: %q", source)
	}
	basePath := strings.TrimSuffix(parsed.EscapedPath(), "/")
	bases := []string{parsed.Scheme + "://" + parsed.Host + basePath}
	if basePath != "" {
		bases = append(bases, parsed.Scheme+"://"+parsed.Host)
	}
	client := http.DefaultClient
	var candidates []indexCandidate
	for _, wellKnownPath := range []string{".well-known/agent-skills", ".well-known/skills"} {
		for _, base := range bases {
			indexURL := strings.TrimSuffix(base, "/") + "/" + wellKnownPath + "/index.json"
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
			if err != nil {
				continue
			}
			response, err := client.Do(req)
			if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
				if response != nil {
					response.Body.Close()
				}
				continue
			}
			var index discoveryIndex
			err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&index)
			response.Body.Close()
			if err != nil || index.Skills == nil {
				continue
			}
			candidates = append(candidates, indexCandidate{baseURL: base, wellKnownPath: wellKnownPath, indexURL: indexURL, index: index})
		}
	}
	return candidates, nil
}

func fetchV1Skills(ctx context.Context, candidate indexCandidate) []Skill {
	entries := make([]v1Entry, 0, len(candidate.index.Skills))
	for _, raw := range candidate.index.Skills {
		var entry v1Entry
		if json.Unmarshal(raw, &entry) != nil || !validV1Entry(entry) {
			return nil
		}
		entries = append(entries, entry)
	}
	var skills []Skill
	for _, entry := range entries {
		if skill := fetchV1Skill(ctx, candidate, entry); skill != nil {
			skills = append(skills, *skill)
		}
	}
	return skills
}

func fetchV1Skill(ctx context.Context, candidate indexCandidate, entry v1Entry) *Skill {
	skillBase := strings.TrimSuffix(candidate.baseURL, "/") + "/" + candidate.wellKnownPath + "/" + entry.Name
	files := make(map[string][]byte, len(entry.Files))
	for _, filePath := range entry.Files {
		content, ok := fetchBytes(ctx, skillBase+"/"+filePath)
		if !ok {
			if strings.EqualFold(filePath, "SKILL.md") {
				return nil
			}
			continue
		}
		files[filePath] = content
	}
	skillMD := files["SKILL.md"]
	name, description, ok := parseSkillMetadata(skillMD)
	if !ok {
		return nil
	}
	return &Skill{Name: name, Description: description, InstallName: entry.Name, SourceURL: skillBase + "/SKILL.md", Files: files}
}

func fetchV2Skills(ctx context.Context, candidate indexCandidate) []Skill {
	var skills []Skill
	for _, raw := range candidate.index.Skills {
		var entry v2Entry
		if json.Unmarshal(raw, &entry) != nil || !validV2Entry(entry) {
			continue
		}
		artifactURL, err := resolveURL(candidate.indexURL, entry.URL)
		if err != nil {
			continue
		}
		content, ok := fetchBytes(ctx, artifactURL)
		if !ok || digest(content) != entry.Digest {
			continue
		}
		var files map[string][]byte
		if entry.Type == "skill-md" {
			files = map[string][]byte{"SKILL.md": content}
		} else {
			files, err = extractArchive(content, artifactURL)
			if err != nil {
				continue
			}
		}
		name, description, ok := parseSkillMetadata(files["SKILL.md"])
		if !ok {
			continue
		}
		skills = append(skills, Skill{Name: name, Description: description, InstallName: entry.Name, SourceURL: artifactURL, Files: files})
	}
	return skills
}

func validV1Entry(entry v1Entry) bool {
	if !validSkillName(entry.Name) || entry.Description == "" || len(entry.Files) == 0 {
		return false
	}
	hasSkillMD := false
	for _, filePath := range entry.Files {
		if !safePath(filePath) {
			return false
		}
		if strings.EqualFold(filePath, "SKILL.md") {
			hasSkillMD = true
		}
	}
	return hasSkillMD
}

func validV2Entry(entry v2Entry) bool {
	if !validSkillName(entry.Name) || entry.Description == "" || len(entry.Description) > 1024 || (entry.Type != "skill-md" && entry.Type != "archive") || entry.URL == "" {
		return false
	}
	if _, err := resolveURL("https://example.com/.well-known/agent-skills/index.json", entry.URL); err != nil {
		return false
	}
	if !strings.HasPrefix(entry.Digest, "sha256:") || len(entry.Digest) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(entry.Digest, "sha256:"))
	return err == nil && entry.Digest == strings.ToLower(entry.Digest)
}

func validSkillName(name string) bool {
	if len(name) == 0 || len(name) > 64 || strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") || strings.Contains(name, "--") {
		return false
	}
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func safePath(filePath string) bool {
	return filePath != "" && !strings.HasPrefix(filePath, "/") && !strings.HasPrefix(filePath, "\\") && !strings.Contains(filePath, "..") && !strings.ContainsRune(filePath, 0)
}

func fetchBytes(ctx context.Context, sourceURL string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, false
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		if response != nil {
			response.Body.Close()
		}
		return nil, false
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxArchiveBytes+1))
	return content, err == nil && len(content) <= maxArchiveBytes
}

func resolveURL(base, reference string) (string, error) {
	baseURL, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	resolved, err := baseURL.Parse(reference)
	if err != nil || (resolved.Scheme != "http" && resolved.Scheme != "https") {
		return "", errors.New("invalid artifact URL")
	}
	return resolved.String(), nil
}

func digest(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func extractArchive(content []byte, artifactURL string) (map[string][]byte, error) {
	lower := strings.ToLower(artifactURL)
	if strings.HasSuffix(lower, ".zip") || (len(content) >= 2 && content[0] == 'P' && content[1] == 'K') {
		return extractZIP(content)
	}
	if strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz") || (len(content) >= 2 && content[0] == 0x1f && content[1] == 0x8b) {
		return extractTarGz(content)
	}
	return nil, errors.New("unsupported archive format")
}

func extractZIP(content []byte) (map[string][]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
	if err != nil {
		return nil, err
	}
	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if file.Mode()&os.ModeType != 0 || !safeArchivePath(file.Name) {
			return nil, errors.New("unsafe archive path")
		}
		if err := addArchiveFile(files, file.Name, file.UncompressedSize64, func() (io.ReadCloser, error) { return file.Open() }); err != nil {
			return nil, err
		}
	}
	return requireSkillMD(files)
}

func extractTarGz(content []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	files := make(map[string][]byte)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeDir {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA || !safeArchivePath(header.Name) {
			return nil, errors.New("unsafe archive path")
		}
		if err := addArchiveFile(files, header.Name, uint64(header.Size), func() (io.ReadCloser, error) { return io.NopCloser(reader), nil }); err != nil {
			return nil, err
		}
	}
	return requireSkillMD(files)
}

func addArchiveFile(files map[string][]byte, name string, declaredSize uint64, open func() (io.ReadCloser, error)) error {
	if len(files) >= maxArchiveFiles || declaredSize > maxArchiveBytes || !safeArchivePath(name) {
		return errors.New("invalid archive entry")
	}
	var total int
	for _, file := range files {
		total += len(file)
	}
	if total+int(declaredSize) > maxArchiveBytes {
		return errors.New("archive exceeds maximum unpacked size")
	}
	reader, err := open()
	if err != nil {
		return err
	}
	defer reader.Close()
	content, err := io.ReadAll(io.LimitReader(reader, int64(declaredSize)+1))
	if err != nil || uint64(len(content)) != declaredSize {
		return errors.New("invalid archive entry size")
	}
	files[name] = content
	return nil
}

func safeArchivePath(name string) bool {
	if !safePath(name) || strings.Contains(name, "\\") || len(name) >= 2 && name[1] == ':' {
		return false
	}
	clean := path.Clean(name)
	return clean == name && clean != "."
}

func requireSkillMD(files map[string][]byte) (map[string][]byte, error) {
	if _, ok := files["SKILL.md"]; !ok {
		return nil, errors.New("archive missing root SKILL.md")
	}
	return files, nil
}

func parseSkillMetadata(content []byte) (name, description string, ok bool) {
	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", false
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		if key, value, found := strings.Cut(trimmed, ":"); found {
			value = strings.Trim(strings.TrimSpace(value), "\"'")
			switch key {
			case "name":
				name = value
			case "description":
				description = value
			}
		}
	}
	return name, description, name != "" && description != ""
}
