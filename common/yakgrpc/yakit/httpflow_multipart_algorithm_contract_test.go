package yakit

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	customMultipart "github.com/yaklang/yaklang/common/utils/multipart"
)

type multipartAlgorithmPart struct {
	fieldName   string
	filename    string
	contentType string
	body        []byte
}

// buildMultipartAlgorithmPacket preserves the supplied part order. The order
// matters because MITMv2 replacement targets use the physical multipart part
// index, including ordinary fields, rather than a file-only ordinal.
func buildMultipartAlgorithmPacket(t *testing.T, parts []multipartAlgorithmPart) []byte {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, part := range parts {
		header := make(textproto.MIMEHeader)
		disposition := `form-data; name="` + part.fieldName + `"`
		if part.filename != "" {
			disposition += `; filename="` + part.filename + `"`
		}
		header.Set("Content-Disposition", disposition)
		if part.contentType != "" {
			header.Set("Content-Type", part.contentType)
		}
		partWriter, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = partWriter.Write(part.body)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	header := "POST /multipart-contract HTTP/1.1\r\n" +
		"Host: example.test\r\n" +
		"Content-Type: " + writer.FormDataContentType() + "\r\n" +
		"Content-Length: " + strconv.Itoa(body.Len()) + "\r\n\r\n"
	return append([]byte(header), body.Bytes()...)
}

func parseMultipartAlgorithmParts(t *testing.T, packet []byte) []multipartAlgorithmPart {
	t.Helper()
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	mediaType, params, err := mime.ParseMediaType(lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	require.NotEmpty(t, params["boundary"])

	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	var parts []multipartAlgorithmPart
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		partBody, err := io.ReadAll(part)
		require.NoError(t, err)
		parts = append(parts, multipartAlgorithmPart{
			fieldName:   part.FormName(),
			filename:    part.FileName(),
			contentType: part.Header.Get("Content-Type"),
			body:        partBody,
		})
	}
	return parts
}

// TestMultipartRepresentationDecisionTable is the top-level executable
// decision table for multipart History/MITM packets. Every valid multipart
// packet keeps its skeleton, and rendering it must reproduce the original part
// bytes. Only malformed multipart falls back to one whole-body resource.
func TestMultipartRepresentationDecisionTable(t *testing.T) {
	const dumpLimit = 4 * 1024
	tests := []struct {
		name         string
		packet       func(*testing.T) []byte
		wellFormed   bool
		wantMode     string
		wantFileTags int
		wantUnquote  bool
	}{
		{
			name: "valid UTF8 parts remain raw inline",
			packet: func(t *testing.T) []byte {
				return buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
					{fieldName: "note", body: []byte("editable")},
					{fieldName: "upload", filename: "readable.pdf", contentType: "application/pdf", body: bytes.Repeat([]byte("A"), 2*1024)},
				})
			},
			wellFormed: true,
			wantMode:   "inline",
		},
		{
			name: "invalid file part remains editable as unquote inline",
			packet: func(t *testing.T) []byte {
				return buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
					{fieldName: "note", body: []byte("editable")},
					{fieldName: "upload", filename: "small.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte{0xff}, 400)},
				})
			},
			wellFormed:  true,
			wantMode:    "inline",
			wantUnquote: true,
		},
		{
			name: "file expansion over D uses one resource per file part",
			packet: func(t *testing.T) []byte {
				return buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
					{fieldName: "note", body: []byte("editable")},
					{fieldName: "upload", filename: "large.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte{0xff}, 2*1024)},
				})
			},
			wellFormed:   true,
			wantMode:     "multipart",
			wantFileTags: 1,
		},
		{
			name: "text-only body over D collapses the ordinary part",
			packet: func(t *testing.T) []byte {
				return buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
					{fieldName: "large-note", body: bytes.Repeat([]byte("T"), dumpLimit+512)},
				})
			},
			wellFormed:   true,
			wantMode:     "multipart",
			wantFileTags: 1,
		},
		{
			name: "ordinary invalid part expansion over D collapses that part",
			packet: func(t *testing.T) []byte {
				return buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
					{fieldName: "binary-note", body: bytes.Repeat([]byte{0xff}, 1200)},
					{fieldName: "upload", filename: "tiny.bin", contentType: "application/octet-stream", body: []byte{0x00, 0x01, 0x02}},
				})
			},
			wellFormed:   true,
			wantMode:     "multipart",
			wantFileTags: 1,
		},
		{
			name: "malformed multipart falls back losslessly to one whole-body resource",
			packet: func(t *testing.T) []byte {
				return buildRepresentationTestPacket(
					"multipart/form-data; boundary=missing-boundary",
					bytes.Repeat([]byte("not-a-multipart-boundary"), 256),
				)
			},
			wantMode:     "flat",
			wantFileTags: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withGlobalMaxContentLength(t, dumpLimit)
			packet := tc.packet(t)
			spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
			require.NoError(t, err)
			t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })

			switch tc.wantMode {
			case "inline":
				require.False(t, spill.IsTooLarge)
				require.False(t, IsMultipartSpillRequestPacket(spill.StoredPacket))
				require.False(t, IsFlatSpillRequestPacket(spill.StoredPacket))
			case "multipart":
				require.True(t, spill.IsTooLarge)
				require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))
				require.False(t, IsFlatSpillRequestPacket(spill.StoredPacket))
			case "flat":
				require.True(t, spill.IsTooLarge)
				require.True(t, IsFlatSpillRequestPacket(spill.StoredPacket))
				require.False(t, IsMultipartSpillRequestPacket(spill.StoredPacket))
			default:
				t.Fatalf("unknown expected mode %q", tc.wantMode)
			}

			fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
			require.NoError(t, err)
			require.True(t, utf8.Valid(fuzzable), "public editor packet must be UTF-8 safe")
			_, displayBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fuzzable)
			require.LessOrEqual(t, len(displayBody), dumpLimit, "public editor body must obey D")
			require.Len(t, fileFuzzTagPaths(fuzzable), tc.wantFileTags)
			require.Equal(t, tc.wantUnquote, bytes.Contains(fuzzable, []byte("{{unquote(")))

			rendered := renderRepresentationTestPacket(t, fuzzable)
			require.Equal(
				t,
				lowhttp.GetHTTPPacketHeader(packet, "Content-Type"),
				lowhttp.GetHTTPPacketHeader(rendered, "Content-Type"),
				"representation must not rewrite the request Content-Type or boundary",
			)
			if tc.wellFormed {
				require.Equal(t, parseMultipartAlgorithmParts(t, packet), parseMultipartAlgorithmParts(t, rendered))
			} else {
				_, originalBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
				_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
				require.Equal(t, originalBody, renderedBody)
			}
		})
	}
}

