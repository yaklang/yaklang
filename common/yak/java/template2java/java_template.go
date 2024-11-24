package template2java

import (
	"github.com/yaklang/yaklang/common/utils"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
	"path/filepath"
	"strings"
)

// TEMPLATE_JAVA_REQUEST_PATH 作为flag,方便进行SyntaxFlow审计
const TEMPLATE_JAVA_REQUEST_PATH = "syntaxflow.template.java"

var _ tl.TemplateRender = (*JavaTemplate)(nil)

type JavaTemplate struct {
	pkgName   string
	className string

	builder strings.Builder
}

func (t *JavaTemplate) String() string {
	return t.builder.String()
}

func (t *JavaTemplate) WritePureText(text string) {
	t.builder.WriteString("\tout.write(\"" + text + "\");\r\n")
}

func (t *JavaTemplate) WriteGetAttribute(variable string) {
	t.builder.WriteString("\t" + variable + " = request.getAttribute(\"" + variable + "\");\r\n")
}

func (t *JavaTemplate) WriteOutput(variable string) {
	t.WriteGetAttribute(variable)
	t.builder.WriteString("\tout.print(" + variable + ");\r\n")
}

func (t *JavaTemplate) WriteEscapeOutput(variable string) {
	t.WriteGetAttribute(variable)
	t.builder.WriteString("\tout.print(escapeHtml(" + variable + "));\r\n")
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
	t.builder.WriteString("import " + TEMPLATE_JAVA_REQUEST_PATH + ".HttpServletRequest;\r\n")
	t.builder.WriteString("import " + TEMPLATE_JAVA_REQUEST_PATH + ".HttpServletResponse;\r\n")
	t.builder.WriteString("\n")
	t.builder.WriteString("public class " + t.className + " {\r\n")
	t.builder.WriteString("public void _JavaTemplateService(" + "HttpServletRequest request, HttpServletResponse response" + ") {\r\n")
	t.builder.WriteString("\tout = request.getOut(); \r\n")
}
