package yakit

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/model"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func buildRepresentationTestPacket(contentType string, body []byte) []byte {
	header := "POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Type: " + contentType +
		"\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n"
	return append([]byte(header), body...)
}

func renderRepresentationTestPacket(t *testing.T, packet []byte) []byte {
	t.Helper()
	results, err := mutate.FuzzTagExec(packet, mutate.Fuzz_WithEnableDangerousTag(), mutate.Fuzz_WithResultLimit(1))
	require.NoError(t, err)
	require.Len(t, results, 1)
	return []byte(results[0])
}

func bindHTTPFlowInsertTestProject(t *testing.T) *gorm.DB {
	t.Helper()
	previous := consts.CaptureProjectDatabaseBinding()
	projectPath := filepath.Join(t.TempDir(), "project.db")
	db, err := consts.CreateProjectDatabase(projectPath)
	require.NoError(t, err)
	consts.BindProjectDatabase(db, projectPath)
	testBinding := consts.CaptureProjectDatabaseBinding()
	t.Cleanup(func() {
		consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path)
		if testBinding.ReadDatabase != nil && testBinding.ReadDatabase != db {
			_ = testBinding.ReadDatabase.DB().Close()
		}
		_ = db.DB().Close()
	})
	return db
}

// TestRequestRepresentationBoundaryMatrix is the executable contract for the
// single user-configured dump limit D. Valid UTF-8 is measured as-is regardless
// of MIME. Invalid UTF-8 is measured after exact {{unquote}} encoding: fixed
// syntax costs 15 bytes and each source byte contributes 1, 2, or 4 bytes.
// Equality remains inline; D+1 is externalized as one {{file}} resource.
func TestRequestRepresentationBoundaryMatrix(t *testing.T) {
	const binarySourceAtWorstCaseBoundary = 16 * 1024
	const dumpLimit = 4*binarySourceAtWorstCaseBoundary + 15
	withGlobalMaxContentLength(t, dumpLimit)

	tests := []struct {
		name          string
		contentType   string
		body          []byte
		wantSpill     bool
		wantResource  bool
		wantFlatSpill bool
		wantUnquote   bool
	}{
		{
			name:        "text exactly at D remains inline",
			contentType: "text/plain",
			body:        bytes.Repeat([]byte("t"), dumpLimit),
		},
		{
			name:          "text above D uses a flat resource",
			contentType:   "text/plain",
			body:          bytes.Repeat([]byte("t"), dumpLimit+1),
			wantSpill:     true,
			wantResource:  true,
			wantFlatSpill: true,
		},
		{
			name:        "four-byte binary expansion exactly at D remains editable",
			contentType: "application/octet-stream",
			body:        bytes.Repeat([]byte{0xff}, binarySourceAtWorstCaseBoundary),
			wantUnquote: true,
		},
		{
			name:          "four-byte binary expansion above D uses a resource",
			contentType:   "application/octet-stream",
			body:          bytes.Repeat([]byte{0xff}, binarySourceAtWorstCaseBoundary+1),
			wantSpill:     true,
			wantResource:  true,
			wantFlatSpill: true,
		},
		{
			name:        "binary MIME valid UTF8 exactly at D remains raw",
			contentType: "application/octet-stream",
			body:        bytes.Repeat([]byte("A"), dumpLimit),
		},
		{
			name:          "binary MIME valid UTF8 above D uses a resource by raw size",
			contentType:   "application/pdf",
			body:          bytes.Repeat([]byte("A"), dumpLimit+1),
			wantSpill:     true,
			wantResource:  true,
			wantFlatSpill: true,
		},
		{
			name:        "valid UTF8 quote bytes are not escaped",
			contentType: "application/octet-stream",
			body:        bytes.Repeat([]byte{'"'}, dumpLimit),
		},
		{
			name:        "valid UTF8 fuzztag delimiters are not escaped",
			contentType: "application/octet-stream",
			body:        bytes.Repeat([]byte{'('}, dumpLimit),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			packet := buildRepresentationTestPacket(tc.contentType, tc.body)
			spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
			require.NoError(t, err)
			t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
			require.Equal(t, tc.wantSpill, spill.IsTooLarge)
			require.Equal(t, tc.wantFlatSpill, IsFlatSpillRequestPacket(spill.StoredPacket))

			fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
			require.NoError(t, err)
			resourceCount := len(fileFuzzTagPaths(fuzzable))
			require.Equal(t, tc.wantResource, resourceCount > 0)
			if tc.wantResource {
				require.Contains(t, string(fuzzable), "{{file(")
			}
			require.Equal(t, tc.wantUnquote, bytes.Contains(fuzzable, []byte("{{unquote(")))
			if !tc.wantResource {
				_, displayBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fuzzable)
				require.LessOrEqual(t, len(displayBody), dumpLimit)
			}
			if tc.wantResource {
				require.Less(t, len(fuzzable), 4*1024, "externalized display packets must stay bounded")
			}

			rendered := renderRepresentationTestPacket(t, fuzzable)
			_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
			require.Equal(t, tc.body, renderedBody)
		})
	}
}

