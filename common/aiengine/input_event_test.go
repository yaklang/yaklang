package aiengine

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewInputEvent(t *testing.T) {
	event, err := NewInputEvent(map[string]any{"IsInteractiveMessage": true, "InteractiveId": "review-1", "InteractiveJSONInput": `{"suggestion":"continue"}`})
	require.NoError(t, err)
	require.True(t, event.IsInteractiveMessage)
	require.Equal(t, "review-1", event.InteractiveId)
	require.JSONEq(t, `{"suggestion":"continue"}`, event.InteractiveJSONInput)
	_, err = NewInputEvent(map[string]any{"IsInteractiveMessage": "invalid"})
	require.Error(t, err)
}
