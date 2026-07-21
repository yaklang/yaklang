package yakit

import (
	"bytes"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// buildMultipartRequest builds a POST multipart/form-data request packet
// carrying the given text fields and file parts. fileParts maps field name to
// (filename, content). Returns the full packet bytes and the boundary used.
func buildMultipartRequest(t *testing.T, textFields map[string]string, fileParts map[string]struct {
	Filename    string
	ContentType string
	Content     []byte
}) (packet []byte, boundary string) {
	t.Helper()
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for name, val := range textFields {
		if err := mw.WriteField(name, val); err != nil {
			t.Fatalf("write field %q: %v", name, err)
		}
	}
	for name, fp := range fileParts {
		hdr := make(textproto.MIMEHeader)
		hdr.Set("Content-Disposition", `form-data; name="`+name+`"; filename="`+fp.Filename+`"`)
		if fp.ContentType != "" {
			hdr.Set("Content-Type", fp.ContentType)
		}
		pw, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatalf("create file part %q: %v", name, err)
		}
		if _, err := pw.Write(fp.Content); err != nil {
			t.Fatalf("write file part %q: %v", name, err)
		}
	}
	boundary = mw.Boundary()
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	header := "POST /upload/case/safe HTTP/1.1\r\n" +
		"Host: 127.0.0.1:8080\r\n" +
		"Content-Type: " + mw.FormDataContentType() + "\r\n" +
		"Content-Length: " + strconv.Itoa(body.Len()) + "\r\n\r\n"
	return []byte(header + body.String()), boundary
}

func TestSpillMultipartFilesIfNeeded_SingleFile(t *testing.T) {
	fileContent := bytes.Repeat([]byte("X"), maxHTTPFlowRequestBodyInDBBytes+1024)
	packet, _ := buildMultipartRequest(t, map[string]string{"desc": "hello"}, map[string]struct {
		Filename    string
		ContentType string
		Content     []byte
	}{
		"filename": {Filename: "Yakit-1.4.8.exe", ContentType: "application/x-msdownload", Content: fileContent},
	})

	res, err := spillMultipartFilesIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, res.IsTooLarge, "oversized multipart with file part should spill")
	require.NotEmpty(t, res.HeaderFile)
	require.NotEmpty(t, res.BodyFile)
	require.NotEmpty(t, res.MultipartDir)
	require.Len(t, res.Manifest, 1)
	defer func() {
		_ = os.RemoveAll(res.MultipartDir)
		_ = os.Remove(res.HeaderFile)
		_ = os.Remove(res.BodyFile)
	}()

	// Skeleton stored packet contains the placeholder, not the file bytes.
	require.Contains(t, string(res.StoredPacket), multipartSkeletonMarker)
	require.NotContains(t, string(res.StoredPacket), string(fileContent[:100]))

	// Skeleton keeps the editable field and filename.
	require.Contains(t, string(res.StoredPacket), `name="desc"`)
	require.Contains(t, string(res.StoredPacket), `filename="Yakit-1.4.8.exe"`)
	require.Contains(t, string(res.StoredPacket), "hello")

	// Part file on disk holds the original content.
	partPath := filepath.Join(res.MultipartDir, res.Manifest[0].File)
	got, err := os.ReadFile(partPath)
	require.NoError(t, err)
	require.Equal(t, fileContent, got)

	// Rebuilt body file parses back to the same parts.
	bodyOnDisk, err := os.ReadFile(res.BodyFile)
	require.NoError(t, err)
	parts := parseMultipartParts(t, bodyOnDisk, boundaryFromBodyPacket(t, packet))
	require.Len(t, parts, 2)
	require.Equal(t, "hello", string(parts["desc"].body))
	require.Equal(t, fileContent, parts["filename"].body)
}

