package template2java

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	tl "github.com/yaklang/yaklang/common/yak/templateLanguage"
)

type JavaTemplateType int

const (
	JSP JavaTemplateType = iota
	Freemarker
)

const (
	JAVA_TEMPLATE_SERVER_NAME = "_JavaTemplateService"
)

var _ tl.TemplateGeneratedInfo = (*GeneratedJavaInfo)(nil)

// GeneratedJavaInfo 生成的java代码信息
type GeneratedJavaInfo struct {
	pkgName       string
	className     string
	content       string
	java2Template map[int]*memedit.Range // java code line -> template token
}

func (g *GeneratedJavaInfo) GetRangeMap() map[int]*memedit.Range {
	if g == nil {
		return nil
	}
	return g.java2Template
}

func (g *GeneratedJavaInfo) GetTemplateServerName() string {
	return JAVA_TEMPLATE_SERVER_NAME
}

func (g *GeneratedJavaInfo) GetClassName() string {
	if g == nil {
		return ""
	}
	return g.className
}

func (g *GeneratedJavaInfo) GetPkgName() string {
	if g == nil {
		return ""
	}
	return g.pkgName
}

func (g *GeneratedJavaInfo) GetContent() string {
	if g == nil {
		return ""
	}
	return g.content
}

func ConvertTemplateToJava(typ JavaTemplateType, content, filePath string) (tl.TemplateGeneratedInfo, error) {
	if content == "" || filePath == "" {
		return nil, utils.Errorf("content or filePath is empty")
	}
	editor := memedit.NewMemEditorWithFileUrl(content, filePath)
	return ConvertTemplateToJavaWithEditor(typ, editor)
}

func ConvertTemplateToJavaWithEditor(typ JavaTemplateType, editor *memedit.MemEditor) (tl.TemplateGeneratedInfo, error) {
	var visitor tl.TemplateVisitor
	var err error
	switch typ {
	case JSP:
		visitor, err = NewTemplateVisitor(editor, tl.TEMPLATE_JAVA_JSP)
		if err != nil {
			return nil, err
		}
	case Freemarker:
		visitor, err = NewTemplateVisitor(editor, tl.TEMPLATE_JAVA_FREEMARKER)
		if err != nil {
			return nil, err
		}
	default:
		return nil, utils.Errorf("not support java template type: %v", typ)
	}
	filePath := editor.GetUrl()
	t, err := CreateJavaTemplate(filePath)
	if err != nil {
		return nil, err
	}
	interpreter := tl.NewInterpreter(visitor.GetInstructions())
	interpreter.SetTemplate(t)
	err = interpreter.GenerateCode()
	if err != nil {
		return nil, err
	}
	info := &GeneratedJavaInfo{
		pkgName:       t.pkgName,
		className:     t.className,
		content:       interpreter.GetGeneratedCode(),
		java2Template: interpreter.GetRangeMap(),
	}
	return info, nil
}
