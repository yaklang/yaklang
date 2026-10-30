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

func TestGRPCMUSTPASS_LANGUAGE_AIProviderLibrary(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	query := func(t *testing.T, inspect, code, word string) *ypb.YaklangLanguageSuggestionResponse {
		t.Helper()
		start := strings.Index(code, word) + 1
		resp, err := client.YaklangLanguageSuggestion(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
			InspectType: inspect, YakScriptType: "yak", YakScriptCode: code, ModelID: uuid.NewString(),
			Range: &ypb.Range{Code: word, StartLine: 1, EndLine: 1, StartColumn: int64(start), EndColumn: int64(start + len(word))},
		})
		require.NoError(t, err)
		return resp
	}
	for _, name := range []string{"ReadSSE", "Retry", "IsTransientError"} {
		t.Run(name, func(t *testing.T) {
			// The same ordinary Yak context used by all Yak Monaco editors must
			// discover the library through the engine's public export table.
			resp := query(t, COMPLETION, "ai.", "ai.")
			var names []string
			for _, item := range resp.SuggestionMessage {
				names = append(names, item.Label)
			}
			require.Contains(t, names, name)
			word := "ai." + name
			for _, inspect := range []string{HOVER, SIGNATURE} {
				resp := query(t, inspect, "fn = "+word, word)
				require.NotEmpty(t, resp.SuggestionMessage, inspect)
				require.Contains(t, resp.SuggestionMessage[0].Label, name, inspect)
				if inspect == SIGNATURE {
					require.NotEmpty(t, resp.SuggestionMessage[0].Description)
				}
			}
			item := doc.GetDefaultDocumentHelper().Libs["ai"].Functions[name]
			require.NotNil(t, item)
			require.NotEmpty(t, item.Document)
		})
	}
}
