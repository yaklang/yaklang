package yakit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// MaxHTTPFlowRequestBodyInDBBytes aligns with History list preview (200KB).
const MaxHTTPFlowRequestBodyInDBBytes = 200 * 1024

const maxHTTPFlowRequestBodyInDBBytes = MaxHTTPFlowRequestBodyInDBBytes

var (
	largeRequestNotifyMu       sync.Mutex
	lastLargeRequestNotifyTime time.Time
)

const largeRequestNotifyMinInterval = 3 * time.Second

const storedHTTPFlowLargeRequestTruncateNotice = "[[request too large(%s), truncated]] use GetHTTPFlowBodyById(IsRequest=true) for full body"

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

	header, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	res.OriginalBodyLen = len(body)
	if len(body) <= maxHTTPFlowRequestBodyInDBBytes {
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
		httpctx.SetPlainRequestBytes(req, res.StoredPacket)
	}
	return res.StoredPacket
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
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(packet)
	return len(body)
}

// LargeHTTPFlowRequestUserNotice returns a user-facing message when request body is spilled to disk.
func LargeHTTPFlowRequestUserNotice(flow *schema.HTTPFlow) string {
	if flow == nil {
		return ""
	}
	size := utils.ByteSize(uint64(flow.RequestLength))
	if flow.RequestLength <= 0 {
		_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket([]byte(flow.GetRequest()))
		size = utils.ByteSize(uint64(len(body)))
	}
	msg := fmt.Sprintf("检测到超大请求包（%s）\n\n", size)
	msg += "请求 body 未完整写入数据库（防止卡顿/崩溃），已落盘保存。\n"
	msg += "History 列表中不会展示完整请求体，请在详情中查看完整请求，或通过「下载 Body」流式读取。\n"
	if flow.TooLargeRequestBodyFile != "" {
		msg += fmt.Sprintf("\nBody 文件：\n%s", flow.TooLargeRequestBodyFile)
	}
	if flow.TooLargeRequestHeaderFile != "" {
		msg += fmt.Sprintf("\nHeader 文件：\n%s", flow.TooLargeRequestHeaderFile)
	}
	if flow.Url != "" {
		msg += fmt.Sprintf("\n\nURL: %s", flow.Url)
	}
	return msg
}

// ShouldNotifyLargeHTTPFlowRequest throttles repeated MITM notifications for bulk uploads.
func ShouldNotifyLargeHTTPFlowRequest() bool {
	largeRequestNotifyMu.Lock()
	defer largeRequestNotifyMu.Unlock()
	if time.Since(lastLargeRequestNotifyTime) < largeRequestNotifyMinInterval {
		return false
	}
	lastLargeRequestNotifyTime = time.Now()
	return true
}
