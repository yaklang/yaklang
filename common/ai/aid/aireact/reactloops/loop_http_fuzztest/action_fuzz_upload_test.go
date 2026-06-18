package loop_http_fuzztest

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
)

func buildTestMultipartUploadRequest(fileField, fileName string, fileContent, textField, textValue string) []byte {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if textField != "" {
		_ = writer.WriteField(textField, textValue)
	}
	if fileField != "" {
		part, err := writer.CreateFormFile(fileField, fileName)
		if err == nil {
			_, _ = part.Write([]byte(fileContent))
		}
	}
	_ = writer.Close()

	var packet bytes.Buffer
	packet.WriteString("POST /upload HTTP/1.1\r\n")
	packet.WriteString("Host: example.test\r\n")
	packet.WriteString(fmt.Sprintf("Content-Type: multipart/form-data; boundary=%s\r\n", writer.Boundary()))
	packet.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", body.Len()))
	packet.Write(body.Bytes())
	return packet.Bytes()
}

func newHTTPFuzztestLoopForUploadTest(t *testing.T) *reactloops.ReActLoop {
	t.Helper()
	loop := newHTTPFuzztestLoopForPatchTest(t)
	task := aicommon.NewStatefulTaskBase("upload-test", "upload test", context.Background(), loop.GetEmitter())
	loop.SetCurrentTask(task)
	return loop
}

func TestParseMultipartUploadSummary_RecognizesFieldsAndFiles(t *testing.T) {
	raw := buildTestMultipartUploadRequest("avatar", "photo.jpg", "fake-image-bytes", "token", "abc123")
	summary, err := parseMultipartUploadSummary(raw)
	require.NoError(t, err)
	require.True(t, summary.IsMultipart)
	require.Len(t, summary.Parts, 2)

	var fieldPart, filePart *loopHTTPUploadPartSummary
	for i := range summary.Parts {
		part := &summary.Parts[i]
		if part.IsFile {
			filePart = part
		} else {
			fieldPart = part
		}
	}
	require.NotNil(t, fieldPart)
	require.Equal(t, "token", fieldPart.FieldName)
	require.Equal(t, "abc123", fieldPart.Preview)

	require.NotNil(t, filePart)
	require.Equal(t, "avatar", filePart.FieldName)
	require.Equal(t, "photo.jpg", filePart.FileName)
	require.Equal(t, int64(len("fake-image-bytes")), filePart.Size)
	require.NotEmpty(t, filePart.Digest)
}

func TestSanitizeHTTPRequestForPrompt_OmitsLargeFileContent(t *testing.T) {
	largeContent := strings.Repeat("A", loopHTTPUploadPartExternalizeThreshold+128)
	raw := buildTestMultipartUploadRequest("avatar", "big.bin", largeContent, "folder", "/tmp")
	summary, err := parseMultipartUploadSummary(raw)
	require.NoError(t, err)

	promptSafe := sanitizeHTTPRequestForPrompt(raw, summary)
	require.NotContains(t, promptSafe, largeContent)
	require.Contains(t, promptSafe, `upload_file_ref(field="avatar"`)
	require.Contains(t, promptSafe, `filename="big.bin"`)
	require.Contains(t, promptSafe, "folder")
}

func TestSyncLoopHTTPUploadContext_ExternalizesLargeFile(t *testing.T) {
	loop := newHTTPFuzztestLoopForUploadTest(t)
	largeContent := strings.Repeat("B", loopHTTPUploadPartExternalizeThreshold+64)
	raw := buildTestMultipartUploadRequest("file", "payload.bin", largeContent, "path", "../safe")

	syncLoopHTTPUploadContext(loop, raw, false, true)

	summaryText := loop.Get(loopHTTPUploadRequestSummaryKey)
	require.Contains(t, summaryText, `file field "file"`)
	require.Contains(t, summaryText, "resource_id=")

	promptSafe := loop.Get(loopHTTPUploadOriginalPromptSafeKey)
	require.NotContains(t, promptSafe, largeContent)
	require.Contains(t, promptSafe, "upload_file_ref")

	refs := getLoopHTTPUploadFileResources(loop)
	require.Len(t, refs, 1)
	require.Equal(t, "file", refs[0].FieldName)
	require.Equal(t, int64(len(largeContent)), refs[0].Size)
}

