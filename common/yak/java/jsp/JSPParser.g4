parser grammar JSPParser;

options { tokenVocab=JSPLexer; }

jspDocuments
    :jspStart* jspDocument+
    ;

jspDocument
    : xml
    | dtd
    | jspElements
    ;

jspStart
    :jspScript
    |WHITESPACES
    ;

jspElements
    :beforeContent=htmlMiscs (htmlElement|jspScript|jspExpression|style|javaScript) afterContent=htmlMiscs
    ;

htmlMiscs
    :htmlMisc *
    ;

jspScript
    :jspDirective
    |jspScriptlet
    ;

htmlElement
    :htmlBegin  (
        TAG_CLOSE (htmlContents CLOSE_TAG_BEGIN htmlTag TAG_CLOSE)?
        | TAG_SLASH_END)
    ;

htmlBegin
    :TAG_BEGIN htmlTag htmlAttribute*
    ;

htmlTag
    : htmlTagName (JSP_JSTL_COLON htmlTagName)?
    ;

// jsp页面指令
jspDirective
    : DIRECTIVE_BEGIN htmlTagName htmlAttribute*? TAG_WHITESPACE* DIRECTIVE_END
    ;

// html元素中间的内容
htmlContents
    : htmlChardata? (htmlContent htmlChardata?)*
    ;

htmlContent
    : elExpression
    | jspElements
    | xhtmlCDATA
    | htmlComment
    ;

// EL表达式
elExpression
    :  EL_EXPR_START EL_EXPR_CONTENT EL_EXPR_END
    ;

htmlAttribute
    : htmlAttributeName EQUALS htmlAttributeValue #EqualHTMLAttribute
    | htmlAttributeName #PureHTMLAttribute
    | jspExpression     #JSPExpressionAttribute
    ;

htmlAttributeName
    : TAG_IDENTIFIER
    ;

htmlAttributeValue
    :QUOTE? htmlAttributeValueElement* QUOTE?
    ;

htmlAttributeValueElement
    :ATTVAL_ATTRIBUTE
    |elExpression
    |jspExpression
    ;

htmlTagName
    : TAG_IDENTIFIER
    ;

// 静态内容
htmlChardata
    : JSP_STATIC_CONTENT_CHARS
    | WHITESPACES
    ;

htmlMisc
    : htmlComment
    | elExpression
    | jspScriptlet
    | WHITESPACES
    ;

// HTML注释
htmlComment
    : JSP_COMMENT
    | JSP_CONDITIONAL_COMMENT
    ;

xhtmlCDATA
    : CDATA
    ;

dtd
    : DTD dtdElementName (DTD_PUBLIC publicId*)? (DTD_SYSTEM systemId)?  TAG_END
    ;

dtdElementName
    : DTD_IDENTIFIER
    ;

publicId
    : DTD_QUOTED;

systemId
    : DTD_QUOTED;

xml: XML_DECLARATION name=htmlTagName atts+=htmlAttribute*? TAG_END
    ;

// JSP脚本
jspScriptlet
    : scriptletStart scriptletContent
    | jspExpression
    ;

// JSP表达式
jspExpression
    :ECHO_EXPRESSION_OPEN scriptletContent
    ;

scriptletStart
    :SCRIPTLET_OPEN
    |DECLARATION_BEGIN
    ;

scriptletContent
    : BLOB_CONTENT BLOB_CLOSE
    ;

javaScript
    : SCRIPT_OPEN (SCRIPT_BODY | SCRIPT_SHORT_BODY)
    ;

style
    : STYLE_OPEN (STYLE_BODY | STYLE_SHORT_BODY)
    ;