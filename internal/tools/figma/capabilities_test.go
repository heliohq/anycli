package figma

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
)

func TestCapabilitiesReportsPATCoverageAndMCPBoundary(t *testing.T) {
	var got capturedRequest
	server := newTestServer(t, http.StatusOK, `{}`, nil, &got)
	defer server.Close()
	code, stdout, stderr := runService(t, server, "capabilities")
	if code != 0 || stderr != "" {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	var capabilities struct {
		PATOperations      int               `json:"pat_operations"`
		NativeCanvasWrites bool              `json:"native_canvas_writes"`
		PATScopes          []string          `json:"pat_scopes"`
		MCPOnly            []string          `json:"mcp_or_plugin_required"`
		MCPApproximations  map[string]string `json:"mcp_read_approximations"`
	}
	if err := json.Unmarshal([]byte(stdout), &capabilities); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if capabilities.PATOperations != 47 {
		t.Errorf("PAT operations = %d, want 47", capabilities.PATOperations)
	}
	if capabilities.NativeCanvasWrites {
		t.Error("PAT capabilities must not claim native canvas writes")
	}
	if len(capabilities.PATScopes) == 0 {
		t.Errorf("capabilities = %+v", capabilities)
	}
	coveredMCPTools := append([]string{}, capabilities.MCPOnly...)
	for name := range capabilities.MCPApproximations {
		coveredMCPTools = append(coveredMCPTools, name)
	}
	sort.Strings(coveredMCPTools)
	wantMCPTools := strings.Fields(`
		add_code_connect_map create_new_file download_assets generate_diagram
		generate_figma_design get_code_connect_map get_code_connect_suggestions
		get_context_for_code_connect get_design_context get_figjam get_libraries
		get_metadata get_screenshot get_shader_effect get_shader_fill
		get_variable_defs list_shader_effects list_shader_fills
		search_design_system send_code_connect_mappings upload_assets use_figma whoami
	`)
	sort.Strings(wantMCPTools)
	if strings.Join(coveredMCPTools, "\n") != strings.Join(wantMCPTools, "\n") {
		t.Errorf("covered MCP tools = %v, want exact official partition %v", coveredMCPTools, wantMCPTools)
	}
	if got.Path != "" {
		t.Errorf("capabilities sent request to %s", got.Path)
	}
}
