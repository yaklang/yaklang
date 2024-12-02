package jsp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"strings"
)

const (
	JSP_EL_PARSE_METHOD = "elExpr.parse"
)

type TagType int

const (
	JSP_TAG_PURE_HTML TagType = 1 + iota
	// jsp directive tag
	JSP_DIRECTIVE_PAGE

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

func (y *JSPVisitor) GetDirectiveTag(name string) TagType {
	switch name {
	case "page":
		return JSP_DIRECTIVE_PAGE
	}
	return -1
}

// ParseSingleTag parse only open or close tag
func (y *JSPVisitor) ParseSingleTag(text string) {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	switch tagInfo.typ {
	case JSP_TAG_PURE_HTML:
		y.EmitPureText(text)
	case JSP_DIRECTIVE_PAGE:
		value, ok := tagInfo.attrs["import"]
		if ok {
			paths := strings.Split(value, ",")
			for _, path := range paths {
				y.EmitImport(path)
			}
		}
		return
	case JSP_TAG_CORE_OUT:
		elExpr, ok := tagInfo.attrs["value"]
		if !ok {
			log.Errorf("JSTL out tag must have value attribute")
			return
		}
		variable := y.fixElExpr(elExpr)

		// check escapeXml attribute
		var noEscape bool
		if v, ok := tagInfo.attrs["escapeXml"]; ok {
			noEscape = v == "false"
		}
		if noEscape {
			y.EmitOutput(variable)
		} else {
			y.EmitEscapeOutput(variable)
		}
	case JSP_TAG_CORE_SET:
		variable, ok := tagInfo.attrs["var"]
		if !ok {
			log.Errorf("JSTL set tag must have var attribute")
			return
		}
		elExpr, ok := tagInfo.attrs["value"]
		value := y.fixElExpr(elExpr)
		y.EmitPureCode("request.setAttribute(\"" + variable + "\", " + value + ");")
	default:
		log.Errorf("Unknown JSTL tag type: %v", tagInfo.typ)
	}
}

func (y *JSPVisitor) ParseDoubleTag(openTag string, closedTag string, visitContent func()) {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	switch tagInfo.typ {
	case JSP_TAG_PURE_HTML:
		y.EmitPureText(openTag)
		visitContent()
		y.EmitPureText(closedTag)
	case JSP_TAG_CORE_IF:
		condition, ok := tagInfo.attrs["test"]
		if !ok {
			log.Errorf("JSTL if tag must have test attribute")
			return
		}
		condition = y.fixElExpr(condition)
		y.EmitPureCode("if (" + condition + ") {")
		visitContent()
		y.EmitPureCode("}")
	case JSP_TAG_CORE_CHOOSE:
		y.EmitPureCode("switch (true) {")
		visitContent()
		y.EmitPureCode("}")
	case JSP_TAG_CORE_WHEN:
		condition, ok := tagInfo.attrs["test"]
		if !ok {
			log.Errorf("JSTL when tag must have test attribute")
			return
		}
		condition = y.fixElExpr(condition)
		y.EmitPureCode("case " + condition + ":")
		visitContent()
	case JSP_TAG_CORE_OTHERWISE:
		y.EmitPureCode("default:")
		visitContent()
	case JSP_TAG_CORE_FOREACH:
		variable, ok := tagInfo.attrs["var"]
		if !ok {
			log.Errorf("JSTL foreach tag must have var attribute")
			return
		}
		items, ok := tagInfo.attrs["items"]
		if !ok {
			log.Errorf("JSTL foreach tag must have items attribute")
			return
		}
		y.EmitPureCode("for (Object " + variable + " : " + items + ") {")
		visitContent()
		y.EmitPureCode("}")
	default:
		log.Errorf("Unknown JSTL tag type: %v", tagInfo.typ)
	}
}

func (y *JSPVisitor) fixElExpr(expr string) string {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "${") && strings.HasSuffix(expr, "}") {
		expr = fmt.Sprintf("%s(\"%s\")", JSP_EL_PARSE_METHOD, expr)
	}
	return expr
}
