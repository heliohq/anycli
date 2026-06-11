package middleware

import (
	"fmt"
	"strings"

	"github.com/heliohq/anycli/internal/registry"
)

// Context holds the mutable state of a command execution passing through the middleware pipeline.
type Context struct {
	// Args is the argument list (may be modified by before hooks).
	Args []string
	// Env holds additional environment variables to set.
	Env map[string]string
	// Stdout captured from the real command execution.
	Stdout []byte
	// Stderr captured from the real command execution.
	Stderr []byte
	// ExitCode from the real command execution.
	ExitCode int
}

// RunBefore executes all before rules in order, modifying ctx.
func RunBefore(rules []registry.Rule, ctx *Context) error {
	for _, r := range rules {
		if !evalCondition(r.When, ctx) {
			continue
		}
		if err := execBeforeRule(r, ctx); err != nil {
			return fmt.Errorf("before rule %q: %w", r.Name, err)
		}
	}
	return nil
}

// RunAfter executes all after rules in order, modifying ctx.
func RunAfter(rules []registry.Rule, ctx *Context) error {
	for _, r := range rules {
		if !evalCondition(r.When, ctx) {
			continue
		}
		if err := execAfterRule(r, ctx); err != nil {
			return fmt.Errorf("after rule %q: %w", r.Name, err)
		}
	}
	return nil
}

func execBeforeRule(r registry.Rule, ctx *Context) error {
	switch r.Rule {
	case "set_env":
		return ruleSetEnv(r, ctx)
	case "append_flag":
		return ruleAppendFlag(r, ctx)
	case "prepend_args":
		return rulePrependArgs(r, ctx)
	default:
		return fmt.Errorf("unknown before rule type: %s", r.Rule)
	}
}

func execAfterRule(r registry.Rule, ctx *Context) error {
	switch r.Rule {
	case "map_exit_code":
		return ruleMapExitCode(r, ctx)
	case "ensure_json":
		return ruleEnsureJSON(r, ctx)
	default:
		return fmt.Errorf("unknown after rule type: %s", r.Rule)
	}
}

// evalCondition checks whether a rule's "when" clause is satisfied.
// An empty/nil when clause always evaluates to true.
func evalCondition(when map[string]interface{}, ctx *Context) bool {
	if len(when) == 0 {
		return true
	}
	for key, val := range when {
		switch key {
		case "has_flag":
			if !hasFlag(ctx.Args, val.(string)) {
				return false
			}
		case "not_has_flag":
			if hasFlag(ctx.Args, val.(string)) {
				return false
			}
		case "exit_code_is":
			code := int(val.(float64))
			if ctx.ExitCode != code {
				return false
			}
		case "exit_code_not":
			code := int(val.(float64))
			if ctx.ExitCode == code {
				return false
			}
		case "output_contains":
			if !strings.Contains(string(ctx.Stdout), val.(string)) {
				return false
			}
		}
	}
	return true
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// --- Before rule implementations ---

func ruleSetEnv(r registry.Rule, ctx *Context) error {
	envVar, _ := r.Config["env_var"].(string)
	if envVar == "" {
		return fmt.Errorf("set_env: env_var is required")
	}

	// If "from" is "credentials", the value should already be loaded into ctx.Env
	// by the exec layer. Otherwise use a static "value" field.
	if from, _ := r.Config["from"].(string); from == "credentials" {
		// Credential injection is handled by the exec layer before middleware runs.
		// If not found, skip silently — the tool may work without auth or prompt itself.
		return nil
	}

	if value, ok := r.Config["value"].(string); ok {
		ctx.Env[envVar] = value
	}
	return nil
}

func ruleAppendFlag(r registry.Rule, ctx *Context) error {
	flag, _ := r.Config["flag"].(string)
	if flag == "" {
		return fmt.Errorf("append_flag: flag is required")
	}
	ctx.Args = append(ctx.Args, flag)
	return nil
}

func rulePrependArgs(r registry.Rule, ctx *Context) error {
	argsRaw, ok := r.Config["args"]
	if !ok {
		return fmt.Errorf("prepend_args: args is required")
	}
	argsList, ok := argsRaw.([]interface{})
	if !ok {
		return fmt.Errorf("prepend_args: args must be an array")
	}
	var prepend []string
	for _, a := range argsList {
		prepend = append(prepend, fmt.Sprint(a))
	}
	ctx.Args = append(prepend, ctx.Args...)
	return nil
}

// --- After rule implementations ---

func ruleMapExitCode(r registry.Rule, ctx *Context) error {
	mapping, ok := r.Config["mapping"]
	if !ok {
		return nil
	}
	m, ok := mapping.(map[string]interface{})
	if !ok {
		return nil
	}
	key := fmt.Sprintf("%d", ctx.ExitCode)
	if newCode, ok := m[key]; ok {
		ctx.ExitCode = int(newCode.(float64))
	}
	return nil
}

func ruleEnsureJSON(r registry.Rule, ctx *Context) error {
	out := strings.TrimSpace(string(ctx.Stdout))
	// If already JSON, leave it alone
	if len(out) > 0 && (out[0] == '{' || out[0] == '[') {
		return nil
	}
	// Wrap plain text as JSON
	wrapped := fmt.Sprintf(`{"output":%q}`, out)
	ctx.Stdout = []byte(wrapped)
	return nil
}