// D is a collapse budget for part bodies, not a reason to destroy a valid
// multipart skeleton. With D below even the structural skeleton, every part is
// externalized and the skeleton is intentionally allowed to remain above D.
func TestMultipartStructuralSkeletonAboveDRemainsMultipart(t *testing.T) {
	parts := []multipartAlgorithmPart{
		{fieldName: "note", body: []byte("ordinary")},
		{fieldName: "upload", filename: "small.bin", contentType: "application/octet-stream", body: []byte("file")},
	}
	packet := buildMultipartAlgorithmPacket(t, parts)
	withGlobalMaxContentLength(t, 1)
	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, spill.IsTooLarge)
	require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))
	require.False(t, IsFlatSpillRequestPacket(spill.StoredPacket))

	manifest, err := loadMultipartManifest(filepath.Dir(spill.BodyFile))
	require.NoError(t, err)
	require.Len(t, manifest, 2)
	fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
	require.NoError(t, err)
	_, displayBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fuzzable)
	require.Greater(t, len(displayBody), 1)
	require.Len(t, fileFuzzTagPaths(fuzzable), 2)
	require.Equal(t, parseMultipartAlgorithmParts(t, packet), parseMultipartAlgorithmParts(t, renderRepresentationTestPacket(t, fuzzable)))
}

// A replacement index is the physical part position, not the ordinal among
// file parts and not a lookup by field name. Duplicate names are legal, so this
// test interleaves ordinary/file parts with the same name and replaces only the
// file at physical index 3.
func TestMultipartReplacementUsesPhysicalPartIndex(t *testing.T) {
	withGlobalMaxContentLength(t, 4*1024)
	parts := []multipartAlgorithmPart{
		{fieldName: "upload", body: []byte("ordinary-before")},
		{fieldName: "upload", filename: "first.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte("A"), 6*1024)},
		{fieldName: "upload", body: []byte("ordinary-between")},
		{fieldName: "upload", filename: "second.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte("B"), 6*1024)},
	}
	packet := buildMultipartAlgorithmPacket(t, parts)
	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))

	manifest, err := loadMultipartManifest(filepath.Dir(spill.BodyFile))
	require.NoError(t, err)
	require.Len(t, manifest, 2)
	require.Equal(t, []int{1, 3}, []int{manifest[0].Index, manifest[1].Index})

	fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
	require.NoError(t, err)
	require.Len(t, fileFuzzTagPaths(fuzzable), 2)

	replacementBody := []byte("only-the-second-file-is-replaced")
	replacementPath := filepath.Join(t.TempDir(), "replacement.bin")
	require.NoError(t, os.WriteFile(replacementPath, replacementBody, 0o600))
	rewritten, resourceCount, err := RewriteLargeRequestFileFuzzTags(
		fuzzable,
		spill.BodyFile,
		true,
		"",
		map[int]string{3: replacementPath},
	)
	require.NoError(t, err)
	require.Equal(t, 2, resourceCount)
	rendered := renderRepresentationTestPacket(t, rewritten)
	got := parseMultipartAlgorithmParts(t, rendered)
	require.Len(t, got, 4)
	require.Equal(t, parts[0], got[0])
	require.Equal(t, parts[1], got[1], "physical part 1 must keep its original file")
	require.Equal(t, parts[2], got[2])
	require.Equal(t, "upload", got[3].fieldName)
	require.Equal(t, "second.bin", got[3].filename)
	require.Equal(t, "application/octet-stream", got[3].contentType)
	require.Equal(t, replacementBody, got[3].body)

	_, _, err = RewriteLargeRequestFileFuzzTags(
		fuzzable,
		spill.BodyFile,
		true,
		"",
		map[int]string{2: replacementPath},
	)
	require.ErrorContains(t, err, "part 2 not found in manifest", "ordinary parts are not replaceable file targets")
}

