parser grammar ASPParser;

options { tokenVocab=ASPLexer; }

aspDocuments
    : aspDocument* EOF
    ;

aspDocument
    : aspScript
    | htmlElement
    | htmlCloseElement
    | htmlMisc
    | script
    | style
    ;

aspScript
    : aspDirective
    | aspDeclaration
    | aspExpression
    | aspDatabind
    | aspScriptlet
    ;

aspDirective
    : DIRECTIVE_BEGIN blobContent? BLOB_CLOSE
    ;

aspDeclaration
    : DECLARATION_BEGIN blobContent? BLOB_CLOSE
    ;

aspExpression
    : ECHO_EXPRESSION_OPEN blobContent? BLOB_CLOSE
    ;

aspDatabind
    : DATABIND_OPEN blobContent? BLOB_CLOSE
    ;

aspScriptlet
    : SCRIPTLET_OPEN blobContent? BLOB_CLOSE
    ;

blobContent
    : BLOB_CONTENT+
    ;

htmlElement
    : TAG_BEGIN htmlTag htmlAttribute* TAG_CLOSE htmlContent* CLOSE_TAG_BEGIN htmlTag TAG_CLOSE
    | TAG_BEGIN htmlTag htmlAttribute* TAG_SLASH_END
    | TAG_BEGIN htmlTag htmlAttribute* TAG_CLOSE
    ;

htmlCloseElement
    : CLOSE_TAG_BEGIN htmlTag TAG_CLOSE
    ;

htmlTag
    : TAG_IDENTIFIER
    ;

htmlAttribute
    : TAG_IDENTIFIER TAG_EQUALS ATTVAL_VALUE
    | TAG_IDENTIFIER TAG_EQUALS aspScript
    | TAG_IDENTIFIER
    ;

htmlContent
    : htmlMisc
    | htmlElement
    | htmlCloseElement
    | aspScript
    | script
    | style
    ;

htmlMisc
    : ASP_STATIC_CONTENT_CHARS
    | WHITESPACES
    ;

script
    : SCRIPT_OPEN SCRIPT_BODY
    ;

style
    : STYLE_OPEN STYLE_BODY
    ;
