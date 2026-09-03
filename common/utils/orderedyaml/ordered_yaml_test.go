package orderedyaml

import (
	"strings"
	"testing"
	"time"
)

func TestMapSliceV3ScalarTypesAndOrder(t *testing.T) {
	var got MapSlice
	input := []byte("first: 1\n2026-09-01: date-key\nwhen: 2026-09-01T08:30:00Z\n")
	if err := Unmarshal(input, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Key != "first" || got[0].Value != 1 {
		t.Fatalf("unexpected ordered mapping: %#v", got)
	}
	if _, ok := got[1].Key.(time.Time); !ok {
		t.Fatalf("timestamp-shaped map key should follow yaml.v3 semantics, got %T", got[1].Key)
	}
	if _, ok := got[2].Value.(time.Time); !ok {
		t.Fatalf("timestamp value should follow yaml.v3 semantics, got %T", got[2].Value)
	}

	encoded, err := Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	first := strings.Index(text, "first: 1")
	dateKey := strings.Index(text, "2026-09-01T00:00:00Z: date-key")
	when := strings.Index(text, "when: 2026-09-01T08:30:00Z")
	if first < 0 || dateKey <= first || when <= dateKey {
		t.Fatalf("order or yaml.v3 timestamp representation changed:\n%s", encoded)
	}
}

func TestMapSlicePreservesNonStringKeys(t *testing.T) {
	var got MapSlice
	if err := Unmarshal([]byte("1: one\ntrue: enabled\n"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Key != 1 || got[1].Key != true {
		t.Fatalf("non-string key types changed: %#v", got)
	}
}
