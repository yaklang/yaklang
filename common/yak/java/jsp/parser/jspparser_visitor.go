// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package jspparser // JSPParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by JSPParser.
type JSPParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by JSPParser#jspDocuments.
	VisitJspDocuments(ctx *JspDocumentsContext) interface{}

	// Visit a parse tree produced by JSPParser#jspDocument.
	VisitJspDocument(ctx *JspDocumentContext) interface{}

	// Visit a parse tree produced by JSPParser#jspStart.
	VisitJspStart(ctx *JspStartContext) interface{}

	// Visit a parse tree produced by JSPParser#jspElements.
	VisitJspElements(ctx *JspElementsContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlMiscs.
	VisitHtmlMiscs(ctx *HtmlMiscsContext) interface{}

	// Visit a parse tree produced by JSPParser#jspScript.
	VisitJspScript(ctx *JspScriptContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlElement.
	VisitHtmlElement(ctx *HtmlElementContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlBegin.
	VisitHtmlBegin(ctx *HtmlBeginContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlTag.
	VisitHtmlTag(ctx *HtmlTagContext) interface{}

	// Visit a parse tree produced by JSPParser#jspDirective.
	VisitJspDirective(ctx *JspDirectiveContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlContents.
	VisitHtmlContents(ctx *HtmlContentsContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlContent.
	VisitHtmlContent(ctx *HtmlContentContext) interface{}

	// Visit a parse tree produced by JSPParser#elExpression.
	VisitElExpression(ctx *ElExpressionContext) interface{}

	// Visit a parse tree produced by JSPParser#EqualHTMLAttribute.
	VisitEqualHTMLAttribute(ctx *EqualHTMLAttributeContext) interface{}

	// Visit a parse tree produced by JSPParser#PureHTMLAttribute.
	VisitPureHTMLAttribute(ctx *PureHTMLAttributeContext) interface{}

	// Visit a parse tree produced by JSPParser#JSPExpressionAttribute.
	VisitJSPExpressionAttribute(ctx *JSPExpressionAttributeContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeName.
	VisitHtmlAttributeName(ctx *HtmlAttributeNameContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeValue.
	VisitHtmlAttributeValue(ctx *HtmlAttributeValueContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeValueElement.
	VisitHtmlAttributeValueElement(ctx *HtmlAttributeValueElementContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlTagName.
	VisitHtmlTagName(ctx *HtmlTagNameContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlChardata.
	VisitHtmlChardata(ctx *HtmlChardataContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlMisc.
	VisitHtmlMisc(ctx *HtmlMiscContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlComment.
	VisitHtmlComment(ctx *HtmlCommentContext) interface{}

	// Visit a parse tree produced by JSPParser#xhtmlCDATA.
	VisitXhtmlCDATA(ctx *XhtmlCDATAContext) interface{}

	// Visit a parse tree produced by JSPParser#dtd.
	VisitDtd(ctx *DtdContext) interface{}

	// Visit a parse tree produced by JSPParser#dtdElementName.
	VisitDtdElementName(ctx *DtdElementNameContext) interface{}

	// Visit a parse tree produced by JSPParser#publicId.
	VisitPublicId(ctx *PublicIdContext) interface{}

	// Visit a parse tree produced by JSPParser#systemId.
	VisitSystemId(ctx *SystemIdContext) interface{}

	// Visit a parse tree produced by JSPParser#xml.
	VisitXml(ctx *XmlContext) interface{}

	// Visit a parse tree produced by JSPParser#jspScriptlet.
	VisitJspScriptlet(ctx *JspScriptletContext) interface{}

	// Visit a parse tree produced by JSPParser#jspExpression.
	VisitJspExpression(ctx *JspExpressionContext) interface{}

	// Visit a parse tree produced by JSPParser#scriptletStart.
	VisitScriptletStart(ctx *ScriptletStartContext) interface{}

	// Visit a parse tree produced by JSPParser#scriptletContent.
	VisitScriptletContent(ctx *ScriptletContentContext) interface{}

	// Visit a parse tree produced by JSPParser#javaScript.
	VisitJavaScript(ctx *JavaScriptContext) interface{}

	// Visit a parse tree produced by JSPParser#style.
	VisitStyle(ctx *StyleContext) interface{}
}
