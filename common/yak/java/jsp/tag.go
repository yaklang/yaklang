package jsp

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
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

	// Directives that do not produce Java (taglib, attribute, variable, tag).
	JSP_DIRECTIVE_IGNORED
)

type TagInfo struct {
	typ   TagType
	attrs map[string]string
	funcs []func()
	// deferBody means c:forTokens saw <jsp:attribute>/<jsp:body>. The
	// loop is emitted after those children fill attrs, not before.
	deferBody bool
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

func tagUsesFragmentAttrs(t TagType) bool {
	switch t {
	case JSP_TAG_CORE_OUT, JSP_TAG_CORE_SET, JSP_TAG_CORE_IF, JSP_TAG_CORE_WHEN,
		JSP_TAG_CORE_FOREACH, JSP_TAG_CORE_FOR_TOKENS, JSP_TAG_CORE_IMPORT, JSP_TAG_CORE_URL:
		return true
	default:
		return false
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
	default:
		return JSP_DIRECTIVE_IGNORED
	}
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
	case JSP_DIRECTIVE_INCLUDE, JSP_DIRECTIVE_IGNORED:
		return
	case JSP_TAG_CORE_CHOOSE, JSP_TAG_CORE_WHEN, JSP_TAG_CORE_OTHERWISE:
		// A choose/when/otherwise without a body is not a statement. The paired
		// path owns the real switch. Emitting the tag text here only logged
		// "Unknown JSTL tag type" and wrote the markup into the Java.
		return
	case JSP_TAG_CORE_IF:
		y.emitCoreIf(nil)
	case JSP_TAG_CORE_FOREACH:
		y.emitCoreForEach(nil)
	case JSP_TAG_CORE_FOR_TOKENS:
		y.emitCoreForTokens(nil)
	case JSP_TAG_CORE_URL:
		y.emitCoreURL(nil)
	case JSP_TAG_CORE_PARAM:
		y.recordCoreParam()
	case JSP_TAG_CORE_IMPORT:
		y.emitCoreImport()
	case JSP_TAG_CORE_OUT:
		y.emitCoreOut(nil)
	case JSP_TAG_CORE_SET:
		y.emitCoreSet()
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
		y.emitCoreIf(visitContent)
	case JSP_TAG_CORE_SET:
		y.emitCoreSet()
		visitContent()
	case JSP_TAG_CORE_FOR_TOKENS:
		y.emitCoreForTokens(visitContent)
	case JSP_TAG_CORE_IMPORT:
		y.emitCoreImport()
		visitContent()
	case JSP_TAG_CORE_URL:
		y.urlParams = append(y.urlParams, nil)
		visitContent()
		params := y.urlParams[len(y.urlParams)-1]
		y.urlParams = y.urlParams[:len(y.urlParams)-1]
		y.emitCoreURL(params)
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
		y.emitCoreForEach(visitContent)
	case JSP_TAG_CORE_OUT:
		// <c:out value="${x}"></c:out> is paired, not self-closing. The
		// single-tag path already handled the value; this one must too.
		y.emitCoreOut(visitContent)
	case JSP_TAG_CORE_PARAM:
		y.recordCoreParam()
		if visitContent != nil {
			visitContent()
		}
	default:
		log.Errorf("Unknown JSTL tag type: %v", tagInfo.typ)
		y.EmitAttrFunc()
	}
}

func (y *JSPVisitor) emitCoreOut(body func()) {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	variable, ok := tag.attrs["value"]
	if !ok {
		// JSTL uses the element body when value is absent.
		if body != nil {
			body()
			return
		}
		log.Errorf("JSTL out tag must have value attribute")
		return
	}
	var noEscape bool
	if v, ok := tag.attrs["escapeXml"]; ok {
		noEscape = v == "false"
	}
	// ${name} -  is an expression plus literal text. Passing it through raw
	// makes printWithEscape(elExpr.parse("name") - ), which is not Java.
	expr := javaValueArg(variable)
	if noEscape {
		y.EmitOutput(expr)
		return
	}
	y.EmitEscapeOutput(expr)
}

func (y *JSPVisitor) emitCoreIf(body func()) {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	condition, ok := tag.attrs["test"]
	if !ok {
		log.Errorf("JSTL if tag must have test attribute")
		return
	}
	cond := y.asJavaCondition(condition)
	if variable, ok := tag.attrs["var"]; ok {
		y.EmitPureCode("request.setAttribute(\"" + javaLiteralBody(variable) + "\", " + cond + ");")
	}
	if body == nil {
		return
	}
	y.EmitPureCode("if (" + cond + ") {")
	body()
	y.EmitPureCode("}")
}

