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

func TestRecordsEqualIgnoringFileFields(t *testing.T) {
	a := map[string]any{"id": 1, "name": "x", "cover": "https://cdn.example/a.jpg"}
	b := map[string]any{"id": 1, "name": "x", "cover": "https://cdn.example/b.jpg"}
	if !recordsEqualIgnoring(a, b, []string{"cover"}) {
		t.Fatal("expected equal when ignoring differing file URLs")
	}
	b["name"] = "y"
	if recordsEqualIgnoring(a, b, []string{"cover"}) {
		t.Fatal("expected different when non-file field differs")
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

func TestPayloadWithoutID(t *testing.T) {
	in := map[string]any{"id": float64(1), "nome": "x", "attivo": true}
	out := payloadWithoutID(in)
	if _, ok := out["id"]; ok {
		t.Fatal("id should be omitted")
	}
	if out["nome"] != "x" || out["attivo"] != true {
		t.Fatalf("fields lost: %#v", out)
	}
	if _, ok := in["id"]; !ok {
		t.Fatal("input must not be mutated")
	}
}
