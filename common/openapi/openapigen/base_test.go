package openapigen

import (
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strings"
	"testing"
)

func packetToPathItem(packet []byte, pathItems *omap.OrderedMap[string, *openapi3.PathItem]) *omap.OrderedMap[string, *openapi3.PathItem] {
	pathRaw := lowhttp.GetHTTPRequestPath(packet)
	pathWithoutQuery, query, haveQuery := strings.Cut(pathRaw, "?")
	method := lowhttp.GetHTTPRequestMethod(packet)

	var item *openapi3.PathItem
	for _, pathOrigin := range pathItems.Keys() {
		if pathWithoutQuery == pathOrigin {

		}
	}
	_ = method
	_ = query
	_ = haveQuery
	_ = item
	return pathItems
}

func Generator(packet []byte) {
	var v3 openapi3.T
	v3.Servers = append(v3.Servers, &openapi3.Server{
		URL: "https://www.example.com",
	})
}

func TestGenerator(t *testing.T) {

}
