package format

import (
	"strings"
)

// tomlHandler provides simple TOML file patching using stdlib only.
//
// Limitations (by design — no toml library):
//   - Create: generates simple TOML with [section] headers and key = "value" pairs.
//     Only supports up to 2 levels of nesting (section.key). Deeper paths use
//     dotted section headers (e.g., [a.b] for path "a.b.c").
//   - Patch: line-based search-and-replace for key = "value" patterns within the
//     correct [section]. Only handles simple string values.
//   - For complex TOML (inline tables, arrays of tables, etc.), use a custom patcher.
type tomlHandler struct{}

func (tomlHandler) Create(fields map[string]string) ([]byte, error) {
	// Group by section (all parts except the last form the section header).
	type entry struct {
		key string
		val string
	}
	sections := make(map[string][]entry)
	var sectionOrder []string
	var topLevel []entry

	for dotPath, val := range fields {
		parts := strings.Split(dotPath, ".")
		if len(parts) == 1 {
			topLevel = append(topLevel, entry{parts[0], val})
		} else {
			section := strings.Join(parts[:len(parts)-1], ".")
			key := parts[len(parts)-1]
			if _, exists := sections[section]; !exists {
				sectionOrder = append(sectionOrder, section)
			}
			sections[section] = append(sections[section], entry{key, val})
		}
	}

	var sb strings.Builder

	// Write top-level keys first.
	for _, e := range topLevel {
		sb.WriteString(e.key + " = " + tomlQuote(e.val) + "\n")
	}
	if len(topLevel) > 0 && len(sectionOrder) > 0 {
		sb.WriteString("\n")
	}

	// Write sections.
	for i, sec := range sectionOrder {
		sb.WriteString("[" + sec + "]\n")
		for _, e := range sections[sec] {
			sb.WriteString(e.key + " = " + tomlQuote(e.val) + "\n")
		}
		if i < len(sectionOrder)-1 {
			sb.WriteString("\n")
		}
	}

	return []byte(sb.String()), nil
}

func (tomlHandler) Patch(existing []byte, fields map[string]string) ([]byte, error) {
	lines := strings.Split(string(existing), "\n")

	for dotPath, val := range fields {
		parts := strings.Split(dotPath, ".")
		if len(parts) == 1 {
			lines = patchTOMLKey(lines, "", parts[0], val)
		} else {
			section := strings.Join(parts[:len(parts)-1], ".")
			key := parts[len(parts)-1]
			lines = patchTOMLKey(lines, section, key, val)
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

func (tomlHandler) Remove(existing []byte, fields map[string]string) ([]byte, error) {
	lines := strings.Split(string(existing), "\n")

	for dotPath := range fields {
		parts := strings.Split(dotPath, ".")
		if len(parts) == 1 {
			lines = removeTOMLKey(lines, "", parts[0])
		} else {
			section := strings.Join(parts[:len(parts)-1], ".")
			key := parts[len(parts)-1]
			lines = removeTOMLKey(lines, section, key)
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// tomlQuote wraps a string value in double quotes with proper escaping.
func tomlQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return `"` + s + `"`
}

// parseTOMLSection extracts the section name from a line like "[section]" or "[a.b]".
// Returns the section name and true if the line is a section header.
func parseTOMLSection(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") &&
		!strings.HasPrefix(trimmed, "[[") {
		return trimmed[1 : len(trimmed)-1], true
	}
	return "", false
}

// patchTOMLKey patches a key within a section (or top-level if section is "").
// If the key doesn't exist, appends it in the right section.
func patchTOMLKey(lines []string, section, key, val string) []string {
	currentSection := ""
	for i, line := range lines {
		if sec, ok := parseTOMLSection(line); ok {
			currentSection = sec
			continue
		}

		if currentSection != section {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			continue
		}
		lineKey := strings.TrimSpace(trimmed[:eqIdx])
		if lineKey == key {
			lines[i] = key + " = " + tomlQuote(val)
			return lines
		}
	}

	// Key not found — append it.
	return appendTOMLKey(lines, section, key, val)
}

// appendTOMLKey adds a key-value pair to a section (creating the section if needed).
func appendTOMLKey(lines []string, section, key, val string) []string {
	newLine := key + " = " + tomlQuote(val)

	if section == "" {
		// Insert at the top, before any section headers.
		for i, line := range lines {
			if _, ok := parseTOMLSection(line); ok {
				result := make([]string, 0, len(lines)+1)
				result = append(result, lines[:i]...)
				result = append(result, newLine)
				result = append(result, lines[i:]...)
				return result
			}
		}
		// No sections found — just append.
		return append(lines, newLine)
	}

	// Find the section and append at its end.
	for i, line := range lines {
		if sec, ok := parseTOMLSection(line); ok && sec == section {
			// Find the end of this section (next section header or EOF).
			end := len(lines)
			for j := i + 1; j < len(lines); j++ {
				if _, ok := parseTOMLSection(lines[j]); ok {
					end = j
					break
				}
			}
			result := make([]string, 0, len(lines)+1)
			result = append(result, lines[:end]...)
			result = append(result, newLine)
			result = append(result, lines[end:]...)
			return result
		}
	}

	// Section not found — create it.
	extra := []string{"", "[" + section + "]", newLine}
	return append(lines, extra...)
}

// removeTOMLKey removes a key from a section.
func removeTOMLKey(lines []string, section, key string) []string {
	currentSection := ""
	for i, line := range lines {
		if sec, ok := parseTOMLSection(line); ok {
			currentSection = sec
			continue
		}

		if currentSection != section {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			continue
		}
		lineKey := strings.TrimSpace(trimmed[:eqIdx])
		if lineKey == key {
			result := make([]string, 0, len(lines)-1)
			result = append(result, lines[:i]...)
			result = append(result, lines[i+1:]...)
			return result
		}
	}
	return lines
}
