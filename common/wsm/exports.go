package wsm

import (
	"github.com/yaklang/yaklang/common/wsm/payloads/behinder"
)

var WebShellExports = map[string]interface{}{
	"NewWebshell": NewWebShell,

	// 设置 shell 信息
	"tools":        SetShellType,
	"setProxy":     SetProxy,
	"useBehinder":  SetBeinderTool,
	"useGodzilla":  SetGodzillaTool,
	"useYakshell":  SetYakShellTool(),
	"useBase64":    SetBase64Aes,
	"useRaw":       SetRawAes,
	"useXorBase64": SetBase64Xor(),
	"script":       SetShellScript,
	"secretKey":    SetSecretKey,
	"passParams":   SetPass,

	// 设置参数
	"cmdPath": behinder.SetCommandPath,
}
