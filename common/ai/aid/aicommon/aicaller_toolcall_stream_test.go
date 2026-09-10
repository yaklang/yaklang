package aicommon

import (
	"context"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestAIChatToAICallbackType_PreservesToolCallArguments(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		action string
	}{
		{
			name:   "formatted JSON with newline before array",
			raw:    "{\r\n\t\"@action\": \"search_logs\",\n\t\"queries\":\n\t[\"ERROR\", \"WARN\"],\n\t\"offset\": 0\n}",
			action: "search_logs",
		},
		{
			// The tolerant action parser also accepts unescaped control bytes.
			name:   "literal multiline argument",
			raw:    "{\"@action\":\"directly_answer\",\"answer_payload\":\"line1\n\tline2\r\nline3\"}",
			action: "directly_answer",
		},
		{
			name:   "escaped multiline argument",
			raw:    `{"@action":"directly_answer","answer_payload":"line1\n\tline2\r\nline3"}`,
			action: "directly_answer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cfg := NewTestConfig(ctx)
			cb := AIChatToAICallbackType(func(_ string, opts ...aispec.AIConfigOption) (string, error) {
				ac := aispec.NewDefaultAIConfig(opts...)
				if ac.ToolCallArgumentsStreamHandler == nil {
					return "", io.ErrUnexpectedEOF
				}
				ac.ToolCallArgumentsStreamHandler(iotest.OneByteReader(strings.NewReader(tc.raw)))
				return "", nil
			})
			resp, err := cb(cfg, NewAIRequest("test", WithAIRequest_EnableToolCallArgumentsStream()))
			require.NoError(t, err)
			got, err := io.ReadAll(resp.GetUnboundStreamReader(false))
			require.NoError(t, err)
			require.NoError(t, resp.GetError())
			require.Equal(t, tc.raw, string(got), "the adapter must preserve tool argument bytes")

			action, err := ExtractValidActionFromStream(ctx, strings.NewReader(string(got)), tc.action)
			require.NoError(t, err)
			if tc.action == "search_logs" {
				require.Equal(t, []string{"ERROR", "WARN"}, action.GetStringSlice("queries"))
			} else {
				require.Equal(t, "line1\n\tline2\r\nline3", action.GetString("answer_payload"))
			}
		})
	}
}
