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

func TestParseAttachedResourceDataByType(t *testing.T) {
	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `[1]`))
	require.NoError(t, err)
	require.IsType(t, &AttachedHTTPFlowResourceData{}, resource)
	require.Equal(t, AttachedResourceTypeHTTPFlowID, resource.Type())

	resource, err = ParseAttachedResourceData(NewAttachedResource("HTTPFlowID", AttachedResourceKeyID, `[1]`))
	require.NoError(t, err)
	require.IsType(t, &AttachedHTTPFlowResourceData{}, resource)

	resource, err = ParseAttachedResourceData(NewAttachedResource("httppacket", AttachedResourceKeyContent, "GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	require.NoError(t, err)
	require.IsType(t, &AttachedHTTPFuzzRequestData{}, resource)

	textPath := t.TempDir() + "/a.txt"
	require.NoError(t, os.WriteFile(textPath, []byte("hello attached file"), 0600))
	resource, err = ParseAttachedResourceData(NewAttachedResource(CONTEXT_PROVIDER_TYPE_FILE, CONTEXT_PROVIDER_KEY_FILE_PATH, textPath))
	require.NoError(t, err)
	require.IsType(t, &AttachedFileResourceData{}, resource)
	require.Equal(t, AttachedResourceTypeFile, resource.Type())
	fileResource := resource.(*AttachedFileResourceData)
	require.Equal(t, AttachedFileKindText, fileResource.Kind)
	require.Contains(t, resource.ToAttachData(nil), "hello attached file")

	resource, err = ParseAttachedResourceData(NewAttachedResource(CONTEXT_PROVIDER_KEY_FILE_PATH, "", textPath))
	require.NoError(t, err)
	require.IsType(t, &AttachedFileResourceData{}, resource)
	require.Equal(t, AttachedResourceTypeFile, resource.Type())

	dirPath := t.TempDir()
	require.NoError(t, os.WriteFile(dirPath+"/child.txt", []byte("child"), 0600))
	resource, err = ParseAttachedResourceData(NewAttachedResource(CONTEXT_PROVIDER_TYPE_FILE, CONTEXT_PROVIDER_KEY_FILE_PATH, dirPath))
	require.NoError(t, err)
	require.IsType(t, &AttachedFileResourceData{}, resource)
	require.Equal(t, AttachedFileKindDirectory, resource.(*AttachedFileResourceData).Kind)
	require.Contains(t, resource.ToAttachData(nil), "Directory Glance")

	imagePath := t.TempDir() + "/a.png"
	require.NoError(t, os.WriteFile(imagePath, []byte("png"), 0600))
	resource, err = ParseAttachedResourceData(NewAttachedResource(CONTEXT_PROVIDER_TYPE_FILE, CONTEXT_PROVIDER_KEY_FILE_PATH, imagePath))
	require.NoError(t, err)
	require.IsType(t, &AttachedFileResourceData{}, resource)
	require.Equal(t, AttachedFileKindImage, resource.(*AttachedFileResourceData).Kind)
	require.Contains(t, resource.ToAttachData(nil), "Content dump skipped for image file")

	resource, err = ParseAttachedResourceData(NewAttachedResource(CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE, CONTEXT_PROVIDER_KEY_NAME, "kb"))
	require.NoError(t, err)
	require.IsType(t, &AttachedKnowledgeBaseResourceData{}, resource)
	require.Equal(t, AttachedResourceTypeKnowledgeBase, resource.Type())
	require.Empty(t, resource.ToAttachData(nil))

	resource, err = ParseAttachedResourceData(NewAttachedResource("unknown_type", "custom_key", "hello"))
	require.NoError(t, err)
	require.IsType(t, &DefaultAttachedResourceData{}, resource)
	require.Equal(t, "unknown_type", resource.Type())
	require.Contains(t, resource.ToAttachData(nil), "no structured attached-resource parser")
}

func TestAttachedHTTPFlowIDsFromResourceJSON(t *testing.T) {
	resource := &AttachedHTTPFlowResourceData{}
	err := resource.Unmarshal(`[1,2,2,3]`)
	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, resource.IDs)

	resource = &AttachedHTTPFlowResourceData{}
	err = resource.Unmarshal(`{"ids":[4,5]}`)
	require.NoError(t, err)
	require.Equal(t, []int64{4, 5}, resource.IDs)

	resource = &AttachedHTTPFlowResourceData{}
	err = resource.Unmarshal(`{"ids":["7895","7894","7896"]}`)
	require.NoError(t, err)
	require.Equal(t, []int64{7895, 7894, 7896}, resource.IDs)

	// Test single number
	resource = &AttachedHTTPFlowResourceData{}
	err = resource.Unmarshal(`21`)
	require.NoError(t, err)
	require.Equal(t, []int64{21}, resource.IDs)

	// Test single number string
	resource = &AttachedHTTPFlowResourceData{}
	err = resource.Unmarshal(`"42"`)
	require.NoError(t, err)
	require.Equal(t, []int64{42}, resource.IDs)

	// Test single number string with whitespace
	resource = &AttachedHTTPFlowResourceData{}
	err = resource.Unmarshal(`"  99  "`)
	require.NoError(t, err)
	require.Equal(t, []int64{99}, resource.IDs)

	err = (&AttachedHTTPFlowResourceData{}).Unmarshal(`not-json`)
	require.Error(t, err)

	err = (&AttachedHTTPFlowResourceData{}).Unmarshal(`[]`)
	require.Error(t, err)

	// Test invalid single string
	err = (&AttachedHTTPFlowResourceData{}).Unmarshal(`"not-a-number"`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid http flow id string")
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
	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload))
	require.NoError(t, err)
	out := resource.ToAttachData(nil)
	require.Contains(t, out, "Attached Code Selection")
	require.Contains(t, out, "/tmp/foo.yak")
	require.Contains(t, out, "Lines: 10-20")
	require.Contains(t, out, "```yak")
	require.Contains(t, out, `println("hi")`)
}

