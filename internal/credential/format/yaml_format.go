package format

import (
	"strings"
)

// yamlHandler provides simple YAML file patching using stdlib only.
//
// Limitations (by design — no yaml library):
//   - Create: generates simple nested key-value YAML. Does not support lists, multi-line
//     strings, anchors, or other advanced YAML features.
//   - Patch: performs line-based search-and-replace for "key: value" patterns. Only works
//     for simple scalar values at known indentation levels. Does not handle flow mappings,
//     multi-line values, or comments on the same line as the key.
//   - For complex YAML structures, use a custom patcher instead.
type yamlHandler struct{}

func (yamlHandler) Create(fields map[string]string) ([]byte, error) {
	tree := buildTree(fields)
	var sb strings.Builder
	writeYAMLTree(&sb, tree, 0)
	return []byte(sb.String()), nil
}

func (yamlHandler) Patch(existing []byte, fields map[string]string) ([]byte, error) {
	lines := strings.Split(string(existing), "\n")

	for dotPath, val := range fields {
		parts := strings.Split(dotPath, ".")
		lines = patchYAMLLines(lines, parts, val)
	}

	return []byte(strings.Join(lines, "\n")), nil
}

func (yamlHandler) Remove(existing []byte, fields map[string]string) ([]byte, error) {
	lines := strings.Split(string(existing), "\n")

	for dotPath := range fields {
		parts := strings.Split(dotPath, ".")
		lines = removeYAMLLines(lines, parts)
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// node represents a tree of key-value pairs for YAML generation.
type node struct {
	key      string
	value    string   // leaf value (empty for branch nodes)
	children []*node  // ordered children
	order    []string // insertion order tracking
}

// buildTree converts a flat dot-path map into an ordered tree.
func buildTree(fields map[string]string) []*node {
	root := &node{}
	for dotPath, val := range fields {
		parts := strings.Split(dotPath, ".")
		insertNode(root, parts, val)
	}
	return root.children
}

func insertNode(parent *node, parts []string, val string) {
	if len(parts) == 0 {
		return
	}

	key := parts[0]

	// Find existing child.
	var child *node
	for _, c := range parent.children {
		if c.key == key {
			child = c
			break
		}
	}

	if child == nil {
		child = &node{key: key}
		parent.children = append(parent.children, child)
	}

	if len(parts) == 1 {
		child.value = val
	} else {
		insertNode(child, parts[1:], val)
	}
}

func writeYAMLTree(sb *strings.Builder, nodes []*node, indent int) {
	prefix := strings.Repeat("  ", indent)
	for _, n := range nodes {
		if len(n.children) > 0 {
			sb.WriteString(prefix + n.key + ":\n")
			writeYAMLTree(sb, n.children, indent+1)
		} else {
			sb.WriteString(prefix + n.key + ": " + yamlQuote(n.value) + "\n")
		}
	}
}

// yamlQuote wraps a value in quotes if it contains characters that need quoting.
func yamlQuote(s string) string {
	if s == "" {
		return `""`
	}
	// Quote if contains special chars, starts with special chars, or looks like a number/bool.
	needsQuote := false
	for _, c := range s {
		if c == ':' || c == '#' || c == '\'' || c == '"' || c == '{' || c == '}' ||
			c == '[' || c == ']' || c == ',' || c == '&' || c == '*' || c == '!' ||
			c == '|' || c == '>' || c == '%' || c == '@' || c == '`' || c == '\n' {
			needsQuote = true
			break
		}
	}
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "null", "~":
		needsQuote = true
	}
	if needsQuote {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

// patchYAMLLines does a line-based patch for a dot-path in YAML content.
// It walks the lines looking for each key segment at the expected indentation level.
func patchYAMLLines(lines []string, parts []string, val string) []string {
	if len(parts) == 0 {
		return lines
	}

	targetIndent := 0
	lineIdx := 0

	// Walk through path segments to find the right position.
	for seg := 0; seg < len(parts); seg++ {
		key := parts[seg]
		found := false

		for i := lineIdx; i < len(lines); i++ {
			line := lines[i]
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}

			currentIndent := len(line) - len(trimmed)
			if currentIndent < targetIndent {
				// We've left the current section without finding the key.
				break
			}
			if currentIndent != targetIndent {
				continue
			}

			colonIdx := strings.Index(trimmed, ":")
			if colonIdx < 0 {
				continue
			}
			lineKey := trimmed[:colonIdx]

			if lineKey == key {
				if seg == len(parts)-1 {
					// This is the leaf key — replace the value.
					prefix := strings.Repeat(" ", targetIndent)
					lines[i] = prefix + key + ": " + yamlQuote(val)
					return lines
				}
				// Intermediate key — descend.
				targetIndent += 2
				lineIdx = i + 1
				found = true
				break
			}
		}

		if !found {
			// Key not found at this level — append it.
			return appendYAMLPath(lines, parts, seg, targetIndent, val)
		}
	}

	return lines
}

// appendYAMLPath appends remaining path segments to the YAML lines at the right indentation.
func appendYAMLPath(lines []string, parts []string, fromSeg, indent int, val string) []string {
	var extra []string
	for i := fromSeg; i < len(parts); i++ {
		prefix := strings.Repeat(" ", indent)
		if i == len(parts)-1 {
			extra = append(extra, prefix+parts[i]+": "+yamlQuote(val))
		} else {
			extra = append(extra, prefix+parts[i]+":")
		}
		indent += 2
	}

	// Find where to insert: after the last line at a parent indentation, or at the end.
	// For simplicity, append at the end of the file.
	result := make([]string, 0, len(lines)+len(extra))
	result = append(result, lines...)

	// Ensure there's no trailing empty line duplication.
	if len(result) > 0 && result[len(result)-1] == "" {
		result = append(result[:len(result)-1], extra...)
		result = append(result, "")
	} else {
		result = append(result, extra...)
	}

	return result
}

// removeYAMLLines removes a key (and its children if it's a section) from YAML lines.
func removeYAMLLines(lines []string, parts []string) []string {
	if len(parts) == 0 {
		return lines
	}

	targetIndent := 0
	lineIdx := 0

	for seg := 0; seg < len(parts); seg++ {
		key := parts[seg]
		found := false

		for i := lineIdx; i < len(lines); i++ {
			line := lines[i]
			trimmed := strings.TrimLeft(line, " ")
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}

			currentIndent := len(line) - len(trimmed)
			if currentIndent < targetIndent {
				break
			}
			if currentIndent != targetIndent {
				continue
			}

			colonIdx := strings.Index(trimmed, ":")
			if colonIdx < 0 {
				continue
			}
			lineKey := trimmed[:colonIdx]

			if lineKey == key {
				if seg == len(parts)-1 {
					// Found the leaf — remove it (and any child lines).
					end := i + 1
					for end < len(lines) {
						nextLine := lines[end]
						nextTrimmed := strings.TrimLeft(nextLine, " ")
						if nextTrimmed == "" {
							end++
							continue
						}
						nextIndent := len(nextLine) - len(nextTrimmed)
						if nextIndent > targetIndent {
							end++
						} else {
							break
						}
					}
					result := make([]string, 0, len(lines)-(end-i))
					result = append(result, lines[:i]...)
					result = append(result, lines[end:]...)
					return result
				}
				targetIndent += 2
				lineIdx = i + 1
				found = true
				break
			}
		}

		if !found {
			return lines // key not found, nothing to remove
		}
	}

	return lines
}
