package anycli

import (
	"slices"
	"testing"
)

func TestInspect(t *testing.T) {
	cases := []struct {
		name string
		tool string
		args []string

		wantAction   string
		wantSide     bool
		wantParsed   bool
		wantRunnable bool
		wantHelp     bool
		wantArgs     []string
		// wantFlags spot-checks entries of the Flags map (nil = skip).
		wantFlags map[string]Flag
		wantErr   bool
	}{
		{
			name:         "leaf with slice and scalar flags",
			tool:         "gmail",
			args:         []string{"messages", "send", "--to", "a@b", "--subject", "hi", "--body", "x"},
			wantAction:   "gmail.messages_send",
			wantSide:     true, // gmail is not yet annotated; absent = true
			wantParsed:   true,
			wantRunnable: true,
			wantFlags: map[string]Flag{
				"to":      {Values: []string{"a@b"}, Set: true, IsSlice: true, Type: "stringSlice"},
				"subject": {Value: "hi", Set: true, Type: "string"},
				"cc":      {Set: false, IsSlice: true, Type: "stringSlice"},
				// inherited persistent flag from the gmail root, at default
				"json": {Value: "false", Set: false, Type: "bool"},
			},
		},
		{
			name:         "typo below a group stops on the group",
			tool:         "gmail",
			args:         []string{"messages", "snd"},
			wantAction:   "gmail.messages",
			wantSide:     true,
			wantParsed:   true, // dry-run parse runs on the group node and succeeds
			wantRunnable: false,
			wantArgs:     []string{"snd"},
		},
		{
			name:         "typo at the root stays on the root",
			tool:         "gmail",
			args:         []string{"nope"},
			wantAction:   "gmail",
			wantSide:     true,
			wantParsed:   true,
			wantRunnable: false,
			wantArgs:     []string{"nope"},
		},
		{
			name:         "bare group command",
			tool:         "gmail",
			args:         []string{"messages"},
			wantAction:   "gmail.messages",
			wantSide:     true,
			wantParsed:   true,
			wantRunnable: false,
		},
		{
			name:         "flag parse failure is a fact not an error",
			tool:         "gmail",
			args:         []string{"messages", "send", "--nope"},
			wantAction:   "gmail.messages_send",
			wantSide:     true,
			wantParsed:   false,
			wantRunnable: true,
			// Parsed=false: Flags and Args empty, Help false.
		},
		{
			name:         "help flag consumed by cobra",
			tool:         "gmail",
			args:         []string{"messages", "send", "--help"},
			wantAction:   "gmail.messages_send",
			wantSide:     true,
			wantParsed:   true,
			wantRunnable: true,
			wantHelp:     true,
		},
		{
			name:         "help shorthand consumed by cobra",
			tool:         "gmail",
			args:         []string{"messages", "send", "-h"},
			wantAction:   "gmail.messages_send",
			wantSide:     true,
			wantParsed:   true,
			wantRunnable: true,
			wantHelp:     true,
		},
		{
			name:         "literal --help as a flag value is not a help request",
			tool:         "gmail",
			args:         []string{"messages", "send", "--subject", "--help"},
			wantAction:   "gmail.messages_send",
			wantSide:     true,
			wantParsed:   true,
			wantRunnable: true,
			wantHelp:     false,
			wantFlags: map[string]Flag{
				"subject": {Value: "--help", Set: true, Type: "string"},
			},
		},
		{
			name:    "registry miss is an error",
			tool:    "no-such-tool",
			args:    []string{"whatever"},
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := Inspect(tc.tool, tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Inspect(%q, %v) = %+v, want error", tc.tool, tc.args, inv)
				}
				return
			}
			if err != nil {
				t.Fatalf("Inspect(%q, %v): %v", tc.tool, tc.args, err)
			}
			if inv.Action != tc.wantAction {
				t.Errorf("Action = %q, want %q", inv.Action, tc.wantAction)
			}
			if inv.SideEffect != tc.wantSide {
				t.Errorf("SideEffect = %v, want %v", inv.SideEffect, tc.wantSide)
			}
			if inv.Parsed != tc.wantParsed {
				t.Errorf("Parsed = %v, want %v", inv.Parsed, tc.wantParsed)
			}
			if inv.Runnable != tc.wantRunnable {
				t.Errorf("Runnable = %v, want %v", inv.Runnable, tc.wantRunnable)
			}
			if inv.Help != tc.wantHelp {
				t.Errorf("Help = %v, want %v", inv.Help, tc.wantHelp)
			}
			if !inv.Parsed {
				if len(inv.Flags) != 0 || len(inv.Args) != 0 {
					t.Errorf("Parsed=false must leave Flags/Args empty, got Flags=%v Args=%v", inv.Flags, inv.Args)
				}
				if inv.Help {
					t.Errorf("Parsed=false must force Help=false")
				}
			}
			if !slices.Equal(inv.Args, tc.wantArgs) {
				t.Errorf("Args = %v, want %v", inv.Args, tc.wantArgs)
			}
			for name, want := range tc.wantFlags {
				got, ok := inv.Flags[name]
				if !ok {
					t.Errorf("Flags missing %q (have %d flags)", name, len(inv.Flags))
					continue
				}
				if got.Value != want.Value || !slices.Equal(got.Values, want.Values) ||
					got.Set != want.Set || got.IsSlice != want.IsSlice || got.Type != want.Type {
					t.Errorf("Flags[%q] = %+v, want %+v", name, got, want)
				}
			}
		})
	}
}

