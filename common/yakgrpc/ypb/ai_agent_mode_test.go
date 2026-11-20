package ypb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAgentModeNormalization(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params *AIStartParams
		want   string
	}{
		{"default", &AIStartParams{}, AgentModeSecurity},
		{"coding", &AIStartParams{AgentMode: AgentModeCoding}, AgentModeCoding},
		{"security", &AIStartParams{AgentMode: AgentModeSecurity}, AgentModeSecurity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := proto.Marshal(tc.params)
			require.NoError(t, err)
			decoded := &AIStartParams{}
			require.NoError(t, proto.Unmarshal(wire, decoded))
			require.NoError(t, decoded.NormalizeAgentMode())
			require.Equal(t, tc.want, decoded.AgentMode)
		})
	}
	params := &AIStartParams{AgentMode: "unsupported"}
	require.ErrorContains(t, params.NormalizeAgentMode(), "unsupported agent mode")
}
