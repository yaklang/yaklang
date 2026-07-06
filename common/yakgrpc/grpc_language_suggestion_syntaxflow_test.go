//go:build !irify_exclude

package yakgrpc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_SyntaxFlowCompletion(t *testing.T) {
	local, err := NewLocalClient()
	require.NoError(t, err)

	t.Run("native-call", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "completion", "syntaxflow", t, `
<
		`, &ypb.Range{
			Code:        "<",
			StartLine:   2,
			StartColumn: 2,
			EndLine:     2,
			EndColumn:   3,
		}, id)
		require.True(t, len(resp.SuggestionMessage) > 0)
	})

	t.Run("library", func(t *testing.T) {
		id := uuid.NewString()
		resp := GetSuggestion(local, "completion", "syntaxflow", t, `
<include()>
`, &ypb.Range{
			Code:        "<include(",
			StartLine:   2,
			StartColumn: 10,
			EndLine:     2,
			EndColumn:   10,
		}, id)

		require.True(t, len(resp.SuggestionMessage) > 0)
	})
}
