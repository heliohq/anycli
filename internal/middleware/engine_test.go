package middleware

import (
	"testing"

	"github.com/shipbase/anycli/internal/registry"
)

func TestEvalCondition_Empty(t *testing.T) {
	ctx := &Context{Args: []string{"--verbose"}}
	if !evalCondition(nil, ctx) {
		t.Error("nil condition should return true")
	}
	if !evalCondition(map[string]interface{}{}, ctx) {
		t.Error("empty condition should return true")
	}
}

func TestEvalCondition_HasFlag(t *testing.T) {
	ctx := &Context{Args: []string{"pr", "list", "--json", "title"}}

	when := map[string]interface{}{"has_flag": "--json"}
	if !evalCondition(when, ctx) {
		t.Error("should detect --json flag")
	}

	when = map[string]interface{}{"has_flag": "--verbose"}
	if evalCondition(when, ctx) {
		t.Error("should not detect missing --verbose flag")
	}
}

func TestEvalCondition_NotHasFlag(t *testing.T) {
	ctx := &Context{Args: []string{"pr", "list"}}

	when := map[string]interface{}{"not_has_flag": "--json"}
	if !evalCondition(when, ctx) {
		t.Error("should return true when flag is absent")
	}

	ctx.Args = append(ctx.Args, "--json")
	if evalCondition(when, ctx) {
		t.Error("should return false when flag is present")
	}
}

func TestEvalCondition_ExitCode(t *testing.T) {
	ctx := &Context{ExitCode: 2}

	if !evalCondition(map[string]interface{}{"exit_code_is": float64(2)}, ctx) {
		t.Error("exit_code_is should match")
	}
	if evalCondition(map[string]interface{}{"exit_code_is": float64(0)}, ctx) {
		t.Error("exit_code_is should not match")
	}
	if !evalCondition(map[string]interface{}{"exit_code_not": float64(0)}, ctx) {
		t.Error("exit_code_not should match")
	}
}

func TestEvalCondition_OutputContains(t *testing.T) {
	ctx := &Context{Stdout: []byte("error: not found")}

	if !evalCondition(map[string]interface{}{"output_contains": "not found"}, ctx) {
		t.Error("should match output substring")
	}
	if evalCondition(map[string]interface{}{"output_contains": "success"}, ctx) {
		t.Error("should not match absent substring")
	}
}

func TestRunBefore_AppendFlag(t *testing.T) {
	rules := []registry.Rule{
		{
			Name: "add-json",
			Rule: "append_flag",
			Config: map[string]interface{}{
				"flag": "--json",
			},
		},
	}
	ctx := &Context{Args: []string{"pr", "list"}, Env: map[string]string{}}

	if err := RunBefore(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Args[len(ctx.Args)-1] != "--json" {
		t.Errorf("expected --json appended, got %v", ctx.Args)
	}
}

func TestRunBefore_AppendFlag_Conditional(t *testing.T) {
	rules := []registry.Rule{
		{
			Name: "add-json",
			Rule: "append_flag",
			When: map[string]interface{}{"not_has_flag": "--json"},
			Config: map[string]interface{}{
				"flag": "--json",
			},
		},
	}

	// Should append when --json is absent
	ctx := &Context{Args: []string{"pr", "list"}, Env: map[string]string{}}
	if err := RunBefore(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasFlag(ctx.Args, "--json") {
		t.Error("should have appended --json")
	}

	// Should skip when --json is already present
	ctx = &Context{Args: []string{"pr", "list", "--json", "title"}, Env: map[string]string{}}
	origLen := len(ctx.Args)
	if err := RunBefore(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ctx.Args) != origLen {
		t.Error("should not have appended --json when already present")
	}
}

func TestRunBefore_PrependArgs(t *testing.T) {
	rules := []registry.Rule{
		{
			Name: "add-prefix",
			Rule: "prepend_args",
			Config: map[string]interface{}{
				"args": []interface{}{"--global"},
			},
		},
	}
	ctx := &Context{Args: []string{"status"}, Env: map[string]string{}}

	if err := RunBefore(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Args[0] != "--global" || ctx.Args[1] != "status" {
		t.Errorf("expected [--global status], got %v", ctx.Args)
	}
}

func TestRunBefore_SetEnv_StaticValue(t *testing.T) {
	rules := []registry.Rule{
		{
			Name: "set-token",
			Rule: "set_env",
			Config: map[string]interface{}{
				"env_var": "MY_TOKEN",
				"value":   "abc123",
			},
		},
	}
	ctx := &Context{Args: []string{}, Env: map[string]string{}}

	if err := RunBefore(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.Env["MY_TOKEN"] != "abc123" {
		t.Errorf("expected MY_TOKEN=abc123, got %s", ctx.Env["MY_TOKEN"])
	}
}

func TestRunAfter_MapExitCode(t *testing.T) {
	rules := []registry.Rule{
		{
			Name: "normalize",
			Rule: "map_exit_code",
			Config: map[string]interface{}{
				"mapping": map[string]interface{}{
					"2": float64(1),
					"3": float64(1),
				},
			},
		},
	}

	ctx := &Context{ExitCode: 2, Env: map[string]string{}}
	if err := RunAfter(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", ctx.ExitCode)
	}

	// Unmapped exit code should stay the same
	ctx = &Context{ExitCode: 5, Env: map[string]string{}}
	if err := RunAfter(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ctx.ExitCode != 5 {
		t.Errorf("expected exit code 5, got %d", ctx.ExitCode)
	}
}

func TestRunAfter_EnsureJSON_AlreadyJSON(t *testing.T) {
	rules := []registry.Rule{
		{Name: "json", Rule: "ensure_json"},
	}

	ctx := &Context{Stdout: []byte(`{"key":"value"}`), Env: map[string]string{}}
	if err := RunAfter(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(ctx.Stdout) != `{"key":"value"}` {
		t.Errorf("JSON output should not be modified, got %s", ctx.Stdout)
	}
}

func TestRunAfter_EnsureJSON_PlainText(t *testing.T) {
	rules := []registry.Rule{
		{Name: "json", Rule: "ensure_json"},
	}

	ctx := &Context{Stdout: []byte("hello world"), Env: map[string]string{}}
	if err := RunAfter(rules, ctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := `{"output":"hello world"}`
	if string(ctx.Stdout) != expected {
		t.Errorf("expected %s, got %s", expected, ctx.Stdout)
	}
}

func TestRunBefore_UnknownRule(t *testing.T) {
	rules := []registry.Rule{
		{Name: "bad", Rule: "nonexistent"},
	}
	ctx := &Context{Args: []string{}, Env: map[string]string{}}

	err := RunBefore(rules, ctx)
	if err == nil {
		t.Error("expected error for unknown rule type")
	}
}
