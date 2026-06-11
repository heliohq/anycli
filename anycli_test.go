package anycli

import (
	"context"
	"testing"
	"time"
)

// staticResolver is a minimal CredentialResolver for wiring tests.
type staticResolver struct{}

func (staticResolver) Resolve(ctx context.Context, tool Tool, account string) (*Credential, error) {
	return &Credential{Data: map[string]any{}, CacheUntil: time.Now().Add(time.Hour)}, nil
}

func TestExecuteWith_UnknownTool(t *testing.T) {
	e, err := New(Config{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	exitCode, err := e.ExecuteWith(context.Background(), Tool("definitely-not-a-shipped-tool"), nil, staticResolver{}, ExecOptions{Account: "a1"})
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}

func TestExecute_DelegatesToExecuteWith(t *testing.T) {
	e, err := New(Config{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	// Execute is the default-account short form: same behavior as ExecuteWith
	// with empty options for an unknown tool.
	exitCode, err := e.Execute(context.Background(), Tool("definitely-not-a-shipped-tool"), nil, staticResolver{})
	if err == nil {
		t.Fatal("expected an error for an unknown tool")
	}
	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
}
