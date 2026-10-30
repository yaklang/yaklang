package aicommon

import (
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAIHTTPResponseError(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 429, 500, 503} {
		for _, fixture := range []struct{ name, body string }{
			{"nested", `{"error":{"message":"provider detail"}}`},
			{"string", `{"error":"provider detail"}`},
			{"top-level", `{"message":"provider detail"}`},
			{"plain", "provider detail"},
		} {
			for _, chunked := range []bool{false, true} {
				t.Run(fmt.Sprintf("%d/%s/chunked=%v", status, fixture.name, chunked), func(t *testing.T) {
					body := fixture.body
					header := fmt.Sprintf("HTTP/1.1 %d Error\r\n", status)
					if chunked {
						header += "Transfer-Encoding: chunked\r\n"
						body = fmt.Sprintf("%x\r\n%s\r\n0\r\n\r\n", len(body), body)
					}
					rsp := NewAIResponse(nil)
					defer rsp.Close()
					rsp.SetRawHTTPResponseData([]byte(header+"\r\n"), []byte(body))
					require.EqualError(t, aiHTTPResponseError(rsp), fmt.Sprintf("AI provider returned HTTP %d: provider detail", status))
					rsp.SetError(io.ErrUnexpectedEOF)
					err := aiHTTPResponseError(rsp)
					require.ErrorContains(t, err, "provider detail")
					require.ErrorIs(t, err, io.ErrUnexpectedEOF)
				})
			}
		}
	}
}

func TestAIHTTPResponseBodyAlreadyDecoded(t *testing.T) {
	rsp := NewAIResponse(nil)
	defer rsp.Close()
	rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 429 Error\r\nTransfer-Encoding: chunked\r\n\r\n"), []byte(`{"error":{"message":"quota"}}`))
	require.Equal(t, "quota", parse429Body(rsp).Error.Message)
	require.EqualError(t, aiHTTPResponseError(rsp), "AI provider returned HTTP 429: quota")
}
