package config

import (
	"encoding/json"
	"fmt"
	"strings"

	toon "github.com/toon-format/toon-go"

	"github.com/qjebbs/go-jsons"
)

const formatTOON jsons.Format = "toon"

// newConfigMerger creates a jsons.Merger that can parse both JSON and TOON
// config files. The merger's built-in JSON loader handles .json files;
// the registered TOON loader handles .toon files. When raw []byte is passed
// (no file extension), the merger tries all loaders in turn.
func newConfigMerger() *jsons.Merger {
	m := jsons.NewMerger()
	_ = m.RegisterLoader(
		formatTOON,
		[]string{".toon"},
		func(data []byte) (map[string]any, error) {
			decoded, err := toon.Decode(data)
			if err != nil {
				return nil, err
			}
			m, ok := decoded.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("toon: expected top-level object, got %T", decoded)
			}
			return m, nil
		},
	)
	return m
}

// parseConfigBytes parses config file bytes that may be either TOON or JSON
// into a map. It tries TOON first, then falls back to JSON.
func parseConfigBytes(data []byte) (map[string]any, error) {
	// Try TOON first.
	decoded, err := toon.Decode(data)
	if err == nil {
		if m, ok := decoded.(map[string]any); ok {
			return m, nil
		}
	}
	// Fall back to JSON.
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse config as TOON or JSON: %w", err)
	}
	return m, nil
}

// marshalTOON marshals a map to TOON bytes.
func marshalTOON(m map[string]any) ([]byte, error) {
	return toon.Marshal(m)
}

// normalizeValue converts a Go value (which may be a struct) into a
// map[string]any / []any / primitive suitable for insertion into a TOON map.
// It does this via a JSON round-trip so that json struct tags are respected.
func normalizeValue(value any) any {
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var normalized any
	if err := json.Unmarshal(data, &normalized); err != nil {
		return value
	}
	return normalized
}

// mapGet retrieves a value at a dot-separated path in a nested map.
func mapGet(m map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	current := any(m)
	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// mapSet sets a value at a dot-separated path, creating intermediate maps as
// needed. It returns the (possibly new) root map.
func mapSet(m map[string]any, path string, value any) map[string]any {
	if m == nil {
		m = make(map[string]any)
	}
	parts := strings.Split(path, ".")
	current := m
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			child := make(map[string]any)
			current[part] = child
			current = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			child = make(map[string]any)
			current[part] = child
		}
		current = child
	}
	current[parts[len(parts)-1]] = value
	return m
}

// mapDelete removes a value at a dot-separated path. It returns the root map.
func mapDelete(m map[string]any, path string) map[string]any {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		delete(m, parts[0])
		return m
	}
	current := m
	for _, part := range parts[:len(parts)-1] {
		next, ok := current[part]
		if !ok {
			return m
		}
		child, ok := next.(map[string]any)
		if !ok {
			return m
		}
		current = child
	}
	delete(current, parts[len(parts)-1])
	return m
}
