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
	java2Template map[int]memedit.RangeIf // java code line -> template token
}

func (g *GeneratedJavaInfo) GetRangeMap() map[int]memedit.RangeIf {
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
	var visitor tl.TemplateVisitor
	var err error
	switch typ {
	case JSP:
		visitor, err = NewTemplateVisitor(memedit.NewMemEditor(content), tl.TEMPLATE_JAVA_JSP)
		if err != nil {
			return nil, err
		}
	default:
		return nil, utils.Errorf("not support java template type: %v", typ)
	}
	t, err := CreateJavaTemplate(filePath)
	if err != nil {
		return nil, err
	}
	interpreter := tl.NewInterpreter(visitor.GetInstructions())
	interpreter.SetTemplate(t)
	interpreter.GenerateCode()
	info := &GeneratedJavaInfo{
		pkgName:       t.pkgName,
		className:     t.className,
		content:       interpreter.GetGeneratedCode(),
		java2Template: interpreter.GetRangeMap(),
	}
	return info, nil
}
