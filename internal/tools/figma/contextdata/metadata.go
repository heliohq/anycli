// Package contextdata converts Figma REST documents into bounded,
// agent-oriented context without depending on HTTP or Cobra.
package contextdata

import (
	"encoding/json"
	"fmt"
	"sort"
)

type Metadata struct {
	FileName  string `json:"file_name,omitempty"`
	Roots     []Node `json:"roots"`
	NodeCount int    `json:"node_count"`
	Truncated bool   `json:"truncated"`
}

type Node struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name,omitempty"`
	Type                   string          `json:"type,omitempty"`
	Visible                *bool           `json:"visible,omitempty"`
	Bounds                 *Bounds         `json:"bounds,omitempty"`
	LayoutMode             string          `json:"layout_mode,omitempty"`
	LayoutSizingHorizontal string          `json:"layout_sizing_horizontal,omitempty"`
	LayoutSizingVertical   string          `json:"layout_sizing_vertical,omitempty"`
	ComponentID            string          `json:"component_id,omitempty"`
	Characters             string          `json:"characters,omitempty"`
	Children               []Node          `json:"children,omitempty"`
	ComponentProperties    json.RawMessage `json:"component_properties,omitempty"`
}

type Bounds struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type responseEnvelope struct {
	Name     string                 `json:"name"`
	Document *rawNode               `json:"document"`
	Nodes    map[string]nodeWrapper `json:"nodes"`
}

type nodeWrapper struct {
	Document *rawNode `json:"document"`
}

type rawNode struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Type                   string          `json:"type"`
	Visible                *bool           `json:"visible"`
	AbsoluteBoundingBox    *Bounds         `json:"absoluteBoundingBox"`
	LayoutMode             string          `json:"layoutMode"`
	LayoutSizingHorizontal string          `json:"layoutSizingHorizontal"`
	LayoutSizingVertical   string          `json:"layoutSizingVertical"`
	ComponentID            string          `json:"componentId"`
	Characters             string          `json:"characters"`
	Children               []rawNode       `json:"children"`
	ComponentProperties    json.RawMessage `json:"componentProperties"`
}

type extractionState struct {
	max       int
	count     int
	truncated bool
}

func ExtractMetadata(raw []byte, maxNodes int) (Metadata, error) {
	if maxNodes <= 0 {
		return Metadata{}, fmt.Errorf("max nodes must be positive")
	}
	var envelope responseEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Metadata{}, fmt.Errorf("decode Figma document: %w", err)
	}
	roots := responseRoots(envelope)
	if len(roots) == 0 {
		return Metadata{}, fmt.Errorf("figma response contains no document nodes")
	}
	state := extractionState{max: maxNodes}
	metadata := Metadata{FileName: envelope.Name}
	for _, root := range roots {
		node, included := extractNode(root, &state)
		if included {
			metadata.Roots = append(metadata.Roots, node)
		}
	}
	metadata.NodeCount = state.count
	metadata.Truncated = state.truncated
	return metadata, nil
}

func responseRoots(envelope responseEnvelope) []rawNode {
	if envelope.Document != nil {
		return []rawNode{*envelope.Document}
	}
	keys := make([]string, 0, len(envelope.Nodes))
	for key := range envelope.Nodes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	roots := make([]rawNode, 0, len(keys))
	for _, key := range keys {
		if envelope.Nodes[key].Document != nil {
			roots = append(roots, *envelope.Nodes[key].Document)
		}
	}
	return roots
}

func extractNode(raw rawNode, state *extractionState) (Node, bool) {
	if state.count >= state.max {
		state.truncated = true
		return Node{}, false
	}
	state.count++
	node := Node{
		ID: raw.ID, Name: raw.Name, Type: raw.Type, Visible: raw.Visible,
		Bounds: raw.AbsoluteBoundingBox, LayoutMode: raw.LayoutMode,
		LayoutSizingHorizontal: raw.LayoutSizingHorizontal,
		LayoutSizingVertical:   raw.LayoutSizingVertical,
		ComponentID:            raw.ComponentID,
		Characters:             raw.Characters,
		ComponentProperties:    raw.ComponentProperties,
	}
	for _, child := range raw.Children {
		converted, included := extractNode(child, state)
		if included {
			node.Children = append(node.Children, converted)
		}
	}
	return node, true
}
