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

func TestAttachedHTTPFlowIDsFromResourceJSON(t *testing.T) {
	ids, err := attachedHTTPFlowIDsFromResource(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `[1,2,2,3]`))
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, ids)

	ids, err = attachedHTTPFlowIDsFromResource(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `{"ids":[4,5]}`))
	require.NoError(t, err)
	require.Equal(t, []int64{4, 5}, ids)

	ids, err = attachedHTTPFlowIDsFromResource(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `{"ids":["7895","7894","7896"]}`))
	require.NoError(t, err)
	require.Equal(t, []int64{7895, 7894, 7896}, ids)

	_, err = attachedHTTPFlowIDsFromResource(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `not-json`))
	require.Error(t, err)

	_, err = attachedHTTPFlowIDsFromResource(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `[]`))
	require.Error(t, err)
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
		flow.SetRequest(strings.Repeat("R", AttachedHTTPFlowRequestInlineLimit))
		flow.SetResponse(strings.Repeat("S", AttachedHTTPFlowResponseInlineLimit))
		sections = append(sections, FormatAttachedHTTPFlow(flow, nil))
	}

	full := strings.Join(sections, "\n\n---\n\n")
	require.Greater(t, len(full), AttachedHTTPFlowListInlineLimit)

	inline, note := inlineOrSpillAttachedText("attached_http_flow_list", full, AttachedHTTPFlowListInlineLimit, nil)
	require.Contains(t, note, "attached_http_flow_list length")
	require.Contains(t, note, "full content saved to file")
	require.Len(t, inline, AttachedHTTPFlowListInlineLimit)
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

func TestFormatAttachedCodeSelectionJSON(t *testing.T) {
	payload := `{"path":"/tmp/foo.yak","startLine":10,"endLine":20,"language":"yak","content":"println(\"hi\")"}`
	out := RenderAttachedSelectedResource(NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload), nil)
	require.Contains(t, out, "Attached Code Selection")
	require.Contains(t, out, "/tmp/foo.yak")
	require.Contains(t, out, "Lines: 10-20")
	require.Contains(t, out, "```yak")
	require.Contains(t, out, `println("hi")`)
}

func TestAttachedSelectedTextFromResourceJSON(t *testing.T) {
	payload := `{"path":"/tmp/a.yak","startLine":1,"endLine":2,"language":"yak","content":"x=1"}`
	text := attachedSelectedTextFromResource(NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload))
	require.Equal(t, "x=1", text)
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

	_, err := RenderAttachedHTTPFlowResource(db, NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `not-json`), nil)
	require.Error(t, err)

	_, err = RenderAttachedHTTPFlowResource(db, NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `{"ids":[999999999]}`), nil)
	require.NoError(t, err)

	rendered, err := RenderAttachedHTTPFlowResource(db, NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, fmt.Sprintf(`{"ids":[%d]}`, flow.ID)), nil)
	require.NoError(t, err)
	require.Contains(t, rendered, token)
	require.Contains(t, rendered, fmt.Sprintf("HTTP Flow #%d", flow.ID))
	require.Contains(t, rendered, "Requested IDs:")
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