// Public multipart packets can pass through more than one display/storage
// layer. Existing engine-owned file tags must remain stable, while a deleted
// part file must fail closed instead of leaving a clickable but dead tag.
func TestMultipartFuzzableRepresentationIsIdempotentAndValidatesResources(t *testing.T) {
	withGlobalMaxContentLength(t, 4*1024)
	packet := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "upload", filename: "large.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte{0xff}, 2*1024)},
	})
	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))

	first, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
	require.NoError(t, err)
	second, err := BuildFuzzableHTTPFlowRequestPacket(first, spill.BodyFile)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Len(t, fileFuzzTagPaths(second), 1)

	partPath := fileFuzzTagPaths(first)[0]
	require.NoError(t, os.Remove(partPath))
	_, err = BuildFuzzableHTTPFlowRequestPacket(first, spill.BodyFile)
	require.Error(t, err, "a missing multipart part resource must fail closed")
}

type multipartRejectWriter struct{}

func (multipartRejectWriter) Write([]byte) (int, error) {
	return 0, errors.New("reject multipart write")
}

type multipartRejectPayloadWriter struct {
	needle []byte
	output bytes.Buffer
}

func (w *multipartRejectPayloadWriter) Write(p []byte) (int, error) {
	if bytes.Contains(p, w.needle) {
		return 0, errors.New("reject multipart payload")
	}
	return w.output.Write(p)
}

