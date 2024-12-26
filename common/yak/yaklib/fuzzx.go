package yaklib

import (
	"github.com/yaklang/yaklang/common/fuzzx"
	"github.com/yaklang/yaklang/common/mutate"
)

var FuzzxExports = map[string]interface{}{
	"NewRequest":     fuzzx.NewFuzzHTTPRequest,
	"MustNewRequest": fuzzx.MustNewFuzzHTTPRequest,

	"source":             mutate.WithPoolOpt_Source,
	"fromPlugin":         mutate.WithPoolOpt_FromPlugin,
	"runtimeID":          mutate.WithPoolOpt_RuntimeId,
	"https":              mutate.WithPoolOpt_Https,
	"host":               mutate.WithPoolOpt_Addr,
	"port":               mutate.WithPoolOpt_Port,
	"context":            mutate.WithPoolOpt_Context,
	"timeout":            mutate.WithPoolOpt_Timeout,
	"concurrentLimit":    mutate.WithPoolOpt_Concurrent,
	"proxy":              mutate.WithPoolOpt_Proxy,
	"redirectTimes":      mutate.WithPoolOpt_RedirectTimes,
	"noRedirect":         mutate.WithPoolOpt_NoFollowRedirect,
	"noFixContentLength": mutate.WithPoolOpt_noFixContentLength,
	"connPool":           mutate.WithPoolOpt_UseConnPool,
	"delay":              mutate.WithPoolOPt_DelaySeconds,
	"namingContext":      mutate.WithPoolOpt_NamingContext,
}
