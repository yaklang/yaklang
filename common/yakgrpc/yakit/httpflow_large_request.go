package yakit

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// defaultHTTPFlowRequestBodyInDBBytes matches consts default GlobalMaxContentLength (10MB).
const defaultHTTPFlowRequestBodyInDBBytes = 10 * 1024 * 1024

// GetMaxHTTPFlowRequestBodyInDBBytes returns the request-body spill threshold for HTTPFlow DB.
// Controlled solely by GlobalMaxContentLength (「转储数据包大小」); when unset, defaults to 10MB.
func GetMaxHTTPFlowRequestBodyInDBBytes() int {
	n := consts.GetGlobalMaxContentLength()
	if n == 0 {
		return defaultHTTPFlowRequestBodyInDBBytes
	}
	return int(n)
}

const storedHTTPFlowLargeRequestTruncateNotice = "[[request too large(%s), truncated]] use GetHTTPFlowBodyById(IsRequest=true) for full body"

var storedHTTPFlowLargeRequestTruncatePattern = regexp.MustCompile(
	`(?mi)^\[\[request(?: |-)too(?: |-)large\([^)]+\), truncated\]\](?: use GetHTTPFlowBodyById\(IsRequest=true\) for full body)?\r?$`,
)

// fileFuzzTagPattern covers the canonical {{file(path)}} syntax.
// Generated sidecar paths do not contain ')', '|', or line delimiters.
var fileFuzzTagPattern = regexp.MustCompile(`\{\{file\(([^)\r\n|]+)\)\}\}`)

var multipartSpillPartFilePattern = regexp.MustCompile(`^part-[0-9]+-data\.txt$`)

func buildFileFuzzTag(path string) (string, error) {
	if path == "" {
		return "", utils.Error("file fuzztag path is empty")
	}
	if strings.ContainsAny(path, ")|\r\n") || strings.Contains(path, "{{") || strings.Contains(path, "}}") {
		return "", utils.Errorf("file fuzztag path %q contains unsupported delimiter characters", path)
	}
	return "{{file(" + path + ")}}", nil
}

func fileFuzzTagPaths(packet []byte) []string {
	matches := fileFuzzTagPattern.FindAllSubmatch(packet, -1)
	paths := make([]string, 0, len(matches))
	for _, match := range matches {
		paths = append(paths, string(match[1]))
	}
	return paths
}

func countFileFuzzTagPath(packet []byte, path string) int {
	count := 0
	for _, candidate := range fileFuzzTagPaths(packet) {
		if candidate == path {
			count++
		}
	}
	return count
}

func replaceFileFuzzTagPath(packet []byte, oldPath, newPath string) ([]byte, int, error) {
	count := 0
	var buildErr error
	rewritten := fileFuzzTagPattern.ReplaceAllFunc(packet, func(tag []byte) []byte {
		match := fileFuzzTagPattern.FindSubmatch(tag)
		if len(match) != 2 || string(match[1]) != oldPath {
			return tag
		}
		count++
		replacement, err := buildFileFuzzTag(newPath)
		if err != nil {
			buildErr = err
			return tag
		}
		return []byte(replacement)
	})
	if buildErr != nil {
		return nil, 0, buildErr
	}
	return rewritten, count, nil
}

func isEngineOwnedLargeRequestPath(path string) bool {
	if path == "" || !utils.IsSubPath(path, consts.GetDefaultYakitBaseTempDir()) {
		return false
	}
	base := filepath.Base(path)
	if strings.HasPrefix(base, "large-request-body-") {
		return true
	}
	dirBase := filepath.Base(filepath.Dir(path))
	return strings.HasPrefix(dirBase, "large-request-body-") && strings.HasSuffix(dirBase, "-parts") &&
		strings.HasPrefix(base, "part-")
}

func engineOwnedLargeRequestHeaderSuffix(path string) (string, bool) {
	if path == "" || !utils.IsSubPath(path, consts.GetDefaultYakitBaseTempDir()) {
		return "", false
	}
	tempDir := filepath.Clean(consts.GetDefaultYakitBaseTempDir())
	if filepath.Clean(filepath.Dir(path)) != tempDir {
		return "", false
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "large-request-header-") || !strings.HasSuffix(base, ".txt") {
		return "", false
	}
	suffix := strings.TrimSuffix(strings.TrimPrefix(base, "large-request-header-"), ".txt")
	return suffix, suffix != ""
}