// TestMultipartSplitReaderAndFailureContracts covers the public sidecar reader
// APIs and realistic corrupt/missing resource failures. These checks matter to
// HTTP History because GetHTTPFlowById and GetHTTPFlowBodyById consume the same
// manifest and part files that the editor representation references.
func TestMultipartSplitReaderAndFailureContracts(t *testing.T) {
	withGlobalMaxContentLength(t, 2*1024)
	packet := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "note", body: []byte("ordinary")},
		{fieldName: "upload", filename: "first.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte("A"), 3*1024)},
		{fieldName: "upload", filename: "second.bin", body: bytes.Repeat([]byte("B"), 3*1024)},
	})
	spill, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, IsMultipartSpillRequestPacket(spill.StoredPacket))

	flow := &schema.HTTPFlow{
		IsTooLargeRequest:         true,
		TooLargeRequestHeaderFile: spill.HeaderFile,
		TooLargeRequestBodyFile:   spill.BodyFile,
	}
	flow.SetRequest(string(spill.StoredPacket))

	t.Run("flow readers expose manifest parts and rebuilt body", func(t *testing.T) {
		require.Equal(t, filepath.Dir(spill.BodyFile), FlowMultipartSidecarDir(flow))
		manifest, err := LoadFlowMultipartManifest(flow)
		require.NoError(t, err)
		require.Len(t, manifest, 2)
		require.Equal(t, []int{1, 2}, []int{manifest[0].Index, manifest[1].Index})
		require.NotEmpty(t, FlowMultipartSkeletonBody(flow))

		part, filename, err := OpenFlowMultipartPart(flow, 2)
		require.NoError(t, err)
		partBody, err := io.ReadAll(part)
		require.NoError(t, err)
		require.NoError(t, part.Close())
		require.Equal(t, "second.bin", filename)
		require.Equal(t, bytes.Repeat([]byte("B"), 3*1024), partBody)

		rebuiltReader, err := RebuildFlowMultipartBody(flow)
		require.NoError(t, err)
		rebuiltBody, err := io.ReadAll(rebuiltReader)
		require.NoError(t, err)
		header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
		rebuiltPacket := lowhttp.ReplaceHTTPPacketBody([]byte(header), rebuiltBody, false)
		require.Equal(t, parseMultipartAlgorithmParts(t, packet), parseMultipartAlgorithmParts(t, rebuiltPacket))
	})

	t.Run("non multipart flow readers return explicit empty or error results", func(t *testing.T) {
		require.Empty(t, FlowMultipartSidecarDir(nil))
		require.Empty(t, FlowMultipartSidecarDir(&schema.HTTPFlow{}))
		require.Nil(t, FlowMultipartSkeletonBody(nil))
		manifest, err := LoadFlowMultipartManifest(nil)
		require.NoError(t, err)
		require.Nil(t, manifest)
		_, err = RebuildFlowMultipartBody(&schema.HTTPFlow{})
		require.ErrorContains(t, err, "not a multipart spill")
		_, _, err = OpenFlowMultipartPart(flow, 99)
		require.ErrorContains(t, err, "part 99 not found")
		require.Empty(t, multipartSidecarDirFromBodyFile(""))
	})

	t.Run("request rebuild rejects invalid inputs", func(t *testing.T) {
		_, err := RebuildMultipartRequestPacket([]byte("POST / HTTP/1.1\r\n\r\nraw"), spill.BodyFile, nil)
		require.ErrorContains(t, err, "not a multipart spill skeleton")
		_, err = RebuildMultipartRequestPacket(spill.StoredPacket, "", nil)
		require.ErrorContains(t, err, "body file is empty")
		err = rebuildMultipartBodyToWriterWithReplacements([]byte("no boundary"), filepath.Dir(spill.BodyFile), nil, io.Discard)
		require.ErrorContains(t, err, "boundary not found")
		err = rebuildMultipartBodyToWriterWithReplacements(
			FlowMultipartSkeletonBody(flow), filepath.Dir(spill.BodyFile), map[int]string{1: ""}, io.Discard,
		)
		require.ErrorContains(t, err, "empty file path")
	})

	t.Run("boundary extraction covers closing delimiter and absence", func(t *testing.T) {
		boundary, err := detectBoundary([]byte("--contract-boundary--\r\n"))
		require.NoError(t, err)
		require.Equal(t, "contract-boundary", boundary)
		_, err = detectBoundary([]byte("ordinary data"))
		require.ErrorContains(t, err, "boundary not found")
	})

	t.Run("header writer propagates output failures and ignores empty values", func(t *testing.T) {
		err := writePartHeaders(multipartRejectWriter{}, textproto.MIMEHeader{
			"Content-Disposition": {`form-data; name="x"`},
		})
		require.ErrorContains(t, err, "reject multipart write")
		err = writePartHeaders(multipartRejectWriter{}, textproto.MIMEHeader{
			"X-Test": {"value"},
		})
		require.ErrorContains(t, err, "reject multipart write")
		var output bytes.Buffer
		require.NoError(t, writePartHeaders(&output, textproto.MIMEHeader{"X-Empty": nil}))
		require.Empty(t, output.Bytes())
	})

	t.Run("manifest and part helpers fail closed", func(t *testing.T) {
		require.Error(t, writeMultipartManifest("", nil))
		blockedSidecar := filepath.Join(t.TempDir(), "not-a-directory")
		require.NoError(t, os.WriteFile(blockedSidecar, []byte("block"), 0o600))
		require.Error(t, writeMultipartManifest(blockedSidecar, nil))
		manifest, err := loadMultipartManifest("")
		require.NoError(t, err)
		require.Nil(t, manifest)
		manifest, err = loadMultipartManifest(filepath.Join(t.TempDir(), "missing"))
		require.NoError(t, err)
		require.Nil(t, manifest)

		badJSONDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(badJSONDir, manifestFileName), []byte("{"), 0o600))
		_, err = loadMultipartManifest(badJSONDir)
		require.Error(t, err)
		badFlow := &schema.HTTPFlow{
			IsTooLargeRequest:       true,
			TooLargeRequestBodyFile: filepath.Join(badJSONDir, "part-1-data.txt"),
		}
		badFlow.SetRequest(string(spill.StoredPacket))
		_, _, err = OpenFlowMultipartPart(badFlow, 1)
		require.Error(t, err)
		_, err = jsonUnmarshalManifest([]byte("not-json"))
		require.Error(t, err)

		manifestDirectory := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(manifestDirectory, manifestFileName), 0o700))
		_, err = loadMultipartManifest(manifestDirectory)
		require.Error(t, err)

		_, err = openMultipartPart("", multipartPartMeta{})
		require.ErrorContains(t, err, "empty sidecar dir")
		_, err = openMultipartPart(t.TempDir(), multipartPartMeta{File: "missing.bin"})
		require.Error(t, err)
	})

	t.Run("missing manifest and missing part surface through flow APIs", func(t *testing.T) {
		sidecarDir := filepath.Dir(spill.BodyFile)
		manifestPath := filepath.Join(sidecarDir, manifestFileName)
		manifestBytes, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		require.NoError(t, os.Remove(manifestPath))
		_, _, err = OpenFlowMultipartPart(flow, 1)
		require.ErrorContains(t, err, "part 1 not found")
		require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0o600))

		manifest, err := loadMultipartManifest(sidecarDir)
		require.NoError(t, err)
		require.NotEmpty(t, manifest)
		firstPath := filepath.Join(sidecarDir, manifest[0].File)
		firstBytes, err := os.ReadFile(firstPath)
		require.NoError(t, err)
		require.NoError(t, os.Remove(firstPath))
		_, _, err = OpenFlowMultipartPart(flow, manifest[0].Index)
		require.Error(t, err)
		require.NoError(t, os.WriteFile(firstPath, firstBytes, 0o600))
	})
}