func (y *JSPVisitor) emitCoreSet() {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	value := tag.attrs["value"]
	if variable, ok := tag.attrs["var"]; ok {
		y.EmitPureCode("request.setAttribute(\"" + javaLiteralBody(variable) + "\", " + javaValueArg(value) + ");")
		return
	}
	target, hasTarget := tag.attrs["target"]
	property, hasProperty := tag.attrs["property"]
	if !hasTarget || !hasProperty {
		log.Errorf("JSTL set tag must have var or target/property")
		return
	}
	valueExpr := javaValueArg(value)
	if strings.Contains(target, "pageContext.request") && property == "characterEncoding" {
		y.EmitPureCode("request.setCharacterEncoding(" + valueExpr + ");")
		return
	}
	y.EmitPureCode("if (" + target + " instanceof java.util.Map) { ((java.util.Map)" + target + ").put(\"" + javaLiteralBody(property) + "\", " + valueExpr + "); }")
}

func (y *JSPVisitor) emitCoreForEach(body func()) {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	variable, hasVar := tag.attrs["var"]
	if items, ok := tag.attrs["items"]; ok {
		if !hasVar {
			log.Errorf("JSTL foreach tag must have var attribute")
			return
		}
		// items may be several EL calls plus literal text. javaValueArg joins
		// those into one expression; a raw splice does not parse.
		y.EmitPureCode("for (Object " + variable + " : " + javaValueArg(items) + ") {")
		if body != nil {
			body()
		}
		y.EmitPureCode("}")
		return
	}
	// JSTL also allows the counting form begin/end/step without items. Dropping
	// it removes the body from the generated Java, so every statement inside a
	// page's numeric loop went unanalyzed.
	end, hasEnd := tag.attrs["end"]
	if !hasEnd {
		log.Errorf("JSTL foreach tag must have items or begin/end attributes")
		return
	}
	begin := tag.attrs["begin"]
	if strings.TrimSpace(begin) == "" {
		begin = "0"
	}
	step := tag.attrs["step"]
	if strings.TrimSpace(step) == "" {
		step = "1"
	}
	if !hasVar {
		variable = "_jspForEachIndex"
	}
	y.EmitPureCode(fmt.Sprintf("for (int %s = %s; %s <= %s; %s += %s) {",
		variable, jspIntExpr(begin), variable, jspIntExpr(end), variable, jspIntExpr(step)))
	if body != nil {
		body()
	}
	y.EmitPureCode("}")
}

// jspIntExpr turns a JSTL numeric attribute into a Java int expression.
// Literals stay literals so the SSA sees a constant bound; EL values go through
// Number.intValue, which is valid Java for the Object that elExpr.parse returns.
func jspIntExpr(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0"
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value
	}
	expr := javaValueArg(value)
	if elParseCallRe.FindString(expr) == expr {
		return "((Number)" + expr + ").intValue()"
	}
	return "java.lang.Integer.parseInt(String.valueOf(" + expr + "))"
}

func (y *JSPVisitor) emitCoreForTokens(body func()) {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	variable, ok := tag.attrs["var"]
	if !ok {
		log.Errorf("JSTL forTokens tag must have var attribute")
		return
	}
	items, ok := tag.attrs["items"]
	if !ok {
		log.Errorf("JSTL forTokens tag must have items attribute")
		return
	}
	delims := tag.attrs["delims"]
	if delims == "" {
		delims = ","
	}
	// A literal such as 0500|0930 is not a Java expression: 0930 is not an
	// octal token, and the '|' is bitwise or. Quote it. EL stays an expression.
	y.EmitPureCode("for (String " + variable + " : String.valueOf(" + javaValueArg(items) + ").split(\"" + javaLiteralBody(delims) + "\")) {")
	if body != nil {
		body()
	}
	y.EmitPureCode("}")
}

func (y *JSPVisitor) emitCoreImport() {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	url, ok := tag.attrs["url"]
	if !ok {
		log.Errorf("JSTL import tag must have url attribute")
		return
	}
	expr := javaValueArg(url)
	if variable, ok := tag.attrs["var"]; ok {
		y.EmitPureCode("request.setAttribute(\"" + javaLiteralBody(variable) + "\", " + expr + ");")
		return
	}
	y.EmitPureCode("out.print(String.valueOf(" + expr + "));")
}

func (y *JSPVisitor) recordCoreParam() {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	name := tag.attrs["name"]
	value := javaValueArg(tag.attrs["value"])
	piece := "\"" + javaLiteralBody(name) + "=\" + " + value
	if len(y.urlParams) == 0 {
		y.EmitPureCode("request.getParameterMap().put(\"" + javaLiteralBody(name) + "\", new String[]{" + value + "});")
		return
	}
	last := len(y.urlParams) - 1
	y.urlParams[last] = append(y.urlParams[last], piece)
}

