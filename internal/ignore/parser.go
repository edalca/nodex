package ignore

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"
)

// decodeDocument reads one schema-1 document and returns its preset
// identifiers and exclude patterns in document order. It does not compile
// the patterns and does not sort the preset identifiers.
func decodeDocument(data []byte) ([]string, []string, error) {
	if !utf8.Valid(data) {
		return nil, nil, ErrInvalidUTF8
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, nil, fmt.Errorf("ignore document: empty")
	}
	if trimmed[0] != '{' {
		return nil, nil, ErrNotObject
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	var fields map[string]json.RawMessage
	if err := dec.Decode(&fields); err != nil {
		return nil, nil, fmt.Errorf("ignore document: %w", err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, nil, ErrTrailingData
		}
		return nil, nil, fmt.Errorf("%w: %v", ErrTrailingData, err)
	}

	var unknown []string
	for key := range fields {
		if key == "schema" || key == "presets" || key == "exclude" {
			continue
		}
		unknown = append(unknown, key)
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, nil, &UnknownFieldError{Name: unknown[0]}
	}

	schema, ok := fields["schema"]
	if !ok {
		return nil, nil, ErrMissingSchema
	}
	if err := validateSchema(schema); err != nil {
		return nil, nil, err
	}

	presetsRaw, ok := fields["presets"]
	if !ok {
		return nil, nil, ErrMissingPresets
	}
	presets, err := decodePresets(presetsRaw)
	if err != nil {
		return nil, nil, err
	}

	exclude, ok := fields["exclude"]
	if !ok {
		return nil, nil, ErrMissingExclude
	}
	patterns, err := decodeExclude(exclude)
	if err != nil {
		return nil, nil, err
	}
	return presets, patterns, nil
}

// decodePresets accepts a JSON array of strings. Null is reported separately
// from a missing field and from any other JSON type. Duplicate identifiers
// are rejected. The result keeps document order.
func decodePresets(raw json.RawMessage) ([]string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, ErrNullPresets
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, ErrPresetsType
	}
	ids := make([]string, 0, len(items))
	for i, item := range items {
		literal := bytes.TrimSpace(item)
		if len(literal) == 0 || literal[0] != '"' {
			return nil, &PresetError{Index: i, Err: fmt.Errorf("preset is not a string")}
		}
		var id string
		if err := json.Unmarshal(literal, &id); err != nil {
			return nil, &PresetError{Index: i, Err: fmt.Errorf("preset is not a string")}
		}
		ids = append(ids, id)
	}
	if err := validatePresetIDs(ids); err != nil {
		return nil, err
	}
	return ids, nil
}

// validatePresetIDs rejects an empty identifier or a repeated identifier.
// It does not interpret the identifier.
func validatePresetIDs(ids []string) error {
	seen := make(map[string]struct{}, len(ids))
	for i, id := range ids {
		if id == "" {
			return &PresetError{Index: i, Err: fmt.Errorf("preset is empty")}
		}
		if _, ok := seen[id]; ok {
			return &PresetError{Index: i, ID: id, Err: ErrDuplicatePreset}
		}
		seen[id] = struct{}{}
	}
	return nil
}

// validateSchema accepts only the JSON integer token for SchemaVersion.
// A float, an exponent, a string, or any other JSON value is unsupported.
func validateSchema(raw json.RawMessage) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte(strconv.Itoa(SchemaVersion))) {
		return nil
	}
	return ErrUnsupportedSchema
}

// decodeExclude accepts a JSON array of strings. Null is reported separately
// from a missing field and from any other JSON type.
func decodeExclude(raw json.RawMessage) ([]string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, ErrNullExclude
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, ErrExcludeType
	}
	patterns := make([]string, 0, len(items))
	for i, item := range items {
		literal := bytes.TrimSpace(item)
		if len(literal) == 0 || literal[0] != '"' {
			return nil, &RuleError{Index: i, Err: fmt.Errorf("pattern is not a string")}
		}
		var pattern string
		if err := json.Unmarshal(literal, &pattern); err != nil {
			return nil, &RuleError{Index: i, Err: fmt.Errorf("pattern is not a string")}
		}
		if pattern == "" {
			return nil, &RuleError{Index: i, Err: fmt.Errorf("pattern is empty")}
		}
		patterns = append(patterns, pattern)
	}
	return patterns, nil
}
