package format

import (
	"strings"
)

// iniHandler provides INI file patching.
//
// Dot-path mapping:
//   - "section.key" maps to [section] + key = value
//   - "key" (no dot) maps to a top-level key (before any section header)
//   - "a.b.c" maps to [a.b] + c = value (last segment is key, rest is section)
type iniHandler struct{}

func (iniHandler) Create(fields map[string]string) ([]byte, error) {
	type entry struct {
		key string
		val string
	}
	sections := make(map[string][]entry)
	var sectionOrder []string
	var topLevel []entry

	for dotPath, val := range fields {
		section, key := splitINIPath(dotPath)
		if section == "" {
			topLevel = append(topLevel, entry{key, val})
		} else {
			if _, exists := sections[section]; !exists {
				sectionOrder = append(sectionOrder, section)
			}
			sections[section] = append(sections[section], entry{key, val})
		}
	}

	var sb strings.Builder

	for _, e := range topLevel {
		sb.WriteString(e.key + " = " + e.val + "\n")
	}
	if len(topLevel) > 0 && len(sectionOrder) > 0 {
		sb.WriteString("\n")
	}

	for i, sec := range sectionOrder {
		sb.WriteString("[" + sec + "]\n")
		for _, e := range sections[sec] {
			sb.WriteString(e.key + " = " + e.val + "\n")
		}
		if i < len(sectionOrder)-1 {
			sb.WriteString("\n")
		}
	}

	return []byte(sb.String()), nil
}

func (iniHandler) Patch(existing []byte, fields map[string]string) ([]byte, error) {
	lines := strings.Split(string(existing), "\n")

	for dotPath, val := range fields {
		section, key := splitINIPath(dotPath)
		lines = patchINIKey(lines, section, key, val)
	}

	return []byte(strings.Join(lines, "\n")), nil
}

func (iniHandler) Remove(existing []byte, fields map[string]string) ([]byte, error) {
	lines := strings.Split(string(existing), "\n")

	for dotPath := range fields {
		section, key := splitINIPath(dotPath)
		lines = removeINIKey(lines, section, key)
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// splitINIPath splits a dot-path into section and key.
// "section.key" -> ("section", "key")
// "a.b.c" -> ("a.b", "c")
// "key" -> ("", "key")
func splitINIPath(dotPath string) (section, key string) {
	idx := strings.LastIndex(dotPath, ".")
	if idx < 0 {
		return "", dotPath
	}
	return dotPath[:idx], dotPath[idx+1:]
}

// parseINISection extracts the section name from a line like "[section]".
func parseINISection(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return trimmed[1 : len(trimmed)-1], true
	}
	return "", false
}

// patchINIKey patches or inserts a key within a section.
func patchINIKey(lines []string, section, key, val string) []string {
	currentSection := ""
	for i, line := range lines {
		if sec, ok := parseINISection(line); ok {
			currentSection = sec
			continue
		}

		if currentSection != section {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			continue
		}

		eqIdx := strings.Index(trimmed, "=")
		if eqIdx < 0 {
			continue
		}
		lineKey := strings.TrimSpace(trimmed[:eqIdx])
		if lineKey == key {
			lines[i] = key + " = " + val
			return lines
		}
	}

	return appendINIKey(lines, section, key, val)
}

// appendINIKey adds a key-value pair to a section (creating the section if needed).
func appendINIKey(lines []string, section, key, val string) []string {
	newLine := key + " = " + val

	if section == "" {
		// Insert before the first section header.
		for i, line := range lines {
			if _, ok := parseINISection(line); ok {
				result := make([]string, 0, len(lines)+1)
				result = append(result, lines[:i]...)
				result = append(result, newLine)
				result = append(result, lines[i:]...)
				return result
			}
		}
		return append(lines, newLine)
	}

	// Find the section.
	for i, line := range lines {
		if sec, ok := parseINISection(line); ok && sec == section {
			end := len(lines)
			for j := i + 1; j < len(lines); j++ {
				if _, ok := parseINISection(lines[j]); ok {
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

// removeINIKey removes a key from a section.
func removeINIKey(lines []string, section, key string) []string {
	currentSection := ""
	for i, line := range lines {
		if sec, ok := parseINISection(line); ok {
			currentSection = sec
			continue
		}

		if currentSection != section {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
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