func TestRewriteLargeRequestFileFuzzTagsPreservesFileTag(t *testing.T) {
	originalPath := filepath.Join(t.TempDir(), "original.pdf")
	replacementPath := filepath.Join(t.TempDir(), "replacement.bin")
	require.NoError(t, os.WriteFile(originalPath, []byte("original"), 0o600))
	require.NoError(t, os.WriteFile(replacementPath, []byte{0x00, 0xff, 0x41}, 0o600))
	packet := []byte("POST / HTTP/1.1\r\nHost: example.test\r\n\r\n{{file(" + originalPath + ")}}")

	rewritten, count, err := RewriteLargeRequestFileFuzzTags(packet, originalPath, false, replacementPath, nil)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.Contains(t, string(rewritten), "{{file("+replacementPath+")}}")

	rendered := renderRepresentationTestPacket(t, rewritten)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, []byte{0x00, 0xff, 0x41}, body)
}

func TestRequestRepresentationMultipartBoundaryMatrix(t *testing.T) {
	tests := []struct {
		name         string
		contentType  string
		filename     string
		file         []byte
		limitDelta   int
		wantSpill    bool
		wantResource bool
		wantUnquote  bool
	}{
		{
			name:        "multipart expanded representation exactly at D remains editable",
			contentType: "application/octet-stream",
			file:        bytes.Repeat([]byte{0xff}, 16*1024),
			wantUnquote: true,
		},
		{
			name:         "multipart expanded representation D plus one uses per-part resource",
			contentType:  "application/octet-stream",
			file:         bytes.Repeat([]byte{0xff}, 16*1024),
			limitDelta:   -1,
			wantSpill:    true,
			wantResource: true,
		},
		{
			name:         "resource path does not inherit fuzztag delimiters from upload filename",
			contentType:  "application/pdf",
			filename:     "report (final)|draft.pdf",
			file:         bytes.Repeat([]byte{0xff}, 16*1024),
			limitDelta:   -1,
			wantSpill:    true,
			wantResource: true,
		},
		{
			name:        "large UTF-8 text file below D remains raw without a fixed binary budget",
			contentType: "text/plain",
			file:        bytes.Repeat([]byte("t"), 300*1024),
		},
		{
			name:        "binary MIME valid UTF8 multipart data remains raw",
			contentType: "application/pdf",
			file:        bytes.Repeat([]byte("A"), 32*1024),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			filename := tc.filename
			if filename == "" {
				filename = "sample.bin"
			}
			packet, boundary := buildMultipartRequest(t, map[string]string{"note": "editable"}, map[string]struct {
				Filename    string
				ContentType string
				Content     []byte
			}{
				"upload": {
					Filename:    filename,
					ContentType: tc.contentType,
					Content:     tc.file,
				},
			})
			measured, _, err := lowhttp.MeasureHTTPRequestFuzzTagBodySize(packet)
			require.NoError(t, err)
			withGlobalMaxContentLength(t, uint64(measured+tc.limitDelta))

			spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
			require.NoError(t, err)
			t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
			require.Equal(t, tc.wantSpill, spill.IsTooLarge)
			require.Equal(t, tc.wantSpill, IsMultipartSpillRequestPacket(spill.StoredPacket))

			fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
			require.NoError(t, err)
			require.Equal(t, tc.wantResource, len(fileFuzzTagPaths(fuzzable)) > 0)
			if tc.wantResource {
				require.Contains(t, string(fuzzable), "{{file(")
				if tc.filename != "" {
					for _, path := range fileFuzzTagPaths(fuzzable) {
						require.NotContains(t, filepath.Base(path), tc.filename)
					}
				}
			}
			require.Equal(t, tc.wantUnquote, bytes.Contains(fuzzable, []byte("{{unquote(")))
			if !tc.wantSpill {
				_, displayBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fuzzable)
				require.Equal(t, measured, len(displayBody), "measured size must equal the actual editor representation")
			}

			rendered := renderRepresentationTestPacket(t, fuzzable)
			_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
			parts := parseMultipartParts(t, renderedBody, boundary)
			require.Equal(t, "editable", string(parts["note"].body))
			require.Equal(t, tc.file, parts["upload"].body)
			if tc.filename != "" {
				require.Contains(t, parts["upload"].header.Get("Content-Disposition"), `filename="`+tc.filename+`"`)
			}
		})
	}
}

