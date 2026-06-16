package loop_http_flow_analyze

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestFormatAttachedHTTPFlowInline(t *testing.T) {
	flow := &schema.HTTPFlow{
		Url:        "https://example.com/api",
		Method:     "GET",
		StatusCode: 200,
		SourceType: schema.HTTPFlow_SourceType_MITM,
		Tags:       "test|demo",
	}
	flow.ID = 42
	flow.SetRequest("GET /api HTTP/1.1\r\nHost: example.com\r\n\r\n")
	flow.SetResponse("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nok")

	out := formatAttachedHTTPFlow(flow, nil)
	require.Contains(t, out, "HTTP Flow #42")
	require.Contains(t, out, "Method: GET")
	require.Contains(t, out, "StatusCode: 200")
	require.Contains(t, out, "Tags: test|demo")
	require.Contains(t, out, "GET /api HTTP/1.1")
	require.Contains(t, out, "HTTP/1.1 200 OK")
	require.NotContains(t, out, "exceeds inline limit")
}

func TestFormatAttachedHTTPFlowSpillToFile(t *testing.T) {
	largeReq := strings.Repeat("R", aicommon.AttachedHTTPFlowRequestInlineLimit+128)
	largeRsp := strings.Repeat("S", aicommon.AttachedHTTPFlowResponseInlineLimit+256)

	flow := &schema.HTTPFlow{
		Url:        "https://example.com/large",
		Method:     "POST",
		StatusCode: 500,
	}
	flow.ID = 99
	flow.SetRequest(largeReq)
	flow.SetResponse(largeRsp)

	out := formatAttachedHTTPFlow(flow, nil)
	require.Contains(t, out, "request length")
	require.Contains(t, out, "response length")
	require.Contains(t, out, "full content saved to file")
	require.Contains(t, out, strings.Repeat("R", 64))
	require.Contains(t, out, strings.Repeat("S", 64))
}

func TestFormatAttachedHTTPFlowMetadata(t *testing.T) {
	flow := &schema.HTTPFlow{
		Url:           "https://example.com/test",
		Method:        "POST",
		StatusCode:    201,
		RequestLength: 1024,
		BodyLength:    2048,
		ContentType:   "application/json",
		Tags:          "api|test",
		Host:          "example.com",
		IPAddress:     "192.168.1.1",
	}
	flow.ID = 123

	metadata := formatAttachedHTTPFlowMetadata(flow)
	require.Contains(t, metadata, "ID: 123")
	require.Contains(t, metadata, "Method: POST")
	require.Contains(t, metadata, "StatusCode: 201")
	require.Contains(t, metadata, "RequestLength: 1024")
	require.Contains(t, metadata, "BodyLength: 2048")
	require.Contains(t, metadata, "ContentType: application/json")
	require.Contains(t, metadata, "Tags: api|test")
	require.Contains(t, metadata, "Host: example.com")
	require.Contains(t, metadata, "IPAddress: 192.168.1.1")
}

func TestAttachedHTTPFlowRequest(t *testing.T) {
	flow := &schema.HTTPFlow{}
	flow.SetRequest("GET / HTTP/1.1\r\n\r\n")

	req := attachedHTTPFlowRequest(flow)
	require.Equal(t, "GET / HTTP/1.1\r\n\r\n", req)

	// Test nil flow
	require.Empty(t, attachedHTTPFlowRequest(nil))
}

func TestAttachedHTTPFlowResponse(t *testing.T) {
	flow := &schema.HTTPFlow{}
	flow.SetResponse("HTTP/1.1 200 OK\r\n\r\n")

	rsp := attachedHTTPFlowResponse(flow)
	require.Equal(t, "HTTP/1.1 200 OK\r\n\r\n", rsp)

	// Test nil flow
	require.Empty(t, attachedHTTPFlowResponse(nil))
}

func TestFormatAttachedNullableString(t *testing.T) {
	require.Equal(t, "test", formatAttachedNullableString("test"))
	require.Equal(t, "test", formatAttachedNullableString("  test  "))
	require.Equal(t, "-", formatAttachedNullableString(""))
	require.Equal(t, "-", formatAttachedNullableString("   "))
}

func TestFormatAttachedProcessName(t *testing.T) {
	flow := &schema.HTTPFlow{}
	require.Equal(t, "-", formatAttachedProcessName(flow))

	flow.ProcessName.Valid = true
	flow.ProcessName.String = "chrome"
	require.Equal(t, "chrome", formatAttachedProcessName(flow))

	flow.ProcessName.String = "  firefox  "
	require.Equal(t, "firefox", formatAttachedProcessName(flow))
}

func TestInlineOrSpillAttachedText(t *testing.T) {
	// Test inline (within limit)
	content := "short content"
	inline, spillNote := inlineOrSpillAttachedText("test", content, 100, nil)
	require.Equal(t, content, inline)
	require.Empty(t, spillNote)

	// Test spill (exceeds limit)
	longContent := strings.Repeat("X", 200)
	inline, spillNote = inlineOrSpillAttachedText("test", longContent, 100, nil)
	require.Len(t, inline, 100)
	require.Contains(t, spillNote, "test length 200 exceeds inline limit 100")
	require.Contains(t, spillNote, "full content saved to file")

	// Test empty content
	inline, spillNote = inlineOrSpillAttachedText("test", "", 100, nil)
	require.Equal(t, "(empty)", inline)
	require.Empty(t, spillNote)
}

func TestFormatAttachedHTTPFlowListSpillToFile(t *testing.T) {
	var sections []string
	for i := 0; i < 8; i++ {
		flow := &schema.HTTPFlow{
			Url:        fmt.Sprintf("https://example.com/%d", i),
			Method:     "GET",
			StatusCode: 200,
		}
		flow.ID = uint(i + 1)
		flow.SetRequest(strings.Repeat("R", aicommon.AttachedHTTPFlowRequestInlineLimit))
		flow.SetResponse(strings.Repeat("S", aicommon.AttachedHTTPFlowResponseInlineLimit))
		sections = append(sections, formatAttachedHTTPFlow(flow, nil))
	}

	full := strings.Join(sections, "\n\n---\n\n")
	require.Greater(t, len(full), aicommon.AttachedHTTPFlowListInlineLimit)

	inline, note := inlineOrSpillAttachedText("attached_http_flow_list", full, aicommon.AttachedHTTPFlowListInlineLimit, nil)
	require.Contains(t, note, "attached_http_flow_list length")
	require.Contains(t, note, "full content saved to file")
	require.Len(t, inline, aicommon.AttachedHTTPFlowListInlineLimit)
}
