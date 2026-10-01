package aicommon

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestPlanReferenceBoundaryKindContentAndRestore(t *testing.T) {
	timeline := NewTimeline(nil, nil)
	data := "fixed reference\n  "
	definition := timeline.WrapPlanReferenceForPrompt("PLAN_DEFINITION", data)
	require.Equal(t, definition, timeline.WrapPlanReferenceForPrompt("PLAN_DEFINITION", data))
	require.NotEqual(t, definition, timeline.WrapPlanReferenceForPrompt("PLAN_RUNTIME_STATE", data))
	require.NotEqual(t, definition, timeline.WrapUserInputForPrompt(data))
	require.NotEqual(t, definition, NewTimeline(nil, nil).WrapPlanReferenceForPrompt("PLAN_DEFINITION", data))
	close := definition[strings.LastIndex(definition, "<|"):]
	injected := timeline.WrapPlanReferenceForPrompt("PLAN_DEFINITION", data+close)
	require.NotEqual(t, close, injected[strings.LastIndex(injected, "<|"):], "copied old closer cannot terminate the new record")
	snapshot, err := MarshalTimeline(timeline)
	require.NoError(t, err)
	restored, err := UnmarshalTimeline(snapshot)
	require.NoError(t, err)
	require.Equal(t, definition, restored.WrapPlanReferenceForPrompt("PLAN_DEFINITION", data))
}