func (y *JSPVisitor) emitCoreURL(params []string) {
	tag := y.PeekTagInfo()
	if tag == nil {
		return
	}
	value, ok := tag.attrs["value"]
	if !ok {
		log.Errorf("JSTL url tag must have value attribute")
		return
	}
	expr := javaValueArg(value)
	if context, ok := tag.attrs["context"]; ok && context != "" {
		expr = javaValueArg(context) + " + " + expr
	}
	for i, param := range params {
		sep := "?"
		if i > 0 {
			sep = "&"
		}
		expr += " + \"" + sep + "\" + " + param
	}
	if variable, ok := tag.attrs["var"]; ok {
		y.EmitPureCode("request.setAttribute(\"" + javaLiteralBody(variable) + "\", " + expr + ");")
		return
	}
	y.EmitPureCode("out.print(String.valueOf(" + expr + "));")
}

var (
	// elParseCallRe matches a finished elExpr.parse("...") call, including
	// escaped quotes. The previous ([^)]*) form stopped at the first ')' inside
	// the expression, so fn:escapeXml(...) became an unclosed Java string
	// ("token recognition error at: '"fn:escapeXml(...)').
	elParseCallRe = regexp.MustCompile(`elExpr\.parse\("(?:\\.|[^"\\])*"\)`)
)

func (y *JSPVisitor) appendElParseMethod(expr string) string {
	expr = strings.TrimSpace(expr)
	expr = yakunquote.TryUnquote(expr)
	return replaceDollarEL(expr)
}

// replaceDollarEL finds ${...} with nested braces. ${param.idx eq '!{index}'}
// contains a '}' that a non-greedy regex treats as the end of the EL.
func replaceDollarEL(expr string) string {
	var b strings.Builder
	i := 0
	for i < len(expr) {
		if i+1 < len(expr) && expr[i] == '$' && expr[i+1] == '{' {
			end := matchingBrace(expr, i+1)
			if end < 0 {
				b.WriteByte(expr[i])
				i++
				continue
			}
			inner := expr[i+2 : end]
			b.WriteString(fmt.Sprintf("%s(\"%s\")", JSP_EL_PARSE_METHOD, javaLiteralBody(inner)))
			i = end + 1
			continue
		}
		b.WriteByte(expr[i])
		i++
	}
	return b.String()
}

func matchingBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
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
	if !strings.Contains(value, "elExpr.parse(") {
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			return value
		}
		return `"` + javaLiteralBody(value) + `"`
	}
	// ${a}${b} and greet_${name} are stored as adjacent elExpr.parse calls
	// with the literal text left in between. Java needs '+' between them.
	// Returning on a prefix match left the tail (`elExpr.parse("a"),`) raw.
	var parts []string
	rest := value
	for {
		loc := elParseCallRe.FindStringIndex(rest)
		if loc == nil {
			if rest != "" {
				parts = append(parts, `"`+javaLiteralBody(rest)+`"`)
			}
			break
		}
		if loc[0] > 0 {
			parts = append(parts, `"`+javaLiteralBody(rest[:loc[0]])+`"`)
		}
		parts = append(parts, rest[loc[0]:loc[1]])
		rest = rest[loc[1]:]
	}
	switch len(parts) {
	case 0:
		return `""`
	case 1:
		return parts[0]
	default:
		return strings.Join(parts, " + ")
	}
}

// asJavaCondition makes a tag test usable as a Java expression. EL becomes
// elExpr.parse; a raw test such as ajc.result.code == 'FA_SECRITY' is quoted
// the same way, because a multi-character single-quoted literal is not Java.
func (y *JSPVisitor) asJavaCondition(condition string) string {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return "false"
	}
	// <%= UserUtil.getKMSSUser().isAdmin() %> is already a Java call. EL
	// functions use a colon (fn:escapeXml) and must stay quoted.
	if strings.Contains(condition, "(") && !strings.Contains(condition, ":") && !strings.Contains(condition, "${") && !strings.Contains(condition, "elExpr.parse(") {
		return condition
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
	var b strings.Builder
	i := 0
	for i < len(text) {
		if i+1 < len(text) && text[i] == '$' && text[i+1] == '{' {
			end := matchingBrace(text, i+1)
			if end < 0 {
				b.WriteByte(text[i])
				i++
				continue
			}
			y.EmitOutput(y.appendElParseMethod(text[i : end+1]))
			i = end + 1
			continue
		}
		b.WriteByte(text[i])
		i++
	}
	text = b.String()
	text = elParseCallRe.ReplaceAllStringFunc(text, func(match string) string {
		y.EmitOutput(match)
		return ""
	})
	return text
}