func TestSpillMultipartFilesIfNeededEarlyDecisions(t *testing.T) {
	withGlobalMaxContentLength(t, 1)

	result, err := spillMultipartFilesIfNeeded(nil)
	require.NoError(t, err)
	require.False(t, result.IsTooLarge)

	withoutBoundary := buildRepresentationTestPacket(
		"multipart/form-data",
		bytes.Repeat([]byte("x"), 64),
	)
	result, err = spillMultipartFilesIfNeeded(withoutBoundary)
	require.NoError(t, err)
	require.False(t, result.IsTooLarge)

	nonMultipart := buildRepresentationTestPacket("application/octet-stream", bytes.Repeat([]byte("x"), 64))
	result, err = spillMultipartFilesIfNeeded(nonMultipart)
	require.NoError(t, err)
	require.False(t, result.IsTooLarge)
}

// These cases cover the selection invariant and both-pass consistency checks
// that protect the sidecar from containing a different multipart layout than
// the request measured during the first pass.
func TestMultipartSpillSelectionAndPassConsistencyContracts(t *testing.T) {
	t.Run("zero parsed parts falls back", func(t *testing.T) {
		withGlobalMaxContentLength(t, 1)
		packet := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
			{fieldName: "note", body: []byte("data")},
		})
		ops := defaultMultipartSpillOps()
		ops.newReader = func(io.Reader) multipartPartReader {
			return multipartErrorPartReader{err: io.EOF}
		}
		result, err := spillMultipartFilesIfNeededWithOps(packet, ops)
		require.NoError(t, err)
		require.False(t, result.IsTooLarge)
	})

	t.Run("oversized epilogue with bounded parts falls back losslessly", func(t *testing.T) {
		withGlobalMaxContentLength(t, 512)
		packet := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
			{fieldName: "note", body: []byte("small")},
		})
		packet = append(packet, bytes.Repeat([]byte("E"), 1024)...)
		result, err := spillMultipartFilesIfNeeded(packet)
		require.NoError(t, err)
		require.False(t, result.IsTooLarge)
		require.Empty(t, result.MultipartDir)
	})

	t.Run("aggregate selection skips an already oversized part", func(t *testing.T) {
		withGlobalMaxContentLength(t, 1024)
		packet := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
			{fieldName: "already-large", body: bytes.Repeat([]byte("L"), 2*1024)},
			{fieldName: "largest-remaining", body: bytes.Repeat([]byte("R"), 900)},
		})
		result, err := spillMultipartFilesIfNeeded(packet)
		require.NoError(t, err)
		t.Cleanup(func() { removeLargeRequestSpillFiles(result.HeaderFile, result.BodyFile) })
		require.True(t, result.IsTooLarge)
		require.Len(t, result.Manifest, 2)
		require.Equal(t, []int{0, 1}, []int{result.Manifest[0].Index, result.Manifest[1].Index})
	})

	fullPacket := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "first", body: []byte("first")},
		{fieldName: "second", body: []byte("second")},
	})
	onePartPacket := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "first", body: []byte("first")},
	})
	_, fullBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fullPacket)
	_, onePartBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(onePartPacket)

	for _, tc := range []struct {
		name             string
		firstPassBody    []byte
		secondPassBody   []byte
		wantCreatedFiles int
	}{
		{name: "second pass has an extra part", firstPassBody: onePartBody, secondPassBody: fullBody, wantCreatedFiles: 1},
		{name: "second pass loses a part", firstPassBody: fullBody, secondPassBody: onePartBody, wantCreatedFiles: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withGlobalMaxContentLength(t, 1)
			ops := defaultMultipartSpillOps()
			readerCall := 0
			ops.newReader = func(io.Reader) multipartPartReader {
				readerCall++
				if readerCall == 1 {
					return customMultipart.NewReader(bytes.NewReader(tc.firstPassBody))
				}
				return customMultipart.NewReader(bytes.NewReader(tc.secondPassBody))
			}
			createdFiles := 0
			originalCreate := ops.create
			ops.create = func(path string) (*os.File, error) {
				createdFiles++
				return originalCreate(path)
			}
			result, err := spillMultipartFilesIfNeededWithOps(fullPacket, ops)
			require.NoError(t, err)
			require.False(t, result.IsTooLarge)
			require.Equal(t, tc.wantCreatedFiles, createdFiles)
			require.NotEmpty(t, result.MultipartDir)
			require.NoDirExists(t, result.MultipartDir)
		})
	}
}

