//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"image/jpeg"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type mitmV2RequestOutcomeCase struct {
	name                     string
	originalPayload          []byte
	inlineEditedPayload      []byte
	manualFuzzTagPayload     []byte
	replacementPayload       []byte
	localReplacementFilename string
	localReplacementType     string
	sentPayload              []byte
	contentType              string
	multipart                bool
	uploadFilename           string
	uploadContentType        string
	forwardOriginal          bool
	dropRequest              bool
	hijackResponse           bool
	wantHijackTag            string
	wantHijackRaw            bool
	wantResourceFlow         bool
	wantCurrentRaw           bool
	wantBareResource         bool
	wantHijackFileFields     []string
	wantCurrentFileFields    []string
	wantBareFileFields       []string
	wantBareFileTags         int
	wantManualEditTag        bool
	wantRenderedUserFuzzTag  bool
	wantCurrentBinaryExports bool
	wantBareBinaryExports    bool
	extraMultipartParts      []mitmV2OutcomeMultipartPart
	replacementPartName      string
	deleteAfterValidation    bool
}

type mitmV2OutcomeMultipartPart struct {
	fieldName       string
	filename        string
	contentType     string
	originalPayload []byte
	sentPayload     []byte
}

func buildMITMV2OutcomePacket(t *testing.T, target, token string, tc mitmV2RequestOutcomeCase) []byte {
	t.Helper()
	body := tc.originalPayload
	contentType := tc.contentType
	if tc.multipart {
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		require.NoError(t, writer.WriteField("note", "editable"))
		filename := tc.uploadFilename
		if filename == "" {
			filename = "sample.bin"
		}
		partContentType := tc.uploadContentType
		if partContentType == "" {
			partContentType = "application/octet-stream"
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="upload"; filename="%s"`, filename))
		header.Set("Content-Type", partContentType)
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(tc.originalPayload)
		require.NoError(t, err)
		for _, extra := range tc.extraMultipartParts {
			header := make(textproto.MIMEHeader)
			disposition := fmt.Sprintf(`form-data; name="%s"`, extra.fieldName)
			if extra.filename != "" {
				disposition += fmt.Sprintf(`; filename="%s"`, extra.filename)
			}
			header.Set("Content-Disposition", disposition)
			if extra.contentType != "" {
				header.Set("Content-Type", extra.contentType)
			}
			extraWriter, createErr := writer.CreatePart(header)
			require.NoError(t, createErr)
			_, writeErr := extraWriter.Write(extra.originalPayload)
			require.NoError(t, writeErr)
		}
		require.NoError(t, writer.Close())
		body = buf.Bytes()
		contentType = writer.FormDataContentType()
	}
	header := fmt.Sprintf(
		"POST /outcome/%s/%s HTTP/1.1\r\nHost: %s\r\nContent-Type: %s\r\nContent-Length: %d\r\nX-Outcome-Case: %s\r\nConnection: close\r\n\r\n",
		tc.name,
		token,
		target,
		contentType,
		len(body),
		token,
	)
	return append([]byte(header), body...)
}

type mitmV2CapturedRequest struct {
	body        []byte
	contentType string
	edited      string
	userFuzzTag string
}

var mitmV2FileTagPattern = regexp.MustCompile(`\{\{file\(([^)\r\n|]+)\)\}\}`)

func mitmV2FileTagPaths(packet []byte) []string {
	matches := mitmV2FileTagPattern.FindAllSubmatch(packet, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, string(match[1]))
	}
	return paths
}

type mitmV2MultipartFileTagPart struct {
	physicalIndex int32
	fieldName     string
	path          string
}

func mitmV2MultipartFileTagParts(t *testing.T, packet []byte) []mitmV2MultipartFileTagPart {
	t.Helper()
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	mediaType, params, err := mime.ParseMediaType(lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var tagged []mitmV2MultipartFileTagPart
	for physicalIndex := int32(0); ; physicalIndex++ {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		partBody, err := io.ReadAll(part)
		require.NoError(t, err)
		matches := mitmV2FileTagPattern.FindAllSubmatch(partBody, -1)
		for _, match := range matches {
			tagged = append(tagged, mitmV2MultipartFileTagPart{
				physicalIndex: physicalIndex,
				fieldName:     part.FormName(),
				path:          string(match[1]),
			})
		}
	}
	return tagged
}

func mitmV2MultipartFileTagFields(t *testing.T, packet []byte) []string {
	t.Helper()
	parts := mitmV2MultipartFileTagParts(t, packet)
	var fields []string
	for _, part := range parts {
		fields = append(fields, part.fieldName)
	}
	return fields
}

func mitmV2FileTagPartIndex(t *testing.T, packet []byte) int32 {
	return mitmV2FileTagPartIndexForField(t, packet, "")
}

func mitmV2FileTagPartIndexForField(t *testing.T, packet []byte, fieldName string) int32 {
	t.Helper()
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	_, params, err := mime.ParseMediaType(lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type"))
	require.NoError(t, err)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	for index := int32(0); ; index++ {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		partBody, err := io.ReadAll(part)
		require.NoError(t, err)
		if mitmV2FileTagPattern.Match(partBody) && (fieldName == "" || part.FormName() == fieldName) {
			return index
		}
	}
	t.Fatal("multipart file tag part not found")
	return -1
}

func extractMITMV2OutcomeMultipartParts(t *testing.T, captured mitmV2CapturedRequest) map[string]mitmV2OutcomeMultipartPart {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(captured.contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(captured.body), params["boundary"])
	parts := make(map[string]mitmV2OutcomeMultipartPart)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		body, err := io.ReadAll(part)
		require.NoError(t, err)
		parts[part.FormName()] = mitmV2OutcomeMultipartPart{
			fieldName:       part.FormName(),
			filename:        part.FileName(),
			contentType:     part.Header.Get("Content-Type"),
			originalPayload: body,
			sentPayload:     body,
		}
	}
	return parts
}

func expectedMITMV2OutcomeMultipartParts(tc mitmV2RequestOutcomeCase, original bool) []mitmV2OutcomeMultipartPart {
	if !tc.multipart {
		return nil
	}
	filename := tc.uploadFilename
	if filename == "" {
		filename = "sample.bin"
	}
	contentType := tc.uploadContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	parts := []mitmV2OutcomeMultipartPart{
		{fieldName: "note", originalPayload: []byte("editable"), sentPayload: []byte("editable")},
		{
			fieldName:       "upload",
			filename:        filename,
			contentType:     contentType,
			originalPayload: tc.originalPayload,
			sentPayload:     tc.sentPayload,
		},
	}
	for _, extra := range tc.extraMultipartParts {
		copy := extra
		if copy.sentPayload == nil {
			copy.sentPayload = copy.originalPayload
		}
		parts = append(parts, copy)
	}
	if original {
		for index := range parts {
			parts[index].sentPayload = parts[index].originalPayload
		}
	}
	return parts
}

func requireMITMV2OutcomeMultipartParts(
	t *testing.T,
	captured mitmV2CapturedRequest,
	expected []mitmV2OutcomeMultipartPart,
	label string,
) {
	t.Helper()
	actual := extractMITMV2OutcomeMultipartParts(t, captured)
	require.Len(t, actual, len(expected), "%s part count", label)
	for _, want := range expected {
		got, ok := actual[want.fieldName]
		require.True(t, ok, "%s missing part %q", label, want.fieldName)
		require.Equal(t, want.filename, got.filename, "%s filename for %q", label, want.fieldName)
		require.Equal(t, want.contentType, got.contentType, "%s content type for %q", label, want.fieldName)
		require.Equal(t, want.sentPayload, got.sentPayload, "%s bytes for %q", label, want.fieldName)
	}
}

func expectedMITMV2OutcomeFileParts(
	tc mitmV2RequestOutcomeCase,
	original bool,
	selectedFields []string,
) []mitmV2OutcomeMultipartPart {
	if !tc.multipart {
		payload := tc.sentPayload
		if original {
			payload = tc.originalPayload
		}
		return []mitmV2OutcomeMultipartPart{{originalPayload: tc.originalPayload, sentPayload: payload}}
	}
	selected := make(map[string]struct{}, len(selectedFields))
	for _, fieldName := range selectedFields {
		selected[fieldName] = struct{}{}
	}
	var files []mitmV2OutcomeMultipartPart
	for _, part := range expectedMITMV2OutcomeMultipartParts(tc, original) {
		if _, ok := selected[part.fieldName]; ok {
			files = append(files, part)
		}
	}
	return files
}

func requireMITMV2OutcomeMultipartFileTags(
	t *testing.T,
	packet []byte,
	tc mitmV2RequestOutcomeCase,
	original bool,
	wantFields []string,
	label string,
) []mitmV2MultipartFileTagPart {
	t.Helper()
	tagged := mitmV2MultipartFileTagParts(t, packet)
	require.Len(t, tagged, len(wantFields), "%s resource count", label)
	expectedParts := expectedMITMV2OutcomeMultipartParts(tc, original)
	for ordinal, wantField := range wantFields {
		require.Equal(t, wantField, tagged[ordinal].fieldName, "%s resource field order", label)
		expectedIndex := -1
		for physicalIndex, expectedPart := range expectedParts {
			if expectedPart.fieldName == wantField {
				expectedIndex = physicalIndex
				break
			}
		}
		require.NotEqual(t, -1, expectedIndex, "%s unknown resource field %q", label, wantField)
		if expectedIndex < 0 {
			continue
		}
		require.Equal(t, int32(expectedIndex), tagged[ordinal].physicalIndex, "%s physical part index", label)
		require.FileExists(t, tagged[ordinal].path)
		storedPart, err := os.ReadFile(tagged[ordinal].path)
		require.NoError(t, err)
		require.Equal(t, expectedParts[expectedIndex].sentPayload, storedPart, "%s resource bytes for %q", label, wantField)
	}
	return tagged
}

func extractMITMV2OutcomePayload(t *testing.T, captured mitmV2CapturedRequest, multipartPayload bool) []byte {
	t.Helper()
	if !multipartPayload {
		return bytes.Clone(captured.body)
	}
	mediaType, params, err := mime.ParseMediaType(captured.contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(captured.body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		partBody, err := io.ReadAll(part)
		require.NoError(t, err)
		if part.FormName() == "upload" {
			return partBody
		}
	}
	t.Fatal("multipart upload part not found")
	return nil
}

func extractMITMV2OutcomeMultipartFileMetadata(t *testing.T, captured mitmV2CapturedRequest) (string, string) {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(captured.contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(captured.body), params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if part.FormName() == "upload" {
			return part.FileName(), part.Header.Get("Content-Type")
		}
	}
	t.Fatal("multipart upload part not found")
	return "", ""
}

// replaceMITMV2OutcomeInlineUpload models the exact packet produced when the
// renderer opens a multipart Binary chip, edits its bytes in the HEX editor,
// and writes the edited bytes back as one {{unquote(...)}} part. Keeping this
// transformation in the integration test is important: merely editing an HTTP
// header does not prove that SendPacket expands the edited binary tag.
func replaceMITMV2OutcomeInlinePayload(t *testing.T, packet, editedPayload []byte, multipartPacket bool) []byte {
	t.Helper()
	if !multipartPacket {
		return lowhttp.ReplaceHTTPPacketBody(packet, []byte(lowhttp.ToUnquoteFuzzTagForce(editedPayload)), false)
	}
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	mediaType, params, err := mime.ParseMediaType(lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)

	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var rebuilt bytes.Buffer
	writer := multipart.NewWriter(&rebuilt)
	require.NoError(t, writer.SetBoundary(params["boundary"]))
	foundUpload := false
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		partBody, err := io.ReadAll(part)
		require.NoError(t, err)
		out, err := writer.CreatePart(part.Header)
		require.NoError(t, err)
		if part.FormName() == "upload" {
			foundUpload = true
			_, err = out.Write([]byte(lowhttp.ToUnquoteFuzzTagForce(editedPayload)))
		} else {
			_, err = out.Write(partBody)
		}
		require.NoError(t, err)
	}
	require.True(t, foundUpload)
	require.NoError(t, writer.Close())
	return lowhttp.ReplaceHTTPPacketBody([]byte(header), rebuilt.Bytes(), false)
}

// requireMITMV2OutcomeBinaryExportContract models the data boundary used by
// History's Binary chip. The packet exposed to the renderer must be UTF-8-safe
// Fuzztag text, while rendering that text ("导出原始数据") must recover the
// exact file bytes. Keeping literal backticks out also makes "导出带FuzzTag的
// 数据" safe to embed in the common fuzz.Strings(`...`) workflow.
func requireMITMV2OutcomeBinaryExportContract(
	t *testing.T,
	packet []byte,
	wantPayload []byte,
	multipartPacket bool,
	label string,
) {
	t.Helper()
	require.True(t, utf8.Valid(packet), "%s History packet must be safe editor text", label)
	require.Contains(t, string(packet), "{{unquote(", "%s must expose a Binary chip", label)
	require.NotContains(t, string(packet), "`", "%s Fuzztag export must be Yak raw-string safe", label)

	rendered, err := renderMITMSubmittedRequest(packet)
	require.NoError(t, err)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	actual := extractMITMV2OutcomePayload(t, mitmV2CapturedRequest{
		body:        body,
		contentType: lowhttp.GetHTTPPacketHeader(rendered, "Content-Type"),
	}, multipartPacket)
	require.Equal(t, wantPayload, actual, "%s raw export must recover the exact source bytes", label)
}

func replayMITMV2OutcomeRequest(t *testing.T, client ypb.YakClient, request []byte, replayKind string) {
	t.Helper()
	request = lowhttp.ReplaceHTTPPacketHeader(request, "X-Outcome-Replay", replayKind)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request:   string(request),
		ForceFuzz: true,
	})
	require.NoError(t, err)
	for {
		if _, err := stream.Recv(); err != nil {
			break
		}
	}
}

