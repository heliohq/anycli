package tools

import "testing"

// TestBuiltinServicesRegistered guards against a service package being added
// without wiring it into init() — the calendar tool shipped dead once because
// its definition had type:"service" but no RegisterService call, so exec's
// GetService lookup failed at runtime.
func TestBuiltinServicesRegistered(t *testing.T) {
	for _, name := range []string{
		"slack", "notion", "gmail", "calendar", "contacts",
		"drive", "discord", "figma", "linkedin", "x", "sheets",
		"meet", "docs", "tasks", "bitly", "mongodb", "gate-probe",
		"meet", "docs", "tasks", "bitly", "mongodb", "attio",
		"meet", "docs", "tasks", "bitly", "mongodb", "expensify", "gate-probe",
		"meet", "docs", "tasks", "bitly", "mongodb", "help-scout",
		"meet", "docs", "tasks", "bitly", "mongodb", "instantly",
		"meet", "docs", "tasks", "bitly", "mongodb", "knock",
	} {
		if _, err := GetService(name); err != nil {
			t.Errorf("GetService(%q) = %v, want a registered service", name, err)
		}
	}
}