func TestAttachedSelectedTextFromResourceJSON(t *testing.T) {
	payload := `{"path":"/tmp/a.yak","startLine":1,"endLine":2,"language":"yak","content":"x=1"}`
	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeSelected, AttachedResourceKeyContent, payload))
	require.NoError(t, err)
	selected := resource.(*AttachedSelectedResourceData)
	require.Equal(t, "x=1", selected.PlainText)
	require.NotNil(t, selected.Selected)
}

func TestAttachedHTTPFuzzRequestData(t *testing.T) {
	packet := "POST /login HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\n\r\n{\"user\":\"admin\"}"
	payload := fmt.Sprintf(`{"http_packet":%q,"is_https":"true"}`, packet)
	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeHTTPFuzzRequest, AttachedResourceKeyContent, payload))
	require.NoError(t, err)
	packetData := resource.(*AttachedHTTPFuzzRequestData)
	require.Equal(t, packet, packetData.Packet)
	require.True(t, packetData.IsHTTPS)

	out := resource.ToAttachData(nil)
	require.Contains(t, out, "Attached HTTP Fuzz Request")
	require.Contains(t, out, "Resource Type: http_fuzz_request")
	require.Contains(t, out, "IsHTTPS: true")
	require.Contains(t, out, "POST /login HTTP/1.1")
	require.Contains(t, out, "Use this raw HTTP packet as the current target request")
}

func TestAttachedHTTPFlowResourceDataFromDB(t *testing.T) {
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

	_, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, `not-json`))
	require.Error(t, err)

	missingResource := &AttachedHTTPFlowResourceData{}
	err = missingResource.Unmarshal(`{"ids":[999999999]}`)
	require.NoError(t, err)
	require.Contains(t, missingResource.renderSummary(db), "Load Errors")

	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeHTTPFlowID, AttachedResourceKeyID, fmt.Sprintf(`{"ids":[%d]}`, flow.ID)))
	require.NoError(t, err)
	rendered := resource.(*AttachedHTTPFlowResourceData).renderSummary(db)
	require.Contains(t, rendered, token)
	require.Contains(t, rendered, fmt.Sprintf("ID %d", flow.ID))
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