func TestFuzzUploadFileName_GeneratesMultipleRequests(t *testing.T) {
	raw := buildTestMultipartUploadRequest("avatar", "photo.jpg", "image", "token", "1")
	fuzzReq, err := mutate.NewFuzzHTTPRequest(raw)
	require.NoError(t, err)

	results, err := fuzzReq.FuzzUploadFileName("avatar", []string{"shell.php", "shell.phtml"}).Results()
	require.NoError(t, err)
	require.Len(t, results, 2)

	names := make([]string, 0, len(results))
	for _, req := range results {
		dump, dumpErr := utils.DumpHTTPRequest(req, true)
		require.NoError(t, dumpErr)
		names = append(names, string(dump))
	}
	require.Contains(t, names[0], "shell.php")
	require.Contains(t, names[1], "shell.phtml")
}

func TestFuzzUploadMultipartField_ModifiesPlainField(t *testing.T) {
	raw := buildTestMultipartUploadRequest("avatar", "photo.jpg", "image", "folder", "/uploads")
	fuzzReq, err := mutate.NewFuzzHTTPRequest(raw)
	require.NoError(t, err)

	results, err := fuzzReq.FuzzUploadKVPair("folder", []string{"/tmp", "/var"}).Results()
	require.NoError(t, err)
	require.Len(t, results, 2)

	for _, req := range results {
		dump, dumpErr := utils.DumpHTTPRequest(req, true)
		require.NoError(t, dumpErr)
		body := string(dump)
		require.Contains(t, body, "name=\"folder\"")
	}
}

func TestFuzzUploadFileContent_UsesProfile(t *testing.T) {
	raw := buildTestMultipartUploadRequest("avatar", "probe.svg", "old", "token", "1")
	fuzzReq, err := mutate.NewFuzzHTTPRequest(raw)
	require.NoError(t, err)

	content, err := resolveUploadContentProfile("svg_xss")
	require.NoError(t, err)

	results, err := fuzzReq.FuzzUploadFile("avatar", []string{"probe.svg"}, content).Results()
	require.NoError(t, err)
	require.Len(t, results, 1)

	dump, err := utils.DumpHTTPRequest(results[0], true)
	require.NoError(t, err)
	require.Contains(t, string(dump), "<svg")
}

func TestCompareRequestsForPrompt_DoesNotIncludeLargeUploadBody(t *testing.T) {
	largeContent := strings.Repeat("Z", loopHTTPUploadPartExternalizeThreshold+32)
	original := string(buildTestMultipartUploadRequest("file", "a.jpg", largeContent, "x", "1"))
	modified := string(buildTestMultipartUploadRequest("file", "shell.php", "small", "x", "1"))

	diff := compareRequestsForPrompt(original, modified)
	require.NotContains(t, diff, largeContent)
}

func TestBuildLoopHTTPFuzzProcessedResult_UsesPromptSafeRequestDiff(t *testing.T) {
	largeContent := strings.Repeat("Y", loopHTTPUploadPartExternalizeThreshold+16)
	original := string(buildTestMultipartUploadRequest("file", "a.jpg", largeContent, "x", "1"))
	modified := string(buildTestMultipartUploadRequest("file", "b.jpg", "small", "x", "2"))

	processed := buildLoopHTTPFuzzProcessedResult(1, &mutate.HttpResult{
		Request:     &http.Request{Method: "POST", Host: "example.test"},
		RequestRaw:  []byte(modified),
		ResponseRaw: []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"),
		Payloads:    []string{"b.jpg"},
	}, original, 2)

	require.NotContains(t, processed.RequestDiff, largeContent)
	require.Contains(t, processed.RequestSummary, "MULTIPART")
}

func TestResolveUploadContentBytes_FromExternalizedResource(t *testing.T) {
	loop := newHTTPFuzztestLoopForUploadTest(t)
	raw := buildTestMultipartUploadRequest("file", "a.txt", strings.Repeat("C", loopHTTPUploadPartExternalizeThreshold+8), "k", "v")
	syncLoopHTTPUploadContext(loop, raw, false, true)

	refs := getLoopHTTPUploadFileResources(loop)
	require.Len(t, refs, 1)

	content, err := resolveUploadContentBytes("", "", refs[0].ID, loop)
	require.NoError(t, err)
	require.Equal(t, int(refs[0].Size), len(content))
}

func TestLoopHTTPFuzztestPersistentInstruction_CoversFileUploadRules(t *testing.T) {
	checks := []string{
		"fuzz_upload",
		"multipart/form-data",
		"content_profile",
		"file_resource_id",
		"generate_and_send_packet",
	}
	for _, needle := range checks {
		require.Contains(t, instruction, needle)
	}
}

func TestLoopHTTPFuzztestReactiveData_CoversUploadSummaryBlock(t *testing.T) {
	require.Contains(t, reactiveData, "Multipart Upload Summary")
	require.Contains(t, reactiveData, "UploadRequestSummary")
}
