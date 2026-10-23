package jsp

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync/atomic"

	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
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

// xmlDeclPattern / doctypePattern match markup that the JSP lexer only accepts
// at the start of a document. Coremail WAP pages emit <?xml ...?> and a WML
// doctype after tags, and the lexer then rejects the '=' inside the declaration.
var xmlDeclPattern = regexp.MustCompile(`(?is)<\?xml\b.*?\?>`)
var doctypePattern = regexp.MustCompile(`(?is)<!DOCTYPE\b[^>]*>`)

const staticJSPFrontPlaceholder = "<yak:fragment></yak:fragment>"

type VisitorCreator struct {
}

// CreateLinearJSPVisitor turns a page into Java without the JSP grammar:
// scriptlets stay code, everything else is text. Used when the structured
// translation does not parse as Java.
func CreateLinearJSPVisitor(editor *memedit.MemEditor) (tl.TemplateVisitor, error) {
	if editor == nil {
		return nil, utils.Errorf("jsp editor is nil")
	}
	visitor := NewJSPVisitor()
	visitor.Editor = editor
	return buildLinearScriptletJSPVisitor(visitor, editor.GetSourceCode())
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
		// A page the grammar cannot structure still has to become Java that
		// java2ssa can parse: keep scriptlets as code and the rest as text.
		return buildLinearScriptletJSPVisitor(visitor, src)
	}
	// A partially recovered tree can miss tokens the visitor assumes are there.
	// Treat that like a parse failure: the page still has to reach java2ssa, and
	// the grammar-free translator preserves every scriptlet.
	if err := visitJspDocumentsSafely(visitor, ast); err != nil {
		log.Warnf("[jsp] structured visit failed, using linear translation: %v", err)
		linear := NewJSPVisitor()
		linear.Editor = editor
		return buildLinearScriptletJSPVisitor(linear, src)
	}
	appendTrailingScriptletIfNeeded(visitor, src)
	return visitor, nil
}

func visitJspDocumentsSafely(visitor *JSPVisitor, ast jspparser.IJspDocumentsContext) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = utils.Errorf("jsp visit panic: %v", recovered)
		}
	}()
	visitor.VisitJspDocuments(ast)
	return nil
}

// TODO: This parser is on the real template2java path for JSP. If profiling
// later shows JSP mixed static-content/template parsing has the same kind of
// ANTLR decision explosion as PHP, consider adding a JSP-specific token-source
// coalescing optimization here instead of treating it as test-only code.
// maxJSPStructuredParseBytes bounds the pages that may use the LL diagnostic
// path. The default path never runs LL.
const maxJSPStructuredParseBytes = 64 * 1024