// A multipart spill only externalizes file parts. Ordinary fields remain in the
// editor packet and must still be converted independently when they contain
// bytes that are not valid UTF-8.
func TestBuildFuzzableHTTPFlowRequestPacket_MultipartSpillConvertsInvalidOrdinaryPart(t *testing.T) {
	invalidField := []byte{0xff, 0x00, 'A'}
	fileContent := bytes.Repeat([]byte{0xa5}, 32*1024)
	packet, boundary := buildMultipartRequest(t, map[string]string{
		"note": string(invalidField),
	}, map[string]struct {
		Filename    string
		ContentType string
		Content     []byte
	}{
		"upload": {
			Filename:    "sample.bin",
			ContentType: "application/octet-stream",
			Content:     fileContent,
		},
	})

	measured, _, err := lowhttp.MeasureHTTPRequestFuzzTagBodySize(packet)
	require.NoError(t, err)
	require.Greater(t, measured, 1)
	withGlobalMaxContentLength(t, uint64(measured-1))

	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, spill.IsTooLarge)
	require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))

	fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
	require.NoError(t, err)
	require.True(t, utf8.Valid(fuzzable), "the History/MITM editor packet must always be valid UTF-8")
	require.Contains(t, string(fuzzable), "{{file(")
	require.Contains(t, string(fuzzable), "{{unquote(")

	rendered := renderRepresentationTestPacket(t, fuzzable)
	_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	parts := parseMultipartParts(t, renderedBody, boundary)
	require.Equal(t, invalidField, parts["note"].body)
	require.Equal(t, fileContent, parts["upload"].body)
}

