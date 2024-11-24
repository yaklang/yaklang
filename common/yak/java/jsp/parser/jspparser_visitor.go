// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package jspparser // JSPParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by JSPParser.
type JSPParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by JSPParser#jspDocument.
	VisitJspDocument(ctx *JspDocumentContext) interface{}

	// Visit a parse tree produced by JSPParser#jspStart.
	VisitJspStart(ctx *JspStartContext) interface{}

	// Visit a parse tree produced by JSPParser#jspElements.
	VisitJspElements(ctx *JspElementsContext) interface{}

	// Visit a parse tree produced by JSPParser#JspElementWithTagAndContent.
	VisitJspElementWithTagAndContent(ctx *JspElementWithTagAndContentContext) interface{}

	// Visit a parse tree produced by JSPParser#JspElementWithSelfClosingTag.
	VisitJspElementWithSelfClosingTag(ctx *JspElementWithSelfClosingTagContext) interface{}

	// Visit a parse tree produced by JSPParser#JspElementWithOpenTagOnly.
	VisitJspElementWithOpenTagOnly(ctx *JspElementWithOpenTagOnlyContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlTag.
	VisitHtmlTag(ctx *HtmlTagContext) interface{}

	// Visit a parse tree produced by JSPParser#jspDirective.
	VisitJspDirective(ctx *JspDirectiveContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlContents.
	VisitHtmlContents(ctx *HtmlContentsContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlContent.
	VisitHtmlContent(ctx *HtmlContentContext) interface{}

	// Visit a parse tree produced by JSPParser#jspExpression.
	VisitJspExpression(ctx *JspExpressionContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttribute.
	VisitHtmlAttribute(ctx *HtmlAttributeContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeName.
	VisitHtmlAttributeName(ctx *HtmlAttributeNameContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeValue.
	VisitHtmlAttributeValue(ctx *HtmlAttributeValueContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeValueExpr.
	VisitHtmlAttributeValueExpr(ctx *HtmlAttributeValueExprContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlAttributeValueConstant.
	VisitHtmlAttributeValueConstant(ctx *HtmlAttributeValueConstantContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlTagName.
	VisitHtmlTagName(ctx *HtmlTagNameContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlChardata.
	VisitHtmlChardata(ctx *HtmlChardataContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlMisc.
	VisitHtmlMisc(ctx *HtmlMiscContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlComment.
	VisitHtmlComment(ctx *HtmlCommentContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlCommentText.
	VisitHtmlCommentText(ctx *HtmlCommentTextContext) interface{}

	// Visit a parse tree produced by JSPParser#htmlConditionalCommentText.
	VisitHtmlConditionalCommentText(ctx *HtmlConditionalCommentTextContext) interface{}

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

	// Visit a parse tree produced by JSPParser#scriptlet.
	VisitScriptlet(ctx *ScriptletContext) interface{}
}
