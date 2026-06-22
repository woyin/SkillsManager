package registry

import (
	"os"
	"strings"
)

// skillFrontmatter holds the fields sm extracts from a SKILL.md YAML
// frontmatter header.
type skillFrontmatter struct {
	Description string
	Internal    bool
}

// parseSkillFrontmatter reads the YAML frontmatter of a SKILL.md file and
// extracts the description and the metadata.internal flag. It performs a
// minimal, line-oriented parse (the same approach npx skills uses) rather
// than pulling in a full YAML dependency.
func parseSkillFrontmatter(skillMDPath string) skillFrontmatter {
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		return skillFrontmatter{}
	}
	return parseFrontmatterBytes(data)
}

// parseFrontmatterBytes extracts the description and metadata.internal flag
// from raw SKILL.md bytes. Exported so other packages (cmd/find, cmd/add) can
// share one implementation instead of each rolling their own parser.
func parseFrontmatterBytes(data []byte) skillFrontmatter {
	var fm skillFrontmatter
	content := string(data)

	// YAML frontmatter is delimited by a leading "---" line and a closing "---".
	if !strings.HasPrefix(content, "---") {
		return fm
	}
	rest := content[3:]
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		// Fallback: closing marker may be at EOF without a trailing newline.
		endIdx = strings.Index(rest, "---")
		if endIdx < 0 {
			return fm
		}
	}
	frontmatter := rest[:endIdx]

	inMetadata := false
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)

		// Detect the start of the `metadata:` block so we can recognise its
		// nested `internal:` child by indentation rather than prefix match.
		if trimmed == "metadata:" || strings.HasPrefix(trimmed, "metadata:") {
			inMetadata = true
			continue
		}
		// Any top-level key (no leading whitespace) ends the metadata block.
		if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inMetadata = false
		}

		if strings.HasPrefix(trimmed, "description:") {
			desc := strings.TrimSpace(strings.TrimPrefix(trimmed, "description:"))
			// Strip surrounding quotes if present.
			if len(desc) >= 2 && (desc[0] == '"' || desc[0] == '\'') && desc[len(desc)-1] == desc[0] {
				desc = desc[1 : len(desc)-1]
			}
			fm.Description = desc
		}
		if inMetadata && strings.HasPrefix(trimmed, "internal:") {
			val := strings.TrimSpace(strings.TrimPrefix(trimmed, "internal:"))
			switch val {
			case "true", "True", "TRUE", "yes", "1":
				fm.Internal = true
			}
		}
	}
	return fm
}

// ParseFrontmatterDescription reads a SKILL.md file and returns its
// frontmatter description. Exported for cmd packages that previously
// duplicated this parsing logic.
func ParseFrontmatterDescription(skillMDPath string) string {
	return parseSkillFrontmatter(skillMDPath).Description
}

// ParseFrontmatterFromBytes extracts the description from raw SKILL.md bytes.
// Exported so cmd/find can share the single parser implementation.
func ParseFrontmatterFromBytes(data []byte) string {
	return parseFrontmatterBytes(data).Description
}

// internalSkillsVisible reports whether skills marked metadata.internal
// should be shown. Mirrors npx skills: visible only when the
// INSTALL_INTERNAL_SKILLS environment variable is set to a truthy value.
func internalSkillsVisible() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("INSTALL_INTERNAL_SKILLS")))
	switch v {
	case "1", "true", "yes":
		return true
	}
	return false
}

// ParseFrontmatterFromString extracts the description from a SKILL.md content
// string. Exported so cmd/find can share the single parser implementation
// without an unnecessary string→[]byte→string round-trip.
func ParseFrontmatterFromString(content string) string {
	return parseFrontmatterBytes([]byte(content)).Description
}
