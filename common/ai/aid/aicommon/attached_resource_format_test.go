package aicommon

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestIsAttachedHTTPFlowResource(t *testing.T) {
	require.True(t, IsAttachedHTTPFlowResource(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, "1")))
	require.True(t, IsAttachedHTTPFlowResource(NewAttachedResource("HTTPFlowID", AttachedResourceKeyID, "1")))
	require.False(t, IsAttachedHTTPFlowResource(NewAttachedResource(CONTEXT_PROVIDER_TYPE_FILE, CONTEXT_PROVIDER_KEY_FILE_PATH, "/tmp/a.txt")))
}

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

	out := FormatAttachedHTTPFlow(flow, nil)
	require.Contains(t, out, "HTTP Flow #42")
	require.Contains(t, out, "Method: GET")
	require.Contains(t, out, "StatusCode: 200")
	require.Contains(t, out, "Tags: test|demo")
	require.Contains(t, out, "GET /api HTTP/1.1")
	require.Contains(t, out, "HTTP/1.1 200 OK")
	require.NotContains(t, out, "exceeds inline limit")
}

func TestFormatAttachedHTTPFlowSpillToFile(t *testing.T) {
	largeReq := strings.Repeat("R", AttachedHTTPFlowRequestInlineLimit+128)
	largeRsp := strings.Repeat("S", AttachedHTTPFlowResponseInlineLimit+256)

	flow := &schema.HTTPFlow{
		Url:        "https://example.com/large",
		Method:     "POST",
		StatusCode: 500,
	}
	flow.ID = 99
	flow.SetRequest(largeReq)
	flow.SetResponse(largeRsp)

	out := FormatAttachedHTTPFlow(flow, nil)
	require.Contains(t, out, "request length")
	require.Contains(t, out, "response length")
	require.Contains(t, out, "full content saved to file")
	require.Contains(t, out, strings.Repeat("R", 64))
	require.Contains(t, out, strings.Repeat("S", 64))
}

func TestFormatAttachedSelectedTextInlineAndSpill(t *testing.T) {
	inline := FormatAttachedSelectedText("hello selection", nil)
	require.Contains(t, inline, "hello selection")
	require.NotContains(t, inline, "exceeds inline limit")

	large := strings.Repeat("X", AttachedSelectedTextInlineLimit+64)
	spilled := FormatAttachedSelectedText(large, nil)
	require.Contains(t, spilled, "selected_text length")
	require.Contains(t, spilled, "full content saved to file")
	require.Contains(t, spilled, strings.Repeat("X", 64))
}

func TestRenderAttachedHTTPFlowResourceFromDB(t *testing.T) {
	db := consts.GetGormProjectDatabase()
	if db == nil {
		t.Skip("project database unavailable")
	}

	token := "attached-resource-test"
	flow := &schema.HTTPFlow{
		Url:        "https://example.com/" + token,
		Method:     "GET",
		StatusCode: 201,
		SourceType: schema.HTTPFlow_SourceType_MITM,
		Tags:       token,
	}
	flow.SetRequest("GET /" + token + " HTTP/1.1\r\nHost: example.com\r\n\r\n")
	flow.SetResponse("HTTP/1.1 201 Created\r\n\r\n" + token)

	require.NoError(t, yakit.InsertHTTPFlow(db, flow))
	require.NotZero(t, flow.ID)
	t.Cleanup(func() {
		_ = yakit.DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Id: []int64{int64(flow.ID)}})
	})

	_, err := RenderAttachedHTTPFlowResource(db, NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, "not-a-number"), nil)
	require.Error(t, err)

	_, err = RenderAttachedHTTPFlowResource(db, NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, "999999999"), nil)
	require.Error(t, err)

	rendered, err := RenderAttachedHTTPFlowResource(db, NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, fmt.Sprintf("%d", flow.ID)), nil)
	require.NoError(t, err)
	require.Contains(t, rendered, token)
	require.Contains(t, rendered, fmt.Sprintf("HTTP Flow #%d", flow.ID))
}

func TestInlineOrSpillAttachedTextCreatesFile(t *testing.T) {
	content := strings.Repeat("Z", AttachedSelectedTextInlineLimit+10)
	inline, note := inlineOrSpillAttachedText("selected_text", content, AttachedSelectedTextInlineLimit, nil)
	require.Len(t, inline, AttachedSelectedTextInlineLimit)
	require.Contains(t, note, "saved to file")

	parts := strings.Split(note, "saved to file: ")
	require.Len(t, parts, 2)
	filePath := strings.TrimSpace(strings.Split(parts[1], "\n")[0])
	require.FileExists(t, filePath)
	data, err := os.ReadFile(filePath)
	require.NoError(t, err)
	require.Equal(t, content, string(data))
	_ = os.Remove(filePath)
}