func TestSpillMultipartFilesIfNeeded_MultipleFiles(t *testing.T) {
	f1 := bytes.Repeat([]byte("A"), maxHTTPFlowRequestBodyInDBBytes/2)
	f2 := bytes.Repeat([]byte("B"), maxHTTPFlowRequestBodyInDBBytes/2)
	f3 := bytes.Repeat([]byte("C"), 1024)
	packet, _ := buildMultipartRequest(t,
		map[string]string{"token": "abc", "case": "safe"},
		map[string]struct {
			Filename    string
			ContentType string
			Content     []byte
		}{
			"file1": {Filename: "report.pdf", ContentType: "application/pdf", Content: f1},
			"file2": {Filename: "data.bin", ContentType: "application/octet-stream", Content: f2},
			"file3": {Filename: "logo.png", ContentType: "image/png", Content: f3},
		})

	res, err := spillMultipartFilesIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, res.IsTooLarge)
	require.Len(t, res.Manifest, 3)
	defer func() {
		_ = os.RemoveAll(res.MultipartDir)
		_ = os.Remove(res.HeaderFile)
		_ = os.Remove(res.BodyFile)
	}()

	// Each file part has its own disk file, named by its parse-order index.
	seenIndexes := map[int]bool{}
	for _, m := range res.Manifest {
		require.True(t, strings.HasPrefix(m.File, "part-"+strconv.Itoa(m.Index)+"-"),
			"part file should be named part-<index>-*, got %s", m.File)
		require.False(t, seenIndexes[m.Index], "duplicate part index %d", m.Index)
		seenIndexes[m.Index] = true
		p := filepath.Join(res.MultipartDir, m.File)
		got, err := os.ReadFile(p)
		require.NoError(t, err)
		require.NotEmpty(t, got)
	}

	// Skeleton keeps all three filenames and the text fields editable.
	skel := string(res.StoredPacket)
	require.Contains(t, skel, `filename="report.pdf"`)
	require.Contains(t, skel, `filename="data.bin"`)
	require.Contains(t, skel, `filename="logo.png"`)
	require.Contains(t, skel, "abc")
	require.Contains(t, skel, "safe")
	// No raw file bytes leak into the skeleton.
	require.NotContains(t, skel, string(f1[:64]))

	// Rebuilt body file parses back to all parts with correct contents.
	boundary := boundaryFromBodyPacket(t, packet)
	bodyOnDisk, err := os.ReadFile(res.BodyFile)
	require.NoError(t, err)
	parts := parseMultipartParts(t, bodyOnDisk, boundary)
	require.Len(t, parts, 5)
	require.Equal(t, "abc", string(parts["token"].body))
	require.Equal(t, "safe", string(parts["case"].body))
	require.Equal(t, f1, parts["file1"].body)
	require.Equal(t, f2, parts["file2"].body)
	require.Equal(t, f3, parts["file3"].body)
}

func TestSpillMultipartFilesIfNeeded_TextOnlyNotSpilled(t *testing.T) {
	// Oversized multipart but no file parts: must NOT skeletonize; fall back
	// to flat spill is handled by the caller, so here IsTooLarge is false.
	big := bytes.Repeat([]byte("z"), maxHTTPFlowRequestBodyInDBBytes+512)
	packet, _ := buildMultipartRequest(t, map[string]string{"blob": string(big)}, nil)

	res, err := spillMultipartFilesIfNeeded(packet)
	require.NoError(t, err)
	require.False(t, res.IsTooLarge, "text-only multipart should not skeletonize")
	require.Empty(t, res.MultipartDir)
}

func TestSpillMultipartFilesIfNeeded_SmallMultipartNotSpilled(t *testing.T) {
	packet, _ := buildMultipartRequest(t, map[string]string{"a": "1"}, map[string]struct {
		Filename    string
		ContentType string
		Content     []byte
	}{
		"f": {Filename: "tiny.txt", Content: []byte("hi")},
	})
	res, err := spillMultipartFilesIfNeeded(packet)
	require.NoError(t, err)
	require.False(t, res.IsTooLarge, "small multipart should not spill")
}

