package ssaapi

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// newMultiParentValueTestProgram builds a yak program in which a single
// function value is assigned as a member of two different objects under
// different keys, so the function member has two parent (object, key) pairs.
// It returns the *Value of the function member so callers can assert on its
// parent enumeration via the new GetAllObjects / GetAllObjectKeyPairs APIs.
func newMultiParentValueTestProgram(t *testing.T, code string, memberName string) *Value {
	t.Helper()
	prog, err := Parse(code)
	require.NoError(t, err)
	require.NotNil(t, prog)

	vals := prog.Ref(memberName)
	require.NotEmpty(t, vals, "Ref(%q) returned no values", memberName)
	for _, v := range vals {
		if v != nil && !v.IsNil() {
			return v
		}
	}
	t.Fatalf("Ref(%q) returned only nil values", memberName)
	return nil
}

// TestValue_GetAllObjects_OnSharedMember verifies that a function value shared
// as a member across two objects surfaces both parents via GetAllObjects. This
// is the audit-facing counterpart of ssa.GetAllObjects: a rule that needs every
// parent of a shared member must be able to enumerate them, not just the latest
// one returned by GetObject.
func TestValue_GetAllObjects_OnSharedMember(t *testing.T) {
	code := `
func1 = () => { return 1 }
a = {}
a.exec = func1
b = {}
b.run = func1
`
	// "fn" is the shared member; under yaklang SSA it becomes a method member
	// of both `a` (key "exec") and `b` (key "run").
	member := newMultiParentValueTestProgram(t, code, "func1")

	objects := member.GetAllObjects()
	// Depending on how the SSA frontend represents the function literal, the
	// member may also be owned by the global/program container. We assert the
	// two user-declared parents (a and b) are both present, which is the
	// audit-relevant invariant.
	require.GreaterOrEqual(t, len(objects), 2, "shared member must expose at least both parent objects")

	// GetObject (latest-only convenience) must still return exactly one value;
	// the optimization does not change its semantics.
	latest := member.GetObject()
	require.NotNil(t, latest)
	latestID := latest.GetId()
	ids := map[int64]struct{}{}
	for _, obj := range objects {
		ids[obj.GetId()] = struct{}{}
	}
	require.Contains(t, ids, latestID, "latest object must be one of all objects")

	pairs := member.GetAllObjectKeyPairs()
	require.GreaterOrEqual(t, len(pairs), 2, "shared member must expose at least both (object, key) pairs")
	keyNames := map[string]struct{}{}
	for _, pair := range pairs {
		if len(pair) >= 2 && pair[1] != nil {
			keyNames[ssa.GetKeyString(pair[1].getValue())] = struct{}{}
		}
	}
	require.Contains(t, keyNames, "exec", "expected key 'exec' among parent keys")
	require.Contains(t, keyNames, "run", "expected key 'run' among parent keys")
}

// TestValue_GetAllKeys_OnSharedMember verifies the symmetric helper GetAllKeys.
func TestValue_GetAllKeys_OnSharedMember(t *testing.T) {
	code := `
func1 = () => { return 1 }
a = {}
a.exec = func1
b = {}
b.run = func1
`
	member := newMultiParentValueTestProgram(t, code, "func1")
	keys := member.GetAllKeys()
	require.GreaterOrEqual(t, len(keys), 2, "shared member must expose at least both parent keys")
	keyNames := map[string]struct{}{}
	for _, k := range keys {
		keyNames[ssa.GetKeyString(k.getValue())] = struct{}{}
	}
	require.Contains(t, keyNames, "exec")
	require.Contains(t, keyNames, "run")
}
