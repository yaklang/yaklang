parser grammar JSPParser;

options { tokenVocab=JSPLexer; }

jspDocuments
    : jspDocument+
    ;

jspDocument
    : jspStart* xml
    | jspStart* dtd
    | jspStart* jspElements+
    ;

jspStart
    :jspDirective
    |scriptlet
    |WHITESPACES
    ;

jspElements
    : htmlMisc* (jspElement|jspDirective| scriptlet) htmlMisc*
    ;

jspElement
    :htmlBegin  (TAG_CLOSE (htmlContents CLOSE_TAG_BEGIN htmlTag TAG_CLOSE)? | TAG_SLASH_END)
    ;

htmlBegin
    :TAG_BEGIN htmlTag htmlAttribute*
    ;

htmlTag
    : htmlTagName (JSP_JSTL_COLON htmlTagName)?
    ;

jspDirective
    : DIRECTIVE_BEGIN htmlTagName htmlAttribute*? TAG_WHITESPACE* DIRECTIVE_END
    ;

htmlContents
    : htmlChardata?  (htmlContent htmlChardata?)*
    ;

htmlContent
    : jspExpression
    | jspElements
    | xhtmlCDATA
    | htmlComment
    | scriptlet
    | jspDirective
    ;

jspExpression
    :  EL_EXPR
    ;

htmlAttribute
    //: jspElement
    : htmlAttributeName EQUALS htmlAttributeValue
    | htmlAttributeName
    | scriptlet
    ;

htmlAttributeName
    : TAG_IDENTIFIER
    ;

htmlAttributeValue
    : QUOTE jspElement QUOTE
    | QUOTE? htmlAttributeValueExpr  QUOTE?
    | QUOTE htmlAttributeValueConstant? QUOTE
    ;

htmlAttributeValueExpr
    : EL_EXPR
    ;

htmlAttributeValueConstant
    : ATTVAL_ATTRIBUTE
    ;

htmlTagName
    : TAG_IDENTIFIER
    ;

htmlChardata
    : JSP_STATIC_CONTENT_CHARS_MIXED
    | JSP_STATIC_CONTENT_CHARS
    | WHITESPACES
    | HTML_TEXT? EL_EXPR? HTML_TEXT?
    ;

htmlMisc
    : htmlComment
    | jspExpression
    | scriptlet
    | WHITESPACES
    ;

htmlComment
    : JSP_COMMENT_START htmlCommentText? JSP_COMMENT_END
    | JSP_CONDITIONAL_COMMENT_START htmlConditionalCommentText? JSP_CONDITIONAL_COMMENT_END
    ;

htmlCommentText
    : JSP_COMMENT_TEXT+?
    ;

htmlConditionalCommentText
    : JSP_CONDITIONAL_COMMENT
    ;
xhtmlCDATA
    : CDATA
    ;

dtd
    : DTD dtdElementName (DTD_PUBLIC publicId)? (DTD_SYSTEM systemId)?  TAG_END
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

scriptlet
    : SCRIPTLET_OPEN BLOB_CONTENT BLOB_CLOSE
    | ECHO_EXPRESSION_OPEN  BLOB_CONTENT BLOB_CLOSE
    | DECLARATION_BEGIN BLOB_CONTENT BLOB_CLOSE
    ;