func engineOwnedLargeRequestBodySuffix(path string) (suffix string, multipart bool, ok bool) {
	if path == "" || !utils.IsSubPath(path, consts.GetDefaultYakitBaseTempDir()) {
		return "", false, false
	}
	tempDir := filepath.Clean(consts.GetDefaultYakitBaseTempDir())
	dir := filepath.Clean(filepath.Dir(path))
	base := filepath.Base(path)
	if dir == tempDir && strings.HasPrefix(base, "large-request-body-") && strings.HasSuffix(base, ".txt") {
		suffix = strings.TrimSuffix(strings.TrimPrefix(base, "large-request-body-"), ".txt")
		return suffix, false, suffix != ""
	}

	dirBase := filepath.Base(dir)
	if filepath.Clean(filepath.Dir(dir)) != tempDir ||
		!strings.HasPrefix(dirBase, "large-request-body-") ||
		!strings.HasSuffix(dirBase, "-parts") ||
		!multipartSpillPartFilePattern.MatchString(base) {
		return "", false, false
	}
	suffix = strings.TrimSuffix(strings.TrimPrefix(dirBase, "large-request-body-"), "-parts")
	if suffix == "" {
		return "", false, false
	}
	manifest, err := loadMultipartManifest(dir)
	if err != nil {
		return "", false, false
	}
	for _, meta := range manifest {
		if meta.File == base {
			return suffix, true, true
		}
	}
	return "", false, false
}

func removeEngineOwnedLargeRequestBody(path string) bool {
	_, multipart, ok := engineOwnedLargeRequestBodySuffix(path)
	if !ok {
		return false
	}
	if multipart {
		cleanupMultipartSidecar(path)
	} else {
		_ = os.Remove(path)
	}
	return true
}

func validateEngineOwnedFileFuzzTags(packet []byte) error {
	for _, path := range fileFuzzTagPaths(packet) {
		if !isEngineOwnedLargeRequestPath(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return utils.Wrapf(err, "stat large request sidecar %q", path)
		}
		if !info.Mode().IsRegular() {
			return utils.Errorf("large request sidecar %q is not a regular file", path)
		}
	}
	return nil
}

func validateRegularRequestResource(path, description string) error {
	info, err := os.Stat(path)
	if err != nil {
		return utils.Wrapf(err, "stat %s %q", description, path)
	}
	if !info.Mode().IsRegular() {
		return utils.Errorf("%s %q is not a regular file", description, path)
	}
	return nil
}

func countEngineOwnedFileFuzzTags(packet []byte) int {
	count := 0
	for _, path := range fileFuzzTagPaths(packet) {
		if isEngineOwnedLargeRequestPath(path) {
			count++
		}
	}
	return count
}

// SyncLargeHTTPFlowFlagsFromStoredPacket restores is_too_large_* flags from stored
// request/response packets when JSON/HAR omits them (e.g. legacy share payloads).
func SyncLargeHTTPFlowFlagsFromStoredPacket(flow *schema.HTTPFlow, recordedReqLen, recordedRspLen int64) {
	if flow == nil {
		return
	}

	req := flow.GetRequest()
	if !flow.IsTooLargeRequest && containsLargeRequestTruncateMarker(req) {
		flow.IsTooLargeRequest = true
	}
	if flow.IsTooLargeRequest && flow.RequestLength <= 0 {
		flow.RequestLength = pickHTTPFlowRecordedBodyLength(recordedReqLen, req)
	}

	rsp := flow.GetResponse()
	if !flow.IsTooLargeResponse && containsLargeResponseTruncateMarker(rsp) {
		flow.IsTooLargeResponse = true
	}
	if flow.IsTooLargeResponse && flow.BodyLength <= 0 {
		flow.BodyLength = pickHTTPFlowRecordedBodyLength(recordedRspLen, rsp)
	}
}

func containsLargeRequestTruncateMarker(packet string) bool {
	lower := strings.ToLower(packet)
	return strings.Contains(lower, "request too large(") ||
		strings.Contains(lower, "request-too-large(") ||
		countEngineOwnedFileFuzzTags([]byte(packet)) > 0
}