func TestMultipartRepeatedPartHeadersSurviveSpillAndRebuild(t *testing.T) {
	withGlobalMaxContentLength(t, 1)
	require.Equal(t,
		len("--boundary\r\n")+len("\r\n")+len("\r\n"),
		multipartSkeletonPartOverhead("boundary", textproto.MIMEHeader{"X-Empty": nil}),
	)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="upload"; filename="repeated.bin"`)
	header.Set("Content-Type", "application/octet-stream")
	header.Add("X-Trace", "one")
	header.Add("X-Trace", "two")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	partBody := []byte("payload")
	_, err = part.Write(partBody)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	packet := buildRepresentationTestPacket(writer.FormDataContentType(), body.Bytes())

	spill, err := spillMultipartFilesIfNeeded(packet)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(spill.HeaderFile, spill.BodyFile) })
	require.True(t, spill.IsTooLarge)
	fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(spill.StoredPacket, spill.BodyFile)
	require.NoError(t, err)
	require.True(t, IsMultipartSpillRequestPacket(fuzzable))
	_, skeletonBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(spill.StoredPacket)
	rebuilt := readAll(t, rebuildMultipartBodyToReader([]byte(skeletonBody), spill.MultipartDir))

	reader := multipart.NewReader(bytes.NewReader(rebuilt), writer.Boundary())
	rebuiltPart, err := reader.NextPart()
	require.NoError(t, err)
	require.Equal(t, []string{"one", "two"}, rebuiltPart.Header.Values("X-Trace"))
	rebuiltPartBody, err := io.ReadAll(rebuiltPart)
	require.NoError(t, err)
	require.Equal(t, partBody, rebuiltPartBody)
	_, err = reader.NextPart()
	require.ErrorIs(t, err, io.EOF)
}

func TestBuildFuzzableMultipartRequestPacketFailureContracts(t *testing.T) {
	markerPacket := func(partIndex string) []byte {
		return []byte("POST / HTTP/1.1\r\nContent-Type: multipart/form-data; boundary=x\r\n\r\n" +
			"--x\r\nContent-Disposition: form-data; name=\"x\"\r\n\r\n" +
			multipartSkeletonMarker + ", part=" + partIndex + ", file=x, size=1]]\r\n--x--\r\n")
	}

	t.Run("empty body file", func(t *testing.T) {
		_, err := buildFuzzableMultipartRequestPacket(markerPacket("0"), "")
		require.ErrorContains(t, err, "body file is empty")
	})

	t.Run("manifest read error", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(dir, manifestFileName), 0o700))
		_, err := buildFuzzableMultipartRequestPacket(markerPacket("0"), filepath.Join(dir, "part-0-data.txt"))
		require.Error(t, err)
	})

	t.Run("marker missing", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeMultipartManifest(dir, nil))
		_, err := buildFuzzableMultipartRequestPacket(
			[]byte("POST / HTTP/1.1\r\nContent-Type: multipart/form-data; boundary=x\r\n\r\n--x--\r\n"),
			filepath.Join(dir, "part-0-data.txt"),
		)
		require.ErrorContains(t, err, "marker is missing")
	})

	t.Run("part index overflow", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeMultipartManifest(dir, nil))
		_, err := buildFuzzableMultipartRequestPacket(
			markerPacket(string(bytes.Repeat([]byte("9"), 100))),
			filepath.Join(dir, "part-0-data.txt"),
		)
		require.ErrorContains(t, err, "parse multipart spill part index")
	})

	t.Run("part absent from manifest", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeMultipartManifest(dir, nil))
		_, err := buildFuzzableMultipartRequestPacket(markerPacket("0"), filepath.Join(dir, "part-0-data.txt"))
		require.ErrorContains(t, err, "part 0 not found in manifest")
	})

	t.Run("unsafe manifest file path", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, writeMultipartManifest(dir, []multipartPartMeta{{Index: 0, File: "bad).txt"}}))
		_, err := buildFuzzableMultipartRequestPacket(markerPacket("0"), filepath.Join(dir, "part-0-data.txt"))
		require.ErrorContains(t, err, "unsupported delimiter")
	})
}

type multipartErrorPartReader struct {
	err error
}

func (r multipartErrorPartReader) NextPart() (*customMultipart.Part, error) {
	return nil, r.err
}

// TestSpillMultipartFilesIfNeededFailureCleanup injects each I/O boundary used
// by multipart splitting. Every failure either asks the caller to use flat
// spill (parse/read failures) or returns an error, and no partial sidecar/header
// may remain in the engine temp directory.
func TestSpillMultipartFilesIfNeededFailureCleanup(t *testing.T) {
	packet := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "note", body: bytes.Repeat([]byte("N"), 256)},
		{fieldName: "upload", filename: "large.bin", contentType: "application/octet-stream", body: bytes.Repeat([]byte("F"), 2*1024)},
	})
	injectedErr := errors.New("injected multipart spill failure")

	tests := []struct {
		name      string
		configure func(*multipartSpillOps)
		wantError bool
	}{
		{
			name: "first-pass parser failure falls back",
			configure: func(ops *multipartSpillOps) {
				ops.newReader = func(io.Reader) multipartPartReader { return multipartErrorPartReader{err: injectedErr} }
			},
		},
		{
			name: "first-pass file read failure falls back",
			configure: func(ops *multipartSpillOps) {
				ops.copy = func(io.Writer, io.Reader) (int64, error) { return 0, injectedErr }
			},
		},
		{
			name: "sidecar mkdir failure",
			configure: func(ops *multipartSpillOps) {
				ops.mkdirAll = func(string, os.FileMode) error { return injectedErr }
			},
			wantError: true,
		},
		{
			name: "second-pass parser failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				readerCall := 0
				ops.newReader = func(reader io.Reader) multipartPartReader {
					readerCall++
					if readerCall == 2 {
						return multipartErrorPartReader{err: injectedErr}
					}
					return customMultipart.NewReader(reader)
				}
			},
		},
		{
			name: "skeleton header write failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				ops.writeHeaders = func(io.Writer, textproto.MIMEHeader) error { return injectedErr }
			},
			wantError: true,
		},
		{
			name: "part file create failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				ops.create = func(string) (*os.File, error) { return nil, injectedErr }
			},
			wantError: true,
		},
		{
			name: "ordinary part copy failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				copyCall := 0
				originalCopy := ops.copy
				ops.copy = func(dst io.Writer, src io.Reader) (int64, error) {
					copyCall++
					if copyCall == 3 {
						return 0, injectedErr
					}
					return originalCopy(dst, src)
				}
			},
			wantError: true,
		},
		{
			name: "file part copy failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				copyCall := 0
				originalCopy := ops.copy
				ops.copy = func(dst io.Writer, src io.Reader) (int64, error) {
					copyCall++
					if copyCall == 4 {
						return 0, injectedErr
					}
					return originalCopy(dst, src)
				}
			},
			wantError: true,
		},
		{
			name: "header temp file creation failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				ops.openTempFile = func(string) (*os.File, error) { return nil, injectedErr }
			},
			wantError: true,
		},
		{
			name: "header file write failure cleans sidecar",
			configure: func(ops *multipartSpillOps) {
				ops.openTempFile = func(string) (*os.File, error) { return os.Open(os.DevNull) }
			},
			wantError: true,
		},
		{
			name: "manifest write failure cleans header and sidecar",
			configure: func(ops *multipartSpillOps) {
				ops.writeManifest = func(string, []multipartPartMeta) error { return injectedErr }
			},
			wantError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withGlobalMaxContentLength(t, 1024)
			ops := defaultMultipartSpillOps()
			tc.configure(&ops)
			result, err := spillMultipartFilesIfNeededWithOps(packet, ops)
			require.Equal(t, tc.wantError, err != nil)
			require.False(t, result.IsTooLarge)

			entries, readErr := os.ReadDir(consts.GetDefaultYakitBaseTempDir())
			require.NoError(t, readErr)
			for _, entry := range entries {
				require.NotContains(t, entry.Name(), "large-request-body-", "partial multipart sidecar leaked")
				require.NotContains(t, entry.Name(), "large-request-header-", "partial header sidecar leaked")
			}
		})
	}
}

// TestMultipartRebuildFailureContracts exercises every read/write boundary of
// skeleton reconstruction. It prevents History downloads and MITM replacement
// sends from silently returning partial multipart bodies when a manifest,
// source file, parser, or destination stream fails.
func TestMultipartRebuildFailureContracts(t *testing.T) {
	withGlobalMaxContentLength(t, 1024)
	fileBody := bytes.Repeat([]byte("A"), 3*1024)
	fileOnlyPacket := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "upload", filename: "only.bin", contentType: "application/octet-stream", body: fileBody},
	})
	fileSpill, err := spillLargeHTTPFlowRequestIfNeeded(fileOnlyPacket)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(fileSpill.HeaderFile, fileSpill.BodyFile) })
	require.True(t, IsMultipartSpillRequestPacket(fileSpill.StoredPacket))
	_, fileSkeleton := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fileSpill.StoredPacket)
	fileSidecar := filepath.Dir(fileSpill.BodyFile)
	manifest, err := loadMultipartManifest(fileSidecar)
	require.NoError(t, err)
	require.Len(t, manifest, 1)
	fileFuzzable, err := BuildFuzzableHTTPFlowRequestPacket(fileSpill.StoredPacket, fileSpill.BodyFile)
	require.NoError(t, err)
	_, fileFuzzableSkeleton := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fileFuzzable)

	ordinaryAndFilePacket := buildMultipartAlgorithmPacket(t, []multipartAlgorithmPart{
		{fieldName: "note", body: []byte("ordinary-payload")},
		{fieldName: "upload", filename: "with-note.bin", contentType: "application/octet-stream", body: fileBody},
	})
	mixedSpill, err := spillLargeHTTPFlowRequestIfNeeded(ordinaryAndFilePacket)
	require.NoError(t, err)
	t.Cleanup(func() { removeLargeRequestSpillFiles(mixedSpill.HeaderFile, mixedSpill.BodyFile) })
	require.True(t, IsMultipartSpillRequestPacket(mixedSpill.StoredPacket))
	_, mixedSkeleton := lowhttp.SplitHTTPHeadersAndBodyFromPacket(mixedSpill.StoredPacket)
	mixedSidecar := filepath.Dir(mixedSpill.BodyFile)

	t.Run("manifest read error", func(t *testing.T) {
		badSidecar := t.TempDir()
		require.NoError(t, os.Mkdir(filepath.Join(badSidecar, manifestFileName), 0o700))
		err := rebuildMultipartBodyToWriterWithReplacements(fileSkeleton, badSidecar, nil, io.Discard)
		require.Error(t, err)
	})

	t.Run("invalid writer boundary", func(t *testing.T) {
		tooLongBoundary := string(bytes.Repeat([]byte("x"), 71))
		err := rebuildMultipartBodyToWriterWithReplacements(
			[]byte("--"+tooLongBoundary+"\r\n--"+tooLongBoundary+"--\r\n"), fileSidecar, nil, io.Discard,
		)
		require.Error(t, err)
	})

	t.Run("malformed skeleton parser", func(t *testing.T) {
		err := rebuildMultipartBodyToWriterWithReplacements(
			[]byte("invalid preamble\r\n--valid-boundary\r\nContent-Disposition: form-data; name=\"x\"\r\n\r\ndata\r\n--valid-boundary--\r\n"),
			fileSidecar,
			nil,
			io.Discard,
		)
		require.Error(t, err)
	})

	t.Run("file part absent from manifest", func(t *testing.T) {
		emptySidecar := t.TempDir()
		require.NoError(t, writeMultipartManifest(emptySidecar, nil))
		err := rebuildMultipartBodyToWriterWithReplacements(fileSkeleton, emptySidecar, nil, io.Discard)
		require.ErrorContains(t, err, "not found in manifest")
	})

	t.Run("engine file resource absent from manifest", func(t *testing.T) {
		manifestPath := filepath.Join(fileSidecar, manifestFileName)
		originalManifest, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		defer func() { _ = os.WriteFile(manifestPath, originalManifest, 0o600) }()
		require.NoError(t, writeMultipartManifest(fileSidecar, nil))
		err = rebuildMultipartBodyToWriterWithReplacements(fileFuzzableSkeleton, fileSidecar, nil, io.Discard)
		require.ErrorContains(t, err, "not found in manifest")
	})

	t.Run("manifest source file missing", func(t *testing.T) {
		missingSidecar := t.TempDir()
		require.NoError(t, writeMultipartManifest(missingSidecar, manifest))
		err := rebuildMultipartBodyToWriterWithReplacements(fileSkeleton, missingSidecar, nil, io.Discard)
		require.ErrorContains(t, err, "open spilled part file")
	})

	t.Run("file part header destination failure", func(t *testing.T) {
		err := rebuildMultipartBodyToWriterWithReplacements(fileSkeleton, fileSidecar, nil, multipartRejectWriter{})
		require.ErrorContains(t, err, "reject multipart write")
	})

	t.Run("file part payload destination failure", func(t *testing.T) {
		dst := &multipartRejectPayloadWriter{needle: bytes.Repeat([]byte("A"), 32)}
		err := rebuildMultipartBodyToWriterWithReplacements(fileSkeleton, fileSidecar, nil, dst)
		require.ErrorContains(t, err, "reject multipart payload")
	})

	t.Run("ordinary part header destination failure", func(t *testing.T) {
		err := rebuildMultipartBodyToWriterWithReplacements(mixedSkeleton, mixedSidecar, nil, multipartRejectWriter{})
		require.ErrorContains(t, err, "reject multipart write")
	})

	t.Run("ordinary part payload destination failure", func(t *testing.T) {
		dst := &multipartRejectPayloadWriter{needle: []byte("ordinary-payload")}
		err := rebuildMultipartBodyToWriterWithReplacements(mixedSkeleton, mixedSidecar, nil, dst)
		require.ErrorContains(t, err, "reject multipart payload")
	})
}
