package format

import (
	"encoding/json"
	"strings"
)

type jsonHandler struct{}

func (jsonHandler) Create(fields map[string]string) ([]byte, error) {
	root := make(map[string]interface{})
	for dotPath, val := range fields {
		setNested(root, strings.Split(dotPath, "."), val)
	}
	return json.MarshalIndent(root, "", "  ")
}

func (jsonHandler) Patch(existing []byte, fields map[string]string) ([]byte, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, err
	}
	for dotPath, val := range fields {
		setNested(root, strings.Split(dotPath, "."), val)
	}
	return json.MarshalIndent(root, "", "  ")
}

func (jsonHandler) Remove(existing []byte, fields map[string]string) ([]byte, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(existing, &root); err != nil {
		return nil, err
	}
	for dotPath := range fields {
		deleteNested(root, strings.Split(dotPath, "."))
	}
	return json.MarshalIndent(root, "", "  ")
}

// setNested sets a value at a nested path in a map, creating intermediate maps as needed.
func setNested(m map[string]interface{}, keys []string, val string) {
	for i, k := range keys {
		if i == len(keys)-1 {
			m[k] = val
			return
		}
		child, ok := m[k]
		if !ok {
			child = make(map[string]interface{})
			m[k] = child
		}
		childMap, ok := child.(map[string]interface{})
		if !ok {
			childMap = make(map[string]interface{})
			m[k] = childMap
		}
		m = childMap
	}
}

// deleteNested removes a value at a nested path, pruning empty parent maps.
func deleteNested(m map[string]interface{}, keys []string) {
	if len(keys) == 0 {
		return
	}
	if len(keys) == 1 {
		delete(m, keys[0])
		return
	}
	child, ok := m[keys[0]]
	if !ok {
		return
	}
	childMap, ok := child.(map[string]interface{})
	if !ok {
		return
	}
	deleteNested(childMap, keys[1:])
	// Prune empty parent maps.
	if len(childMap) == 0 {
		delete(m, keys[0])
	}
}
