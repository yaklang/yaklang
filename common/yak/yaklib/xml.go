package yaklib

import (
	"github.com/yaklang/yaklang/common/utils/yakxml/xml-tools"
)

var XMLExports = map[string]interface{}{
	"Escape":   xml_tools.XmlEscape,
	"dumps":    xml_tools.XmlDumps,
	"loads":    xml_tools.XmlLoads,
	"Prettify": xml_tools.XmlPrettify,
	"escape":   xml_tools.WithHTMLEscape,
}
