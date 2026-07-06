package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestSlimGate_YaklangLanguageSuggestion is the CI gate for irify_exclude builds:
// Yak script completion must return non-empty suggestions (not Unimplemented).
func TestSlimGate_YaklangLanguageSuggestion(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	id := uuid.NewString()
	resp, err := local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   COMPLETION,
		YakScriptType: "yak",
		YakScriptCode: `println("hello")`,
		Range: &ypb.Range{
			Code:        "pr",
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   3,
		},
		ModelID: id,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Greater(t, len(resp.SuggestionMessage), 0, "yak completion must work under irify_exclude slim build")
}
