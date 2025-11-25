package testdata

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCheckRef(t *testing.T) {
	ssatest.CheckSyntaxFlowSource(t, `
function nodeZip(options) {
    console.log(options);
}
function zip(options) {
    let promise;
    promise = nodeZip(options);
}

function zzzip(options) {
    zip(options);
}
var option = {}
zzzip(option);
`, `
		option--> as $result
	`, map[string][]string{
		"result": {"console.log(options)"},
	}, ssaapi.WithLanguage(ssaconfig.TS),
	)
}
