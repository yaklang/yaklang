package test

import "testing"

// TestBashUnsupportedFeatures records language features the frontend does not
// model yet. Every case is skipped, so the gap stays visible in `go test -v`
// without turning the suite red. The matching implementation sites carry a
// `TODO(ssa-bash)` comment.
func TestBashUnsupportedFeatures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		script string
		reason string
	}{
		{"errexit_set_e", `set -e; false; echo unreachable`, "TODO(ssa-bash): set -e/errexit does not stop later statements"},
		{"trap_handler", `trap 'echo cleanup' EXIT; echo work`, "TODO(ssa-bash): trap handlers never join the CFG"},
		{"eval_dynamic", `cmd='echo hi'; eval "$cmd"`, "TODO(ssa-bash): eval with a non-constant payload is not inlined"},
		{"background_order", `slow & echo next`, "TODO(ssa-bash): background job completion order is not modeled"},
		{"dynamic_status", `probe; code=$?`, "TODO(ssa-bash): a non-constant $? stays an intrinsic"},
		{"dynamic_array_index", `i=1; a=(x y); echo "${a[$i]}"`, "TODO(ssa-bash): a dynamic subscript stays a member-call value"},
		{"source_path_lookup", `source util.sh`, "TODO(ssa-bash): source outside project files is unresolved"},
		{"heredoc_normalization", "cat <<-EOF\n\tvalue\nEOF", "TODO(ssa-bash): <<- tab stripping is not modeled"},
		{"process_substitution_path", `diff <(echo a) <(echo b)`, "TODO(ssa-bash): process substitution returns stdout, not a path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("script: %s", tc.script)
			t.Skip(tc.reason)
		})
	}
}
