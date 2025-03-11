package template2java

import (
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
}

func (t *JavaTemplate) WritePureOut(expression string) {
	expression = tryUnquote(expression)
	t.builder.WriteString("\tout.print(" + expression + ");\r\n")
}

func (t *JavaTemplate) WritePureCode(code string) {
	t.builder.WriteString("\t" + code + "\r\n")
}

func (t *JavaTemplate) String() string {
	return t.builder.String()
}

func (t *JavaTemplate) WritePureText(texts string) {
	texts = tryUnquote(texts)
	for _, text := range strings.Split(texts, "\n") {
		t.builder.WriteString("\tout.write(\"" + text + "\");\r\n")
	}
}

func (t *JavaTemplate) WriteGetAttribute(variable string) {
	variable = tryUnquote(variable)
	t.builder.WriteString("\t" + variable + " = request.getAttribute(\"" + variable + "\");\r\n")
}

func (t *JavaTemplate) WriteOutput(variable string) {
	variable = tryUnquote(variable)
	t.builder.WriteString("\tout." + JAVA_UNESCAPE_OUTPUT_PRINT + "(" + variable + ");\r\n")
}

func (t *JavaTemplate) WriteEscapeOutput(variable string) {
	variable = tryUnquote(variable)
	t.builder.WriteString("\tout.printWithEscape(" + variable + ");\r\n")
}

func (t *JavaTemplate) Finish() {
	t.builder.WriteString("}}")
}

func CreateJavaTemplate(filePath string) (*JavaTemplate, error) {
	if filePath == "" {
		return nil, utils.Errorf("filePath is empty")
	}
	var builder strings.Builder
	t := &JavaTemplate{
		builder: builder,
	}
	t.className = validateClassName(filepath.Base(filePath))
	t.pkgName = validatePackagePath(filepath.Dir(filePath))
	t.generateTemplate()
	return t, nil
}

func (t *JavaTemplate) generateTemplate() {
	if t.pkgName != "" {
		t.builder.WriteString("package " + t.pkgName + ";\r\n")
	}
	t.builder.WriteString("import " + JAVA_REQUEST_PATH + ".HttpServletRequest;\r\n")
	t.builder.WriteString("import " + JAVA_REQUEST_PATH + ".HttpServletResponse;\r\n")
	t.builder.WriteString("\n")
	t.builder.WriteString("public class " + t.className + " {\r\n")
	t.classDeclLine = len(strings.Split(t.builder.String(), "\r\n")) - 1
	t.builder.WriteString("public void _JavaTemplateService(" + "HttpServletRequest request, HttpServletResponse response" + ") {\r\n")
	t.builder.WriteString("\tout = response.getWriter(); \r\n")
}

func tryUnquote(text string) string {
	text = yakunquote.TryUnquote(text)
	text = strings.ReplaceAll(text, "\r", "")
	return text
}
