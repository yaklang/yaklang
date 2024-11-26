lexer grammar  JSPLexer;

JSP_COMMENT_START
    : JSP_COMMENT_START_TAG -> pushMode(IN_JSP_COMMENT)
    ;

JSP_COMMENT_END
    : JSP_COMMENT_END_TAG
    ;

JSP_COMMENT_START_TAG
    :'<!--'
    ;

WHITESPACES
    :  (' ' | '\t' | '\r'? '\n')+
    ;

HTML_TEXT: ~('<'|'$')+;

JSP_COMMENT_END_TAG
    : '-->'
    ;

JSP_CONDITIONAL_COMMENT_START
    : JSP_CONDITIONAL_COMMENT_START_TAG -> pushMode(IN_CONDITIONAL_COMMENT)
    ;

JSP_CONDITIONAL_COMMENT_START_TAG
    : '<!['
    ;

JSP_CONDITIONAL_COMMENT_END_TAG
    : ']>'
    ;

XML_DECLARATION
    : '<?xml' -> pushMode(TAG)
    ;

CDATA
    : '<![CDATA[' .*? ']]>'
    ;

DTD
    : DTD_START -> pushMode(IN_DTD)
    ;

DTD_START
    : '<!DOCTYPE'
    ;

WHITESPACE_SKIP
    : WHITESPACE -> skip
    ;

CLOSE_TAG_BEGIN
    : END_ELEMENT_OPEN_TAG ->pushMode(TAG)
    ;

TAG_BEGIN
    : BEGIN_ELEMENT_OPEN_TAG -> pushMode(TAG)
    ;

DIRECTIVE_BEGIN
    : ('<%@'|'<jsp:directive') -> pushMode(TAG)
    ;

DECLARATION_BEGIN
    : ('<%!'|'<jsp:declaration') -> pushMode(JSP_BLOB)
    ;

ECHO_EXPRESSION_OPEN
    : ('<%='|'<jsp:expression') -> pushMode(JSP_BLOB)
    ;

SCRIPTLET_OPEN
    : ('<%'|'jsp:scriptlet') -> pushMode(JSP_BLOB)
    ;

EXPRESSION_OPEN
    : ('${'|'#{') ->pushMode(IN_JSP_EXPRESSION)
    ;




DOUBLE_QUOTE
    : '"'
    ;
SINGLE_QUOTE
    : '\''
    ;

QUOTE
    : SINGLE_QUOTE|DOUBLE_QUOTE
    ;

TAG_END
   : CLOSE_TAG
   ;

EQUALS
    : EQUALS_CHAR
    ;

fragment CLOSE_TAG
    : '>'
    ;


fragment IDENTIFIER
    : TAG_NameStartChar TAG_NameChar*
    ;

fragment EL_EXPR_BODY
    : ~[\\}]+
    ;

fragment EL_EXPR_OPEN
    : ('${'|'#{')
    ;

fragment EL_EXPR_CLOSE
    : '}'
    ;

fragment EL_EXPR_TXT
    : EL_EXPR_OPEN EL_EXPR_BODY EL_EXPR_CLOSE
    ;

fragment BEGIN_ELEMENT_OPEN_TAG
    : '<'
    ;

fragment END_ELEMENT_OPEN_TAG
    : '</'
    ;

fragment EMPTY_ELEMENT_CLOSE
    : '/>'
    ;

fragment ESCAPED_DOLLAR
    : '\\$'
    ;

TOP_EL_EXPR
    : EL_EXPR_TXT -> type(EL_EXPR)
    ;


JSP_STATIC_CONTENT_CHARS_MIXED
    :
        JSP_STATIC_CONTENT_CHAR+? { (this.LA(1) == '\$') && (this.LA(2) == '{')}? -> pushMode(IN_JSP_EXPRESSION)
    ;

JSP_STATIC_CONTENT_CHARS
    :
        JSP_STATIC_CONTENT_CHAR+? {(this.LA(1) == '<') }?
    ;

JSP_STATIC_CONTENT_CHAR
    : ~[<\\$]+
    | ESCAPED_DOLLAR
    ;

JSP_END
    : '%>' ->popMode
    ;

mode IN_CONDITIONAL_COMMENT;

