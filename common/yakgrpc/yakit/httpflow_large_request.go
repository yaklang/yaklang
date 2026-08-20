package yakit

import (
	"fmt"
	"net/http"
	"os"
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
	return strings.Contains(lower, "request too large(") || strings.Contains(lower, "request-too-large(")
}

// IsFlatSpillRequestPacket reports whether packet is the editable truncated
// representation of a non-multipart oversized request body.
func IsFlatSpillRequestPacket(packet []byte) bool {
	return storedHTTPFlowLargeRequestTruncatePattern.Match(packet)
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
	if len(body) <= GetMaxHTTPFlowRequestBodyInDBBytes() {
		return res, nil
	}

	// Multipart/form-data uploads carrying file parts are skeletonized: each
	// file part spills to its own disk file and the in-DB body keeps only an
	// editable skeleton with placeholders. Falls back to flat spill when the
	// body is not multipart or carries no file part.
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

	if req != nil {
		httpctx.SetRequestTooLarge(req, true)
		httpctx.SetRequestTooLargeHeaderFile(req, res.HeaderFile)
		httpctx.SetRequestTooLargeBodyFile(req, res.BodyFile)
		httpctx.SetRequestTooLargeSize(req, int64(res.OriginalBodyLen))
		httpctx.SetRequestDisplayPacket(req, res.StoredPacket)
	}
	return res.StoredPacket
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

func removeLargeRequestSpillFiles(headerFile, bodyFile string) {
	cleanupMultipartSidecar(bodyFile)
	if bodyFile != "" {
		// For multipart spills cleanupMultipartSidecar already removed the
		// containing directory; Remove is harmless in that case. Flat spills
		// need the body file removed directly.
		_ = os.Remove(bodyFile)
	}
	if headerFile != "" {
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
	if spillRes.IsTooLarge && reqIns != nil {
		httpctx.SetRequestTooLarge(reqIns, true)
		httpctx.SetRequestTooLargeHeaderFile(reqIns, spillRes.HeaderFile)
		httpctx.SetRequestTooLargeBodyFile(reqIns, spillRes.BodyFile)
		httpctx.SetRequestTooLargeSize(reqIns, int64(spillRes.OriginalBodyLen))
		httpctx.SetRequestDisplayPacket(reqIns, spillRes.StoredPacket)
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
