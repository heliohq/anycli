package figma

import (
	"sort"

	"github.com/spf13/cobra"
)

type figmaCapabilities struct {
	Auth                   string            `json:"auth"`
	OperationCatalogSource string            `json:"operation_catalog_source"`
	PATOperations          int               `json:"pat_operations"`
	PATScopes              []string          `json:"pat_scopes"`
	PlanOrOAuthOperations  []string          `json:"plan_or_oauth_operations"`
	FirstClassCommands     bool              `json:"first_class_commands"`
	RawRESTEscapeHatch     bool              `json:"raw_rest_escape_hatch"`
	NativeCanvasWrites     bool              `json:"native_canvas_writes"`
	MCPReadApproximations  map[string]string `json:"mcp_read_approximations"`
	MCPOrPluginRequired    []string          `json:"mcp_or_plugin_required"`
	EntitlementLimitations []string          `json:"entitlement_limitations"`
}

func (s *Service) newCapabilitiesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Describe PAT coverage and the hosted-MCP boundary",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			catalog, err := loadOperationCatalog()
			if err != nil {
				return err
			}
			return s.emitJSON(capabilitiesFromCatalog(catalog))
		},
	}
}

func capabilitiesFromCatalog(catalog *operationCatalog) figmaCapabilities {
	scopes := map[string]struct{}{}
	unsupported := make([]string, 0)
	patOperations := 0
	for _, operation := range catalog.Operations {
		if !operation.PAT {
			unsupported = append(unsupported, operation.ID)
			continue
		}
		patOperations++
		for _, scope := range operation.Scopes {
			scopes[scope] = struct{}{}
		}
	}
	patScopes := make([]string, 0, len(scopes))
	for scope := range scopes {
		patScopes = append(patScopes, scope)
	}
	sort.Strings(patScopes)
	sort.Strings(unsupported)
	return figmaCapabilities{
		Auth:                   "personal_access_token",
		OperationCatalogSource: catalog.Source,
		PATOperations:          patOperations,
		PATScopes:              patScopes,
		PlanOrOAuthOperations:  unsupported,
		FirstClassCommands:     true,
		RawRESTEscapeHatch:     true,
		NativeCanvasWrites:     false,
		MCPReadApproximations: map[string]string{
			"download_assets":      "assets download and assets download-fills",
			"get_design_context":   "context design returns exact REST nodes, renders, and optional variables without Figma-hosted code generation",
			"get_figjam":           "context figjam returns exact REST nodes and renders",
			"get_libraries":        "libraries commands enumerate assets for known team or file IDs",
			"get_metadata":         "context metadata returns a bounded sparse node tree",
			"get_screenshot":       "context screenshot returns official render URLs",
			"get_variable_defs":    "context variables returns Enterprise Variables REST data",
			"search_design_system": "library assets are searchable by the agent after enumeration; PAT REST cannot discover every subscribed library",
			"whoami":               "me returns the REST user profile; PAT REST does not return every plan and seat",
		},
		MCPOrPluginRequired: []string{
			"add_code_connect_map",
			"create_new_file",
			"generate_diagram",
			"generate_figma_design",
			"get_code_connect_map",
			"get_code_connect_suggestions",
			"get_context_for_code_connect",
			"get_shader_effect",
			"get_shader_fill",
			"list_shader_effects",
			"list_shader_fills",
			"send_code_connect_mappings",
			"upload_assets",
			"use_figma",
		},
		EntitlementLimitations: []string{
			"Variables read/write and library analytics require eligible Enterprise access in addition to PAT scopes.",
			"A PAT cannot call plan-token-only developer logs or AI usage, and organization endpoints may require OAuth.",
			"File, team, project, seat, and plan permissions still apply after a scope is granted.",
		},
	}
}
