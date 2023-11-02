package bin_parser

import (
	"fmt"
	"github.com/yaklang/yaklang/common/binx"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

func DumpBinResult(resultIf binx.ResultIf) {
	println(sdumpBinResult(resultIf, 0))
}

func sdumpBinResult(resultIf binx.ResultIf, deep int) (result string) {
	switch ret := resultIf.(type) {
	case *binx.ListResult:
		result += fmt.Sprintf("%s%s: %d\n", strings.Repeat(" ", deep*2), ret.Identifier, ret.Length)
		for _, v := range ret.Result {
			result += sdumpBinResult(v, deep+1)
		}
	case *binx.Result:
		//v := ret.Value()
		//if v1, ok := v.([]byte); ok {
		//	v = codec.EncodeToHex(v1)
		//}
		v := codec.EncodeToHex(ret.GetBytes())
		result += fmt.Sprintf("%s%s: %v\n", strings.Repeat(" ", deep*2), ret.Identifier, v)
	}
	return
}
