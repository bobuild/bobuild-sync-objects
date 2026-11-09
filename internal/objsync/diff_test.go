package objsync

import (
	"encoding/json"
	"testing"
)

func TestRecordID(t *testing.T) {
	cases := []struct {
		in map[string]any
		id int
		ok bool
	}{
		{map[string]any{"id": float64(3)}, 3, true},
		{map[string]any{"id": 7}, 7, true},
		{map[string]any{"id": json.Number("9")}, 9, true},
		{map[string]any{"id": nil}, 0, false},
		{map[string]any{}, 0, false},
	}
	for i, c := range cases {
		id, ok := recordID(c.in)
		if id != c.id || ok != c.ok {
			t.Fatalf("case %d: got %d,%v want %d,%v", i, id, ok, c.id, c.ok)
		}
	}
}

func TestRecordsEqualNormalizesNumbers(t *testing.T) {
	a := map[string]any{"id": float64(1), "n": json.Number("2"), "name": "x"}
	b := map[string]any{"id": 1, "n": 2, "name": "x"}
	if !recordsEqual(a, b) {
		t.Fatal("expected equal after normalization")
	}
	b["name"] = "y"
	if recordsEqual(a, b) {
		t.Fatal("expected different")
	}
}

func TestIndexByID(t *testing.T) {
	rows := []map[string]any{
		{"id": float64(1), "n": "a"},
		{"n": "no-id"},
		{"id": float64(2), "n": "b"},
	}
	byID, invalid := indexByID(rows)
	if len(byID) != 2 || len(invalid) != 1 {
		t.Fatalf("byID=%d invalid=%d", len(byID), len(invalid))
	}
}
