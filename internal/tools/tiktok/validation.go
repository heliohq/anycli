package tiktok

import "fmt"

func errRequired(flag string) error {
	return fmt.Errorf("%s is required", flag)
}

func requireRange(flag string, value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("%s must be between %d and %d", flag, min, max)
	}
	return nil
}

// requireExactlyOne enforces that exactly one of a set of mutually-exclusive
// string flags is provided. pairs is a flat list of (flagName, value).
func requireExactlyOne(pairs ...string) error {
	if len(pairs)%2 != 0 {
		return fmt.Errorf("requireExactlyOne: odd argument count")
	}
	var set []string
	for i := 0; i < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			set = append(set, pairs[i])
		}
	}
	if len(set) == 1 {
		return nil
	}
	var names []string
	for i := 0; i < len(pairs); i += 2 {
		names = append(names, pairs[i])
	}
	if len(set) == 0 {
		return fmt.Errorf("exactly one of %v is required", names)
	}
	return fmt.Errorf("%v are mutually exclusive", set)
}