func TestRequestRepresentationMultipartAggregateAndSkeletonBounds(t *testing.T) {
	t.Run("aggregate expanded size at D remains inline and D plus one collapses the largest part", func(t *testing.T) {
		files := map[string]struct {
			Filename    string
			ContentType string
			Content     []byte
		}{
			"first": {
				Filename: "first.bin", ContentType: "application/octet-stream",
				Content: bytes.Repeat([]byte{0xa1}, 8*1024),
			},
			"second": {
				Filename: "second.bin", ContentType: "application/octet-stream",
				Content: bytes.Repeat([]byte{0xb2}, 8*1024),
			},
		}
		packet, boundary := buildMultipartRequest(t, nil, files)
		measured, _, err := lowhttp.MeasureHTTPRequestFuzzTagBodySize(packet)
		require.NoError(t, err)
		for _, tc := range []struct {
			name          string
			limit         int
			wantSpill     bool
			wantResources int
		}{
			{name: "D", limit: measured},
			{name: "D plus one", limit: measured - 1, wantSpill: true, wantResources: 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				withGlobalMaxContentLength(t, uint64(tc.limit))
				spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
				require.NoError(t, err)
				t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
				require.Equal(t, tc.wantSpill, spill.IsTooLarge)
				fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
				require.NoError(t, err)
				require.Len(t, fileFuzzTagPaths(fuzzable), tc.wantResources)

				rendered := renderRepresentationTestPacket(t, fuzzable)
				_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
				parts := parseMultipartParts(t, body, boundary)
				require.Equal(t, files["first"].Content, parts["first"].body)
				require.Equal(t, files["second"].Content, parts["second"].body)
			})
		}
	})

	t.Run("over-D ordinary field collapses without losing multipart skeleton", func(t *testing.T) {
		const dumpLimit = 512 * 1024
		withGlobalMaxContentLength(t, dumpLimit)
		largeText := string(bytes.Repeat([]byte("x"), dumpLimit+1))
		packet, _ := buildMultipartRequest(t, map[string]string{"large": largeText}, map[string]struct {
			Filename    string
			ContentType string
			Content     []byte
		}{
			"tiny": {Filename: "tiny.bin", ContentType: "application/octet-stream", Content: []byte{0xff}},
		})
		spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
		require.NoError(t, err)
		t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
		require.True(t, spill.IsTooLarge)
		require.False(t, IsFlatSpillRequestPacket(spill.StoredPacket))
		require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))

		fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
		require.NoError(t, err)
		paths := fileFuzzTagPaths(fuzzable)
		require.Equal(t, []string{spill.BodyFile}, paths)
		require.Contains(t, string(fuzzable), `filename="tiny.bin"`)

		rendered := renderRepresentationTestPacket(t, fuzzable)
		_, originalBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
		_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
		require.Equal(t, originalBody, renderedBody)
		require.Equal(
			t,
			lowhttp.GetHTTPPacketHeader(packet, "Content-Type"),
			lowhttp.GetHTTPPacketHeader(rendered, "Content-Type"),
		)
	})

	t.Run("ordinary invalid UTF8 expansion over D collapses that part", func(t *testing.T) {
		const dumpLimit = 128 * 1024
		withGlobalMaxContentLength(t, dumpLimit)

		// The raw ordinary field is comfortably below D, but every 0xff byte
		// expands to four printable bytes inside {{unquote}}. A small file part
		// makes the request eligible for multipart spilling. The decision must be
		// based on the final editor representation, not the raw skeleton length.
		invalidOrdinaryField := bytes.Repeat([]byte{0xff}, 40*1024)
		packet, boundary := buildMultipartRequest(t, map[string]string{
			"binary-note": string(invalidOrdinaryField),
		}, map[string]struct {
			Filename    string
			ContentType string
			Content     []byte
		}{
			"upload": {
				Filename:    "small.bin",
				ContentType: "application/octet-stream",
				Content:     []byte{0x00, 0x01, 0x02},
			},
		})

		measured, converted, err := lowhttp.MeasureHTTPRequestFuzzTagBodySize(packet)
		require.NoError(t, err)
		require.True(t, converted)
		require.Greater(t, measured, dumpLimit)

		spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
		require.NoError(t, err)
		t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
		require.True(t, spill.IsTooLarge)
		require.False(t, IsFlatSpillRequestPacket(spill.StoredPacket))
		require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))

		fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
		require.NoError(t, err)
		_, displayBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fuzzable)
		require.LessOrEqual(t, len(displayBody), dumpLimit)
		require.Equal(t, []string{spill.BodyFile}, fileFuzzTagPaths(fuzzable))

		rendered := renderRepresentationTestPacket(t, fuzzable)
		_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
		parts := parseMultipartParts(t, renderedBody, boundary)
		require.Equal(t, invalidOrdinaryField, parts["binary-note"].body)
		require.Equal(t, []byte{0x00, 0x01, 0x02}, parts["upload"].body)
	})
}

func TestBuildFuzzableHTTPFlowRequestPacket_MissingSidecarFailsClosed(t *testing.T) {
	withGlobalMaxContentLength(t, 8)
	packet := buildRepresentationTestPacket("application/octet-stream", bytes.Repeat([]byte{0xff}, 9))
	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, spill.IsTooLarge)

	require.NoError(t, os.Remove(spill.BodyFile))
	_, err = BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
	require.Error(t, err, "history/MITM must not silently expose a dead resource tag")
}

// The global dump limit is the single authoritative request-storage limit.
// A historical fixed 16 MiB cap must not silently truncate an otherwise
// in-limit request when users configure D above 16 MiB (up to the 50 MiB UI
// maximum).
func TestCreateHTTPFlow_RequestBelowDumpLimitIsNeverSecondarilyTruncated(t *testing.T) {
	const bodySize = 16*1024*1024 + 1
	withGlobalMaxContentLength(t, bodySize+1024)
	body := bytes.Repeat([]byte("x"), bodySize)
	packet := buildRepresentationTestPacket("text/plain", body)

	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://example.test/upload"),
		CreateHTTPFlowWithRequestRaw(packet),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
	)
	require.NoError(t, err)
	require.False(t, flow.IsTooLargeRequest)
	_, storedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(flow.GetRequest()))
	require.Equal(t, body, storedBody)
}

