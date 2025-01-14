package jsp

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"regexp"
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

type TagInfo struct {
	typ   TagType
	attrs map[string]string
	funcs []func()
}

func newTagInfo(tag TagType) *TagInfo {
	return &TagInfo{
		typ:   tag,
		attrs: map[string]string{},
	}
}

func (y *JSPVisitor) PushTagInfo(tag TagType) {
	y.tagStack.Push(newTagInfo(tag))
}

func (y *JSPVisitor) PopTagInfo() {
	y.tagStack.Pop()
}

func (y *JSPVisitor) AddTagAttr(key, value string) {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	tagInfo.attrs[key] = value
}

func (y *JSPVisitor) AddAttrFunc(f func()) {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	tagInfo.funcs = append(tagInfo.funcs, f)
}

func (y *JSPVisitor) EmitAttrFunc() {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	for _, f := range tagInfo.funcs {
		f()
	}
}

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
	default:
		return JSP_TAG_PURE_HTML
	}
}

func (y *JSPVisitor) GetDirectiveTag(name string) TagType {
	switch name {
	case "page":
		return JSP_DIRECTIVE_PAGE
	}
	return -1
}

// ParseSingleTag parse only open or close tag
func (y *JSPVisitor) ParseSingleTag() {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	switch tagInfo.typ {
	case JSP_TAG_PURE_HTML:
		y.EmitAttrFunc()
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
		variable, ok := tagInfo.attrs["value"]
		if !ok {
			log.Errorf("JSTL out tag must have value attribute")
			return
		}
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
		value, ok := tagInfo.attrs["value"]
		y.EmitPureCode("request.setAttribute(\"" + variable + "\", " + value + ");")
	default:
		log.Errorf("Unknown JSTL tag type: %v", tagInfo.typ)
		y.EmitAttrFunc()
	}
}

func (y *JSPVisitor) ParseDoubleTag(endText string, visitContent func()) {
	tagInfo := y.PeekTagInfo()
	if tagInfo == nil {
		return
	}
	switch tagInfo.typ {
	case JSP_TAG_PURE_HTML:
		y.EmitAttrFunc()
		visitContent()
		y.EmitPureText(endText)
	case JSP_TAG_CORE_IF:
		condition, ok := tagInfo.attrs["test"]
		if !ok {
			log.Errorf("JSTL if tag must have test attribute")
			return
		}
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
		y.EmitAttrFunc()
	}
}

func (y *JSPVisitor) appendElParseMethod(expr string) string {
	expr = strings.TrimSpace(expr)
	re := regexp.MustCompile(`\$\{(.+?)\}`)
	expr = yakunquote.TryUnquote(expr)
	return re.ReplaceAllString(expr, fmt.Sprintf("%s(\"$1\")", JSP_EL_PARSE_METHOD))
}

func (y *JSPVisitor) replaceElExprInText(text string) string {
	re := regexp.MustCompile(`\$\{(.+?)\}`)
	expr := re.FindString(text)
	if expr != "" {
		expr = y.appendElParseMethod(expr)
		y.EmitOutput(expr)
	}
	output := re.ReplaceAllString(text, "")

	methodRe := regexp.MustCompile(`elExpr\.parse\(([^)]*)\)`)
	expr = methodRe.FindString(output)
	if expr != "" {
		expr = y.appendElParseMethod(expr)
		y.EmitOutput(expr)
	}
	output = methodRe.ReplaceAllString(output, "")
	return output
}
