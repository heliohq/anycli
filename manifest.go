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
	// ToolKindCLI wraps an external binary provisioned by the host environment.
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

func manifestFor(definition *registry.Definition) (ToolManifest, error) {
	kind := ToolKindCLI
	switch definition.Type {
	case "", string(ToolKindCLI):
		if definition.Binary == "" {
			return ToolManifest{}, fmt.Errorf("CLI tool %q has no binary", definition.Name)
		}
	case string(ToolKindService):
		kind = ToolKindService
		if !tools.HasService(definition.Name) {
			return ToolManifest{}, fmt.Errorf("service tool %q has no registered implementation", definition.Name)
		}
	default:
		return ToolManifest{}, fmt.Errorf("tool %q has unsupported type %q", definition.Name, definition.Type)
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
