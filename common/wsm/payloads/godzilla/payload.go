package godzilla

import (
	"encoding/hex"
	"fmt"

	"github.com/yaklang/yaklang/common/wsm/payloads"
)

// hexPayload 从统一的 payloads.tar.gz 中还原 payload 并转换为 hex。
func hexPayload(name string) string {
	data, err := payloads.ReadGodzillaPayload(name)
	if err != nil {
		panic(fmt.Sprintf("restore godzilla payload %s failed: %v", name, err))
	}
	return hex.EncodeToString(data)
}

var (
	// JavaClassPayload 对应 static/payload2.class，由 zulu-1.8.0_275 arm64 编译
	JavaClassPayload = hexPayload("payload2.class")
	PhpCodePayload   = hexPayload("payload.php")
	AspCodePayload   = hexPayload("payload.asp")
	CsharpDllPayload = hexPayload("payload_aspx.bin")
)
