package gorgias

import "strconv"

// parseID converts a string flag value to an integer resource id, returning a
// usageError (exit 2) when it is not a valid integer.
func parseID(flag, raw string) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &usageError{msg: "gorgias: --" + flag + " must be an integer id, got " + strconv.Quote(raw)}
	}
	return n, nil
}

// boolString renders a bool as a lowercase query value ("true"/"false").
func boolString(b bool) string {
	return strconv.FormatBool(b)
}
