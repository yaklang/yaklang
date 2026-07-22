package yakgrpc

import (
	"context"
	"io"
	"mime/multipart"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// buildBigMultipartRequest builds an oversized multipart/form-data request
// carrying one text field and two file parts (so it triggers skeleton spill).
func buildBigMultipartRequest(t *testing.T) ([]byte, []byte, []byte) {
	t.Helper()
	boundary := "----YakTestBoundary"

	body := &strings.Builder{}
	// text field
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(`Content-Disposition: form-data; name="desc"` + "\r\n\r\n")
	body.WriteString("hello-field" + "\r\n")

	// file part 0 (oversized, > 200KB threshold to trigger spill)
	f0 := []byte(strings.Repeat("F", 250 * 1024))
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(`Content-Disposition: form-data; name="file0"; filename="big0.bin"` + "\r\n")
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(f0)
	body.WriteString("\r\n")

	// file part 1 (small)
	f1 := []byte("small-1")
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(`Content-Disposition: form-data; name="file1"; filename="note.txt"` + "\r\n")
	body.WriteString("Content-Type: text/plain\r\n\r\n")
	body.Write(f1)
	body.WriteString("\r\n")

	body.WriteString("--" + boundary + "--\r\n")
	bodyStr := body.String()
	header := "POST /upload/case/safe HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8080\r\n" +
		"Content-Type: multipart/form-data; boundary=" + boundary + "\r\n" +
		"Content-Length: " + strconv.Itoa(len(bodyStr)) + "\r\n\r\n"
	return []byte(header + bodyStr), f0, f1
}

func TestGRPCMUSTPASS_GetHTTPFlowBodyById_MultipartPartIndex(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	// Clean up any prior flows of this source type.
	yakit.DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
		Filter: &ypb.QueryHTTPFlowRequest{SourceType: "multipart-spill-test"},
	})

	packet, f0, f1 := buildBigMultipartRequest(t)
	flow, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(
		false, packet, []byte("HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n"),
		"multipart-spill-test", "http://127.0.0.1:8080/upload/case/safe", "",
	)
	require.NoError(t, err)
	flow.CalcHash()
	require.NoError(t, consts.GetGormProjectDatabase().Save(flow).Error)
	defer func() {
		_ = yakit.DeleteHTTPFlow(consts.GetGormProjectDatabase(), &ypb.DeleteHTTPFlowRequest{
			Filter: &ypb.QueryHTTPFlowRequest{SourceType: "multipart-spill-test"},
		})
	}()

	// Detail query should expose MultipartFiles so the frontend can render a
	// dropdown.
	detail, err := client.GetHTTPFlowById(context.Background(), &ypb.GetHTTPFlowByIdRequest{Id: int64(flow.ID)})
	require.NoError(t, err)
	require.True(t, detail.GetIsTooLargeRequest(), "should be marked too large")
	require.NotEmpty(t, detail.GetTooLargeRequestBodyFile(), "body placeholder path should be set")
	files := detail.GetMultipartFiles()
	require.Len(t, files, 2, "should expose two file parts")
	// Find the big0.bin part index.
	var big0Idx int32 = -1
	for _, f := range files {
		if f.GetFilename() == "big0.bin" {
			big0Idx = f.GetPartIndex()
		}
	}
	require.GreaterOrEqual(t, big0Idx, int32(0), "big0.bin part not found in manifest")

	// Download that single part via PartIndex.
	stream, err := client.GetHTTPFlowBodyById(context.Background(), &ypb.GetHTTPFlowBodyByIdRequest{
		Id:        int64(flow.ID),
		IsRequest: true,
		PartIndex: &big0Idx,
	})
	require.NoError(t, err)
	var (
		gotBody   []byte
		gotName   string
		sawHeader bool
	)
	for {
		msg, rerr := stream.Recv()
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			t.Fatal(rerr)
		}
		if msg == nil {
			break
		}
		if !sawHeader {
			gotName = msg.GetFilename()
			sawHeader = true
		}
		gotBody = append(gotBody, msg.GetData()...)
		if msg.GetEOF() {
			break
		}
	}
	require.Equal(t, "big0.bin", gotName, "streamed filename should be the part's original name")
	require.Equal(t, f0, gotBody, "streamed part content should match the uploaded file")

	// Without PartIndex: full rebuilt multipart body should contain both files
	// and the text field, parseable as multipart.
	fullStream, err := client.GetHTTPFlowBodyById(context.Background(), &ypb.GetHTTPFlowBodyByIdRequest{
		Id:        int64(flow.ID),
		IsRequest: true,
	})
	require.NoError(t, err)
	var fullBody []byte
	for {
		msg, rerr := fullStream.Recv()
		if rerr != nil {
			if rerr == io.EOF {
				break
			}
			t.Fatal(rerr)
		}
		if msg == nil {
			break
		}
		fullBody = append(fullBody, msg.GetData()...)
		if msg.GetEOF() {
			break
		}
	}
	// Parse the rebuilt body and verify parts.
	mr := multipart.NewReader(strings.NewReader(string(fullBody)), "----YakTestBoundary")
	parts := map[string][]byte{}
	for {
		p, rerr := mr.NextPart()
		if rerr != nil {
			break
		}
		b, _ := io.ReadAll(p)
		parts[p.FormName()] = b
	}
	require.Equal(t, "hello-field", string(parts["desc"]))
	require.Equal(t, f0, parts["file0"])
	require.Equal(t, f1, parts["file1"])
}