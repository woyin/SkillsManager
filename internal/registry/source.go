package registry

import "strings"

// ── Source parsing and normalization ──

// SkillNameFromPath extracts the name from a path or URL.
// For tree URLs, extracts the last path component (the skill name).
func SkillNameFromPath(source string) string {
	source = strings.TrimRight(source, "/")
	// For tree URLs, the skill name is the last segment
	if idx := strings.Index(source, "/tree/"); idx >= 0 {
		treePath := source[idx+6:] // after "/tree/"
		parts := strings.Split(treePath, "/")
		if len(parts) > 0 {
			name := parts[len(parts)-1]
			name = strings.TrimSuffix(name, ".git")
			if name != "" {
				return name
			}
		}
		// Fallback: use the repo name
		source = source[:idx]
	}
	parts := strings.Split(source, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// Strip .git suffix
		name = strings.TrimSuffix(name, ".git")
		return name
	}
	return source
}

// IsGitURL returns true if source looks like a git URL.
// Supports: GitHub shorthand (owner/repo), full URLs, GitLab, SSH, and generic git URLs.
func IsGitURL(source string) bool {
	// SSH URLs
	if strings.HasPrefix(source, "git@") {
		return true
	}
	// .git suffix
	if strings.HasSuffix(source, ".git") {
		return true
	}
	// HTTPS URLs to known hosts
	for _, prefix := range []string{
		"https://github.com/",
		"https://gitlab.com/",
		"https://bitbucket.org/",
		"http://github.com/",
		"http://gitlab.com/",
		"http://bitbucket.org/",
	} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	// GitHub shorthand: owner/repo (but not local paths)
	if isGitHubShorthand(source) {
		return true
	}
	return false
}

// isGitHubShorthand checks if source is in owner/repo format
func isGitHubShorthand(source string) bool {
	// Must not start with . or / (local paths)
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") {
		return false
	}
	// Must not contain : (SSH URLs handled separately)
	if strings.Contains(source, ":") {
		return false
	}
	// Must have exactly one / and look like owner/repo
	parts := strings.Split(source, "/")
	if len(parts) < 2 || len(parts) > 4 { // owner/repo or owner/repo/tree/path
		return false
	}
	// Owner and repo should not be empty
	for _, p := range parts[:2] {
		if p == "" {
			return false
		}
	}
	return true
}

// NormalizeGitURL converts a source shorthand into a fully-qualified cloneable
// URL. GitHub shorthand (owner/repo) and github.com/... prefixes become
// https URLs; any /tree/<branch>/... suffix is stripped so the result points at
// the repository root. Inputs that are already full URLs (GitLab, SSH, .git)
// are returned unchanged. Exported so cmd packages share one implementation.
func NormalizeGitURL(source string) string {
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source
	}
	if isGitHubShorthand(source) {
		// owner/repo → https://github.com/owner/repo
		// Strip any /tree/ path first
		base := source
		if idx := strings.Index(source, "/tree/"); idx >= 0 {
			base = source[:idx]
		}
		return "https://github.com/" + base
	}
	return source
}

// ParseTreeURL extracts repo URL and sub-path from a tree URL.
// Example: https://github.com/owner/repo/tree/main/skills/my-skill
// Returns: (https://github.com/owner/repo, main, skills/my-skill)
func ParseTreeURL(source string) (repoURL, branch, subPath string, ok bool) {
	// Handle GitHub shorthand with tree: owner/repo/tree/branch/path
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") {
		if strings.Contains(source, "/tree/") {
			// Looks like owner/repo/tree/... shorthand
			parts := strings.SplitN(source, "/", 3)
			if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
				source = "https://github.com/" + source
			}
		}
	}

	for _, host := range []string{"https://github.com/", "https://gitlab.com/", "https://bitbucket.org/"} {
		if strings.HasPrefix(source, host) {
			rest := source[len(host):]
			parts := strings.SplitN(rest, "/", 3) // owner, repo, tree/...
			if len(parts) < 2 {
				return "", "", "", false
			}
			repoURL = host + parts[0] + "/" + parts[1]
			// Strip .git from repo name
			repoURL = strings.TrimSuffix(repoURL, ".git")

			if len(parts) < 3 {
				return repoURL, "", "", true
			}

			treePath := parts[2]
			if !strings.HasPrefix(treePath, "tree/") {
				return repoURL, "", "", true
			}

			treePath = treePath[5:] // strip "tree/"
			branchAndPath := strings.SplitN(treePath, "/", 2)
			branch = branchAndPath[0]
			if len(branchAndPath) > 1 {
				subPath = branchAndPath[1]
			}
			return repoURL, branch, subPath, true
		}
	}
	return "", "", "", false
}
