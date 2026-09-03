// Code generated from ./ASPParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package aspparser // ASPParser
import "github.com/yaklang/antlr/v4"

type BaseASPParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseASPParserVisitor) VisitAspDocuments(ctx *AspDocumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspDocument(ctx *AspDocumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspScript(ctx *AspScriptContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspDirective(ctx *AspDirectiveContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspDeclaration(ctx *AspDeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspExpression(ctx *AspExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspDatabind(ctx *AspDatabindContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitAspScriptlet(ctx *AspScriptletContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitBlobContent(ctx *BlobContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitHtmlElement(ctx *HtmlElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitHtmlCloseElement(ctx *HtmlCloseElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitHtmlTag(ctx *HtmlTagContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitHtmlAttribute(ctx *HtmlAttributeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitHtmlContent(ctx *HtmlContentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitHtmlMisc(ctx *HtmlMiscContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitScript(ctx *ScriptContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseASPParserVisitor) VisitStyle(ctx *StyleContext) interface{} {
	return v.VisitChildren(ctx)
}