func TestCreateHTTPFlow_InlineBinaryMIMEValidUTF8NeedsNoSafeRequest(t *testing.T) {
	withGlobalMaxContentLength(t, 1024)
	body := []byte("printable PDF bytes")
	packet := buildRepresentationTestPacket("application/pdf", body)

	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://example.test/upload"),
		CreateHTTPFlowWithRequestRaw(packet),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { model.DeleteHTTPFlowCacheGRPCModel(flow) })
	require.False(t, flow.IsTooLargeRequest)
	require.Equal(t, int64(len(body)), flow.RequestLength)
	_, storedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(flow.GetRequest()))
	require.Equal(t, body, storedBody, "HTTPFlow DB storage keeps the original in-limit bytes")
	require.NotContains(t, flow.GetRequest(), "{{file(")

	grpcFlow, err := model.ToHTTPFlowGRPCModelFull(flow)
	require.NoError(t, err)
	require.False(t, grpcFlow.InvalidForUTF8Request)
	require.Empty(t, grpcFlow.SafeHTTPRequest, "valid UTF-8 stays in Request even when MIME is binary")
}

func TestCreateHTTPFlow_ExternalizedRequestStoresFuzzablePacket(t *testing.T) {
	withGlobalMaxContentLength(t, 512*1024)
	// Raw bytes fit D, but their four-byte-per-source-byte editor form does not.
	body := bytes.Repeat([]byte{0xff}, 128*1024)
	packet := buildRepresentationTestPacket("application/octet-stream", body)

	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://example.test/upload"),
		CreateHTTPFlowWithRequestRaw(packet),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(flow.TooLargeRequestHeaderFile, flow.TooLargeRequestBodyFile) })
	require.True(t, flow.IsTooLargeRequest)
	require.Contains(t, flow.GetRequest(), "{{file(")
	require.NotContains(t, flow.GetRequest(), "request too large")
	require.Equal(t, int64(len(body)), flow.RequestLength)

	loaded, err := LoadHTTPFlowRequestPacket(flow)
	require.NoError(t, err)
	_, loadedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(loaded)
	require.Equal(t, body, loadedBody)
}

// A request instance may be reused by the MITM save pipeline after the first
// representation pass. The request-scoped cache must contain the public
// {{file(path)}} packet, never the old internal truncation marker; otherwise the
// second consumer would store an un-sendable Flow.
func TestCreateHTTPFlow_RequestContextCachesFuzzablePacket(t *testing.T) {
	withGlobalMaxContentLength(t, 64*1024)
	body := bytes.Repeat([]byte{0xfd}, 64*1024+1)
	packet := buildRepresentationTestPacket("application/octet-stream", body)
	req, err := http.NewRequest("POST", "http://example.test/upload", nil)
	require.NoError(t, err)
	t.Cleanup(func() { CleanupPreparedLargeHTTPFlowRequest(req) })

	create := func() *schema.HTTPFlow {
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithURL("http://example.test/upload"),
			CreateHTTPFlowWithRequestRaw(packet),
			CreateHTTPFlowWithRequestIns(req),
			CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
		)
		require.NoError(t, err)
		return flow
	}

	first := create()
	require.Contains(t, string(httpctx.GetRequestDisplayPacket(req)), "{{file(")
	require.NotContains(t, string(httpctx.GetRequestDisplayPacket(req)), "request too large")

	second := create()
	require.Equal(t, first.GetRequest(), second.GetRequest())
	require.Contains(t, second.GetRequest(), "{{file(")
	rendered := renderRepresentationTestPacket(t, []byte(second.GetRequest()))
	_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, body, renderedBody)
}

func TestCleanupDiscardedHTTPFlowRequestResources(t *testing.T) {
	withGlobalMaxContentLength(t, 64*1024)
	body := bytes.Repeat([]byte{0xfc}, 64*1024+1)
	packet := buildRepresentationTestPacket("application/octet-stream", body)
	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://example.test/discarded"),
		CreateHTTPFlowWithRequestRaw(packet),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
	)
	require.NoError(t, err)
	headerFile := flow.TooLargeRequestHeaderFile
	bodyFile := flow.TooLargeRequestBodyFile
	t.Cleanup(func() { cleanupDiscardedHTTPFlowRequestResources(flow) })
	require.FileExists(t, headerFile)
	require.FileExists(t, bodyFile)

	cleanupDiscardedHTTPFlowRequestResources(flow)
	cleanupDiscardedHTTPFlowRequestResources(flow)
	require.NoFileExists(t, headerFile)
	require.NoFileExists(t, bodyFile)
	require.Empty(t, flow.TooLargeRequestHeaderFile)
	require.Empty(t, flow.TooLargeRequestBodyFile)
}

