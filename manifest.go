package anycli

import (
	"fmt"
	"sort"

	"github.com/heliohq/anycli/definitions"
	"github.com/heliohq/anycli/internal/registry"
	"github.com/heliohq/anycli/internal/tools"
)

// ToolKind identifies how AnyCLI executes a bundled tool.
type ToolKind string

const (
	// ToolKindCLI wraps an external binary provisioned by the host environment
	// or lazily installed from a pinned direct source.
	ToolKindCLI ToolKind = "cli"
	// ToolKindService executes an in-process API client registered with AnyCLI.
	ToolKindService ToolKind = "service"
)

// ToolManifest is the public, credential-safe discovery shape for one bundled
// tool. It exposes capability metadata only; definitions and injection details
// remain internal to AnyCLI.
type ToolManifest struct {
	Name             Tool
	Kind             ToolKind
	Description      string
	CredentialFields []string
}

// ListTools returns every bundled tool in deterministic name order. It also
// validates the execution contract so a service definition cannot ship without
// an implementation and a CLI definition cannot ship without a binary.
func ListTools() ([]ToolManifest, error) {
	bundled, err := definitions.ListBundled()
	if err != nil {
		return nil, err
	}

	manifests := make([]ToolManifest, 0, len(bundled))
	for _, definition := range bundled {
		manifest, err := manifestFor(definition)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

// kindOf is the single owner of the tool-kind discriminator: it classifies a
// bundled definition and validates its execution contract — a CLI definition
// must name a binary, a service definition must have a registered
// implementation, and unknown types are rejected. Every caller that branches
// on definition.Type (manifestFor, WarmEligibleTools, ResolveToolBinary) goes
// through here so the failure posture cannot silently diverge.
func kindOf(definition *registry.Definition) (ToolKind, error) {
	switch definition.Type {
	case "", string(ToolKindCLI):
		if definition.Binary == "" {
			return "", fmt.Errorf("CLI tool %q has no binary", definition.Name)
		}
		return ToolKindCLI, nil
	case string(ToolKindService):
		if !tools.HasService(definition.Name) {
			return "", fmt.Errorf("service tool %q has no registered implementation", definition.Name)
		}
		return ToolKindService, nil
	default:
		return "", fmt.Errorf("tool %q has unsupported type %q", definition.Name, definition.Type)
	}
}

func manifestFor(definition *registry.Definition) (ToolManifest, error) {
	kind, err := kindOf(definition)
	if err != nil {
		return ToolManifest{}, err
	}

	fieldSet := make(map[string]struct{})
	if definition.Auth != nil {
		for index, binding := range definition.Auth.Credentials {
			if binding.Source.Field == "" {
				return ToolManifest{}, fmt.Errorf("tool %q credential binding %d has no source field", definition.Name, index)
			}
			fieldSet[binding.Source.Field] = struct{}{}
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)

	return ToolManifest{
		Name:             Tool(definition.Name),
		Kind:             kind,
		Description:      definition.Description,
		CredentialFields: fields,
	}, nil
}
