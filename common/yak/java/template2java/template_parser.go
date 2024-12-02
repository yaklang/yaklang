package template2java

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/java/freemarker"
	"github.com/yaklang/yaklang/common/yak/java/jsp"
	"github.com/yaklang/yaklang/common/yak/templateLanguage"
)

var TemplateVisitors = map[templateLanguage.TemplateTyp]templateLanguage.VisitorCreator{
	templateLanguage.TEMPLATE_JAVA_JSP:        jsp.Creator,
	templateLanguage.TEMPLATE_JAVA_FREEMARKER: freemarker.Creator,
}

func NewTemplateVisitor(editor *memedit.MemEditor, typ templateLanguage.TemplateTyp) (templateLanguage.TemplateVisitor, error) {
	if visitorCreator, ok := TemplateVisitors[typ]; ok {
		visitor, err := visitorCreator.Create(editor)
		if err != nil {
			return nil, err
		}
		return visitor, nil
	}
	return nil, utils.Errorf("not support template type: %v", typ)
}