JSP_CONDITIONAL_COMMENT_END
    : JSP_CONDITIONAL_COMMENT_END_TAG -> popMode
    ;

JSP_CONDITIONAL_COMMENT
    : ~[\]]+
    | ']' ~[>]
    ;

mode IN_JSP_COMMENT;

IN_COMMENT_JSP_COMMENT_END_TAG
    : JSP_COMMENT_END_TAG -> type(JSP_COMMENT_END),popMode
    ;

JSP_COMMENT_TEXT
    : ~[-]+
    | '-' ~[-]
    | '--' ~[>]
    ;

mode IN_DTD;
//<!DOCTYPE doctypename PUBLIC "publicId" "systemId">

DTD_PUBLIC
     : 'PUBLIC'
     ;

DTD_SYSTEM
     : 'SYSTEM'
     ;

DTD_WHITESPACE_SKIP
    :WHITESPACE+ -> skip
    ;

DTD_QUOTED
    : DOUBLE_QUOTE DOUBLE_QUOTE_STRING_CONTENT* DOUBLE_QUOTE
    ;

DTD_IDENTIFIER
    : IDENTIFIER
    ;

DTD_TAG_CLOSE
    : TAG_CLOSE -> type(TAG_END),popMode
    ;

mode JSP_BLOB;

BLOB_CLOSE
   : JSP_END -> popMode
   ;

BLOB_CONTENT
    : BLOB_CONTENT_FRAGMENT+
    ;

fragment BLOB_CONTENT_FRAGMENT
    : ~('<' | '%' | '>' )
    ;

mode IN_JSP_EXPRESSION;

JSPEXPR_CONTENT
    : EL_EXPR -> type(EL_EXPR),popMode
    ;

JSPEXPR_CONTENT_CLOSE
    : ('}' | '%>') -> popMode
    ;



//
// tag declarations
//
mode TAG;
JSP_JSTL_COLON
    : ':'
    ;

TAG_SLASH_END
    : EMPTY_ELEMENT_CLOSE -> popMode
    ;

SUB_TAG_OPEN:
    BEGIN_ELEMENT_OPEN_TAG -> type(TAG_BEGIN),pushMode(TAG);

SUB_END_TAG_OPEN:
    BEGIN_ELEMENT_OPEN_TAG -> type(CLOSE_TAG_BEGIN),pushMode(TAG);


TAG_CLOSE
    : CLOSE_TAG -> popMode
    ;

TAG_SLASH
    : '/'
    ;

DIRECTIVE_END
    : JSP_END ->popMode
    ;
//
// lexing mode for attribute values
//
TAG_EQUALS
    : EQUALS_CHAR -> type(EQUALS),pushMode(ATTVALUE)
    ;

TAG_IDENTIFIER
    : IDENTIFIER
    ;

TAG_WHITESPACE
    : WHITESPACE ->skip
    ;

