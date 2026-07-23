package semrush

import (
	"strconv"
	"strings"
)

// semicolonDelimiter is the field separator Semrush v3 report responses use.
// Note export_columns itself is comma-separated in the request; only the
// response body is semicolon-delimited.
const semicolonDelimiter = ";"

// parseCSVRows parses a Semrush v3 semicolon-delimited CSV body into a header
// slice and one map per data row, keyed by snake_cased header names. Cells that
// look numeric are coerced to numbers; everything else stays a string. A body
// with only a header row (or empty) yields zero rows.
func parseCSVRows(body string) (headers []string, rows []map[string]any) {
	lines := splitNonEmptyLines(body)
	if len(lines) == 0 {
		return nil, nil
	}
	rawHeaders := strings.Split(lines[0], semicolonDelimiter)
	headers = make([]string, len(rawHeaders))
	for i, h := range rawHeaders {
		headers[i] = snakeCase(h)
	}
	for _, line := range lines[1:] {
		cells := strings.Split(line, semicolonDelimiter)
		row := make(map[string]any, len(headers))
		for i, header := range headers {
			if header == "" {
				continue
			}
			if i < len(cells) {
				row[header] = coerceCell(cells[i])
			} else {
				row[header] = ""
			}
		}
		rows = append(rows, row)
	}
	return headers, rows
}

// splitNonEmptyLines splits a body on newlines (tolerating CRLF) and drops empty
// trailing/blank lines.
func splitNonEmptyLines(body string) []string {
	raw := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}

// snakeCase lowercases a human-readable Semrush header and collapses every run
// of non-alphanumeric characters into a single underscore, trimming leading and
// trailing underscores. "Search Volume" → "search_volume", "Traffic (%)" →
// "traffic", "Number of Results" → "number_of_results". Generic by design — no
// per-report column table (the header names ARE the schema).
func snakeCase(header string) string {
	var b strings.Builder
	prevUnderscore := false
	for _, r := range strings.TrimSpace(header) {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r - 'A' + 'a')
			prevUnderscore = false
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevUnderscore = false
		default:
			if b.Len() > 0 && !prevUnderscore {
				b.WriteByte('_')
				prevUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

// coerceCell converts a CSV cell to a number when it cleanly parses as one, else
// returns the trimmed string. Integers stay integers (json.Marshal renders them
// without a decimal point); non-integer numerics become floats. Thousands
// separators are NOT stripped here — report cells arrive without them, and
// stripping could corrupt a legitimate string value.
func coerceCell(cell string) any {
	trimmed := strings.TrimSpace(cell)
	if trimmed == "" {
		return ""
	}
	if n, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return f
	}
	return trimmed
}
