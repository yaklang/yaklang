package rf

import (
	"reflect"
	"testing"
)

func leaf(labels map[string]int) *TreeNode { return &TreeNode{Labels: labels} }
func TestTreeSplitCompatibility(t *testing.T) {
	for _, tc := range []struct {
		threshold, input any
		want             string
	}{
		{float64(2), float64(1), "left"}, {float64(2), float64(2), "left"}, {float64(2), float64(3), "right"},
		{"cat", "cat", "left"}, {"cat", "dog", "right"},
	} {
		tree := &Tree{Root: &TreeNode{ColumnNo: 0, Value: tc.threshold, Left: leaf(map[string]int{"left": 1}), Right: leaf(map[string]int{"right": 1})}}
		if got := PredicateTree(tree, []interface{}{tc.input}); !reflect.DeepEqual(got, map[string]int{tc.want: 1}) {
			t.Fatalf("%v: %v", tc, got)
		}
	}
}
func TestForestNormalizedVotes(t *testing.T) {
	// Each tree contributes one normalized vote, regardless of leaf sample count.
	f := &Forest{Trees: []*Tree{{Root: leaf(map[string]int{"a": 100})}, {Root: leaf(map[string]int{"b": 2})}, {Root: leaf(map[string]int{"b": 1})}}}
	if got := f.Predicate(nil); got != "b" {
		t.Fatalf("got %q", got)
	}
	// Upstream tie order is unspecified. Do not introduce a deterministic contract.
	f.Trees = f.Trees[:2]
	if got := f.Predicate(nil); got != "a" && got != "b" {
		t.Fatalf("invalid tie winner %q", got)
	}
}
func TestClassificationTraining(t *testing.T) {
	inputs := [][]interface{}{{float64(0)}, {float64(0)}, {float64(1)}, {float64(1)}}
	labels := []string{"zero", "zero", "one", "one"}
	// All samples are supplied directly to remove bootstrap randomness from this check.
	tree := &Tree{Root: buildTree(inputs, labels, 1)}
	for i, v := range inputs {
		if got := PredicateTree(tree, v); got[labels[i]] != 2 {
			t.Fatalf("%v: %v", v, got)
		}
	}
	// Exercise the parallel training entrypoint on a homogeneous population.
	forest := BuildForest(inputs, []string{"same", "same", "same", "same"}, 3, 8, 1)
	if len(forest.Trees) != 3 || forest.Predicate(inputs[0]) != "same" {
		t.Fatal("training failed")
	}
}
