// Package sourceutil provides source classification helpers shared by the
// registry and lockfile packages without coupling those layers together.
package sourceutil

import "strings"

// IsGitURL reports whether source is a supported git source.
func IsGitURL(source string) bool {
	if strings.Contains(source, "::") && !strings.HasPrefix(source, "git@") {
		return false
	}
	if strings.HasPrefix(source, "git@") || strings.HasSuffix(source, ".git") {
		return true
	}
	for _, prefix := range []string{
		"https://github.com/", "https://gitlab.com/", "https://bitbucket.org/",
		"http://github.com/", "http://gitlab.com/", "http://bitbucket.org/",
	} {
		if strings.HasPrefix(source, prefix) {
			return true
		}
	}
	return isGitHubShorthand(source)
}

// NormalizeGitURL converts supported shorthand into a cloneable URL.
func NormalizeGitURL(source string) string {
	if strings.Contains(source, "::") && !strings.HasPrefix(source, "git@") {
		return source
	}
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source
	}
	if isGitHubShorthand(source) {
		if idx := strings.Index(source, "/tree/"); idx >= 0 {
			source = source[:idx]
		}
		return "https://github.com/" + source
	}
	return source
}

// ParseTreeURL splits a supported repository or tree URL into its repository,
// ref, and subpath components.
func ParseTreeURL(source string) (repoURL, branch, subPath string, ok bool) {
	if !strings.Contains(source, "://") && !strings.HasPrefix(source, "git@") && strings.Contains(source, "/tree/") {
		parts := strings.SplitN(source, "/", 3)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			source = "https://github.com/" + source
		}
	}
	for _, host := range []string{"https://github.com/", "https://gitlab.com/", "https://bitbucket.org/"} {
		if !strings.HasPrefix(source, host) {
			continue
		}
		rest := source[len(host):]
		parts := strings.SplitN(rest, "/", 3)
		if len(parts) < 2 {
			return "", "", "", false
		}
		repoURL = strings.TrimSuffix(host+parts[0]+"/"+parts[1], ".git")
		if len(parts) < 3 || !strings.HasPrefix(parts[2], "tree/") {
			return repoURL, "", "", true
		}
		branchAndPath := strings.SplitN(parts[2][5:], "/", 2)
		branch = branchAndPath[0]
		if len(branchAndPath) > 1 {
			subPath = branchAndPath[1]
		}
		return repoURL, branch, subPath, true
	}
	return "", "", "", false
}

func isGitHubShorthand(source string) bool {
	if strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.Contains(source, ":") {
		return false
	}
	parts := strings.Split(source, "/")
	if len(parts) < 2 || len(parts) > 4 {
		return false
	}
	return parts[0] != "" && parts[1] != ""
}