// IsFlatSpillRequestPacket reports whether packet is the editable truncated
// representation of a non-multipart oversized request body.
func IsFlatSpillRequestPacket(packet []byte) bool {
	if storedHTTPFlowLargeRequestTruncatePattern.Match(packet) {
		return true
	}
	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(packet)
	contentType := strings.ToLower(strings.TrimSpace(lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type")))
	if strings.HasPrefix(contentType, "multipart/") {
		return false
	}
	for _, path := range fileFuzzTagPaths(body) {
		if isEngineOwnedLargeRequestPath(path) && !strings.HasSuffix(filepath.Base(filepath.Dir(path)), "-parts") {
			return true
		}
	}
	return false
}

// RebuildFlatSpillRequestPacket restores a non-multipart oversized request
// from its spilled body file. When replacementBodyFile is non-empty, that
// file replaces the complete request body. Header edits in packet are kept
// and Content-Length is recalculated.
func RebuildFlatSpillRequestPacket(packet []byte, bodyFile, replacementBodyFile string) ([]byte, error) {
	if !IsFlatSpillRequestPacket(packet) {
		return nil, utils.Error("request packet is not a flat large-request spill")
	}
	source := replacementBodyFile
	if source == "" {
		source = bodyFile
	}
	if source == "" {
		return nil, utils.Error("large request body file is empty")
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return nil, utils.Wrapf(err, "read large request body file %q failed", source)
	}
	header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	return lowhttp.ReplaceHTTPPacketBodyEx([]byte(header), body, false, true), nil
}

func containsLargeResponseTruncateMarker(packet string) bool {
	lower := strings.ToLower(packet)
	return strings.Contains(lower, "response too large(") || strings.Contains(lower, "response-too-large(")
}

func pickHTTPFlowRecordedBodyLength(recorded int64, packet string) int64 {
	if recorded > 0 {
		return recorded
	}
	if packet == "" {
		return 0
	}
	if cl := lowhttp.GetHTTPPacketHeader([]byte(packet), "Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

type largeRequestSpillResult struct {
	StoredPacket    []byte
	IsTooLarge      bool
	HeaderFile      string
	BodyFile        string
	OriginalBodyLen int
}

func spillLargeHTTPFlowRequestIfNeeded(packet []byte) (largeRequestSpillResult, error) {
	res := largeRequestSpillResult{StoredPacket: packet}
	if len(packet) == 0 {
		return res, nil
	}

	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(packet)
	res.OriginalBodyLen = len(body)
	overDumpLimit := len(body) > GetMaxHTTPFlowRequestBodyInDBBytes()
	contentType := lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Type")
	isMultipart := strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "multipart/")
	requiresSpill := overDumpLimit
	if !requiresSpill {
		inlineBodySize, _, measureErr := lowhttp.MeasureHTTPRequestFuzzTagBodySize(packet)
		requiresSpill = measureErr != nil || inlineBodySize > GetMaxHTTPFlowRequestBodyInDBBytes()
	}
	if !requiresSpill {
		return res, nil
	}

	// Valid multipart uploads keep their boundary/header skeleton and collapse
	// only the part bodies selected by spillMultipartFilesIfNeeded. A flat spill
	// remains the lossless fallback for non-multipart or malformed bodies.
	if isMultipart {
		if mpRes, err := spillMultipartFilesIfNeeded(packet); err != nil {
			log.Errorf("spill multipart request failed: %s, fall back to flat spill", err)
		} else if mpRes.IsTooLarge {
			res.StoredPacket = mpRes.StoredPacket
			res.IsTooLarge = true
			res.HeaderFile = mpRes.HeaderFile
			res.BodyFile = mpRes.BodyFile
			res.OriginalBodyLen = mpRes.OriginalBodyLen
			return res, nil
		}
	}

	uid := ksuid.New().String()
	suffix := fmt.Sprintf(`%v_%v`, time.Now().Format(utils.DatetimePretty()), uid)

	headerFP, err := utils.OpenTempFile(fmt.Sprintf("large-request-header-%v.txt", suffix))
	if err != nil {
		return res, err
	}
	if _, err := headerFP.Write([]byte(header)); err != nil {
		headerFP.Close()
		return res, err
	}
	headerPath := headerFP.Name()
	headerFP.Close()

	bodyFP, err := utils.OpenTempFile(fmt.Sprintf("large-request-body-%v.txt", suffix))
	if err != nil {
		return res, err
	}
	if _, err := bodyFP.Write(body); err != nil {
		bodyFP.Close()
		return res, err
	}
	bodyPath := bodyFP.Name()
	bodyFP.Close()

	notice := []byte(fmt.Sprintf(storedHTTPFlowLargeRequestTruncateNotice, utils.ByteSize(uint64(len(body)))))
	stored := lowhttp.ReplaceHTTPPacketBody([]byte(header), notice, false)

	res.StoredPacket = stored
	res.IsTooLarge = true
	res.HeaderFile = headerPath
	res.BodyFile = bodyPath
	return res, nil
}

// BuildFuzzableHTTPFlowRequestPacket converts internal DB/display spill markers
// into UTF-8-safe executable {{file(path)}} references. The returned packet is
// shared by MITM V2, HTTP History and WebFuzzer and reproduces the original
// bytes lazily. Binary bytes that remain inline continue to use {{unquote}}.
func BuildFuzzableHTTPFlowRequestPacket(storedPacket []byte, bodyFile string) ([]byte, error) {
	if len(storedPacket) == 0 {
		return storedPacket, nil
	}

	var fuzzable []byte
	if countEngineOwnedFileFuzzTags(storedPacket) > 0 {
		fuzzable = storedPacket
	} else {
		switch {
		case IsFlatSpillRequestPacket(storedPacket):
			if bodyFile == "" {
				return nil, utils.Error("large request body file is empty")
			}
			if _, err := os.Stat(bodyFile); err != nil {
				return nil, utils.Wrap(err, "stat large request body file")
			}
			header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(storedPacket)
			tag, err := buildFileFuzzTag(bodyFile)
			if err != nil {
				return nil, err
			}
			fuzzable = lowhttp.ReplaceHTTPPacketBody([]byte(header), []byte(tag), false)
		case IsMultipartSpillRequestPacket(storedPacket):
			var err error
			fuzzable, err = buildFuzzableMultipartRequestPacket(storedPacket, bodyFile)
			if err != nil {
				return nil, err
			}
		default:
			fuzzable = storedPacket
		}
	}
	// Apply the UTF-8-safe inline representation after every spill form has
	// reached its final {{file}} shape. Existing file tags remain intact while
	// invalid bytes in any ordinary multipart part become {{unquote}}.
	fuzzable = lowhttp.ConvertHTTPRequestToFuzzTag(fuzzable)

	if err := validateEngineOwnedFileFuzzTags(fuzzable); err != nil {
		return nil, err
	}
	return fuzzable, nil
}

// PrepareFuzzableHTTPRequestForStorage creates a self-contained, UTF-8-safe
// request snapshot suitable for project KV storage. Externalized bodies remain
// in engine sidecars referenced by file tags; the redundant header sidecar is
// removed because the complete header is already embedded in stored.
func PrepareFuzzableHTTPRequestForStorage(packet []byte) (stored []byte, externalized bool, originalBodyLen int, err error) {
	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	if err != nil {
		return nil, false, requestBodyLengthFromPacket(packet), err
	}
	originalBodyLen = res.OriginalBodyLen
	stored, err = BuildFuzzableHTTPFlowRequestPacket(res.StoredPacket, res.BodyFile)
	if err != nil {
		removeLargeRequestSpillFiles(res.HeaderFile, res.BodyFile)
		return nil, false, originalBodyLen, err
	}
	if res.HeaderFile != "" {
		_ = os.Remove(res.HeaderFile)
	}
	return stored, res.IsTooLarge, originalBodyLen, nil
}

// CleanupFuzzableHTTPRequestResources removes request sidecars referenced by
// an immutable backend-generated snapshot such as MITM BareRequest. Editable
// current Flow packets must never be passed here: their {{file}} tags may have
// been changed by the user, and ownership instead comes from the Flow's paired
// TooLargeRequestHeaderFile/TooLargeRequestBodyFile metadata.
func CleanupFuzzableHTTPRequestResources(packet []byte) {
	removedDirs := make(map[string]struct{})
	for _, path := range fileFuzzTagPaths(packet) {
		_, multipart, ok := engineOwnedLargeRequestBodySuffix(path)
		if !ok {
			continue
		}
		dir := filepath.Dir(path)
		if multipart {
			if _, done := removedDirs[dir]; done {
				continue
			}
			cleanupMultipartSidecar(path)
			removedDirs[dir] = struct{}{}
			continue
		}
		_ = os.Remove(path)
	}
}

// RewriteLargeRequestFileFuzzTags applies MITM's out-of-band replacement
// uploads to file tags generated for one intercepted request. Multipart part
// indexes come from the sidecar
// manifest; they are not encoded into another replacement protocol.
func RewriteLargeRequestFileFuzzTags(
	packet []byte,
	bodyFile string,
	multipart bool,
	bodyReplacement string,
	partReplacements map[int]string,
) ([]byte, int, error) {
	if bodyFile == "" {
		if bodyReplacement != "" || len(partReplacements) > 0 {
			return nil, 0, utils.Error("large request sidecar path is empty")
		}
		return packet, 0, nil
	}

	rewritten := bytes.Clone(packet)
	resourceCount := 0
	if !multipart {
		resourceCount = countFileFuzzTagPath(rewritten, bodyFile)
		if bodyReplacement == "" {
			if resourceCount > 0 {
				if err := validateRegularRequestResource(bodyFile, "large request body sidecar"); err != nil {
					return nil, 0, err
				}
			}
			return rewritten, resourceCount, nil
		}
		if resourceCount == 0 {
			return nil, 0, utils.Error("large request body file tag was removed from the edited request")
		}
		if err := validateRegularRequestResource(bodyReplacement, "large request body replacement"); err != nil {
			return nil, 0, err
		}
		replaced, count, err := replaceFileFuzzTagPath(rewritten, bodyFile, bodyReplacement)
		if err != nil {
			return nil, 0, err
		}
		return replaced, count, nil
	}

	dir := multipartSidecarDirFromBodyFile(bodyFile)
	manifest, err := loadMultipartManifest(dir)
	if err != nil {
		return nil, 0, err
	}
	manifestByIndex := make(map[int]multipartPartMeta, len(manifest))
	for _, meta := range manifest {
		manifestByIndex[meta.Index] = meta
		originalPath := filepath.Join(dir, meta.File)
		count := countFileFuzzTagPath(rewritten, originalPath)
		resourceCount += count
		replacementPath, replacing := partReplacements[meta.Index]
		if !replacing {
			if count > 0 {
				if err := validateRegularRequestResource(originalPath, "multipart request sidecar"); err != nil {
					return nil, 0, err
				}
			}
			continue
		}
		if replacementPath == "" {
			return nil, 0, utils.Errorf("multipart replacement part %d has an empty file path", meta.Index)
		}
		if count == 0 {
			return nil, 0, utils.Errorf("large request multipart part %d file tag was removed from the edited request", meta.Index)
		}
		if err := validateRegularRequestResource(replacementPath, "multipart request replacement"); err != nil {
			return nil, 0, err
		}
		var replaceErr error
		rewritten, _, replaceErr = replaceFileFuzzTagPath(rewritten, originalPath, replacementPath)
		if replaceErr != nil {
			return nil, 0, replaceErr
		}
	}
	for partIndex := range partReplacements {
		if _, ok := manifestByIndex[partIndex]; !ok {
			return nil, 0, utils.Errorf("multipart replacement part %d not found in manifest", partIndex)
		}
	}
	return rewritten, resourceCount, nil
}

func buildFuzzableMultipartRequestPacket(packet []byte, bodyFile string) ([]byte, error) {
	if bodyFile == "" {
		return nil, utils.Error("multipart spill body file is empty")
	}
	dir := multipartSidecarDirFromBodyFile(bodyFile)
	manifest, err := loadMultipartManifest(dir)
	if err != nil {
		return nil, err
	}
	byIndex := make(map[int]multipartPartMeta, len(manifest))
	for _, meta := range manifest {
		byIndex[meta.Index] = meta
	}

	indices := multipartSpillMarkerPattern.FindAllSubmatchIndex(packet, -1)
	if len(indices) == 0 {
		return nil, utils.Error("multipart spill marker is missing")
	}
	var out bytes.Buffer
	last := 0
	for _, index := range indices {
		partIndex, err := strconv.Atoi(string(packet[index[2]:index[3]]))
		if err != nil {
			return nil, utils.Wrap(err, "parse multipart spill part index")
		}
		meta, ok := byIndex[partIndex]
		if !ok {
			return nil, utils.Errorf("multipart part %d not found in manifest", partIndex)
		}
		path := filepath.Join(dir, meta.File)
		tag, err := buildFileFuzzTag(path)
		if err != nil {
			return nil, err
		}
		out.Write(packet[last:index[0]])
		out.WriteString(tag)
		// multipartSpillMarkerPattern accepts and consumes the marker line's
		// optional CR. Preserve it so the following LF remains a valid CRLF
		// separator after the tag expands to raw file bytes.
		if index[1] > index[0] && packet[index[1]-1] == '\r' {
			out.WriteByte('\r')
		}
		last = index[1]
	}
	out.Write(packet[last:])
	return out.Bytes(), nil
}

// PrepareLargeHTTPFlowRequest spills oversized request bodies once, caches display packet on req,
// and returns a truncated packet safe for mirror/history/MITM UI. Idempotent per request.
func PrepareLargeHTTPFlowRequest(req *http.Request, fullPacket []byte) []byte {
	if len(fullPacket) == 0 {
		return fullPacket
	}
	if req != nil && httpctx.GetRequestTooLarge(req) {
		if cached := httpctx.GetRequestDisplayPacket(req); len(cached) > 0 {
			return cached
		}
	}

	res, err := spillLargeHTTPFlowRequestIfNeeded(fullPacket)
	if err != nil {
		log.Errorf("prepare large http flow request failed: %s", err)
		return fullPacket
	}
	if !res.IsTooLarge {
		return fullPacket
	}
	displayPacket, err := BuildFuzzableHTTPFlowRequestPacket(res.StoredPacket, res.BodyFile)
	if err != nil {
		log.Errorf("prepare fuzzable http flow request failed: %s", err)
		removeLargeRequestSpillFiles(res.HeaderFile, res.BodyFile)
		return fullPacket
	}

	if req != nil {
		httpctx.SetRequestTooLarge(req, true)
		httpctx.SetRequestTooLargeHeaderFile(req, res.HeaderFile)
		httpctx.SetRequestTooLargeBodyFile(req, res.BodyFile)
		httpctx.SetRequestTooLargeSize(req, int64(res.OriginalBodyLen))
		httpctx.SetRequestDisplayPacket(req, displayPacket)
	}
	return displayPacket
}

// RefreshPreparedLargeHTTPFlowRequest replaces the request-scoped spill with a
// snapshot of the packet that will actually be sent upstream. Manual hijack can
// rebuild an oversized skeleton with edited headers or replacement file bytes;
// keeping the first spill in the request context would make HTTP History point
// at the pre-edit sidecar instead of the wire request.
//
// The old spill is removed only after the new packet has been prepared
// successfully, so a failed refresh leaves the existing snapshot usable.
func RefreshPreparedLargeHTTPFlowRequest(req *http.Request, fullPacket []byte) ([]byte, error) {
	if req == nil || len(fullPacket) == 0 {
		return fullPacket, nil
	}

	res, err := spillLargeHTTPFlowRequestIfNeeded(fullPacket)
	if err != nil {
		return nil, err
	}

	oldHeaderFile := httpctx.GetRequestTooLargeHeaderFile(req)
	oldBodyFile := httpctx.GetRequestTooLargeBodyFile(req)

	if res.IsTooLarge {
		displayPacket, buildErr := BuildFuzzableHTTPFlowRequestPacket(res.StoredPacket, res.BodyFile)
		if buildErr != nil {
			removeLargeRequestSpillFiles(res.HeaderFile, res.BodyFile)
			return nil, buildErr
		}
		res.StoredPacket = displayPacket
	}

	// Publish the new snapshot only after its executable representation was
	// built successfully. Until this point the old context remains usable.
	httpctx.SetRequestTooLarge(req, res.IsTooLarge)
	httpctx.SetRequestTooLargeHeaderFile(req, res.HeaderFile)
	httpctx.SetRequestTooLargeBodyFile(req, res.BodyFile)
	httpctx.SetRequestTooLargeSize(req, int64(res.OriginalBodyLen))
	if res.IsTooLarge {
		httpctx.SetRequestDisplayPacket(req, res.StoredPacket)
	} else {
		// Keep the complete, below-threshold packet only in the normal modified
		// request slot. The display cache exists solely for spill skeletons.
		httpctx.SetRequestDisplayPacket(req, nil)
	}
	removeLargeRequestSpillFiles(oldHeaderFile, oldBodyFile)
	return res.StoredPacket, nil
}

// CleanupPreparedLargeHTTPFlowRequest releases a request-scoped spill when the
// request reaches a terminal path without transferring ownership to a saved
// HTTPFlow. It is intentionally idempotent so failure and cancellation paths
// can call it defensively.
func CleanupPreparedLargeHTTPFlowRequest(req *http.Request) {
	if req == nil {
		return
	}
	removeLargeRequestSpillFiles(
		httpctx.GetRequestTooLargeHeaderFile(req),
		httpctx.GetRequestTooLargeBodyFile(req),
	)
	httpctx.SetRequestTooLarge(req, false)
	httpctx.SetRequestTooLargeHeaderFile(req, "")
	httpctx.SetRequestTooLargeBodyFile(req, "")
	httpctx.SetRequestTooLargeSize(req, 0)
	httpctx.SetRequestDisplayPacket(req, nil)
}

func removeLargeRequestSpillFiles(headerFile, bodyFile string) {
	headerSuffix, headerOK := engineOwnedLargeRequestHeaderSuffix(headerFile)
	bodySuffix, _, bodyOK := engineOwnedLargeRequestBodySuffix(bodyFile)

	// A persisted spill owns a header/body pair created from the same unique
	// suffix. Refuse partial or cross-Flow metadata instead of deleting a path
	// merely because it resembles an engine resource.
	if headerFile != "" && bodyFile != "" {
		if !headerOK || !bodyOK || headerSuffix != bodySuffix {
			return
		}
		removeEngineOwnedLargeRequestBody(bodyFile)
		_ = os.Remove(headerFile)
		return
	}
	// Partial cleanup is used only while constructing a spill. It remains
	// restricted to one validated engine-owned resource.
	if bodyOK {
		removeEngineOwnedLargeRequestBody(bodyFile)
	}
	if headerOK {
		_ = os.Remove(headerFile)
	}
}

func applyPreparedLargeRequestSpill(reqIns *http.Request, reqRaw []byte) (stored []byte, isTooLarge bool, headerFile, bodyFile string, bodyLen int, err error) {
	stored = reqRaw
	if reqIns != nil && httpctx.GetRequestTooLarge(reqIns) {
		isTooLarge = true
		headerFile = httpctx.GetRequestTooLargeHeaderFile(reqIns)
		bodyFile = httpctx.GetRequestTooLargeBodyFile(reqIns)
		bodyLen = int(httpctx.GetRequestTooLargeSize(reqIns))
		if cached := httpctx.GetRequestDisplayPacket(reqIns); len(cached) > 0 {
			stored = cached
		}
		return
	}

	var spillRes largeRequestSpillResult
	spillRes, err = spillLargeHTTPFlowRequestIfNeeded(reqRaw)
	if err != nil {
		return reqRaw, false, "", "", requestBodyLengthFromPacket(reqRaw), err
	}
	stored = spillRes.StoredPacket
	if spillRes.IsTooLarge {
		stored, err = BuildFuzzableHTTPFlowRequestPacket(spillRes.StoredPacket, spillRes.BodyFile)
		if err != nil {
			removeLargeRequestSpillFiles(spillRes.HeaderFile, spillRes.BodyFile)
			return reqRaw, false, "", "", requestBodyLengthFromPacket(reqRaw), err
		}
	}
	if spillRes.IsTooLarge && reqIns != nil {
		httpctx.SetRequestTooLarge(reqIns, true)
		httpctx.SetRequestTooLargeHeaderFile(reqIns, spillRes.HeaderFile)
		httpctx.SetRequestTooLargeBodyFile(reqIns, spillRes.BodyFile)
		httpctx.SetRequestTooLargeSize(reqIns, int64(spillRes.OriginalBodyLen))
		httpctx.SetRequestDisplayPacket(reqIns, stored)
	}
	return stored, spillRes.IsTooLarge, spillRes.HeaderFile, spillRes.BodyFile, spillRes.OriginalBodyLen, nil
}

func requestBodyLengthFromPacket(packet []byte) int {
	if len(packet) == 0 {
		return 0
	}
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(packet)
	return len(body)
}
