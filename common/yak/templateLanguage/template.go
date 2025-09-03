package templateLanguage

import "github.com/yaklang/yaklang/common/utils/memedit"

type TemplateTyp int

const (
	TEMPLATE_JAVA_JSP TemplateTyp = iota
	TEMPLATE_JAVA_FREEMARKER
	TEMPLATE_JAVA_THYMELEAF
)

// TemplateRender is the interface for the template render
type TemplateRender interface {
	WritePureText(text string)         // Just writing text, usually used to write HTML content in templates
	WriteOutput(variable string)       // Write output, usually used to write variables in templates
	WriteEscapeOutput(variable string) // Write variables to the template output, but they will be HTML escaped
	WritePureCode(code string)         // Write pure code, usually used to write code in templates
	WriteImport(path string)           // Write import dependency statement
	WriteDeclaration(code string)
	String() string
	Finish()
}

// TemplateGeneratedInfo is the interface for the generated template code
type TemplateGeneratedInfo interface {
	GetContent() string
	GetClassName() string
	GetPkgName() string
	GetTemplateServerName() string       // run template server's method name
	GetRangeMap() map[int]*memedit.Range // generated code offset -> template language range
}
