package template2java

import (
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"path/filepath"
	"strings"
)

const (
	JAVA_REQUEST_PATH          = "javax.servlet.http"
	JAVA_UNESCAPE_OUTPUT_PRINT = "print"
)

var _ tl.TemplateRender = (*JavaTemplate)(nil)

type JavaTemplate struct {
	pkgName   string
	className string

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
	for _, text := range strings.Split(texts, "\n") {
		t.append(fmt.Sprintf("	out.write(%q);\n", text))
	}
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
