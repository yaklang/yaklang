package aispec

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamHTTPErrorIsNotEmptySuccess(t *testing.T) {
	for name, process := range map[string]func([]byte, io.ReadCloser, io.Writer, io.Writer, io.Writer, func([]*ToolCall), RawHTTPResponseHeaderCallback, func([]byte, []byte, *ChatUsage), func(*ChatUsage)) error{
		"chat": processAIResponse, "responses": processAIResponseForResponses,
	} {
		for _, status := range []int{400, 401, 403, 500, 502} {
			t.Run(fmt.Sprintf("%s/%d", name, status), func(t *testing.T) {
				const body = `{"error":{"message":"Invalid schema for function: required is null"}}`
				var output, reason bytes.Buffer
				var captured []byte
				err := process([]byte(fmt.Sprintf("HTTP/1.1 %d Error\r\nContent-Type: application/json\r\n\r\n", status)), io.NopCloser(strings.NewReader(body)), &output, &reason, nil, nil, nil, func(_, b []byte, _ *ChatUsage) { captured = append([]byte(nil), b...) }, nil)
				require.Error(t, err)
				require.Contains(t, err.Error(), fmt.Sprint(status))
				require.Contains(t, err.Error(), "Invalid schema")
				require.Equal(t, body, string(captured))
				require.Empty(t, output.String())
			})
		}
	}
}