func TestInsertHTTPFlowExTerminalFailureCleansRequestResources(t *testing.T) {
	withGlobalMaxContentLength(t, 64*1024)
	previous := consts.CaptureProjectDatabaseBinding()
	projectPath := filepath.Join(t.TempDir(), "closed-project.db")
	db, err := consts.CreateProjectDatabase(projectPath)
	require.NoError(t, err)
	consts.BindProjectDatabase(db, projectPath)
	testBinding := consts.CaptureProjectDatabaseBinding()
	t.Cleanup(func() {
		consts.BindProjectDatabaseWithReader(previous.Database, previous.ReadDatabase, previous.Path)
		if testBinding.ReadDatabase != nil && testBinding.ReadDatabase != db {
			_ = testBinding.ReadDatabase.DB().Close()
		}
		_ = db.DB().Close()
	})

	body := bytes.Repeat([]byte{0xfb}, 64*1024+1)
	flow, err := CreateHTTPFlow(
		CreateHTTPFlowWithURL("http://example.test/failed-insert"),
		CreateHTTPFlowWithRequestRaw(buildRepresentationTestPacket("application/octet-stream", body)),
		CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
	)
	require.NoError(t, err)
	headerFile := flow.TooLargeRequestHeaderFile
	bodyFile := flow.TooLargeRequestBodyFile
	t.Cleanup(func() { cleanupDiscardedHTTPFlowRequestResources(flow) })
	require.FileExists(t, headerFile)
	require.FileExists(t, bodyFile)

	finishCalled := false
	require.NoError(t, db.DB().Close())
	require.Error(t, InsertHTTPFlowEx(flow, true, func() { finishCalled = true }))
	require.False(t, finishCalled, "a failed persistence must not run success handlers")
	require.NoFileExists(t, headerFile)
	require.NoFileExists(t, bodyFile)
	require.Empty(t, flow.TooLargeRequestHeaderFile)
	require.Empty(t, flow.TooLargeRequestBodyFile)
}

func TestInsertHTTPFlowExFinishHandlerRunsAfterSyncSuccess(t *testing.T) {
	bindHTTPFlowInsertTestProject(t)
	previousSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(true)
	t.Cleanup(func() { consts.GLOBAL_DB_SAVE_SYNC.SetTo(previousSync) })

	flow := &schema.HTTPFlow{Url: "http://example.test/sync-finish-handler"}
	called := 0
	require.NoError(t, InsertHTTPFlowEx(flow, false, func() {
		require.NotZero(t, flow.ID, "the handler observes the committed Flow identity")
		called++
	}))
	require.Equal(t, 1, called)
}

func TestInsertHTTPFlowExFinishHandlerRunsAfterAsyncSuccess(t *testing.T) {
	db := bindHTTPFlowInsertTestProject(t)
	previousSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(false)
	t.Cleanup(func() { consts.GLOBAL_DB_SAVE_SYNC.SetTo(previousSync) })

	flow := &schema.HTTPFlow{Url: "http://example.test/async-finish-handler"}
	finished := make(chan uint, 1)
	require.NoError(t, InsertHTTPFlowEx(flow, false, func() { finished <- flow.ID }))
	select {
	case id := <-finished:
		require.NotZero(t, id)
		var stored schema.HTTPFlow
		require.NoError(t, db.First(&stored, id).Error)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for successful asynchronous HTTPFlow persistence")
	}
}

