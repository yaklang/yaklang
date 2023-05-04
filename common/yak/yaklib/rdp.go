package yaklib

import (
	"github.com/yaklang/yaklang/common/utils/bruteutils"
	"github.com/yaklang/yaklang/common/utils/extrafp"
)

var RdpExports = map[string]interface{}{
	"Login":   bruteutils.RDPLogin,
	"Version": extrafp.RDPVersion,
}
