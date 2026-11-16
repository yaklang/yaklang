package ypb

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestAgentModeCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params *AIStartParams
		want   string
	}{
		{"default", &AIStartParams{}, AgentModeSecurity},
		{"legacy coding", &AIStartParams{CodingMode: true}, AgentModeCoding},
		{"legacy security", &AIStartParams{CodingMode: false}, AgentModeSecurity},
		{"coding", &AIStartParams{AgentMode: AgentModeCoding}, AgentModeCoding},
		{"explicit security wins", &AIStartParams{AgentMode: AgentModeSecurity, CodingMode: true}, AgentModeSecurity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire, err := proto.Marshal(tc.params)
			require.NoError(t, err)
			decoded := &AIStartParams{}
			require.NoError(t, proto.Unmarshal(wire, decoded))
			require.NoError(t, decoded.NormalizeAgentMode())
			require.Equal(t, tc.want, decoded.AgentMode)
			require.Equal(t, tc.want == AgentModeCoding, decoded.CodingMode)
		})
	}
	params := &AIStartParams{AgentMode: "unsupported", CodingMode: true}
	require.ErrorContains(t, params.NormalizeAgentMode(), "unsupported agent mode")
}
