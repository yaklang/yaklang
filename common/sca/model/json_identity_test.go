package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestIdentityJSONMatchesStandardEncoder(t *testing.T) {
	values := []string{"", "ascii/path@1:2", "<>&", "\"\\", "\x00\n", "\u2028\u2029", "中文", "\xff", strings.Repeat("long", 500)}
	for _, s := range values {
		for mask := 0; mask < 128; mask++ {
			fields := [7]string{}
			for i := range fields {
				if mask&(1<<i) != 0 {
					fields[i] = s
				}
			}
			k := ComponentKey{fields[0], fields[1], fields[2], fields[3], fields[4], fields[5], fields[6]}
			want, _ := json.Marshal(k)
			sum := sha256.Sum256(want)
			if k.ID() != hex.EncodeToString(sum[:]) {
				t.Fatalf("component identity changed: %q %d", s, mask)
			}
			var scratch [512]byte
			if got, ok := componentJSON(scratch[:0], k); ok && !bytes.Equal(got, want) {
				t.Fatalf("component JSON differs: %s / %s", got, want)
			}
			o := Observation{Condition: fields[0], Scope: fields[1], Component: fields[2], Snapshot: fields[3], Project: fields[4], File: fields[5], NativeID: fields[6], Kind: s, DeclaredIntegrity: fields[0], StartLine: mask - 64, EndLine: mask}
			want, _ = json.Marshal(o)
			sum = sha256.Sum256(want)
			if o.ID() != hex.EncodeToString(sum[:]) {
				t.Fatalf("observation identity changed: %q %d", s, mask)
			}
			if got, ok := observationJSON(scratch[:0], o); ok && !bytes.Equal(got, want) {
				t.Fatalf("observation JSON differs: %s / %s", got, want)
			}
			for _, refs := range [][]string{nil, {}, {s}, {s, "other"}} {
				q := Requirement{Candidates: refs, From: fields[0], Target: fields[1], Constraint: fields[2], Resolved: refs, Scope: fields[3], Condition: fields[4], Group: fields[5], Operator: fields[6]}
				want, _ := json.Marshal(q)
				if got, ok := requirementJSON(scratch[:0], q); ok && !bytes.Equal(got, want) {
					t.Fatalf("requirement JSON differs: %s / %s", got, want)
				}
			}

		}
	}
	for _, provides := range [][]string{nil, {}, {"a"}} {
		o := Observation{Provides: provides}
		want, _ := json.Marshal(o)
		sum := sha256.Sum256(want)
		if o.ID() != hex.EncodeToString(sum[:]) {
			t.Fatal("provides identity changed")
		}
	}
}
