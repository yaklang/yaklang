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
	JSP_TAG_SYNTHETIC_FRAGMENT
	// jsp directive tag
	JSP_DIRECTIVE_PAGE
	JSP_DIRECTIVE_INCLUDE

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
	case "include":
		return JSP_DIRECTIVE_INCLUDE
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
	case JSP_TAG_SYNTHETIC_FRAGMENT:
		return
	case JSP_DIRECTIVE_PAGE:
		value, ok := tagInfo.attrs["import"]
		if ok {
			paths := strings.Split(value, ",")
			for _, path := range paths {
				y.EmitImport(path)
			}
		}
		return
	case JSP_DIRECTIVE_INCLUDE:
		// TODO: expand static include target into the generated output when needed.
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
		value := ""
		if v, ok := tagInfo.attrs["value"]; ok {
			value = v
		}
		y.EmitPureCode("request.setAttribute(\"" + javaLiteralBody(variable) + "\", " + javaValueArg(value) + ");")
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
	case JSP_TAG_SYNTHETIC_FRAGMENT:
		visitContent()
	case JSP_TAG_CORE_IF:
		condition, ok := tagInfo.attrs["test"]
		if !ok {
			log.Errorf("JSTL if tag must have test attribute")
			return
		}
		y.EmitPureCode("if (" + y.asJavaCondition(condition) + ") {")
		visitContent()
		y.EmitPureCode("}")
	case JSP_TAG_CORE_CHOOSE:
		y.switchDepth++
		y.EmitPureCode("switch (true) {")
		visitContent()
		y.EmitPureCode("}")
		y.switchDepth--
	case JSP_TAG_CORE_WHEN:
		condition, ok := tagInfo.attrs["test"]
		if !ok {
			log.Errorf("JSTL when tag must have test attribute")
			return
		}
		if y.switchDepth > 0 {
			y.EmitPureCode("case " + y.asJavaCondition(condition) + ":")
			visitContent()
			break
		}
		// The choose was not a Java switch (the tag was split or nested the
		// wrong way out). An if is still a branch java2ssa can parse.
		y.EmitPureCode("if (" + y.asJavaCondition(condition) + ") {")
		visitContent()
		y.EmitPureCode("}")
	case JSP_TAG_CORE_OTHERWISE:
		if y.switchDepth > 0 {
			y.EmitPureCode("default:")
			visitContent()
			break
		}
		y.EmitPureCode("if (true) {")
		visitContent()
		y.EmitPureCode("}")
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

var (
	// elBraceRe matches one ${...} expression. It is compiled once: coremail-sized
	// pages call this on every text node and every attribute.
	elBraceRe = regexp.MustCompile(`\$\{(.+?)\}`)
	// elParseCallRe matches a finished elExpr.parse("...") call, including
	// escaped quotes. The previous ([^)]*) form stopped at the first ')' inside
	// the expression, so fn:escapeXml(...) became an unclosed Java string
	// ("token recognition error at: '"fn:escapeXml(...)').
	elParseCallRe = regexp.MustCompile(`elExpr\.parse\("(?:\\.|[^"\\])*"\)`)
)

func (y *JSPVisitor) appendElParseMethod(expr string) string {
	expr = strings.TrimSpace(expr)
	expr = yakunquote.TryUnquote(expr)
	return elBraceRe.ReplaceAllStringFunc(expr, func(match string) string {
		inner := elBraceRe.FindStringSubmatch(match)
		if len(inner) < 2 {
			return match
		}
		return fmt.Sprintf("%s(\"%s\")", JSP_EL_PARSE_METHOD, javaLiteralBody(inner[1]))
	})
}

// javaLiteralBody escapes text placed between double quotes in generated Java.
// A raw quote, backslash or line break inside an EL argument otherwise terminates
// the literal and the next token is the rest of the expression plus the CRLF.
// javaValueArg turns a JSTL attribute into a Java expression. EL is already
// elExpr.parse(...); a bare literal such as |uid|user| must be a string or the
// generated statement does not parse.
func javaValueArg(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	if strings.HasPrefix(value, "elExpr.parse(") {
		return value
	}
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return value
	}
	return `"` + javaLiteralBody(value) + `"`
}

// asJavaCondition makes a tag test usable as a Java expression. EL becomes
// elExpr.parse; a raw test such as ajc.result.code == 'FA_SECRITY' is quoted
// the same way, because a multi-character single-quoted literal is not Java.
func (y *JSPVisitor) asJavaCondition(condition string) string {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return "false"
	}
	converted := y.appendElParseMethod(condition)
	if strings.Contains(converted, "elExpr.parse(") {
		return converted
	}
	return fmt.Sprintf("%s(\"%s\")", JSP_EL_PARSE_METHOD, javaLiteralBody(converted))
}

func javaLiteralBody(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\r", `\r`,
		"\n", `\n`,
	).Replace(s)
}

func (y *JSPVisitor) replaceElExprInText(text string) string {
	text = elBraceRe.ReplaceAllStringFunc(text, func(match string) string {
		y.EmitOutput(y.appendElParseMethod(match))
		return ""
	})
	text = elParseCallRe.ReplaceAllStringFunc(text, func(match string) string {
		y.EmitOutput(match)
		return ""
	})
	return text
}
