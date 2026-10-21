package aicommon

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

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
	require.Equal(t, CONTEXT_PROVIDER_KEY_NAME, resource.(*AttachedKnowledgeBaseResourceData).Key)
	require.Contains(t, resource.ToAttachData(nil), "```\nkb\n```", "knowledge selections must remain visible as user context")

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

func TestAttachedKnowledgeBasePreferences(t *testing.T) {
	for _, tc := range []struct{ key, value, hint string }{
		{CONTEXT_PROVIDER_KEY_NAME, "all", "Prefer the user's attached knowledge base name"},
		{CONTEXT_PROVIDER_KEY_NAME, "  项目资料\n优先中文说明。 \n", "Prefer the user's attached knowledge base name"},
		{CONTEXT_PROVIDER_KEY_SYSTEM_FLAG, CONTEXT_PROVIDER_VALUE_ALL_KNOWLEDGE_BASE, "explicitly selected all knowledge bases"},
		{CONTEXT_PROVIDER_KEY_SYSTEM_FLAG, CONTEXT_PROVIDER_VALUE_AUTO_SELECT_KNOWLEDGE_BASE + ":优先项目资料", "Do not substitute all=true"},
	} {
		t.Run(tc.value, func(t *testing.T) {
			input := NewAttachedResource(AttachedResourceTypeKnowledgeBase, tc.key, tc.value)
			resource, err := ParseAttachedResourceData(input)
			require.NoError(t, err)
			knowledge := resource.(*AttachedKnowledgeBaseResourceData)
			require.Equal(t, tc.key, knowledge.Key)
			require.Equal(t, tc.value, knowledge.Value)
			rendered := resource.ToAttachData(nil)
			require.Contains(t, rendered, tc.hint)
			require.Contains(t, rendered, "```\n"+tc.value+"\n```", "preserve exact user attachment bytes")
			require.Equal(t, tc.value, input.Value)
		})
	}
}

func TestAttachedKnowledgeBaseLongContentReadBack(t *testing.T) {
	content := " \n" + strings.Repeat("优先中文资料😀\n", 1500) + "TAIL_PREFERENCE\n "
	resource, err := ParseAttachedResourceData(NewAttachedResource(AttachedResourceTypeKnowledgeBase, "selection_notes", content))
	require.NoError(t, err)
	rendered := resource.ToAttachData(nil)
	require.True(t, utf8.ValidString(rendered))
	require.Less(t, len(rendered), 10*1024)
	_, after, found := strings.Cut(rendered, "Full content saved to file: ")
	require.True(t, found)
	path, _, _ := strings.Cut(after, "\n")
	t.Cleanup(func() { _ = os.Remove(path) })
	full, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(full))
	require.Equal(t, rendered, resource.ToAttachData(nil), "reuse the saved original instead of creating duplicate files")
}