// jspFrontLLEnabled keeps the historical SLL-then-LL parse available for
// differential diagnostics. LL full-context recovery on the ambiguous
// HTML/JSP grammar costs tens of seconds and hundreds of megabytes per page,
// so it is off unless explicitly requested.
func jspFrontLLEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("YAK_JSP_FRONT_LL"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func Front(code string) (jspparser.IJspDocumentsContext, error) {
	if canBypassFullJSPFront(code) || canBypassDirectiveOnlyJSP(code) || canUseLinearScriptletJSPFastPath(code) {
		code = staticJSPFrontPlaceholder
	} else {
		code = preprocessMalformedJSPHTML(code)
		code = preprocessJavascriptHrefQuotes(code)
		code = wrapStandaloneJSPFragment(code)
	}
	ast, err := parseJSPDocuments(code)
	if err != nil {
		return nil, utils.Errorf("parse AST FrontEnd error: %v", err)
	}
	return ast, nil
}

func parseJSPDocuments(code string) (jspparser.IJspDocumentsContext, error) {
	defer resetJSPRuntimeCachesIfNeeded()
	return jspparser.WithJSPRuntimeLock(func() (jspparser.IJspDocumentsContext, error) {
		if jspFrontLLEnabled() && len(code) <= maxJSPStructuredParseBytes {
			return antlr4util.ParseASTWithSLLFirst(
				code,
				jspparser.NewJSPLexer,
				jspparser.NewJSPParser,
				newMixedContentCoalescingTokenSource,
				nil,
				func(parser *jspparser.JSPParser) jspparser.IJspDocumentsContext {
					return parser.JspDocuments()
				},
			)
		}
		// SLL with bail is bounded: it aborts at the first syntax error instead of
		// exploring full-context alternatives. A page the grammar cannot structure
		// then goes through the scriptlet-preserving translator in
		// VisitorCreator.Create, which is the same contract as an LL failure.
		ast, err := parseJSPDocumentsSLL(code)
		if err == nil {
			return ast, nil
		}
		// Bail stops at the first error; a page with one bad tag still has
		// structure worth keeping. Recovering SLL stays bounded (no full-context
		// exploration) and returns the partial tree the visitor can walk. Its
		// cost grows with the amount of malformed content, so it is limited to
		// pages that measured in the millisecond range; larger pages go straight
		// to the grammar-free translator.
		if len(code) <= maxJSPStructuredParseBytes {
			if recovered, rerr := parseJSPDocumentsRecovering(code); rerr == nil {
				return recovered, nil
			}
		}
		return nil, err
	})
}

// parseJSPDocumentsRecovering runs SLL with the default recovery strategy so a
// partially valid page still yields a tree. Errors are not reported: the tree is
// the product here, and VisitorCreator.Create falls back to the grammar-free
// translator when generation from it fails.
func parseJSPDocumentsRecovering(code string) (ast jspparser.IJspDocumentsContext, parseErr error) {
	lexer := jspparser.NewJSPLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokens.SetTokenSource(newMixedContentCoalescingTokenSource(lexer))
	parser := jspparser.NewJSPParser(tokens)
	defer antlr4util.DetachParserATNSimulatorCaches(parser)
	defer antlr4util.DetachLexerTokenSource(lexer)
	parser.RemoveErrorListeners()
	parser.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	defer func() {
		if recovered := recover(); recovered != nil {
			ast = nil
			parseErr = fmt.Errorf("recovering SLL parse failed: %v", recovered)
		}
	}()
	ast = parser.JspDocuments()
	if ast == nil {
		return nil, fmt.Errorf("recovering SLL parse produced no tree")
	}
	return ast, nil
}

// jspRuntimeParseCount triggers a periodic rebuild of the shared lexer/parser
// DFA tables. Prediction only adds states to them, so a scan of thousands of
// pages would otherwise keep every state it ever explored.
var jspRuntimeParseCount atomic.Int64

const jspRuntimeResetEvery = 256

func resetJSPRuntimeCachesIfNeeded() {
	if jspRuntimeParseCount.Add(1)%jspRuntimeResetEvery == 0 {
		jspparser.ResetJSPRuntimeCaches()
	}
}

// parseJSPDocumentsSLL avoids LL full-context recovery. Those pages use the
// grammar-free translator in VisitorCreator.Create when their syntax needs
// recovery. The caller holds the shared-DFA read lock.
func parseJSPDocumentsSLL(code string) (ast jspparser.IJspDocumentsContext, parseErr error) {
	var messages []string
	listener := antlr4util.NewErrorListener(antlr4util.SimpleSyntaxErrorHandler(func(msg string, start, _ *memedit.Position) {
		if len(messages) < 8 {
			messages = append(messages, fmt.Sprintf("line %d:%d %s", start.GetLine(), start.GetColumn(), msg))
		}
	}))
	lexer := jspparser.NewJSPLexer(antlr.NewInputStream(code))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(listener)
	tokens := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	tokens.SetTokenSource(newMixedContentCoalescingTokenSource(lexer))
	parser := jspparser.NewJSPParser(tokens)
	defer antlr4util.DetachParserATNSimulatorCaches(parser)
	defer antlr4util.DetachLexerTokenSource(lexer)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(listener)
	parser.GetInterpreter().SetPredictionMode(antlr.PredictionModeSLL)
	parser.SetErrorHandler(antlr4util.NewBailErrorStrategy())
	defer func() {
		if recovered := recover(); recovered != nil {
			switch recovered.(type) {
			case *antlr.ParseCancellationException:
				parseErr = fmt.Errorf("SLL parse cancelled: %s", strings.Join(messages, "; "))
			default:
				if s, ok := recovered.(string); ok && s == "implement me" {
					parseErr = fmt.Errorf("SLL parse cancelled: %s", strings.Join(messages, "; "))
					return
				}
				panic(recovered)
			}
		}
	}()
	ast = parser.JspDocuments()
	if len(messages) > 0 {
		return nil, fmt.Errorf("SLL syntax errors: %s", strings.Join(messages, "; "))
	}
	return ast, nil
}

// stripJSPScriptComments removes <%-- ... --%> before lexing. The comment
// rule only exists in the default lexer mode. Once a quoted "<" or a nested
// tag leaves the lexer in TAG mode, <%-- becomes a scriptlet and the comment
// body is emitted as Java ("-- text --"), which does not parse.
func stripJSPScriptComments(src string) string {
	if !strings.Contains(src, "<%--") {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		if strings.HasPrefix(src[i:], "<%--") {
			end := strings.Index(src[i+4:], "--%>")
			if end < 0 {
				b.WriteString(src[i:])
				break
			}
			b.WriteByte(' ')
			i += 4 + end + len("--%>")
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

// stripJSPIncludeDirectives removes <%@ include ... %> before lexing. The
// lexer skip rule is only active in the default mode. Inside a tag or a
// script the same directive becomes a scriptlet body "@ include file=..."
// and the generated Java does not parse. Includes are not expanded either
// way; this matches the lexer rule when that rule does match.
func stripJSPIncludeDirectives(src string) string {
	if !strings.Contains(src, "<%@") {
		return src
	}
	var b strings.Builder
	b.Grow(len(src))
	i := 0
	for i < len(src) {
		if strings.HasPrefix(src[i:], "<%@") {
			j := i + 3
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if strings.HasPrefix(src[j:], "include") {
				end := strings.Index(src[i:], "%>")
				if end < 0 {
					b.WriteString(src[i:])
					break
				}
				b.WriteByte(' ')
				i += end + 2
				continue
			}
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String()
}

func preprocessMalformedJSPHTML(src string) string {
	src = stripJSPScriptComments(src)
	src = stripJSPIncludeDirectives(src)
	if strings.Contains(src, "<?xml") || strings.Contains(src, "<?XML") {
		src = xmlDeclPattern.ReplaceAllString(src, "")
	}
	if strings.Contains(src, "<!DOCTYPE") || strings.Contains(src, "<!doctype") {
		src = doctypePattern.ReplaceAllString(src, "")
	}
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

// preprocessJavascriptHrefQuotes normalizes href="javascript:..."> bodies so nested
// double quotes do not break the JSP/HTML lexer. Each match spans from after `="` to
// the closing `">` of that attribute value.
func preprocessJavascriptHrefQuotes(src string) string {
	const marker = `="javascript:`
	// searchFrom advances per match; do not restart Index from 0 after edits/skips.
	searchFrom := 0
	for {
		relStart := strings.Index(src[searchFrom:], marker)
		if relStart < 0 {
			return src
		}
		start := searchFrom + relStart
		bodyStart := start + 2 // skip leading `="` of marker
		endRel := strings.Index(src[bodyStart:], `">`)
		if endRel < 0 {
			return src
		}
		end := bodyStart + endRel
		body := src[bodyStart:end]
		if strings.Count(body, `"`) > 0 {
			body = strings.ReplaceAll(body, `"`, `'`)
			src = src[:bodyStart] + body + src[end:]
			searchFrom = bodyStart + len(body)
		} else {
			// Nothing to rewrite; must still skip past `">` or the same match repeats forever.
			searchFrom = end + 2
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
	trimmed := strings.TrimSpace(directiveOnlyJSPPattern.ReplaceAllString(src, ""))
	if trimmed == "" {
		return true
	}
	return canBypassFullJSPFront(trimmed)
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
