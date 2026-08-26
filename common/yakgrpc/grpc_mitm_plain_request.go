//go:build !yakit_exclude

package yakgrpc

import (
	"net/http"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func cachePlainRequestBytesIfStorable(req *http.Request, decoded []byte) {
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(decoded)
	if len(body) <= yakit.GetMaxHTTPFlowRequestBodyInDBBytes() {
		httpctx.SetPlainRequestBytes(req, decoded)
	}
}

func decodeAndCachePlainRequestBytesIfStorable(req *http.Request, wire []byte) []byte {
	decoded, independentlyOwned := lowhttp.DeletePacketEncodingWithOwnership(wire)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(decoded)
	if len(body) <= yakit.GetMaxHTTPFlowRequestBodyInDBBytes() {
		if independentlyOwned {
			httpctx.SetPlainRequestBytesOwned(req, decoded)
		} else if !httpctx.SetPlainRequestBytesBorrowedFromBare(req, decoded) {
			httpctx.SetPlainRequestBytes(req, decoded)
		}
	}
	return decoded
}

// getMITMPlainRequestBytes returns the concrete request bytes used by mirror
// hooks, filters and TrafficGuard. It deliberately does not create History
// sidecars: whether the traffic will be persisted has not been decided yet.
func getMITMPlainRequestBytes(req *http.Request) []byte {
	if req == nil {
		return nil
	}
	if httpctx.GetRequestIsModified(req) {
		return httpctx.GetHijackedRequestBytes(req)
	}
	plainRequest := httpctx.GetPlainRequestBytes(req)
	if len(plainRequest) <= 0 {
		plainRequest = decodeAndCachePlainRequestBytesIfStorable(req, httpctx.GetBareRequestBytes(req))
	}
	return plainRequest
}

// getMITMDisplayRequestBytes adds the bounded, executable History/editor
// representation only when a UI or persistence consumer actually needs it.
func getMITMDisplayRequestBytes(req *http.Request) []byte {
	if req == nil {
		return nil
	}
	if httpctx.GetRequestTooLarge(req) {
		if cached := httpctx.GetRequestDisplayPacket(req); len(cached) > 0 {
			return cached
		}
	}
	return yakit.PrepareLargeHTTPFlowRequest(req, getMITMPlainRequestBytes(req))
}
