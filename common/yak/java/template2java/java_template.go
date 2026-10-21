package template2java

import (
	"path/filepath"
	"strings"

	"golang.org/x/text/encoding"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/java/templateconst"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

const (
	JAVA_REQUEST_PATH          = templateconst.JAVA_REQUEST_PATH
	JAVA_UNESCAPE_OUTPUT_PRINT = templateconst.JAVA_UNESCAPE_OUTPUT_PRINT
)

var _ tl.TemplateRender = (*JavaTemplate)(nil)

type JavaTemplate struct {
	pkgName   string
	className string

	// sourceEncoding is the charset declared by the template header (nil = UTF-8).
	// Text written into the generated Java is decoded with it first, so a GBK JSP
	// page does not leak raw bytes into a Java string literal.
	sourceEncoding encoding.Encoding

	classDeclLine int
	builder       strings.Builder
	currentLine   int
}

func (t *JavaTemplate) WriteDeclaration(code string) {
	origin := t.builder.String()
	lines := strings.Split(origin, "\r\n")
	beforeClassDecl := strings.Join(lines[:t.classDeclLine], "\r\n")
	afterClassDecl := strings.Join(lines[t.classDeclLine:], "\r\n")
	t.builder.Reset()
	t.builder.WriteString(beforeClassDecl)
	t.builder.WriteString(code + "\r\n")
	t.builder.WriteString(afterClassDecl)
	t.recalcCurrentLine()
}

func (t *JavaTemplate) WriteImport(path string) {
	origin := t.builder.String()
	lines := strings.Split(origin, "\r\n")
	pkgDel := lines[0]
	backUp := strings.Join(lines[1:], "\r\n")
	t.builder.Reset()
	t.builder.WriteString(pkgDel + "\r\n")
	t.builder.WriteString("import " + path + ";\r\n")
	t.builder.WriteString(backUp)
	t.classDeclLine++
	t.recalcCurrentLine()
}

func (t *JavaTemplate) WritePureOut(expression string) {
	expression = tryUnquote(expression)
	t.append("\tout.print(" + expression + ");\r\n")
}

func (t *JavaTemplate) WritePureCode(code string) {
	t.append("\t" + code + "\r\n")
}

func (t *JavaTemplate) String() string {
	return t.builder.String()
}

func (t *JavaTemplate) WritePureText(texts string) {
	texts = tryUnquote(texts)
	// Decode with the template's declared charset, then emit a Java-legal literal:
	// Go's %q would produce \xNN escapes for legacy-encoded bytes, which Java cannot parse.
	texts = decodeTemplateText(texts, t.sourceEncoding)
	for _, text := range strings.Split(texts, "\n") {
		t.append("\tout.write(\"" + javaStringLiteral(text) + "\");\n")
	}
}

// SetSourceEncoding records the charset declared by the template header. It must be called
// before the template is generated; a nil encoding keeps UTF-8 passthrough.
func (t *JavaTemplate) SetSourceEncoding(enc encoding.Encoding) {
	if t == nil {
		return
	}
	t.sourceEncoding = enc
}

func (t *JavaTemplate) WriteGetAttribute(variable string) {
	variable = tryUnquote(variable)
	t.append("\t" + variable + " = request.getAttribute(\"" + variable + "\");\r\n")
}

func (t *JavaTemplate) WriteOutput(variable string) {
	variable = tryUnquote(variable)
	t.append("\tout." + JAVA_UNESCAPE_OUTPUT_PRINT + "(" + variable + ");\r\n")
}

func (t *JavaTemplate) WriteEscapeOutput(variable string) {
	variable = tryUnquote(variable)
	t.append("\tout.printWithEscape(" + variable + ");\r\n")
}

func (t *JavaTemplate) CurrentLine() int {
	if t == nil {
		return 0
	}
	if t.currentLine <= 0 {
		t.recalcCurrentLine()
	}
	return t.currentLine
}

func (t *JavaTemplate) Finish() {
	t.append("}}")
}

func CreateJavaTemplate(filePath string) (*JavaTemplate, error) {
	if filePath == "" {
		return nil, utils.Errorf("filePath is empty")
	}
	var builder strings.Builder
	t := &JavaTemplate{
		builder:     builder,
		currentLine: 1,
	}
	t.className = validateClassName(filepath.Base(filePath))
	t.pkgName = validatePackagePath(filepath.Dir(filePath))
	t.generateTemplate()
	return t, nil
}

func (t *JavaTemplate) generateTemplate() {
	if t.pkgName != "" {
		t.append("package " + t.pkgName + ";\r\n")
	}
	t.append("import " + JAVA_REQUEST_PATH + ".HttpServletRequest;\r\n")
	t.append("import " + JAVA_REQUEST_PATH + ".HttpServletResponse;\r\n")
	t.append("\n")
	t.append("public class " + t.className + " {\r\n")
	t.classDeclLine = len(strings.Split(t.builder.String(), "\r\n")) - 1
	t.append("public void _JavaTemplateService(" + "HttpServletRequest request, HttpServletResponse response" + ") {\r\n")
	t.append("\tout = response.getWriter(); \r\n")
}

func tryUnquote(text string) string {
	text = yakunquote.TryUnquote(text)
	text = strings.ReplaceAll(text, "\r", "")
	return text
}

func (t *JavaTemplate) append(s string) {
	t.builder.WriteString(s)
	t.currentLine += strings.Count(s, "\n")
}

func (t *JavaTemplate) recalcCurrentLine() {
	t.currentLine = strings.Count(t.builder.String(), "\n") + 1
}
