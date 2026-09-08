// Code generated from ./ASPParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package aspparser // ASPParser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by ASPParser.
type ASPParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by ASPParser#aspDocuments.
	VisitAspDocuments(ctx *AspDocumentsContext) interface{}

	// Visit a parse tree produced by ASPParser#aspDocument.
	VisitAspDocument(ctx *AspDocumentContext) interface{}

	// Visit a parse tree produced by ASPParser#aspScript.
	VisitAspScript(ctx *AspScriptContext) interface{}

	// Visit a parse tree produced by ASPParser#aspDirective.
	VisitAspDirective(ctx *AspDirectiveContext) interface{}

	// Visit a parse tree produced by ASPParser#aspDeclaration.
	VisitAspDeclaration(ctx *AspDeclarationContext) interface{}

	// Visit a parse tree produced by ASPParser#aspExpression.
	VisitAspExpression(ctx *AspExpressionContext) interface{}

	// Visit a parse tree produced by ASPParser#aspDatabind.
	VisitAspDatabind(ctx *AspDatabindContext) interface{}

	// Visit a parse tree produced by ASPParser#aspScriptlet.
	VisitAspScriptlet(ctx *AspScriptletContext) interface{}

	// Visit a parse tree produced by ASPParser#blobContent.
	VisitBlobContent(ctx *BlobContentContext) interface{}

	// Visit a parse tree produced by ASPParser#htmlElement.
	VisitHtmlElement(ctx *HtmlElementContext) interface{}

	// Visit a parse tree produced by ASPParser#htmlCloseElement.
	VisitHtmlCloseElement(ctx *HtmlCloseElementContext) interface{}

	// Visit a parse tree produced by ASPParser#htmlTag.
	VisitHtmlTag(ctx *HtmlTagContext) interface{}

	// Visit a parse tree produced by ASPParser#htmlAttribute.
	VisitHtmlAttribute(ctx *HtmlAttributeContext) interface{}

	// Visit a parse tree produced by ASPParser#htmlContent.
	VisitHtmlContent(ctx *HtmlContentContext) interface{}

	// Visit a parse tree produced by ASPParser#htmlMisc.
	VisitHtmlMisc(ctx *HtmlMiscContext) interface{}

	// Visit a parse tree produced by ASPParser#script.
	VisitScript(ctx *ScriptContext) interface{}

	// Visit a parse tree produced by ASPParser#style.
	VisitStyle(ctx *StyleContext) interface{}
}
