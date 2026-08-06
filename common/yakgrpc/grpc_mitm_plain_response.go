//go:build !yakit_exclude

package yakgrpc

import (
	"net/http"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func decodeAndCachePlainResponseBytes(req *http.Request, wire []byte) []byte {
	decoded, independentlyOwned := lowhttp.DeletePacketEncodingWithOwnership(wire)
	if independentlyOwned {
		httpctx.SetPlainResponseBytesOwned(req, decoded)
	} else {
		httpctx.SetPlainResponseBytes(req, decoded)
	}
	return decoded
}