func TestInsertHTTPFlowExFinishHandlerSkippedAfterAsyncFailure(t *testing.T) {
	db := bindHTTPFlowInsertTestProject(t)
	previousSync := consts.GLOBAL_DB_SAVE_SYNC.IsSet()
	consts.GLOBAL_DB_SAVE_SYNC.SetTo(false)
	t.Cleanup(func() { consts.GLOBAL_DB_SAVE_SYNC.SetTo(previousSync) })

	finishCalled := make(chan struct{}, 1)
	require.NoError(t, db.DB().Close())
	require.NoError(t, InsertHTTPFlowEx(
		&schema.HTTPFlow{Url: "http://example.test/async-failed-finish-handler"},
		false,
		func() { finishCalled <- struct{}{} },
	))

	// A barrier queued on the same bound database proves that the failed insert
	// callback has completely returned before the assertion below.
	barrier := make(chan struct{})
	require.NoError(t, enqueueDBSaveTo(db, func(*gorm.DB) error {
		close(barrier)
		return nil
	}))
	select {
	case <-barrier:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for asynchronous HTTPFlow persistence")
	}
	select {
	case <-finishCalled:
		t.Fatal("a failed asynchronous persistence ran its success handler")
	default:
	}
}

func TestDeleteHTTPFlowCleansCurrentAndBareRequestResources(t *testing.T) {
	t.Setenv("YAKIT_HOME", t.TempDir())
	withGlobalMaxContentLength(t, 64*1024)
	db := newProjectKVTestDB(t)
	require.NoError(t, db.AutoMigrate(&schema.HTTPFlow{}).Error)

	createFlowWithBare := func(t *testing.T, fill byte) (*schema.HTTPFlow, string) {
		t.Helper()
		body := bytes.Repeat([]byte{fill}, 64*1024+1)
		packet := buildRepresentationTestPacket("application/octet-stream", body)
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithURL("http://example.test/upload"),
			CreateHTTPFlowWithRequestRaw(packet),
			CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			if flow.ID > 0 {
				_ = DeleteHTTPFlowByID(db, int64(flow.ID))
			} else {
				cleanupDiscardedHTTPFlowRequestResources(flow)
			}
		})
		require.NoError(t, db.Create(flow).Error)

		bare, externalized, _, err := PrepareFuzzableHTTPRequestForStorage(packet)
		require.NoError(t, err)
		t.Cleanup(func() { CleanupFuzzableHTTPRequestResources(bare) })
		require.True(t, externalized)
		paths := fileFuzzTagPaths(bare)
		require.Len(t, paths, 1)
		require.NoError(t, SetProjectKeyWithGroup(db, strconv.FormatUint(uint64(flow.ID), 10)+"_request", bare, BARE_REQUEST_GROUP))
		require.FileExists(t, flow.TooLargeRequestBodyFile)
		require.FileExists(t, paths[0])
		return flow, paths[0]
	}

	t.Run("single delete", func(t *testing.T) {
		flow, barePath := createFlowWithBare(t, 0xfa)
		require.NoError(t, DeleteHTTPFlowByID(db, int64(flow.ID)))
		require.NoFileExists(t, flow.TooLargeRequestBodyFile)
		require.NoFileExists(t, flow.TooLargeRequestHeaderFile)
		require.NoFileExists(t, barePath)
		_, err := GetProjectKeyWithError(db, strconv.FormatUint(uint64(flow.ID), 10)+"_request")
		require.Error(t, err)
	})

	t.Run("bulk id delete", func(t *testing.T) {
		flow, barePath := createFlowWithBare(t, 0xfb)
		require.NoError(t, DeleteHTTPFlow(db, &ypb.DeleteHTTPFlowRequest{Id: []int64{int64(flow.ID)}}))
		require.NoFileExists(t, flow.TooLargeRequestBodyFile)
		require.NoFileExists(t, flow.TooLargeRequestHeaderFile)
		require.NoFileExists(t, barePath)
	})

	t.Run("owned multipart spill is removed as one bound resource", func(t *testing.T) {
		packet, _ := buildMultipartRequest(t, nil, map[string]struct {
			Filename    string
			ContentType string
			Content     []byte
		}{
			"upload": {
				Filename:    "large.bin",
				ContentType: "application/octet-stream",
				Content:     bytes.Repeat([]byte{0xff}, 20*1024),
			},
		})
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithURL("http://example.test/multipart-owned"),
			CreateHTTPFlowWithRequestRaw(packet),
			CreateHTTPFlowWithResponseRaw([]byte("HTTP/1.1 204 No Content\r\n\r\n")),
		)
		require.NoError(t, err)
		t.Cleanup(func() { removeLargeRequestSpillFiles(flow.TooLargeRequestHeaderFile, flow.TooLargeRequestBodyFile) })
		require.True(t, flow.IsTooLargeRequest)
		require.True(t, IsMultipartSpillRequestPacket([]byte(flow.GetRequest())))
		partsDir := filepath.Dir(flow.TooLargeRequestBodyFile)
		require.DirExists(t, partsDir)
		require.FileExists(t, flow.TooLargeRequestHeaderFile)
		require.NoError(t, db.Create(flow).Error)

		require.NoError(t, DeleteHTTPFlowByID(db, int64(flow.ID)))
		require.NoDirExists(t, partsDir)
		require.NoFileExists(t, flow.TooLargeRequestHeaderFile)
	})

	t.Run("cross-Flow header and body metadata cannot delete either resource", func(t *testing.T) {
		first, err := spillLargeHTTPFlowRequestIfNeeded(buildRepresentationTestPacket(
			"application/octet-stream",
			bytes.Repeat([]byte{0xf1}, 20*1024),
		))
		require.NoError(t, err)
		second, err := spillLargeHTTPFlowRequestIfNeeded(buildRepresentationTestPacket(
			"application/octet-stream",
			bytes.Repeat([]byte{0xf2}, 20*1024),
		))
		require.NoError(t, err)
		t.Cleanup(func() {
			removeLargeRequestSpillFiles(first.HeaderFile, first.BodyFile)
			removeLargeRequestSpillFiles(second.HeaderFile, second.BodyFile)
		})
		require.True(t, first.IsTooLarge)
		require.True(t, second.IsTooLarge)

		flow := &schema.HTTPFlow{
			Url:                       "http://example.test/cross-owned-metadata",
			IsTooLargeRequest:         true,
			TooLargeRequestHeaderFile: first.HeaderFile,
			TooLargeRequestBodyFile:   second.BodyFile,
		}
		require.NoError(t, db.Create(flow).Error)
		require.NoError(t, DeleteHTTPFlowByID(db, int64(flow.ID)))
		require.FileExists(t, first.HeaderFile)
		require.FileExists(t, first.BodyFile)
		require.FileExists(t, second.HeaderFile)
		require.FileExists(t, second.BodyFile)
	})

	t.Run("user-authored file tag is never an owned Flow resource", func(t *testing.T) {
		tempDir := consts.GetDefaultYakitBaseTempDir()
		require.NoError(t, os.MkdirAll(tempDir, 0o700))
		userFile := filepath.Join(tempDir, "large-request-body-user-authored.txt")
		require.NoError(t, os.WriteFile(userFile, []byte("do-not-delete"), 0o600))
		t.Cleanup(func() { _ = os.Remove(userFile) })

		packet := []byte("POST /user-file HTTP/1.1\r\nHost: example.test\r\n\r\n{{file(" + userFile + ")}}")
		flow := &schema.HTTPFlow{
			Url:     "http://example.test/user-file",
			Request: strconv.Quote(string(packet)),
		}
		require.NoError(t, db.Create(flow).Error)
		require.NoError(t, DeleteHTTPFlowByID(db, int64(flow.ID)))
		require.FileExists(t, userFile, "request text is not resource-ownership metadata")
	})

	t.Run("arbitrary metadata paths and parts directory are preserved", func(t *testing.T) {
		userRoot := t.TempDir()
		userHeader := filepath.Join(userRoot, "request.txt")
		userPartsDir := filepath.Join(userRoot, "user-parts")
		userBody := filepath.Join(userPartsDir, "part-0-data.txt")
		require.NoError(t, os.MkdirAll(userPartsDir, 0o700))
		require.NoError(t, os.WriteFile(userHeader, []byte("header"), 0o600))
		require.NoError(t, os.WriteFile(userBody, []byte("body"), 0o600))

		flow := &schema.HTTPFlow{
			Url:                       "http://example.test/arbitrary-metadata",
			IsTooLargeRequest:         true,
			TooLargeRequestHeaderFile: userHeader,
			TooLargeRequestBodyFile:   userBody,
		}
		require.NoError(t, db.Create(flow).Error)
		require.NoError(t, DeleteHTTPFlowByID(db, int64(flow.ID)))
		require.FileExists(t, userHeader)
		require.FileExists(t, userBody)
		require.DirExists(t, userPartsDir)
	})
}
