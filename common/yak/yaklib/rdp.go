package yaklib

import (
	"yaklang/common/utils/bruteutils"
	"yaklang/common/utils/extrafp"
)

var RdpExports = map[string]interface{}{
	"Login":   bruteutils.RDPLogin,
	"Version": extrafp.RDPVersion,
}
