package jsp

import (
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

type TagType int

const (
	JSP_TAG_PURE_HTML TagType = iota

	// core tags
	JSP_TAG_CORE_OUT
	JSP_TAG_CORE_SET
	JSP_TAG_CORE_IF
	JSP_TAG_CORE_CHOOSE
	JSP_TAG_CORE_WHEN
	JSP_TAG_CORE_OTHERWISE
	JSP_TAG_CORE_FOREACH
	JSP_TAG_CORE_FOR_TOKENS
	JSP_TAG_CORE_IMPORT
	JSP_TAG_CORE_URL
	JSP_TAG_CORE_PARAM
)

func (y *JSPVisitor) GetCoreJSTLTag(name string) TagType {
	switch name {
	case "out":
		return JSP_TAG_CORE_OUT
	case "set":
		return JSP_TAG_CORE_SET
	case "if":
		return JSP_TAG_CORE_IF
	case "choose":
		return JSP_TAG_CORE_CHOOSE
	case "when":
		return JSP_TAG_CORE_WHEN
	case "otherwise":
		return JSP_TAG_CORE_OTHERWISE
	case "foreach":
		return JSP_TAG_CORE_FOREACH
	case "fortokens":
		return JSP_TAG_CORE_FOR_TOKENS
	case "import":
		return JSP_TAG_CORE_IMPORT
	case "url":
		return JSP_TAG_CORE_URL
	case "param":
		return JSP_TAG_CORE_PARAM
	}
	return -1
}

func (y *JSPVisitor) ParseTag(tag TagType, attrs map[string]string) {
	switch tag {
	case JSP_TAG_CORE_OUT:
		elExpr, ok := attrs["value"]
		if !ok {
			log.Errorf("JSTL out tag must have value attribute")
			return
		}
		variable := y.extractELExpression(elExpr)

		// check escapeXml attribute
		var noEscape bool
		if v, ok := attrs["escapeXml"]; ok {
			noEscape = v == "false"
		}
		if noEscape {
			y.EmitOutput(variable, y.CurrentRange)
		} else {
			y.EmitEscapeOutput(variable, y.CurrentRange)
		}
	default:
		log.Errorf("Unknown JSTL tag type: %v", tag)
	}
}

func (y *JSPVisitor) extractELExpression(expr string) string {
	return strings.Trim(expr, "${}")
}