// This is the cross-layer outcome contract requested for MITM V2. A real MITM
// proxy and mock target server verify the editor representation, wire request,
// persisted HTTPFlow, original/bare snapshot and WebFuzzer replay. The cases
// are equivalence classes from the larger unit-test Cartesian matrix, keeping
// this CI test bounded while still crossing every subsystem boundary.
func TestGRPCMUSTPASS_MITMV2_RequestOutcomeMatrix(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	previousLimit := consts.GetGlobalMaxContentLength()
	consts.SetGlobalMaxContentLength(512 * 1024)
	t.Cleanup(func() { consts.SetGlobalMaxContentLength(previousLimit) })

	// These checked-in files are structurally valid user documents, not merely
	// repeated bytes with a magic prefix. Reading and validating them here keeps
	// the cross-layer test representative of the ZIP/PDF/JPEG uploads that found
	// bugs missed by earlier all-"a" and all-0xa5 fixtures.
	zipOriginal := readMITMBinaryRepositoryFixture(t, "common", "aireducer", "testdata", "demo.txt.zip")
	requireRealZIPFixture(t, zipOriginal)
	zipHexEdited := bytes.Clone(zipOriginal)
	for i := 0; i < 29; i++ {
		zipHexEdited[i] = 0x11
	}
	pdfOriginal := readMITMBinaryRepositoryFixture(t, "vtestdata", "demo.pdf")
	requireRealPDFFixture(t, pdfOriginal)
	require.Greater(t, len(pdfOriginal), 256*1024)
	pdfInline := readMITMBinaryRepositoryFixture(t, "vtestdata", "demo1.pdf")
	requireRealPDFFixture(t, pdfInline)
	pdfInlineEdited := bytes.Clone(pdfInline)
	for i := 0; i < 16; i++ {
		pdfInlineEdited[i] = 0x11
	}
	pdfReplacement := readMITMBinaryRepositoryFixture(t, "vtestdata", "zwb.pdf")
	requireRealPDFFixture(t, pdfReplacement)
	require.Greater(t, len(pdfReplacement), 512*1024)
	require.NotEmpty(t, embedJPEG)
	_, err := jpeg.DecodeConfig(bytes.NewReader(embedJPEG))
	require.NoError(t, err)
	jpegHexEdited := bytes.Clone(embedJPEG)
	for i := 0; i < 16; i++ {
		jpegHexEdited[i] = 0x11
	}
	readableTextReplacement := []byte("b'<!DOCTYPE html>\n<html lang=\"en-GB\">\n<title>Vulnerability: SQL Injection (Blind)</title>\n</html>'")
	cases := []mitmV2RequestOutcomeCase{
		{
			name:            "small-text-forward",
			originalPayload: []byte("plain-text"),
			sentPayload:     []byte("plain-text"),
			contentType:     "text/plain",
			forwardOriginal: true,
			wantHijackRaw:   true,
			wantCurrentRaw:  true,
		},
		{
			// Non-multipart Binary-chip edit: the intercepted payload is a real
			// JPEG, and the submitted body is a distinct HEX-edited byte sequence.
			// This prevents a header-only edit from masquerading as binary coverage.
			name:                     "small-binary-edit",
			originalPayload:          embedJPEG,
			inlineEditedPayload:      jpegHexEdited,
			sentPayload:              jpegHexEdited,
			contentType:              "image/jpeg",
			wantHijackTag:            "{{unquote(",
			wantCurrentBinaryExports: true,
			wantBareBinaryExports:    true,
		},
		{
			// Physical part indexes include ordinary fields. This packet is:
			// 0=note, 1=ZIP, 2=description, 3=PDF. The PDF's editor form is
			// over D, so only part 3 is externalized; the ZIP remains an inline
			// unquote part. Only part 3 is replaced, and both files must remain
			// independently represented and replayable across every layer.
			name:                  "multipart-two-real-files-replace-second",
			originalPayload:       zipOriginal,
			sentPayload:           zipOriginal,
			multipart:             true,
			uploadFilename:        "archive.zip",
			uploadContentType:     "application/zip",
			replacementPayload:    pdfReplacement,
			replacementPartName:   "attachment",
			wantHijackTag:         "{{file(",
			wantResourceFlow:      true,
			wantHijackFileFields:  []string{"attachment"},
			wantCurrentFileFields: []string{"attachment"},
			wantBareFileFields:    []string{"attachment"},
			deleteAfterValidation: true,
			extraMultipartParts: []mitmV2OutcomeMultipartPart{
				{fieldName: "description", originalPayload: []byte("keep-this-field")},
				{
					fieldName:       "attachment",
					filename:        "report.pdf",
					contentType:     "application/pdf",
					originalPayload: pdfOriginal,
					sentPayload:     pdfReplacement,
				},
			},
		},
		{
			// A Fuzztag typed by the user is a send-time template, not History
			// data. MITM renders it once; the target, HTTPFlow and WebFuzzer
			// replay must all observe the same concrete result.
			name:                    "manual-user-fuzztag",
			originalPayload:         []byte("original"),
			manualFuzzTagPayload:    []byte("rendered={{int(7)}}"),
			sentPayload:             []byte("rendered=7"),
			contentType:             "text/plain",
			wantCurrentRaw:          true,
			wantRenderedUserFuzzTag: true,
		},
		{
			name:            "small-binary-mime-valid-utf8",
			originalPayload: []byte("printable PDF bytes"),
			sentPayload:     []byte("printable PDF bytes"),
			contentType:     "application/pdf",
			wantHijackRaw:   true,
			wantCurrentRaw:  true,
		},
		{
			name:              "multipart-binary-mime-valid-utf8",
			originalPayload:   []byte("readable PDF part"),
			sentPayload:       []byte("readable PDF part"),
			multipart:         true,
			uploadFilename:    "readable.pdf",
			uploadContentType: "application/pdf",
			forwardOriginal:   true,
			wantHijackRaw:     true,
			wantCurrentRaw:    true,
		},
		{
			// Exact GUI contract: a small ZIP is represented by an inline
			// unquote chip; the user overwrites its first HEX row with 0x11.
			// The target, HTTPFlow replay, and History representation must all
			// preserve bytes 0x11, never the four ASCII bytes "\\x11".
			name:                     "multipart-inline-zip-hex-edit",
			originalPayload:          zipOriginal,
			inlineEditedPayload:      zipHexEdited,
			sentPayload:              zipHexEdited,
			multipart:                true,
			uploadFilename:           "sample.zip",
			uploadContentType:        "application/zip",
			wantHijackTag:            "{{unquote(",
			wantCurrentBinaryExports: true,
			wantBareBinaryExports:    true,
		},
		{
			// The real PDF export regression: both the HEX-edited current file
			// and the captured original file stay below D after unquote expansion.
			// History must expose two independent Binary-chip export sources.
			name:                     "multipart-inline-real-pdf-hex-edit",
			originalPayload:          pdfInline,
			inlineEditedPayload:      pdfInlineEdited,
			sentPayload:              pdfInlineEdited,
			multipart:                true,
			uploadFilename:           "document.pdf",
			uploadContentType:        "application/pdf",
			wantHijackTag:            "{{unquote(",
			wantCurrentBinaryExports: true,
			wantBareBinaryExports:    true,
		},
		{
			name:            "large-binary-mime-valid-utf8-below-D",
			originalPayload: bytes.Repeat([]byte{'w'}, 300*1024),
			sentPayload:     bytes.Repeat([]byte{'w'}, 300*1024),
			contentType:     "application/octet-stream",
			wantHijackRaw:   true,
			wantCurrentRaw:  true,
		},
		{
			name:               "large-binary-replace",
			originalPayload:    pdfOriginal,
			replacementPayload: pdfReplacement,
			sentPayload:        pdfReplacement,
			contentType:        "application/pdf",
			wantHijackTag:      "{{file(",
			wantResourceFlow:   true,
			wantBareFileTags:   1,
		},
		{
			name:               "large-binary-forward-discards-replacement",
			originalPayload:    pdfOriginal,
			replacementPayload: pdfReplacement,
			sentPayload:        pdfOriginal,
			contentType:        "application/pdf",
			forwardOriginal:    true,
			wantHijackTag:      "{{file(",
			wantResourceFlow:   true,
		},
		{
			name:             "large-binary-view-response",
			originalPayload:  pdfOriginal,
			sentPayload:      pdfOriginal,
			contentType:      "application/pdf",
			hijackResponse:   true,
			wantHijackTag:    "{{file(",
			wantResourceFlow: true,
		},
		{
			name:             "large-binary-drop",
			originalPayload:  pdfOriginal,
			sentPayload:      pdfOriginal,
			contentType:      "application/pdf",
			dropRequest:      true,
			wantHijackTag:    "{{file(",
			wantResourceFlow: true,
		},
		{
			name:                  "multipart-file-replace",
			originalPayload:       pdfOriginal,
			replacementPayload:    pdfReplacement,
			sentPayload:           pdfReplacement,
			multipart:             true,
			uploadFilename:        "report.pdf",
			uploadContentType:     "application/pdf",
			wantHijackTag:         "{{file(",
			wantResourceFlow:      true,
			wantHijackFileFields:  []string{"upload"},
			wantCurrentFileFields: []string{"upload"},
			wantBareFileFields:    []string{"upload"},
		},
		{
			// This is the bounded CI analogue of a user intercepting a PDF below
			// the 50 MiB dump limit and replacing it with a tiny text file. The
			// raw multipart request remains below the 512 KiB test dump limit D,
			// while its exact unquote representation exceeds D.
			// The current request must become inline, while GetHTTPFlowBare must
			// retain a bounded {{file}} tag for the original PDF.
			name:                     "multipart-pdf-replace-small",
			originalPayload:          pdfOriginal,
			replacementPayload:       readableTextReplacement,
			localReplacementFilename: "replacement.txt",
			localReplacementType:     "text/plain; charset=utf-8",
			sentPayload:              readableTextReplacement,
			multipart:                true,
			uploadFilename:           "original.pdf",
			uploadContentType:        "application/pdf",
			wantHijackTag:            "{{file(",
			wantCurrentRaw:           true,
			wantBareResource:         true,
			wantHijackFileFields:     []string{"upload"},
			wantBareFileFields:       []string{"upload"},
			wantManualEditTag:        true,
		},
	}

	tokenList := make([]string, 0, len(cases))
	tokensByName := make(map[string]string, len(cases))
	casesByName := make(map[string]mitmV2RequestOutcomeCase, len(cases))
	for _, tc := range cases {
		token := "mitmv2-outcome-" + tc.name + "-" + utils.RandStringBytes(8)
		tokensByName[tc.name] = token
		tokenList = append(tokenList, token)
		casesByName[tc.name] = tc
	}
	registerHTTPFlowTokenCleanup(t, tokenList...)

	mockCtx, mockCancel := context.WithCancel(context.Background())
	t.Cleanup(mockCancel)
	var receivedMu sync.Mutex
	received := make(map[string]mitmV2CapturedRequest)
	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(mockCtx, func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		token := request.Header.Get("X-Outcome-Case")
		replayKind := request.Header.Get("X-Outcome-Replay")
		if replayKind == "" {
			replayKind = "wire"
		}
		receivedMu.Lock()
		received[token+"/"+replayKind] = mitmV2CapturedRequest{
			body:        bytes.Clone(body),
			contentType: request.Header.Get("Content-Type"),
			edited:      request.Header.Get("X-Outcome-Edited"),
			userFuzzTag: request.Header.Get("X-User-Fuzztag"),
		}
		receivedMu.Unlock()
		writer.Header().Set("Content-Type", "text/plain")
		writer.Header().Set("Connection", "close")
		_, err = writer.Write([]byte("ok"))
		require.NoError(t, err)
	})
	target := utils.HostPort(mockHost, mockPort)

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxyAddress := utils.HostPort("127.0.0.1", mitmPort)
	proxy := "http://" + proxyAddress
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	packetsByName := make(map[string][]byte, len(cases))
	for _, tc := range cases {
		packetsByName[tc.name] = buildMITMV2OutcomePacket(t, target, tokensByName[tc.name], tc)
	}

	var handled sync.Map
	var handledResponses sync.Map
	RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
		require.NoError(t, stream.Send(&ypb.MITMV2Request{Host: "127.0.0.1", Port: uint32(mitmPort)}))
		require.NoError(t, stream.Send(&ypb.MITMV2Request{SetAutoForward: true, AutoForwardValue: false}))
	}, func(stream ypb.Yak_MITMV2Client) {
		require.NoError(t, utils.WaitConnect(utils.HostPort("127.0.0.1", mitmPort), 5))
		for _, tc := range cases {
			if tc.dropRequest {
				conn, err := net.DialTimeout("tcp", proxyAddress, 3*time.Second)
				require.NoError(t, err)
				_, err = conn.Write(packetsByName[tc.name])
				require.NoError(t, err)
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(3*time.Second)))
				response := make([]byte, 1024)
				// MITM may synthesize a local response to finish the browser-side
				// connection. The authoritative drop assertion is that the target
				// capture has no /wire entry, checked after History replay below.
				_, _ = conn.Read(response)
				require.NoError(t, conn.Close())
				continue
			}
			response, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packetsByName[tc.name]),
				lowhttp.WithProxy(proxy),
				lowhttp.WithTimeout(15*time.Second),
				lowhttp.WithSaveHTTPFlow(false),
			)
			require.NoError(t, err)
			require.Contains(t, string(response.RawPacket), "200 OK")
		}
		time.Sleep(800 * time.Millisecond)
		cancel()
	}, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
		if len(msg.GetManualHijackList()) != 1 {
			return
		}
		task := msg.GetManualHijackList()[0]
		if task.GetStatus() == Hijack_Status_Response {
			if msg.GetManualHijackListAction() != Hijack_List_Update {
				return
			}
			if _, loaded := handledResponses.LoadOrStore(task.GetTaskID(), struct{}{}); loaded {
				return
			}
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{TaskID: task.GetTaskID(), Forward: true},
			}))
			return
		}
		if msg.GetManualHijackListAction() != Hijack_List_Add {
			return
		}
		if _, loaded := handled.LoadOrStore(task.GetTaskID(), struct{}{}); loaded {
			return
		}
		var tc *mitmV2RequestOutcomeCase
		for name, candidate := range casesByName {
			if bytes.Contains(task.GetRequest(), []byte("/outcome/"+name)) {
				copy := candidate
				tc = &copy
				break
			}
		}
		require.NotNil(t, tc)
		if tc == nil {
			return
		}
		if tc.multipart {
			require.Equal(t, tc.wantHijackFileFields, mitmV2MultipartFileTagFields(t, task.GetRequest()),
				"MITM editor must externalize exactly the selected multipart parts for %s", tc.name)
			requireMITMV2OutcomeMultipartFileTags(
				t, task.GetRequest(), *tc, true, tc.wantHijackFileFields, tc.name+" hijack request",
			)
			rendered, err := renderMITMSubmittedRequest(task.GetRequest())
			require.NoError(t, err)
			_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
			require.Equal(t, strconv.Itoa(len(renderedBody)), lowhttp.GetHTTPPacketHeader(rendered, "Content-Length"),
				"rendered MITM request Content-Length for %s", tc.name)
			requireMITMV2OutcomeMultipartParts(t, mitmV2CapturedRequest{
				body:        renderedBody,
				contentType: lowhttp.GetHTTPPacketHeader(rendered, "Content-Type"),
			}, expectedMITMV2OutcomeMultipartParts(*tc, true), tc.name+" rendered hijack request")
		}
		if tc.wantHijackTag != "" {
			require.Contains(t, string(task.GetRequest()), tc.wantHijackTag, "MITM editor representation for %s", tc.name)
		}
		if tc.wantHijackRaw {
			require.NotContains(t, string(task.GetRequest()), "{{unquote(", "valid UTF-8 must not become a binary chip for %s", tc.name)
			require.False(t, mitmV2FileTagPattern.Match(task.GetRequest()), "in-limit valid UTF-8 must stay inline for %s", tc.name)
			_, taskBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(task.GetRequest())
			if tc.multipart {
				captured := mitmV2CapturedRequest{
					body:        taskBody,
					contentType: lowhttp.GetHTTPPacketHeader(task.GetRequest(), "Content-Type"),
				}
				require.Equal(t, tc.originalPayload, extractMITMV2OutcomePayload(t, captured, true))
			} else {
				require.Equal(t, tc.originalPayload, taskBody)
			}
		}
		if strings.Contains(tc.wantHijackTag, "file") {
			require.True(t, utf8.Valid(task.GetRequest()), "resource-backed hijack packets must be editor-safe UTF-8")
			_, editorBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(task.GetRequest())
			require.LessOrEqual(t, len(editorBody), yakit.GetMaxHTTPFlowRequestBodyInDBBytes(),
				"resource-backed hijack body must obey D when its multipart skeleton fits")
		}
		if strings.Contains(tc.wantHijackTag, "file") && tc.replacementPayload != nil {
			// Frontend contract at the manual-hijack boundary: Monaco receives a
			// bounded UTF-8 packet containing the standard file tag. Multipart
			// headers retain the user-facing filename/type, while the part index
			// used by the replacement RPC is derived from the multipart structure.
			// Externalized part bytes never enter renderer text.
			require.True(t, utf8.Valid(task.GetRequest()))
			paths := mitmV2FileTagPaths(task.GetRequest())
			require.NotEmpty(t, paths)
			if tc.wantBareResource {
				require.Len(t, paths, 1)
				require.Contains(t, string(task.GetRequest()), `filename="`+tc.uploadFilename+`"`)
				require.Contains(t, string(task.GetRequest()), "Content-Type: "+tc.uploadContentType)
				require.False(t, bytes.Contains(task.GetRequest(), tc.originalPayload[:len("%PDF-1.7\n")]))
			}
			partIndex := int32(0)
			if tc.multipart {
				if tc.replacementPartName != "" {
					partIndex = mitmV2FileTagPartIndexForField(t, task.GetRequest(), tc.replacementPartName)
				} else {
					partIndex = mitmV2FileTagPartIndex(t, task.GetRequest())
				}
			}
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
					TaskID:                  task.GetTaskID(),
					IsLargeRequestFileChunk: true,
					LargeRequestReplaceBody: !tc.multipart,
					LargeRequestPartIndex:   partIndex,
					LargeRequestFileData:    tc.replacementPayload,
					LargeRequestFileStart:   true,
					LargeRequestFileEOF:     true,
				},
			}))
		}
		if tc.dropRequest {
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{TaskID: task.GetTaskID(), Drop: true},
			}))
			return
		}
		if tc.forwardOriginal {
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{TaskID: task.GetTaskID(), Forward: true},
			}))
			return
		}
		if tc.hijackResponse {
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
					TaskID: task.GetTaskID(), Request: task.GetRequest(), SendPacket: true, HijackResponse: true,
				},
			}))
			return
		}
		submitted := task.GetRequest()
		if tc.inlineEditedPayload != nil {
			submitted = replaceMITMV2OutcomeInlinePayload(t, submitted, tc.inlineEditedPayload, tc.multipart)
		}
		if tc.manualFuzzTagPayload != nil {
			submitted = lowhttp.ReplaceHTTPPacketBody(submitted, tc.manualFuzzTagPayload, false)
			submitted = lowhttp.ReplaceHTTPPacketHeader(submitted, "X-User-Fuzztag", "{{int(8)}}")
		}
		edited := lowhttp.ReplaceHTTPPacketHeader(submitted, "X-Outcome-Edited", "true")
		require.NoError(t, stream.Send(&ypb.MITMV2Request{
			ManualHijackControl: true,
			ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
				TaskID:     task.GetTaskID(),
				SendPacket: true,
				Request:    edited,
			},
		}))
	})

	for _, tc := range cases {
		token := tokensByName[tc.name]
		t.Logf("asserting persisted and replayed outcome for %s", tc.name)
		flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(8), client, &ypb.QueryHTTPFlowRequest{
			SearchURL:  token,
			SourceType: "mitm",
			Full:       true,
			Pagination: &ypb.Paging{Page: 1, Limit: 10},
		}, 1)
		require.NoError(t, err)
		flow := flows.GetData()[0]
		currentRequest := flow.GetRequest()
		if flow.GetSafeHTTPRequest() != "" {
			currentRequest = []byte(flow.GetSafeHTTPRequest())
		}
		var persisted schema.HTTPFlow
		require.NoError(t, consts.GetGormProjectDatabase().First(&persisted, flow.GetId()).Error)
		var deleteResourcePaths []string
		var deleteResourceDirs []string
		if tc.deleteAfterValidation {
			deleteResourcePaths = append(deleteResourcePaths, mitmV2FileTagPaths(currentRequest)...)
			for _, path := range []string{persisted.TooLargeRequestHeaderFile, persisted.TooLargeRequestBodyFile} {
				if path == "" {
					continue
				}
				require.FileExists(t, path)
				deleteResourcePaths = append(deleteResourcePaths, path)
			}
			if tc.multipart && persisted.TooLargeRequestBodyFile != "" {
				dir := filepath.Dir(persisted.TooLargeRequestBodyFile)
				require.DirExists(t, dir)
				require.FileExists(t, filepath.Join(dir, "manifest.json"))
				deleteResourceDirs = append(deleteResourceDirs, dir)
			}
		}
		require.Equal(t, flow.GetIsTooLargeRequest(), persisted.IsTooLargeRequest)
		persistedRequest := []byte(persisted.GetRequest())
		if flow.GetSafeHTTPRequest() != "" {
			require.Equal(t, currentRequest, lowhttp.ConvertHTTPRequestToFuzzTag(persistedRequest),
				"gRPC SafeHTTPRequest must be derived from the bytes actually stored in HTTPFlow for %s", tc.name)
		} else {
			require.Equal(t, currentRequest, persistedRequest,
				"frontend Request must equal the packet stored in HTTPFlow for %s", tc.name)
		}
		require.Equal(t, tc.wantResourceFlow, mitmV2FileTagPattern.Match(currentRequest))
		require.Equal(t, tc.wantResourceFlow, flow.GetIsTooLargeRequest())
		require.Len(t, flow.GetMultipartFiles(), len(tc.wantCurrentFileFields),
			"History list query must expose the multipart manifest contract for %s", tc.name)
		if tc.multipart {
			detail, err := client.GetHTTPFlowById(utils.TimeoutContextSeconds(5), &ypb.GetHTTPFlowByIdRequest{Id: int64(flow.GetId())})
			require.NoError(t, err)
			require.Len(t, detail.GetMultipartFiles(), len(tc.wantCurrentFileFields),
				"History detail query is the frontend's authoritative multipart metadata for %s", tc.name)
			detailRequest := detail.GetRequest()
			if detail.GetSafeHTTPRequest() != "" {
				detailRequest = []byte(detail.GetSafeHTTPRequest())
			}
			require.Equal(t, currentRequest, detailRequest,
				"History list and detail queries must expose the same editor packet for %s", tc.name)
			taggedCurrent := requireMITMV2OutcomeMultipartFileTags(
				t, currentRequest, tc, false, tc.wantCurrentFileFields, tc.name+" current History request",
			)
			for fileOrdinal, taggedPart := range taggedCurrent {
				expectedPart := expectedMITMV2OutcomeMultipartParts(tc, false)[taggedPart.physicalIndex]
				fileInfo := detail.GetMultipartFiles()[fileOrdinal]
				require.Equal(t, taggedPart.physicalIndex, fileInfo.GetPartIndex())
				require.Equal(t, expectedPart.filename, fileInfo.GetFilename())
				require.Equal(t, expectedPart.contentType, fileInfo.GetContentType())
				require.Equal(t, int64(len(expectedPart.sentPayload)), fileInfo.GetSize())
				require.Equal(t, taggedPart.path, fileInfo.GetFilePath())
			}
		}
		if tc.wantCurrentRaw {
			require.Empty(t, flow.GetSafeHTTPRequest(), "valid UTF-8 current requests stay in Request for %s", tc.name)
			require.NotContains(t, string(currentRequest), "{{unquote(")
			require.False(t, mitmV2FileTagPattern.Match(currentRequest))
		}
		if tc.wantCurrentBinaryExports {
			require.NotEmpty(t, flow.GetSafeHTTPRequest(),
				"binary current request must reach the frontend through SafeHTTPRequest for %s", tc.name)
			requireMITMV2OutcomeBinaryExportContract(t, currentRequest, tc.sentPayload, tc.multipart, tc.name+" current request")
		}
		if tc.wantManualEditTag {
			// HTTP History derives all user-visible branching from these fields:
			// [手动修改] exposes the 原始请求 toggle; currentRequest is the
			// actual sent packet; IsTooLargeRequest controls only the current tab.
			require.Contains(t, flow.GetTags(), yakit.HTTPFlowTagManualEdit)
			require.False(t, flow.GetInvalidForUTF8Request())
			// Whole-file replacement changes only the part bytes. The intercepted
			// request remains authoritative for multipart metadata, so both the wire
			// request and History keep original.pdf/application/pdf. The replacement
			// bytes are valid UTF-8, so History exposes them as raw text despite that
			// binary MIME instead of manufacturing a binary chip.
			require.Empty(t, flow.GetSafeHTTPRequest())
			require.True(t, utf8.Valid(currentRequest))
			require.Less(t, len(currentRequest), 8*1024)
			require.False(t, mitmV2FileTagPattern.Match(currentRequest))
			require.NotContains(t, string(currentRequest), "{{unquote(")
			require.Contains(t, string(currentRequest), string(readableTextReplacement))
			require.Contains(t, string(currentRequest), tc.uploadFilename)
			require.Contains(t, string(currentRequest), "Content-Type: "+tc.uploadContentType)
			require.NotContains(t, string(currentRequest), tc.localReplacementFilename)
			require.NotContains(t, string(currentRequest), "Content-Type: "+tc.localReplacementType)
		}
		if tc.wantRenderedUserFuzzTag {
			_, storedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(currentRequest)
			require.Equal(t, tc.sentPayload, storedBody)
			require.Equal(t, "8", lowhttp.GetHTTPPacketHeader(currentRequest, "X-User-Fuzztag"))
			require.NotContains(t, string(currentRequest), "{{int(", "HTTPFlow must store the concrete request sent on the wire")
		}
		if tc.dropRequest {
			require.Contains(t, flow.GetTags(), yakit.HTTPFlowTagDiscarded)
		}
		replayMITMV2OutcomeRequest(t, client, currentRequest, "current")
		if tc.wantResourceFlow {
			webFuzzerReplacement := []byte("webfuzzer-replacement-" + tc.name)
			replacementPath := filepath.Join(t.TempDir(), "webfuzzer-replacement.bin")
			require.NoError(t, os.WriteFile(replacementPath, webFuzzerReplacement, 0o600))
			webFuzzerRequest := bytes.Clone(currentRequest)
			paths := mitmV2FileTagPaths(currentRequest)
			require.NotEmpty(t, paths)
			webFuzzerRequest = mitmV2FileTagPattern.ReplaceAllFunc(webFuzzerRequest, func([]byte) []byte {
				return []byte("{{file(" + replacementPath + ")}}")
			})
			replayMITMV2OutcomeRequest(t, client, webFuzzerRequest, "webfuzzer-replaced")
			receivedMu.Lock()
			webFuzzerReplay := received[token+"/webfuzzer-replaced"]
			receivedMu.Unlock()
			wantWebFuzzerUpload := webFuzzerReplacement
			if tc.multipart {
				wantWebFuzzerUpload = tc.sentPayload
				for _, fieldName := range tc.wantCurrentFileFields {
					if fieldName == "upload" {
						wantWebFuzzerUpload = webFuzzerReplacement
						break
					}
				}
			}
			require.Equal(t, wantWebFuzzerUpload, extractMITMV2OutcomePayload(t, webFuzzerReplay, tc.multipart))
			if len(tc.extraMultipartParts) > 0 {
				parts := extractMITMV2OutcomeMultipartParts(t, webFuzzerReplay)
				replacedFields := make(map[string]struct{}, len(tc.wantCurrentFileFields))
				for _, fieldName := range tc.wantCurrentFileFields {
					replacedFields[fieldName] = struct{}{}
				}
				for _, expectedPart := range expectedMITMV2OutcomeMultipartParts(tc, false) {
					got := parts[expectedPart.fieldName]
					if _, replaced := replacedFields[expectedPart.fieldName]; replaced {
						require.Equal(t, webFuzzerReplacement, got.sentPayload,
							"WebFuzzer path replacement must affect externalized part %q", expectedPart.fieldName)
					} else {
						require.Equal(t, expectedPart.sentPayload, got.sentPayload,
							"WebFuzzer path replacement must preserve inline part %q", expectedPart.fieldName)
					}
				}
			}
		}

		if !tc.forwardOriginal && !tc.hijackResponse && !tc.dropRequest {
			bare, err := client.GetHTTPFlowBare(utils.TimeoutContextSeconds(5), &ypb.HTTPFlowBareRequest{
				Id: int64(flow.GetId()), BareType: "request",
			})
			require.NoError(t, err)
			require.NotContains(t, string(bare.GetData()), "original request body truncated")
			storedBare, err := yakit.GetProjectKeyWithError(
				consts.GetGormProjectDatabase(),
				strconv.FormatInt(int64(flow.GetId()), 10)+"_request",
			)
			require.NoError(t, err)
			require.Equal(t, bare.GetData(), []byte(storedBare),
				"GetHTTPFlowBare must return the exact BareRequest KV persisted for %s", tc.name)
			barePaths := mitmV2FileTagPaths(bare.GetData())
			if tc.deleteAfterValidation {
				deleteResourcePaths = append(deleteResourcePaths, barePaths...)
				for _, path := range barePaths {
					if strings.HasSuffix(filepath.Base(filepath.Dir(path)), "-parts") {
						deleteResourceDirs = append(deleteResourceDirs, filepath.Dir(path))
					}
				}
			}
			wantBareFileTags := tc.wantBareFileTags
			if tc.multipart {
				wantBareFileTags = len(tc.wantBareFileFields)
				require.Equal(t, tc.wantBareFileFields, mitmV2MultipartFileTagFields(t, bare.GetData()),
					"BareRequest must externalize exactly the selected original multipart parts for %s", tc.name)
				requireMITMV2OutcomeMultipartFileTags(
					t, bare.GetData(), tc, true, tc.wantBareFileFields, tc.name+" original BareRequest",
				)
			}
			require.Len(t, barePaths, wantBareFileTags,
				"BareRequest must use the expected original representation for %s", tc.name)
			expectedBareFiles := expectedMITMV2OutcomeFileParts(tc, true, tc.wantBareFileFields)
			unmatchedBareFiles := append([]mitmV2OutcomeMultipartPart(nil), expectedBareFiles...)
			for _, path := range barePaths {
				require.FileExists(t, path)
				storedOriginal, err := os.ReadFile(path)
				require.NoError(t, err)
				matched := -1
				for index, expectedPart := range unmatchedBareFiles {
					if bytes.Equal(expectedPart.originalPayload, storedOriginal) {
						matched = index
						break
					}
				}
				require.NotEqual(t, -1, matched, "BareRequest resource %q must belong to the original multipart", path)
				if matched >= 0 {
					unmatchedBareFiles = append(unmatchedBareFiles[:matched], unmatchedBareFiles[matched+1:]...)
				}
			}
			if len(barePaths) > 0 {
				require.Empty(t, unmatchedBareFiles, "every externalized original part must own exactly one BareRequest resource")
			}
			if tc.wantBareBinaryExports {
				requireMITMV2OutcomeBinaryExportContract(t, bare.GetData(), tc.originalPayload, tc.multipart, tc.name+" original request")
				if tc.wantCurrentBinaryExports && !bytes.Equal(tc.originalPayload, tc.sentPayload) {
					require.NotEqual(t, currentRequest, bare.GetData(),
						"current and original History exports must not be swapped for %s", tc.name)
				}
			}
			if tc.wantBareResource {
				// GetHTTPFlowBare is passed directly to the original-request text
				// editor. It must define a safe GUI input: one file tag in
				// the original multipart context, with filename/type still visible in
				// the part headers.
				require.True(t, utf8.Valid(bare.GetData()), "BareRequest must be safe to pass directly to a text editor")
				require.Less(t, len(bare.GetData()), 8*1024, "BareRequest must stay bounded for the History editor")
				require.Contains(t, string(bare.GetData()), `filename="`+tc.uploadFilename+`"`)
				require.Contains(t, string(bare.GetData()), "Content-Type: "+tc.uploadContentType)
				require.False(t, bytes.Contains(bare.GetData(), tc.originalPayload[:len("%PDF-1.7\n")]))
				paths := mitmV2FileTagPaths(bare.GetData())
				require.Len(t, paths, 1)
				require.FileExists(t, paths[0])
				info, err := os.Stat(paths[0])
				require.NoError(t, err)
				require.Equal(t, int64(len(tc.originalPayload)), info.Size())
			}
			replayMITMV2OutcomeRequest(t, client, bare.GetData(), "original")
		}

		receivedMu.Lock()
		wirePacket := received[token+"/wire"]
		currentReplay := received[token+"/current"]
		originalReplay := received[token+"/original"]
		receivedMu.Unlock()
		require.Equal(t, tc.sentPayload, extractMITMV2OutcomePayload(t, currentReplay, tc.multipart))
		if !tc.dropRequest {
			require.Equal(t, tc.sentPayload, extractMITMV2OutcomePayload(t, wirePacket, tc.multipart))
		} else {
			require.Empty(t, wirePacket.body, "the mock target must not receive a dropped request")
		}
		if len(tc.extraMultipartParts) > 0 {
			requireMITMV2OutcomeMultipartParts(t, wirePacket, expectedMITMV2OutcomeMultipartParts(tc, false), tc.name+" wire")
			requireMITMV2OutcomeMultipartParts(t, currentReplay, expectedMITMV2OutcomeMultipartParts(tc, false), tc.name+" current replay")
			requireMITMV2OutcomeMultipartParts(t, originalReplay, expectedMITMV2OutcomeMultipartParts(tc, true), tc.name+" original replay")
		}
		if tc.localReplacementFilename != "" && tc.multipart {
			wireFilename, wireContentType := extractMITMV2OutcomeMultipartFileMetadata(t, wirePacket)
			require.Equal(t, tc.uploadFilename, wireFilename)
			require.Equal(t, tc.uploadContentType, wireContentType)
			replayFilename, replayContentType := extractMITMV2OutcomeMultipartFileMetadata(t, currentReplay)
			require.Equal(t, tc.uploadFilename, replayFilename)
			require.Equal(t, tc.uploadContentType, replayContentType)
		}
		if tc.dropRequest {
			require.Empty(t, wirePacket.edited)
		} else if tc.forwardOriginal || tc.hijackResponse {
			require.Empty(t, wirePacket.edited)
		} else {
			require.Equal(t, "true", wirePacket.edited)
			require.Equal(t, tc.originalPayload, extractMITMV2OutcomePayload(t, originalReplay, tc.multipart))
		}
		if tc.wantRenderedUserFuzzTag {
			require.Equal(t, "8", wirePacket.userFuzzTag)
			require.Equal(t, wirePacket.userFuzzTag, currentReplay.userFuzzTag)
		}
		if tc.deleteAfterValidation {
			// This is the History delete button's complete backend contract. The
			// gRPC handler must evict the Flow/cache, remove its BareRequest KV,
			// and clean both the current and original engine-owned sidecars.
			_, err := client.DeleteHTTPFlows(utils.TimeoutContextSeconds(5), &ypb.DeleteHTTPFlowRequest{
				Id: []int64{int64(flow.GetId())},
			})
			require.NoError(t, err)
			_, err = client.GetHTTPFlowById(utils.TimeoutContextSeconds(5), &ypb.GetHTTPFlowByIdRequest{Id: int64(flow.GetId())})
			require.Error(t, err, "deleted Flow must not remain visible through History detail/cache")
			_, err = client.GetHTTPFlowBare(utils.TimeoutContextSeconds(5), &ypb.HTTPFlowBareRequest{
				Id: int64(flow.GetId()), BareType: "request",
			})
			require.Error(t, err, "deleted Flow must not retain its original-request KV")
			_, err = QueryHTTPFlows(utils.TimeoutContextSeconds(5), client, &ypb.QueryHTTPFlowRequest{
				SearchURL:  token,
				SourceType: "mitm",
				Full:       true,
				Pagination: &ypb.Paging{Page: 1, Limit: 10},
			}, 0)
			require.NoError(t, err, "deleted Flow must disappear from the History list")
			for _, path := range deleteResourcePaths {
				require.NoFileExists(t, path, "deleting the Flow must clean its engine-owned request resource")
			}
			for _, dir := range deleteResourceDirs {
				require.NoDirExists(t, dir, "deleting the Flow must clean its multipart sidecar directory")
			}
		}
	}
}