// TestInspect_SliceScalarInvariant checks the Flag derivation rule: IsSlice
// decides which of Value/Values carries the effective value, exclusively.
func TestInspect_SliceScalarInvariant(t *testing.T) {
	inv, err := Inspect("gmail", []string{"messages", "send", "--to", "a@b,c@d", "--subject", "s", "--body", "x"})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	for name, f := range inv.Flags {
		if f.IsSlice && f.Value != "" {
			t.Errorf("Flags[%q]: IsSlice=true must keep Value empty, got %q", name, f.Value)
		}
		if !f.IsSlice && f.Values != nil {
			t.Errorf("Flags[%q]: IsSlice=false must keep Values nil, got %v", name, f.Values)
		}
	}
	to := inv.Flags["to"]
	if !slices.Equal(to.Values, []string{"a@b", "c@d"}) {
		t.Errorf("Flags[to].Values = %v, want the comma-split slice", to.Values)
	}
}

func TestInspect_NoNetwork(t *testing.T) {
	// The send leaf's RunE would hit the Gmail API; Inspect must never run
	// it. An HTTP call with the empty dry-run token would panic/fail loudly
	// inside RunE — reaching here with Parsed=true and no error is the
	// regression signal, plus the command tree seam never wires a client.
	inv, err := Inspect("gmail", []string{"messages", "send", "--to", "a@b", "--subject", "s", "--body", "x"})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !inv.Parsed || !inv.Runnable {
		t.Fatalf("expected a parsed runnable leaf, got %+v", inv)
	}
}

func TestServiceTools(t *testing.T) {
	names := ServiceTools()
	if !slices.Contains(names, "gmail") {
		t.Fatalf("ServiceTools() = %v, want it to contain gmail", names)
	}
	if !slices.IsSorted(names) {
		t.Errorf("ServiceTools() = %v, want sorted output", names)
	}
}

func TestCommandTree(t *testing.T) {
	root, err := CommandTree("gmail")
	if err != nil {
		t.Fatalf("CommandTree(gmail): %v", err)
	}
	if root == nil || !root.HasSubCommands() {
		t.Fatalf("CommandTree(gmail) = %v, want a root with subcommands", root)
	}
	if _, err := CommandTree("no-such-tool"); err == nil {
		t.Fatalf("CommandTree(no-such-tool) succeeded, want registry-miss error")
	}
}
