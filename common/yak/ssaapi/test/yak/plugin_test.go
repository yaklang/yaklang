package ssaapi

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestPlugin(t *testing.T) {
	code := `hijackHTTPRequest = func(isHttps, url, req, forward /*func(modifiedRequest []byte)*/, drop /*func()*/) {
	drop()	
}
`
	// for i := 0; i < 50; i++ {
	ssatest.CheckSyntaxFlow(t, code, `
		hijackHTTPRequest<getFormalParams> as $hijackHTTPRequest
		$hijackHTTPRequest<slice(index=0)> as $param0
		$hijackHTTPRequest<slice(index=1)> as $param1
		$hijackHTTPRequest<slice(index=2)> as $param2
		$hijackHTTPRequest<slice(index=3)> as $param3
		`, map[string][]string{
		"param0": {"Parameter-isHttps"},
		"param1": {"Parameter-url"},
		"param2": {"Parameter-req"},
		"param3": {"Parameter-forward"},
	}, ssaapi.WithLanguage(ssaapi.Yak))
	// }
}