// Auto-forward has no editor action and therefore no bare/original snapshot.
// Both an externalized large file and an inline invalid-UTF8 upload must reach
// the target unchanged, persist in their expected History representation, and
// remain exactly replayable through WebFuzzer.
func TestGRPCMUSTPASS_MITMV2_AutoForwardResourceOutcome(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	previousLimit := consts.GetGlobalMaxContentLength()
	consts.SetGlobalMaxContentLength(512 * 1024)
	t.Cleanup(func() { consts.SetGlobalMaxContentLength(previousLimit) })

	largePDF := readMITMBinaryRepositoryFixture(t, "vtestdata", "zwb.pdf")
	requireRealPDFFixture(t, largePDF)
	unsafeZIP := readMITMBinaryRepositoryFixture(t, "common", "aireducer", "testdata", "demo.txt.zip")
	requireRealZIPFixture(t, unsafeZIP)

	type autoForwardOutcomeCase struct {
		token            string
		request          mitmV2RequestOutcomeCase
		wantFileResource bool
		wantInlineBinary bool
	}
	cases := []autoForwardOutcomeCase{
		{
			token: "mitmv2-outcome-auto-large-" + utils.RandStringBytes(8),
			request: mitmV2RequestOutcomeCase{
				name:            "auto-forward-large-binary",
				originalPayload: largePDF,
				sentPayload:     largePDF,
				contentType:     "application/pdf",
			},
			wantFileResource: true,
		},
		{
			token: "mitmv2-outcome-auto-invalid-utf8-" + utils.RandStringBytes(8),
			request: mitmV2RequestOutcomeCase{
				name:              "auto-forward-invalid-utf8-multipart",
				originalPayload:   unsafeZIP,
				sentPayload:       unsafeZIP,
				multipart:         true,
				uploadFilename:    "archive.zip",
				uploadContentType: "application/zip",
			},
			wantInlineBinary: true,
		},
	}
	tokens := make([]string, 0, len(cases))
	for _, tc := range cases {
		tokens = append(tokens, tc.token)
	}
	registerHTTPFlowTokenCleanup(t, tokens...)

	mockCtx, mockCancel := context.WithCancel(context.Background())
	t.Cleanup(mockCancel)
	var receivedMu sync.Mutex
	received := make(map[string]mitmV2CapturedRequest)
	mockHost, mockPort := utils.DebugMockHTTPHandlerFuncContext(mockCtx, func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		require.NoError(t, err)
		kind := request.Header.Get("X-Outcome-Replay")
		if kind == "" {
			kind = "wire"
		}
		token := request.Header.Get("X-Outcome-Case")
		receivedMu.Lock()
		received[token+"/"+kind] = mitmV2CapturedRequest{
			body:        bytes.Clone(body),
			contentType: request.Header.Get("Content-Type"),
		}
		receivedMu.Unlock()
		_, err = writer.Write([]byte("ok"))
		require.NoError(t, err)
	})
	target := utils.HostPort(mockHost, mockPort)
	packets := make([][]byte, 0, len(cases))
	for _, tc := range cases {
		packets = append(packets, buildMITMV2OutcomePacket(t, target, tc.token, tc.request))
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	var unexpectedlyHijacked atomic.Bool

	RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
		require.NoError(t, stream.Send(&ypb.MITMV2Request{Host: "127.0.0.1", Port: uint32(mitmPort)}))
		require.NoError(t, stream.Send(&ypb.MITMV2Request{SetAutoForward: true, AutoForwardValue: true}))
	}, func(stream ypb.Yak_MITMV2Client) {
		require.NoError(t, utils.WaitConnect(utils.HostPort("127.0.0.1", mitmPort), 5))
		time.Sleep(100 * time.Millisecond)
		for _, packet := range packets {
			response, err := lowhttp.HTTP(
				lowhttp.WithPacketBytes(packet),
				lowhttp.WithProxy(proxy),
				lowhttp.WithTimeout(15*time.Second),
				lowhttp.WithSaveHTTPFlow(false),
			)
			require.NoError(t, err)
			require.Contains(t, string(response.RawPacket), "200 OK")
		}
		time.Sleep(500 * time.Millisecond)
		cancel()
	}, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
		if msg.GetManualHijackListAction() != Hijack_List_Add || len(msg.GetManualHijackList()) == 0 {
			return
		}
		unexpectedlyHijacked.Store(true)
		for _, task := range msg.GetManualHijackList() {
			require.NoError(t, stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{TaskID: task.GetTaskID(), Forward: true},
			}))
		}
	})
	require.False(t, unexpectedlyHijacked.Load(), "auto-forward traffic must not enter the manual editor")

	for _, tc := range cases {
		flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(8), client, &ypb.QueryHTTPFlowRequest{
			SearchURL:  tc.token,
			SourceType: "mitm",
			Full:       true,
			Pagination: &ypb.Paging{Page: 1, Limit: 10},
		}, 1)
		require.NoError(t, err)
		flow := flows.GetData()[0]
		current := flow.GetRequest()
		if flow.GetSafeHTTPRequest() != "" {
			current = []byte(flow.GetSafeHTTPRequest())
		}

		var persisted schema.HTTPFlow
		require.NoError(t, consts.GetGormProjectDatabase().First(&persisted, flow.GetId()).Error)
		require.Equal(t, tc.wantFileResource, persisted.IsTooLargeRequest)
		if tc.wantFileResource {
			require.Contains(t, string(current), "{{file(")
			require.Less(t, len(current), 8*1024)
			require.FileExists(t, persisted.TooLargeRequestBodyFile)
			storedBody, readErr := os.ReadFile(persisted.TooLargeRequestBodyFile)
			require.NoError(t, readErr)
			require.Equal(t, tc.request.originalPayload, storedBody,
				"History sidecar must retain the exact automatically forwarded large file")
			require.Contains(t, mitmV2FileTagPaths(current), persisted.TooLargeRequestBodyFile)
		}
		if tc.wantInlineBinary {
			require.False(t, persisted.IsTooLargeRequest)
			require.NotEmpty(t, flow.GetSafeHTTPRequest(),
				"invalid UTF-8 must use the frontend-safe History field")
			require.True(t, utf8.Valid(current))
			require.Contains(t, string(current), "{{unquote(")
			require.False(t, mitmV2FileTagPattern.Match(current))
			persistedRequest := []byte(persisted.GetRequest())
			require.False(t, utf8.Valid(persistedRequest),
				"HTTPFlow DB must retain the concrete automatically forwarded bytes")
			require.Equal(t, current, lowhttp.ConvertHTTPRequestToFuzzTag(persistedRequest),
				"SafeHTTPRequest must be derived from the exact HTTPFlow DB request")
			_, persistedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(persistedRequest)
			persistedUpload := extractMITMV2OutcomePayload(t, mitmV2CapturedRequest{
				body:        persistedBody,
				contentType: lowhttp.GetHTTPPacketHeader(persistedRequest, "Content-Type"),
			}, true)
			require.Equal(t, tc.request.originalPayload, persistedUpload)
			requireMITMV2OutcomeBinaryExportContract(
				t, current, tc.request.originalPayload, true, tc.request.name+" current request",
			)
		}

		replayMITMV2OutcomeRequest(t, client, current, "current")
		_, err = client.GetHTTPFlowBare(utils.TimeoutContextSeconds(3), &ypb.HTTPFlowBareRequest{
			Id: int64(flow.GetId()), BareType: "request",
		})
		require.Error(t, err, "unmodified auto-forward traffic must not create an original-request duplicate")

		receivedMu.Lock()
		wire := received[tc.token+"/wire"]
		replay := received[tc.token+"/current"]
		receivedMu.Unlock()
		require.Equal(t, tc.request.originalPayload, extractMITMV2OutcomePayload(t, wire, tc.request.multipart),
			"target must receive the exact automatically forwarded bytes")
		require.Equal(t, tc.request.originalPayload, extractMITMV2OutcomePayload(t, replay, tc.request.multipart),
			"WebFuzzer must replay the exact History bytes")
	}
}
