//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
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
	name                      string
	originalPayload           []byte
	inlineEditedPayload       []byte
	manualFuzzTagPayload      []byte
	replacementPayload        []byte
	localReplacementFilename  string
	localReplacementType      string
	sentPayload               []byte
	contentType               string
	multipart                 bool
	uploadFilename            string
	uploadContentType         string
	forwardOriginal           bool
	hijackResponse            bool
	wantHijackTag             string
	wantHijackRaw             bool
	wantResourceFlow          bool
	wantCurrentRaw            bool
	wantBareResource          bool
	wantCurrentMultipartFiles int
	wantBareFileTags          int
	wantManualEditTag         bool
	wantRenderedUserFuzzTag   bool
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

func mitmV2FileTagPartIndex(t *testing.T, packet []byte) int32 {
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
		if mitmV2FileTagPattern.Match(partBody) {
			return index
		}
	}
	t.Fatal("multipart file tag part not found")
	return -1
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
func replaceMITMV2OutcomeInlineUpload(t *testing.T, packet, editedPayload []byte) []byte {
	t.Helper()
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

	// Keep this at the same scale as the redis-poc ZIP that exposed the
	// editor-save regression. A tiny tag would not catch size-dependent parser
	// or submission behavior along the real MITMv2 stream.
	zipOriginal := make([]byte, 47_213)
	// A real ZIP is not a uniform high-byte blob: redis-poc-main.zip contains
	// hundreds of printable (){} bytes. Those bytes are also Fuzztag parser
	// delimiters and exposed the renderer/backend quoting mismatch that the old
	// all-0xa5 fixture could never exercise. Cycle through every byte value so
	// this end-to-end case permanently covers parser-sensitive binary content.
	for i := range zipOriginal {
		zipOriginal[i] = byte((i*37 + 11) & 0xff)
	}
	copy(zipOriginal, []byte{'P', 'K', 0x03, 0x04})
	zipHexEdited := bytes.Clone(zipOriginal)
	for i := 0; i < 29; i++ {
		zipHexEdited[i] = 0x11
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
			name:            "small-binary-edit",
			originalPayload: []byte{0xff, 0x00, 'A'},
			sentPayload:     []byte{0xff, 0x00, 'A'},
			contentType:     "application/octet-stream",
			wantHijackTag:   "{{unquote(",
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
			name:                "multipart-inline-zip-hex-edit",
			originalPayload:     zipOriginal,
			inlineEditedPayload: zipHexEdited,
			sentPayload:         zipHexEdited,
			multipart:           true,
			uploadFilename:      "sample.zip",
			uploadContentType:   "application/zip",
			wantHijackTag:       "{{unquote(",
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
			originalPayload:    bytes.Repeat([]byte{0xdd}, 300*1024),
			replacementPayload: bytes.Repeat([]byte{0xee}, 300*1024+1),
			sentPayload:        bytes.Repeat([]byte{0xee}, 300*1024+1),
			contentType:        "application/octet-stream",
			wantHijackTag:      "{{file(",
			wantResourceFlow:   true,
			wantBareFileTags:   1,
		},
		{
			name:               "large-binary-forward-discards-replacement",
			originalPayload:    bytes.Repeat([]byte{0x87}, 300*1024),
			replacementPayload: bytes.Repeat([]byte{0x88}, 300*1024+1),
			sentPayload:        bytes.Repeat([]byte{0x87}, 300*1024),
			contentType:        "application/octet-stream",
			forwardOriginal:    true,
			wantHijackTag:      "{{file(",
			wantResourceFlow:   true,
		},
		{
			name:             "large-binary-view-response",
			originalPayload:  bytes.Repeat([]byte{0x86}, 300*1024),
			sentPayload:      bytes.Repeat([]byte{0x86}, 300*1024),
			contentType:      "application/octet-stream",
			hijackResponse:   true,
			wantHijackTag:    "{{file(",
			wantResourceFlow: true,
		},
		{
			name:                      "multipart-file-replace",
			originalPayload:           bytes.Repeat([]byte{0xaa}, 300*1024),
			replacementPayload:        bytes.Repeat([]byte{0xbb}, 300*1024+1),
			sentPayload:               bytes.Repeat([]byte{0xbb}, 300*1024+1),
			multipart:                 true,
			wantHijackTag:             "{{file(",
			wantResourceFlow:          true,
			wantCurrentMultipartFiles: 1,
			wantBareFileTags:          1,
		},
		{
			// This is the bounded CI analogue of a user intercepting a PDF below
			// the 50 MiB dump limit and replacing it with a tiny text file. The
			// raw multipart request remains below the 512 KiB test dump limit D,
			// while its exact unquote representation exceeds D.
			// The current request must become inline, while GetHTTPFlowBare must
			// retain a bounded {{file}} tag for the original PDF.
			name: "multipart-pdf-replace-small",
			originalPayload: append(
				[]byte("%PDF-1.7\n"),
				bytes.Repeat([]byte{0xa5}, 300*1024-len("%PDF-1.7\n"))...,
			),
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
			wantBareFileTags:         1,
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
	proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
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
			require.Less(t, len(task.GetRequest()), 8*1024, "resource-backed hijack packets must stay editor-safe")
		}
		if strings.Contains(tc.wantHijackTag, "file") && tc.replacementPayload != nil {
			// Frontend contract at the manual-hijack boundary: Monaco receives a
			// bounded UTF-8 packet containing the standard file tag. Multipart
			// headers retain the user-facing filename/type, while the part index
			// used by the replacement RPC is derived from the multipart structure.
			// Original file bytes never enter renderer text.
			require.True(t, utf8.Valid(task.GetRequest()))
			paths := mitmV2FileTagPaths(task.GetRequest())
			require.NotEmpty(t, paths)
			if tc.wantBareResource {
				require.Len(t, paths, 1)
				require.Contains(t, string(task.GetRequest()), `filename="`+tc.uploadFilename+`"`)
				require.Contains(t, string(task.GetRequest()), "Content-Type: "+tc.uploadContentType)
				require.False(t, bytes.Contains(task.GetRequest(), tc.originalPayload[:len("%PDF-1.7\n")]))
				info, err := os.Stat(paths[0])
				require.NoError(t, err)
				require.Equal(t, int64(len(tc.originalPayload)), info.Size())
			}
			partIndex := int32(0)
			if tc.multipart {
				partIndex = mitmV2FileTagPartIndex(t, task.GetRequest())
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
			submitted = replaceMITMV2OutcomeInlineUpload(t, submitted, tc.inlineEditedPayload)
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
		require.Len(t, flow.GetMultipartFiles(), tc.wantCurrentMultipartFiles,
			"History list query must expose the multipart manifest contract for %s", tc.name)
		if tc.multipart {
			detail, err := client.GetHTTPFlowById(utils.TimeoutContextSeconds(5), &ypb.GetHTTPFlowByIdRequest{Id: int64(flow.GetId())})
			require.NoError(t, err)
			require.Len(t, detail.GetMultipartFiles(), tc.wantCurrentMultipartFiles,
				"History detail query is the frontend's authoritative multipart metadata for %s", tc.name)
			detailRequest := detail.GetRequest()
			if detail.GetSafeHTTPRequest() != "" {
				detailRequest = []byte(detail.GetSafeHTTPRequest())
			}
			require.Equal(t, currentRequest, detailRequest,
				"History list and detail queries must expose the same editor packet for %s", tc.name)
			if tc.wantCurrentMultipartFiles > 0 {
				fileInfo := detail.GetMultipartFiles()[0]
				require.Equal(t, int32(1), fileInfo.GetPartIndex(), "the ordinary note field occupies physical part index 0")
				expectedFilename := tc.uploadFilename
				if expectedFilename == "" {
					expectedFilename = "sample.bin"
				}
				expectedContentType := tc.uploadContentType
				if expectedContentType == "" {
					expectedContentType = "application/octet-stream"
				}
				require.Equal(t, expectedFilename, fileInfo.GetFilename())
				require.Equal(t, expectedContentType, fileInfo.GetContentType())
				require.Equal(t, int64(len(tc.sentPayload)), fileInfo.GetSize())
				require.FileExists(t, fileInfo.GetFilePath())
				storedPart, err := os.ReadFile(fileInfo.GetFilePath())
				require.NoError(t, err)
				require.Equal(t, tc.sentPayload, storedPart, "frontend metadata must point at the actual wire part")
				require.Contains(t, mitmV2FileTagPaths(currentRequest), fileInfo.GetFilePath())
			}
		}
		if tc.wantCurrentRaw {
			require.Empty(t, flow.GetSafeHTTPRequest(), "valid UTF-8 current requests stay in Request for %s", tc.name)
			require.NotContains(t, string(currentRequest), "{{unquote(")
			require.False(t, mitmV2FileTagPattern.Match(currentRequest))
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
			require.Equal(t, webFuzzerReplacement, extractMITMV2OutcomePayload(t, webFuzzerReplay, tc.multipart))
		}

		if !tc.forwardOriginal && !tc.hijackResponse {
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
			require.Len(t, barePaths, tc.wantBareFileTags,
				"BareRequest must use the expected original multipart representation for %s", tc.name)
			for _, path := range barePaths {
				require.FileExists(t, path)
				info, err := os.Stat(path)
				require.NoError(t, err)
				require.Equal(t, int64(len(tc.originalPayload)), info.Size())
				storedOriginal, err := os.ReadFile(path)
				require.NoError(t, err)
				require.Equal(t, tc.originalPayload, storedOriginal)
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
		require.Equal(t, tc.sentPayload, extractMITMV2OutcomePayload(t, wirePacket, tc.multipart))
		require.Equal(t, tc.sentPayload, extractMITMV2OutcomePayload(t, currentReplay, tc.multipart))
		if tc.localReplacementFilename != "" && tc.multipart {
			wireFilename, wireContentType := extractMITMV2OutcomeMultipartFileMetadata(t, wirePacket)
			require.Equal(t, tc.uploadFilename, wireFilename)
			require.Equal(t, tc.uploadContentType, wireContentType)
			replayFilename, replayContentType := extractMITMV2OutcomeMultipartFileMetadata(t, currentReplay)
			require.Equal(t, tc.uploadFilename, replayFilename)
			require.Equal(t, tc.uploadContentType, replayContentType)
		}
		if tc.forwardOriginal || tc.hijackResponse {
			require.Empty(t, wirePacket.edited)
		} else {
			require.Equal(t, "true", wirePacket.edited)
			require.Equal(t, tc.originalPayload, extractMITMV2OutcomePayload(t, originalReplay, tc.multipart))
		}
		if tc.wantRenderedUserFuzzTag {
			require.Equal(t, "8", wirePacket.userFuzzTag)
			require.Equal(t, wirePacket.userFuzzTag, currentReplay.userFuzzTag)
		}
	}
}

// Auto-forward has no editor action and therefore no bare/original snapshot,
// but its persisted current request must use the same bounded representation
// and remain exactly replayable from HTTP History/WebFuzzer.
func TestGRPCMUSTPASS_MITMV2_AutoForwardResourceOutcome(t *testing.T) {
	client := isolateMITMTestSideEffects(t)
	previousLimit := consts.GetGlobalMaxContentLength()
	consts.SetGlobalMaxContentLength(512 * 1024)
	t.Cleanup(func() { consts.SetGlobalMaxContentLength(previousLimit) })

	original := bytes.Repeat([]byte{0xcc}, 300*1024)
	tc := mitmV2RequestOutcomeCase{
		name:            "auto-forward-large-binary",
		originalPayload: original,
		sentPayload:     original,
		contentType:     "application/octet-stream",
	}
	token := "mitmv2-outcome-auto-" + utils.RandStringBytes(8)
	registerHTTPFlowTokenCleanup(t, token)

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
		receivedMu.Lock()
		received[kind] = mitmV2CapturedRequest{
			body:        bytes.Clone(body),
			contentType: request.Header.Get("Content-Type"),
		}
		receivedMu.Unlock()
		_, err = writer.Write([]byte("ok"))
		require.NoError(t, err)
	})
	target := utils.HostPort(mockHost, mockPort)
	packet := buildMITMV2OutcomePacket(t, target, token, tc)

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
		response, err := lowhttp.HTTP(
			lowhttp.WithPacketBytes(packet),
			lowhttp.WithProxy(proxy),
			lowhttp.WithTimeout(15*time.Second),
			lowhttp.WithSaveHTTPFlow(false),
		)
		require.NoError(t, err)
		require.Contains(t, string(response.RawPacket), "200 OK")
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

	flows, err := QueryHTTPFlows(utils.TimeoutContextSeconds(8), client, &ypb.QueryHTTPFlowRequest{
		SearchURL:  token,
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
	require.Contains(t, string(current), "{{file(")
	require.Less(t, len(current), 8*1024)
	replayMITMV2OutcomeRequest(t, client, current, "current")

	_, err = client.GetHTTPFlowBare(utils.TimeoutContextSeconds(3), &ypb.HTTPFlowBareRequest{
		Id: int64(flow.GetId()), BareType: "request",
	})
	require.Error(t, err, "unmodified auto-forward traffic must not create an original-request duplicate")

	receivedMu.Lock()
	wire := received["wire"]
	replay := received["current"]
	receivedMu.Unlock()
	require.Equal(t, original, wire.body)
	require.Equal(t, original, replay.body)
}