func TestSpillMultipartFilesIfNeeded_NonMultipartNotSpilled(t *testing.T) {
	body := bytes.Repeat([]byte("Q"), maxHTTPFlowRequestBodyInDBBytes+64)
	packet := []byte("POST /up HTTP/1.1\r\nHost: a\r\nContent-Type: application/json\r\nContent-Length: " +
		strconv.Itoa(len(body)) + "\r\n\r\n" + string(body))
	res, err := spillMultipartFilesIfNeeded(packet)
	require.NoError(t, err)
	require.False(t, res.IsTooLarge, "non-multipart should not skeletonize")
}

func TestSpillLargeHTTPFlowRequestIfNeeded_MultipartRoute(t *testing.T) {
	fileContent := bytes.Repeat([]byte("M"), maxHTTPFlowRequestBodyInDBBytes+2048)
	packet, _ := buildMultipartRequest(t, map[string]string{"n": "v"}, map[string]struct {
		Filename    string
		ContentType string
		Content     []byte
	}{
		"up": {Filename: "big.bin", Content: fileContent},
	})

	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, res.IsTooLarge)
	require.NotEmpty(t, res.BodyFile)
	require.NotEmpty(t, res.HeaderFile)
	defer func() {
		_ = os.RemoveAll(multipartSidecarDirFromBodyFile(res.BodyFile))
		_ = os.Remove(res.BodyFile)
		_ = os.Remove(res.HeaderFile)
	}()

	// The stored packet is the skeleton (placeholder present), not flat notice.
	require.Contains(t, string(res.StoredPacket), multipartSkeletonMarker)
	require.NotContains(t, string(res.StoredPacket), "request too large(")

	// Rebuilt body file parses back to the original parts.
	boundary := boundaryFromBodyPacket(t, packet)
	bodyOnDisk, err := os.ReadFile(res.BodyFile)
	require.NoError(t, err)
	parts := parseMultipartParts(t, bodyOnDisk, boundary)
	require.Equal(t, "v", string(parts["n"].body))
	require.Equal(t, fileContent, parts["up"].body)
}

func TestCleanupMultipartSidecar_DerivesFromBodyFile(t *testing.T) {
	// Create a fake sidecar dir derived from a body file path and confirm
	// cleanup removes it.
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "large-request-body-test.txt")
	partsDir := filepath.Join(dir, "large-request-body-test-parts")
	require.NoError(t, os.MkdirAll(partsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(partsDir, "part-0-x.bin"), []byte("x"), 0o644))
	require.DirExists(t, partsDir)

	cleanupMultipartSidecar(bodyFile)
	require.NoDirExists(t, partsDir)
}

func TestCleanupMultipartSidecar_FlatBodyFileNoop(t *testing.T) {
	dir := t.TempDir()
	bodyFile := filepath.Join(dir, "large-request-body-flat.txt")
	require.NoError(t, os.WriteFile(bodyFile, []byte("flat"), 0o644))
	// No sidecar dir exists; cleanup must not remove the body file or error.
	cleanupMultipartSidecar(bodyFile)
	require.FileExists(t, bodyFile)
}

// --- helpers ---

type parsedPart struct {
	header textproto.MIMEHeader
	body   []byte
}

func parseMultipartParts(t *testing.T, body []byte, boundary string) map[string]parsedPart {
	t.Helper()
	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	out := make(map[string]parsedPart)
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, readErr := readAllPart(t, p)
		out[p.FormName()] = parsedPart{header: p.Header, body: b}
		if readErr != nil {
			t.Fatalf("read part %q: %v", p.FormName(), readErr)
		}
	}
	return out
}

func readAllPart(t *testing.T, p *multipart.Part) ([]byte, error) {
	t.Helper()
	defer p.Close()
	buf := &bytes.Buffer{}
	_, err := buf.ReadFrom(p)
	return buf.Bytes(), err
}

func boundaryFromBodyPacket(t *testing.T, packet []byte) string {
	t.Helper()
	ct := lowhttp.GetHTTPPacketHeader(packet, "Content-Type")
	require.NotEmpty(t, ct, "missing Content-Type")
	_, params, err := mime.ParseMediaType(ct)
	require.NoError(t, err)
	require.NotEmpty(t, params["boundary"], "missing boundary in Content-Type")
	return params["boundary"]
}