lexer grammar  JSPLexer;

JSP_COMMENT: '<!--' .*? '-->'->skip;

JSP_CONDITIONAL_COMMENT: '<!--[' .*? ']]>'->skip;

SCRIPT_OPEN: '<script' .*? '>' -> pushMode(SCRIPT);

STYLE_OPEN: '<style' .*? '>' -> pushMode(STYLE);

DTD
    : '<!DOCTYPE' -> pushMode(IN_DTD)
    ;

CDATA
    : '<![CDATA[' .*? ']]>'
    ;

WHITESPACES
    :  (' ' | '\t' | '\r'? '\n')+
    ;

XML_DECLARATION
    : '<?xml' -> pushMode(TAG)
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
    : DIRECTIVE_BEGIN_TAG -> pushMode(TAG)
    ;

DECLARATION_BEGIN
    : DECLARATION_BEGIN_TAG -> pushMode(JSP_BLOB)
    ;

ECHO_EXPRESSION_OPEN
    : ECHO_EXPRESSION_OPEN_TAG -> pushMode(JSP_BLOB)
    ;

SCRIPTLET_OPEN
    : SCRIPTLET_OPEN_TAG -> pushMode(JSP_BLOB)
    ;


QUOTE
    : SINGLE_QUOTE
    | DOUBLE_QUOTE
    ;

TAG_END
   : CLOSE_TAG
   ;

EQUALS
    : EQUALS_CHAR -> pushMode(ATTVALUE)
    ;

EL_EXPR_START
    : EL_EXPR_OPEN ->pushMode(EL_EXPR_MODE)
    ;

JSP_STATIC_CONTENT_CHARS
    :  JSP_STATIC_CONTENT_CHAR+
    ;

JSP_END
    : JSP_END_TAG ->popMode
    ;

ATTVAL_ATTRIBUTE
    :' '* ATTVAL_VALUE
    ;

ATTVAL_VALUE
    : ATT_CONSTANTS
    | HEXCHARS
    | DECCHARS
    ;

EL_EXPR_END
    : EL_EXPR_CLOSE
    ;

fragment CLOSE_TAG
    : '>'
    ;

fragment DOUBLE_QUOTE
    : '"'
    ;

fragment SINGLE_QUOTE
    : '\''
    ;

fragment IDENTIFIER
    : TAG_NameStartChar TAG_NameChar*
    ;

fragment EL_EXPR_BODY
    : ~[\\}]+
    ;

fragment EL_EXPR_OPEN
    : '${'
    | '#{'
    ;

fragment EL_EXPR_CLOSE
    : '}'
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

fragment DIRECTIVE_BEGIN_TAG
    :'<%@'
    |'<jsp:directive'
    ;

fragment DECLARATION_BEGIN_TAG
    : '<%!'
    |'<jsp:declaration'
    ;

fragment ECHO_EXPRESSION_OPEN_TAG
    : '<%='
    |'<jsp:expression'
    ;

fragment SCRIPTLET_OPEN_TAG
    : '<%'
    |'jsp:scriptlet'
    ;

fragment EXPRESSION_OPEN_TAG
    :'${'
    |'#{'
    ;

fragment JSP_END_TAG
    : '%>'
    ;

fragment JSP_STATIC_CONTENT_CHAR
    : ~[<\\$]+
    | ESCAPED_DOLLAR
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
    : ~('%')
    | '%' ~'>'
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

TAG_JSP_EXPRESSION_OPEN
    :ECHO_EXPRESSION_OPEN ->type(ECHO_EXPRESSION_OPEN), pushMode(JSP_BLOB)
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
    : ~[<'$]
    | '$' ~ '{'
    | ESCAPED_SINGLE_QUOTE
    ;

fragment DOUBLE_QUOTE_STRING_CONTENT
    : ~[<"$]
    | '$' ~ '{'
    | ESCAPED_DOUBLE_QUOTE
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
    : BEGIN_ELEMENT_OPEN_TAG -> type(TAG_BEGIN),pushMode(TAG)
    ;

ATTVAL_SINGLE_QUOTE_OPEN
    : SINGLE_QUOTE -> type(QUOTE),pushMode(ATTVALUE_SINGLE_QUOTE)
    ;

ATTVAL_DOUBLE_QUOTE_OPEN
    : DOUBLE_QUOTE -> type(QUOTE),pushMode(ATTVALUE_DOUBLE_QUOTE)
    ;

ATTVAL_CONST_VALUE
    :  ATTVAL_ATTRIBUTE  -> type(ATTVAL_ATTRIBUTE), popMode
    ;

ATTVAL_EL_EXPR
    : EL_EXPR_START -> pushMode(EL_EXPR_MODE)
    ;

mode ATTVALUE_SINGLE_QUOTE;

ATTVAL_SINGLE_QUOTE_CLOSING_QUOTE
    : SINGLE_QUOTE -> type(QUOTE),popMode,popMode
    ;

ATTVAL_SINGLE_QUOTE_EXPRESSION
    :EL_EXPR_OPEN -> type(EL_EXPR_START),pushMode(EL_EXPR_MODE)
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

ATTVAL_JSP_IN_SINGLE_QUOTE
    :ECHO_EXPRESSION_OPEN ->type(ECHO_EXPRESSION_OPEN), pushMode(JSP_BLOB)
    ;

mode ATTVALUE_DOUBLE_QUOTE;

ATTVAL_DOUBLE_QUOTE_CLOSING_QUOTE
    : DOUBLE_QUOTE -> type(QUOTE),popMode,popMode
    ;

ATTVAL_DOUBLE_QUOTE_EXPRESSION
    : EL_EXPR_OPEN -> type(EL_EXPR_START),pushMode(EL_EXPR_MODE)
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

ATTVAL_JSP_IN_DOUBLE_QUOTE
    :ECHO_EXPRESSION_OPEN ->type(ECHO_EXPRESSION_OPEN), pushMode(JSP_BLOB)
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
    | '('
    | ')'
    ;

fragment ALPHA_CHAR
    : [0-9a-zA-Z]
    ;

// char and alpha
fragment ATT_CONSTANT
    : ALPHA_CHAR
    | ATTCHAR
    ;

fragment ATT_CONSTANTS
   : ATT_CONSTANT+
   ;

fragment HEXCHARS
    : '#' [0-9a-fA-F]+
    ;

fragment DECCHARS
    : [0-9]+ '%'?
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

mode EL_EXPR_MODE;

EL_EXPR_END_EX
   : EL_EXPR_CLOSE -> type(EL_EXPR_END),popMode
   ;

EL_EXPR_CONTENT
    : EL_EXPR_BODY
    ;


