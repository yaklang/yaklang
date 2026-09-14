//go:build !irify_exclude

package sfbuildin

import (
	"testing"

	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
)

func ids(infos []sfdb.RuleInfo) []string {
	out := make([]string, 0, len(infos))
	for _, info := range infos {
		out = append(out, info.RuleID)
	}
	return out
}

// An unchanged regeneration must reproduce the committed file byte for byte,
// otherwise CI produces a cosmetic diff and keeps opening bot commits.
func TestOrderByBaselineKeepsExistingOrder(t *testing.T) {
	baseline := []string{"c-1", "a-2", "b-3"}
	generated := []sfdb.RuleInfo{{RuleID: "a-2"}, {RuleID: "b-3"}, {RuleID: "c-1"}}

	got := ids(orderByBaseline(generated, baseline))
	want := []string{"c-1", "a-2", "b-3"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// Rules added by the run are appended after the existing ones, and rules deleted
// from the tree disappear instead of leaving stale entries. The caller hands over
// rule-id sorted input, so the appended tail is deterministic as well.
func TestOrderByBaselineAppendsNewAndDropsRemoved(t *testing.T) {
	baseline := []string{"z-1", "m-2"}
	generated := []sfdb.RuleInfo{{RuleID: "a-new"}, {RuleID: "b-new"}, {RuleID: "z-1"}}

	got := ids(orderByBaseline(generated, baseline))
	want := []string{"z-1", "a-new", "b-new"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// An empty baseline (first generation) keeps the caller's deterministic rule-id order.
func TestOrderByBaselineWithoutBaseline(t *testing.T) {
	generated := []sfdb.RuleInfo{{RuleID: "d-2"}, {RuleID: "k-1"}}

	got := ids(orderByBaseline(generated, nil))
	if got[0] != "d-2" || got[1] != "k-1" {
		t.Fatalf("got %v", got)
	}
}
