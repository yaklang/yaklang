package reactloops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestBuildSessionSnapshot_AlwaysEmitsFullPayload(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(), aicommon.WithDisableAutoSkills(true))
	snapshot := BuildSessionSnapshot(cfg, nil, nil)
	require.NotNil(t, snapshot)

	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Contains(t, payload, "revision")
	require.Contains(t, payload, "updated_at")
	require.Contains(t, payload, "execution")
	require.Contains(t, payload, "perception")
	require.Contains(t, payload, "capabilities")
	require.Contains(t, payload, "background_processes")

	var execution map[string]any
	require.NoError(t, json.Unmarshal(payload["execution"], &execution))
	require.Equal(t, "processing", execution["status"])
}
