package main

import "testing"

func TestParseInvocation(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		tool     string
		toolArgs []string
		wantErr  bool
	}{
		{name: "simple", args: []string{"slack", "--", "chat", "send"}, tool: "slack", toolArgs: []string{"chat", "send"}},
		{name: "empty tool args", args: []string{"slack", "--"}, tool: "slack", toolArgs: []string{}},
		{name: "missing dash dash", args: []string{"slack", "chat"}, wantErr: true},
		{name: "no tool", args: []string{}, wantErr: true},
		{name: "unknown flag", args: []string{"slack", "--bogus", "--", "chat"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool, toolArgs, err := parseInvocation(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseInvocation: %v", err)
			}
			if tool != tc.tool {
				t.Errorf("tool = %q, want %q", tool, tc.tool)
			}
			if len(toolArgs) != len(tc.toolArgs) {
				t.Fatalf("toolArgs = %v, want %v", toolArgs, tc.toolArgs)
			}
			for i := range toolArgs {
				if toolArgs[i] != tc.toolArgs[i] {
					t.Errorf("toolArgs[%d] = %q, want %q", i, toolArgs[i], tc.toolArgs[i])
				}
			}
		})
	}
}
