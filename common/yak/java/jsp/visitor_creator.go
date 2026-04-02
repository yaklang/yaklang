package jsp

import (
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	jspparser "github.com/yaklang/yaklang/common/yak/java/jsp/parser"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

var Creator tl.VisitorCreator = (*VisitorCreator)(nil)

var unclosedTagExpressionLinePattern = regexp.MustCompile(`(?m)(<[A-Za-z][^\n>]*<%=[^\n]*%>)([ \t]*)(\r?\n)`)
var doubledHtmlOpenPattern = regexp.MustCompile(`(^|[>[:space:]])<<([A-Za-z!/])`)
var trailingScriptletPattern = regexp.MustCompile(`(?s).*<%([\s\S]*?)%>\s*$`)
var leadingJSPBoilerplatePattern = regexp.MustCompile(`(?s)\A(?:\s|<%--.*?--%>|<%@.*?%>|<%.*?%>)+`)
var customTagPattern = regexp.MustCompile(`(?i)<\s*/?\s*[A-Za-z][\w.-]*:`)
var customTagPrefixPattern = regexp.MustCompile(`(?i)<\s*/?\s*([A-Za-z][\w.-]*):`)
var coreTagNamePattern = regexp.MustCompile(`(?i)<\s*/?\s*c:([A-Za-z][\w.-]*)`)
var transparentChoosePattern = regexp.MustCompile(`(?is)</?\s*c:choose\b[^>]*>`)
var directiveImportPattern = regexp.MustCompile(`(?i)\bimport\s*=\s*(?:"([^"]*)"|'([^']*)')`)
var directiveOnlyJSPPattern = regexp.MustCompile(`(?s)<%@.*?%>|<%--.*?--%>`)

const staticJSPFrontPlaceholder = "<yak:fragment></yak:fragment>"

type VisitorCreator struct {
}

func (b *VisitorCreator) Create(editor *memedit.MemEditor) (tl.TemplateVisitor, error) {
	visitor := NewJSPVisitor()
	visitor.Editor = editor
	src := editor.GetSourceCode()
	if canBypassFullJSPFront(src) {
		visitor.EmitPureText(src)
		return visitor, nil
	}
	if canBypassDirectiveOnlyJSP(src) {
		visitor.EmitPureText(src)
		return visitor, nil
	}
	if canUseLinearScriptletJSPFastPath(src) {
		return buildLinearScriptletJSPVisitor(visitor, src)
	}
	ast, err := Front(src)
	if err != nil {
		return nil, utils.Errorf("failed to get jsp.AST: %v", err)
	}
	visitor.VisitJspDocuments(ast)
	appendTrailingScriptletIfNeeded(visitor, src)
	return visitor, nil
}

// TODO: This parser is on the real template2java path for JSP. If profiling
// later shows JSP mixed static-content/template parsing has the same kind of
// ANTLR decision explosion as PHP, consider adding a JSP-specific token-source
// coalescing optimization here instead of treating it as test-only code.
func Front(code string) (jspparser.IJspDocumentsContext, error) {
	if canBypassFullJSPFront(code) || canBypassDirectiveOnlyJSP(code) || canUseLinearScriptletJSPFastPath(code) {
		code = staticJSPFrontPlaceholder
	} else {
		code = preprocessMalformedJSPHTML(code)
		code = preprocessJavascriptHrefQuotes(code)
		code = wrapStandaloneJSPFragment(code)
	}
	ast, err := antlr4util.ParseASTWithSLLFirst(
		code,
		jspparser.NewJSPLexer,
		jspparser.NewJSPParser,
		newMixedContentCoalescingTokenSource,
		nil,
		func(parser *jspparser.JSPParser) jspparser.IJspDocumentsContext {
			return parser.JspDocuments()
		},
	)
	if err != nil {
		return nil, utils.Errorf("parse AST FrontEnd error: %v", err)
	}
	return ast, nil
}

func preprocessMalformedJSPHTML(src string) string {
	if strings.Contains(src, "<%=") {
		src = unclosedTagExpressionLinePattern.ReplaceAllString(src, `$1>$2$3`)
	}
	if strings.Contains(src, "<<") {
		src = doubledHtmlOpenPattern.ReplaceAllString(src, `$1<$2`)
	}
	return src
}

func wrapStandaloneJSPFragment(src string) string {
	trimmed := leadingJSPBoilerplatePattern.ReplaceAllString(src, "")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return src
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") || strings.HasPrefix(lower, "<head") || strings.HasPrefix(lower, "<body") || strings.HasPrefix(lower, "<?xml") {
		return src
	}
	return "<yak:fragment>" + src + "</yak:fragment>"
}

