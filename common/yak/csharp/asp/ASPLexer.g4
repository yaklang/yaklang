lexer grammar ASPLexer;

ASP_COMMENT: '<%--' .*? '--%>' -> skip;
HTML_COMMENT: '<!--' .*? '-->' -> skip;

SCRIPT_OPEN: '<script' .*? '>' -> pushMode(SCRIPT);
STYLE_OPEN: '<style' .*? '>' -> pushMode(STYLE);

DIRECTIVE_BEGIN: '<%@' -> pushMode(ASP_BLOB);
DECLARATION_BEGIN: '<%!' -> pushMode(ASP_BLOB);
ECHO_EXPRESSION_OPEN: '<%=' -> pushMode(ASP_BLOB);
DATABIND_OPEN: '<%#' -> pushMode(ASP_BLOB);
SCRIPTLET_OPEN: '<%' -> pushMode(ASP_BLOB);

CLOSE_TAG_BEGIN: '</' -> pushMode(TAG);
TAG_BEGIN: '<' -> pushMode(TAG);

WHITESPACES: [ \t\r\n]+;
ASP_STATIC_CONTENT_CHARS: ~[<]+;

mode ASP_BLOB;
BLOB_CLOSE: '%>' -> popMode;
BLOB_CONTENT: (~[%] | '%' ~'>')+;

mode TAG;
TAG_SLASH_END: '/>' -> popMode;
TAG_CLOSE: '>' -> popMode;
TAG_EQUALS: '=' -> pushMode(ATTVALUE);
TAG_IDENTIFIER: [a-zA-Z_:][a-zA-Z0-9_.:-]*;
TAG_WHITESPACE: [ \t\r\n]+ -> skip;
TAG_SCRIPTLET_OPEN: '<%' -> type(SCRIPTLET_OPEN), pushMode(ASP_BLOB);
TAG_ECHO_OPEN: '<%=' -> type(ECHO_EXPRESSION_OPEN), pushMode(ASP_BLOB);

mode ATTVALUE;
ATTVAL_ECHO: '<%=' -> type(ECHO_EXPRESSION_OPEN), popMode, pushMode(ASP_BLOB);
ATTVAL_SCRIPTLET: '<%' -> type(SCRIPTLET_OPEN), popMode, pushMode(ASP_BLOB);
ATTVAL_DQ: '"' ~["<]* '"' -> type(ATTVAL_VALUE), popMode;
ATTVAL_SQ: '\'' ~['<]* '\'' -> type(ATTVAL_VALUE), popMode;
ATTVAL_UQ: ~[>\t\r\n <]+ -> type(ATTVAL_VALUE), popMode;
ATTVAL_VALUE: '\u0000' ;

mode SCRIPT;
SCRIPT_BODY: .*? '</script>' -> popMode;

mode STYLE;
STYLE_BODY: .*? '</style>' -> popMode;
