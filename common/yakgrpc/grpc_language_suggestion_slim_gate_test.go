//go:build irify_exclude

package yakgrpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// TestSlimGate_YaklangLanguageSuggestion is the CI gate for irify_exclude builds:
// Yak script completion must return non-empty suggestions (not Unimplemented).
func TestSlimGate_YaklangLanguageSuggestion(t *testing.T) {
	local, err := NewLocalClient(true)
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

func TestSlimGate_FuzzTagSuggestion(t *testing.T) {
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	resp, err := local.FuzzTagSuggestion(context.Background(), &ypb.FuzzTagSuggestionRequest{
		InspectType:  COMPLETION,
		FuzztagCode:  "{{",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Greater(t, len(resp.SuggestionMessage), 0, "fuzztag completion must work under irify_exclude slim build")
}

func TestSlimGate_YaklangLanguageFind(t *testing.T) {
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	id := uuid.NewString()
	resp, err := local.YaklangLanguageFind(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   DEFINITION,
		YakScriptType: "yak",
		YakScriptCode: `println("hello")`,
		Range: &ypb.Range{
			Code:        "println",
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   8,
		},
		ModelID: id,
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestSlimGate_SyntaxFlowLanguageSuggestion_Degraded(t *testing.T) {
	local, err := NewLocalClient(true)
	require.NoError(t, err)

	_, err = local.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType:   COMPLETION,
		YakScriptType: "syntaxflow",
		YakScriptCode: `alert $a`,
		Range: &ypb.Range{
			Code:        "al",
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   3,
		},
		ModelID: uuid.NewString(),
	})
	require.Error(t, err)
	st, ok := grpcstatus.FromError(err)
	require.True(t, ok)
	require.NotEqual(t, codes.Unimplemented, st.Code(), "syntaxflow must degrade gracefully, not return gRPC Unimplemented")
}
