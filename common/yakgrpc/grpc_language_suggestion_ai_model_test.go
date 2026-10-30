package yakgrpc

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_LANGUAGE_AIModelProvider(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	for _, tc := range []struct{ code, word, want string }{
		{"chat = func(req, emit) { req. }", "req.", "Prompt"},
		{"shouldRetry = func(req, failure) { failure. }", "failure.", "RetryAfter"},
		{"ai.", "ai.", "ModelError"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			col := strings.Index(tc.code, tc.word) + len(tc.word) + 1
			resp, err := client.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
				InspectType: COMPLETION, YakScriptType: "ai-model", YakScriptCode: tc.code, ModelID: uuid.NewString(),
				Range: &ypb.Range{Code: tc.word, StartLine: 1, EndLine: 1, StartColumn: int64(col - len(tc.word)), EndColumn: int64(col)},
			})
			require.NoError(t, err)
			var names []string
			for _, item := range resp.SuggestionMessage {
				names = append(names, item.Label)
			}
			require.Contains(t, names, tc.want)
		})
	}
	for _, name := range []string{"ReadSSE", "ModelError", "Retry", "StopRetry", "IsTransientError"} {
		item := doc.GetDefaultDocumentHelper().Libs["ai"].Functions[name]
		require.NotNil(t, item, name)
		require.NotEmpty(t, item.Document, name)
	}
}
