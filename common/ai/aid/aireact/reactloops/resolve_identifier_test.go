package reactloops

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestReActLoop_ResolveIdentifier_FocusedModeLoop(t *testing.T) {
	// vuln_verify is a registered focused mode loop (registered at init time by loop_vuln_verify).
	// It should be resolved as ResolvedAs_FocusedMode, not Unknown.
	loopName := schema.AI_REACT_LOOP_NAME_VULN_VERIFY

	// Verify the loop is actually registered
	_, ok := GetLoopFactory(loopName)
	if !ok {
		t.Skipf("loop %q is not registered (may not have been imported), skipping", loopName)
	}

	// Create a minimal ReActLoop with a Config for the resolver to work
	cfg := &aicommon.Config{}
	loop := &ReActLoop{config: cfg}

	result := loop.ResolveIdentifier(loopName)
	assert.Equal(t, aicommon.ResolvedAs_FocusedMode, result.IdentityType)
	assert.Equal(t, "", result.ActionType, "focused mode has no direct AI action")
	assert.Equal(t, loopName, result.Name)
	assert.Contains(t, result.Suggestion, "Focused Mode Loop")
	assert.Contains(t, result.Suggestion, "NOT a skill")
	assert.False(t, result.IsUnknown())
}

func TestReActLoop_ResolveIdentifier_FocusedMode_PythonPoc(t *testing.T) {
	loopName := schema.AI_REACT_LOOP_NAME_PYTHON_POC

	_, ok := GetLoopFactory(loopName)
	if !ok {
		t.Skipf("loop %q is not registered, skipping", loopName)
	}

	cfg := &aicommon.Config{}
	loop := &ReActLoop{config: cfg}

	result := loop.ResolveIdentifier(loopName)
	assert.Equal(t, aicommon.ResolvedAs_FocusedMode, result.IdentityType)
	assert.False(t, result.IsUnknown())
	assert.Contains(t, result.Suggestion, "CANNOT enter this mode via loading_skills")
}

func TestReActLoop_ResolveIdentifier_TrulyUnknown(t *testing.T) {
	cfg := &aicommon.Config{}
	loop := &ReActLoop{config: cfg}

	result := loop.ResolveIdentifier("absolutely-nonexistent-identifier")
	assert.Equal(t, aicommon.ResolvedAs_Unknown, result.IdentityType)
	assert.True(t, result.IsUnknown())
	assert.Contains(t, result.Suggestion, "does not exist")
}

func TestReActLoop_ResolveIdentifier_FocusedMode_WithMetadata(t *testing.T) {
	// Register a temporary test loop with metadata to verify description is included
	testLoopName := "__test_resolve_focused_loop__"

	// Clean up: we can't unregister, but using a unique name avoids collision
	err := RegisterLoopFactory(testLoopName,
		func(r aicommon.AIInvokeRuntime, opts ...ReActLoopOption) (*ReActLoop, error) {
			return nil, nil
		},
		WithLoopDescription("Test focused mode for vulnerability verification"),
	)
	if err != nil {
		t.Skipf("could not register test loop (already registered): %v", err)
	}

	cfg := &aicommon.Config{}
	loop := &ReActLoop{config: cfg}

	result := loop.ResolveIdentifier(testLoopName)
	assert.Equal(t, aicommon.ResolvedAs_FocusedMode, result.IdentityType)
	assert.Contains(t, result.Suggestion, "Test focused mode for vulnerability verification")
}
