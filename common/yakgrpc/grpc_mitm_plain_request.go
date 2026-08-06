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
