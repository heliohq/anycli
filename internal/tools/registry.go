package tools

import (
	"context"
	"fmt"
	"os"
	"sort"

	"github.com/heliohq/anycli/internal/tools/execution"
	"github.com/spf13/cobra"
)

// ExecutionResult is the outcome of one built-in service invocation.
type ExecutionResult = execution.Result

// Service is the interface for built-in API client services.
// Each service is a Go package that implements this interface with its own
// cobra command tree for subcommand/flag parsing.
type Service interface {
	// Execute runs the service with the given arguments and credentials.
	// The env map contains resolved credentials (e.g., {"NOTION_TOKEN": "xxx"}).
	Execute(ctx context.Context, args []string, env map[string]string) (ExecutionResult, error)

	// NewCommandTree returns the tool's full cobra command tree built with
	// empty credentials. Credentials are only captured by RunE closures, so
	// an empty token is sufficient for dry-run flag parsing and tree
	// traversal — the returned commands must never be executed. Inspect,
	// lint, and policy coverage tests all take the tree through this seam
	// (design 318).
	NewCommandTree() *cobra.Command
}

// CredentialPatcher handles non-standard credential file formats.
type CredentialPatcher interface {
	// Patch writes credential values to the tool's config file and returns
	// a cleanup function. The cleanup function is called after execution
	// to remove any ephemeral credential data written by Patch.
	// Each call to Patch returns its own independent cleanup handle,
	// so concurrent invocations do not share state.
	Patch(path string, fields map[string]string, mode os.FileMode) (cleanup func() error, err error)
}

var services = map[string]Service{}
var patchers = map[string]CredentialPatcher{}

// RegisterService registers a built-in service implementation.
func RegisterService(name string, svc Service) {
	services[name] = svc
}

// RegisterPatcher registers a custom credential patcher.
func RegisterPatcher(name string, p CredentialPatcher) {
	patchers[name] = p
}

// GetService returns a registered service by name.
func GetService(name string) (Service, error) {
	svc, ok := services[name]
	if !ok {
		return nil, fmt.Errorf("no built-in service registered for %q", name)
	}
	return svc, nil
}

// GetPatcher returns a registered patcher by name.
func GetPatcher(name string) (CredentialPatcher, error) {
	p, ok := patchers[name]
	if !ok {
		return nil, fmt.Errorf("no custom patcher registered for %q", name)
	}
	return p, nil
}

// HasService returns true if a service is registered for the given name.
func HasService(name string) bool {
	_, ok := services[name]
	return ok
}

// ServiceNames returns the registry tool ids of all registered built-in
// services, sorted for deterministic enumeration.
func ServiceNames() []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