func preprocessJavascriptHrefQuotes(src string) string {
	const marker = `="javascript:`
	for {
		start := strings.Index(src, marker)
		if start < 0 {
			return src
		}
		bodyStart := start + 2
		endRel := strings.Index(src[bodyStart:], `">`)
		if endRel < 0 {
			return src
		}
		end := bodyStart + endRel
		body := src[bodyStart:end]
		if strings.Count(body, `"`) > 0 {
			body = strings.ReplaceAll(body, `"`, `'`)
			src = src[:bodyStart] + body + src[end:]
		} else {
			src = src[:end] + src[end:]
		}
	}
}

func canBypassFullJSPFront(src string) bool {
	if src == "" {
		return true
	}

	for _, marker := range []string{"<%", "%>", "${", "#{", "<jsp:", "</jsp:"} {
		if strings.Contains(src, marker) {
			return false
		}
	}
	return !customTagPattern.MatchString(src)
}

func canBypassDirectiveOnlyJSP(src string) bool {
	if src == "" {
		return true
	}
	trimmed := directiveOnlyJSPPattern.ReplaceAllString(src, "")
	return !strings.Contains(trimmed, "<%")
}

func canUseLinearScriptletJSPFastPath(src string) bool {
	if src == "" || canBypassFullJSPFront(src) {
		return false
	}
	for _, marker := range []string{"<![CDATA["} {
		if strings.Contains(src, marker) {
			return false
		}
	}
	matches := customTagPrefixPattern.FindAllStringSubmatch(src, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if strings.ToLower(match[1]) == "c" && !hasOnlyTransparentCoreTags(src) {
			return false
		}
	}
	return strings.Contains(src, "<%")
}

func hasOnlyTransparentCoreTags(src string) bool {
	matches := coreTagNamePattern.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		return true
	}
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		if strings.ToLower(match[1]) != "choose" {
			return false
		}
	}
	return true
}

func buildLinearScriptletJSPVisitor(visitor *JSPVisitor, src string) (*JSPVisitor, error) {
	if visitor == nil {
		return nil, utils.Errorf("jsp visitor is nil")
	}
	src = transparentChoosePattern.ReplaceAllString(src, "")

	emitDirective := func(body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		lower := strings.ToLower(body)
		if strings.HasPrefix(lower, "page") {
			matches := directiveImportPattern.FindAllStringSubmatch(body, -1)
			for _, match := range matches {
				value := ""
				if len(match) > 1 {
					value = match[1]
				}
				if value == "" && len(match) > 2 {
					value = match[2]
				}
				for _, path := range strings.Split(value, ",") {
					path = strings.TrimSpace(path)
					if path != "" {
						visitor.EmitImport(path)
					}
				}
			}
		}
	}

	offset := 0
	for offset < len(src) {
		open := strings.Index(src[offset:], "<%")
		if open < 0 {
			if offset < len(src) {
				visitor.EmitPureText(src[offset:])
			}
			break
		}
		open += offset
		if open > offset {
			visitor.EmitPureText(src[offset:open])
		}

		if strings.HasPrefix(src[open:], "<%--") {
			end := strings.Index(src[open+4:], "--%>")
			if end < 0 {
				return nil, utils.Errorf("unclosed jsp comment")
			}
			offset = open + 4 + end + len("--%>")
			continue
		}

		end := strings.Index(src[open+2:], "%>")
		if end < 0 {
			return nil, utils.Errorf("unclosed jsp scriptlet")
		}
		end += open + 2

		body := src[open+2 : end]
		switch {
		case strings.HasPrefix(body, "@"):
			emitDirective(body[1:])
		case strings.HasPrefix(body, "="):
			expr := normalizeJSPEmbeddedJava(strings.TrimSpace(body[1:]))
			if expr != "" {
				visitor.EmitOutput(expr)
			}
		case strings.HasPrefix(body, "!"):
			code := strings.TrimSpace(body[1:])
			if code != "" {
				visitor.EmitDeclarationCode(code)
			}
		default:
			code := strings.TrimSpace(body)
			if code != "" {
				visitor.EmitPureCode(code)
			}
		}
		offset = end + len("%>")
	}
	return visitor, nil
}

func appendTrailingScriptletIfNeeded(visitor *JSPVisitor, src string) {
	if visitor == nil {
		return
	}
	balance := 0
	for _, ins := range visitor.GetInstructions() {
		if ins == nil || ins.Opcode != tl.OpPureCode {
			continue
		}
		balance += strings.Count(ins.Text, "{")
		balance -= strings.Count(ins.Text, "}")
	}
	if balance <= 0 {
		return
	}

	matches := trailingScriptletPattern.FindStringSubmatch(src)
	if len(matches) != 2 {
		return
	}
	code := strings.TrimSpace(matches[1])
	if code == "" {
		return
	}
	for _, ch := range code {
		if ch != '}' && ch != '{' && ch != ';' && ch != '\r' && ch != '\n' && ch != '\t' && ch != ' ' {
			return
		}
	}
	visitor.EmitPureCode(code)
}
