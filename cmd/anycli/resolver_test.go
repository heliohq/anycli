package main

import (
	"context"
	"testing"
)

func TestEnvResolver_CollectsPrefixedVars(t *testing.T) {
	r := newEnvResolver([]string{
		"ANYCLI_CRED_ACCESS_TOKEN=tok-123",
		"ANYCLI_CRED_ACCOUNT_KEY=W123",
		"PATH=/usr/bin",
		"ANYCLI_CREDX_IGNORED=nope",
	})
	cred, err := r.Resolve(context.Background(), "slack", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := map[string]string{"access_token": "tok-123", "account_key": "W123"}
	if len(cred.Data) != len(want) {
		t.Fatalf("Data = %v, want %v", cred.Data, want)
	}
	for k, v := range want {
		if cred.Data[k] != v {
			t.Errorf("Data[%q] = %q, want %q", k, cred.Data[k], v)
		}
	}
	if cred.CacheUntil.IsZero() {
		t.Error("CacheUntil must be set so the engine caches for the process lifetime")
	}
}

func TestEnvResolver_ErrorWhenNoCredentialVars(t *testing.T) {
	r := newEnvResolver([]string{"PATH=/usr/bin"})
	if _, err := r.Resolve(context.Background(), "slack", ""); err == nil {
		t.Fatal("expected error when no ANYCLI_CRED_* variables are set")
	}
}

func TestEnvResolver_SkipsEmptyValues(t *testing.T) {
	r := newEnvResolver([]string{"ANYCLI_CRED_ACCESS_TOKEN="})
	if _, err := r.Resolve(context.Background(), "slack", ""); err == nil {
		t.Fatal("expected error: empty values do not count as credentials")
	}
}
