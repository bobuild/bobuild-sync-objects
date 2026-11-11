package objsync

import (
	"encoding/json"
	"reflect"
)

func recordID(rec map[string]any) (int, bool) {
	v, ok := rec["id"]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

// recordsEqual compares two API payloads after normalizing JSON number types
// and treating missing keys the same as null.
func recordsEqual(a, b map[string]any) bool {
	return reflect.DeepEqual(normalizeMap(a), normalizeMap(b))
}

// recordsEqualIgnoring compares payloads after dropping ignore keys (e.g. file fields).
func recordsEqualIgnoring(a, b map[string]any, ignore []string) bool {
	return recordsEqual(omitKeys(a, ignore...), omitKeys(b, ignore...))
}

// omitKeys returns a shallow copy of in without the listed keys.
func omitKeys(in map[string]any, keys ...string) map[string]any {
	if len(keys) == 0 {
		return in
	}
	skip := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		skip[k] = struct{}{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		if _, ok := skip[k]; ok {
			continue
		}
		out[k] = v
	}
	return out
}

func normalizeMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = normalizeValue(v)
	}
	return out
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return float64(i)
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case float32:
		return float64(x)
	case map[string]any:
		return normalizeMap(x)
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return x
	}
}

// indexByID builds id → record; rows without a usable id are returned separately.
func indexByID(rows []map[string]any) (map[int]map[string]any, []map[string]any) {
	byID := make(map[int]map[string]any, len(rows))
	var invalid []map[string]any
	for _, row := range rows {
		id, ok := recordID(row)
		if !ok {
			invalid = append(invalid, row)
			continue
		}
		byID[id] = row
	}
	return byID, invalid
}
