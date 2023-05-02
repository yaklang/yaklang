package yaklib

import (
	"yaklang.io/yaklang/common/utils/bruteutils"
	"yaklang.io/yaklang/common/utils/extrafp"
)

var RdpExports = map[string]interface{}{
	"Login":   bruteutils.RDPLogin,
	"Version": extrafp.RDPVersion,
}
