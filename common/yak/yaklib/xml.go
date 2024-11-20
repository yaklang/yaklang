package yaklib

import (
	"github.com/yaklang/yaklang/common/utils"
)

var XMLExports = map[string]interface{}{
	"Escape": utils.XmlEscape,
	"dumps":  utils.XmlDumps,
	"loads":  utils.XmlLoads,
	"escape": utils.WithHTMLEscape,
}