fragment SINGLE_QUOTE_STRING_CONTENT
    : ~[<'] | ESCAPED_SINGLE_QUOTE
    ;

fragment DOUBLE_QUOTE_STRING_CONTENT
    : ~[<"] | ESCAPED_DOUBLE_QUOTE
    ;

fragment WHITESPACE
    : [ \t\r\n]
    ;

fragment INLINE_WHITESPACE
      : [ \t]
      ;

fragment
HEXDIGIT
    : [a-fA-F0-9]
    ;

fragment
DIGIT
    : [0-9]
    ;

fragment
TAG_NameChar
    : TAG_NameStartChar
    | '-'
    | '_'
    | '.'
    | DIGIT
    |   '\u00B7'
    |   '\u0300'..'\u036F'
    |   '\u203F'..'\u2040'
    ;

fragment
TAG_NameStartChar
    :   [a-zA-Z]
    |   '\u2070'..'\u218F'
    |   '\u2C00'..'\u2FEF'
    |   '\u3001'..'\uD7FF'
    |   '\uF900'..'\uFDCF'
    |   '\uFDF0'..'\uFFFD'
    ;

//
// <scripts>
//
mode SCRIPT;

SCRIPT_BODY
    : .*? '</script>' -> popMode
    ;

SCRIPT_SHORT_BODY
    : .*? END_ELEMENT_OPEN_TAG -> popMode
    ;

//
// <styles>
//
mode STYLE;

STYLE_BODY
    : .*? '</style>' -> popMode
    ;

STYLE_SHORT_BODY
    : .*? '</>' -> popMode
    ;

//
// attribute values
//
mode ATTVALUE;

ATTVAL_END_TAG_OPEN:
    END_ELEMENT_OPEN_TAG -> type(CLOSE_TAG_BEGIN),pushMode(TAG);


ATTVAL_TAG_OPEN
    :  BEGIN_ELEMENT_OPEN_TAG -> type(TAG_BEGIN),pushMode(TAG)
;

ATTVAL_SINGLE_QUOTE_OPEN
    : SINGLE_QUOTE -> type(QUOTE),pushMode(ATTVALUE_SINGLE_QUOTE)
    ;

ATTVAL_DOUBLE_QUOTE_OPEN
    : DOUBLE_QUOTE -> type(QUOTE),pushMode(ATTVALUE_DOUBLE_QUOTE)
    ;

ATTVAL_CONST_VALUE
    : WHITESPACES? ATTVAL_ATTRIBUTE  -> type(ATTVAL_ATTRIBUTE),popMode
    ;

ATTVAL_ATTRIBUTE
    : ATTCHARS
    | HEXCHARS
    | DECCHARS;


mode ATTVALUE_SINGLE_QUOTE;

ATTVAL_SINGLE_QUOTE_CLOSING_QUOTE
    : SINGLE_QUOTE -> type(QUOTE),popMode,popMode
    ;

ATTVAL_SINGLE_QUOTE_EXPRESSION
    : EL_EXPR -> type(EL_EXPR)
    ;

ATTVAL_SINGLE_QUOTE_END_TAG_OPEN
    :  END_ELEMENT_OPEN_TAG -> type(CLOSE_TAG_BEGIN),pushMode(TAG)
    ;

ATTVAL_SINGLE_QUOTE_TAG_OPEN
    :  BEGIN_ELEMENT_OPEN_TAG -> type(TAG_BEGIN),pushMode(TAG)
    ;

ATTVAL_SINGLE_QUOTE_TEXT
    : SINGLE_QUOTE_STRING_CONTENT+ ->type(ATTVAL_ATTRIBUTE)
    ;


mode ATTVALUE_DOUBLE_QUOTE;

ATTVAL_DOUBLE_QUOTE_CLOSING_QUOTE
    : DOUBLE_QUOTE -> type(QUOTE),popMode,popMode
    ;

ATTVAL_DOUBLE_QUOTE_EXPRESSION
    : EL_EXPR -> type(EL_EXPR)
    ;

ATTVAL_DOUBLE_QUOTE_END_TAG_OPEN
    :  END_ELEMENT_OPEN_TAG -> type(CLOSE_TAG_BEGIN),pushMode(TAG)
    ;
ATTVAL_DOUBLE_QUOTE_TAG_OPEN
    :  BEGIN_ELEMENT_OPEN_TAG -> type(TAG_BEGIN),pushMode(TAG)
    ;

ATTVAL_DOUBLE_QUOTE_TEXT
    : DOUBLE_QUOTE_STRING_CONTENT+ -> type(ATTVAL_ATTRIBUTE)
    ;


fragment ATTCHAR
    : '-'
    | '_'
    | '.'
    | '/'
    | '+'
    | ','
    | '?'
    | '='
    | ':'
    | ';'
    | '#'
    | ALPHA_CHAR
    ;

fragment ALPHA_CHAR
    : [0-9a-zA-Z]
    ;

fragment ATTCHARS
    : ALPHA_CHAR ATTCHAR* ' '?
    ;

fragment HEXCHARS
    : '#' [0-9a-fA-F]+
    ;

fragment DECCHARS
    : [0-9]+ '%'?
    ;

EL_EXPR
    : '${' ~[}]* '}'
    ;

fragment ESCAPED_SINGLE_QUOTE
    : '\\\''
    ;

fragment EQUALS_CHAR
    : '='
    ;

fragment ESCAPED_DOUBLE_QUOTE
    : '\\\''
    ;