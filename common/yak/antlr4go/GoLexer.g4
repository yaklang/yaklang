/*
 [The "BSD licence"]
 Copyright (c) 2017 Sasa Coh, Michał Błotniak
 Copyright (c) 2019 Ivan
 Kochurkin, kvanttt@gmail.com, Positive Technologies
 Copyright (c) 2019 Dmitry Rassadin,
 flipparassa@gmail.com, Positive Technologies
 Copyright (c) 2021 Martin Mirchev,
 mirchevmartin2203@gmail.com
 All rights reserved.
 
 Redistribution and use in source and binary
 forms, with or without
 modification, are permitted provided that the following conditions
 are
 met:
 1. Redistributions of source code must retain the above copyright
 notice, this list of
 conditions and the following disclaimer.
 2. Redistributions in binary form must reproduce the
 above copyright
 notice, this list of conditions and the following disclaimer in the
 documentation
 and/or other materials provided with the distribution.
 3. The name of the author may not be used
 to endorse or promote products
 derived from this software without specific prior written
 permission.
 
 THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY EXPRESS OR
 IMPLIED
 WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
 OF MERCHANTABILITY AND FITNESS
 FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
 IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY DIRECT,
 INDIRECT,
 INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
 NOT LIMITED
 TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 DATA, OR PROFITS; OR BUSINESS
 INTERRUPTION) HOWEVER CAUSED AND ON ANY
 THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT
 LIABILITY, OR TORT
 (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
 THIS
 SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

/*
 * A Go grammar for ANTLR 4 derived from the Go Language Specification
 https://golang.org/ref/spec
 */

// $antlr-format alignTrailingComments true, columnLimit 150, maxEmptyLinesToKeep 1, reflowComments false, useTab false
// $antlr-format allowShortRulesOnASingleLine true, allowShortBlocksOnASingleLine true, minEmptyLines 0, alignSemicolons ownLine
// $antlr-format alignColons trailing, singleLineOverrulesHangingColon true, alignLexerCommands true, alignLabels true, alignTrailers true

lexer grammar GoLexer;

options {
}

// Keywords

BREAK       : 'break';
DEFAULT     : 'default';
FUNC        : 'func';
INTERFACE   : 'interface';
SELECT      : 'select';
CASE        : 'case';
DEFER       : 'defer';
GO          : 'go';
MAP         : 'map';
STRUCT      : 'struct';
CHAN        : 'chan';
ELSE        : 'else';
GOTO        : 'goto';
PACKAGE     : 'package';
SWITCH      : 'switch';
CONST       : 'const';
FALLTHROUGH : 'fallthrough';
IF          : 'if';
RANGE       : 'range';
TYPE        : 'type';
CONTINUE    : 'continue';
FOR         : 'for';
IMPORT      : 'import';
RETURN      : 'return';
VAR         : 'var';

NIL_LIT: 'nil';

IDENTIFIER: IdentifierStart IdentifierPart*;

// Punctuation

L_PAREN        : '(';
R_PAREN        : ')';
L_CURLY        : '{';
R_CURLY        : '}';
L_BRACKET      : '[';
R_BRACKET      : ']';
ASSIGN         : '=';
COMMA          : ',';
SEMI           : ';';
COLON          : ':';
DOT            : '.';
PLUS_PLUS      : '++';
MINUS_MINUS    : '--';
DECLARE_ASSIGN : ':=';
ELLIPSIS       : '...';

// Logical

LOGICAL_OR  : '||';
LOGICAL_AND : '&&';

// Relation operators

EQUALS            : '==';
NOT_EQUALS        : '!=';
LESS              : '<';
LESS_OR_EQUALS    : '<=';
GREATER           : '>';
GREATER_OR_EQUALS : '>=';

// Arithmetic operators

OR         : '|';
DIV        : '/';
MOD        : '%';
LSHIFT     : '<<';
RSHIFT     : '>>';
BIT_CLEAR  : '&^';
UNDERLYING : '~';

// Unary operators

EXCLAMATION: '!';

// Mixed operators

PLUS      : '+';
MINUS     : '-';
CARET     : '^';
STAR      : '*';
AMPERSAND : '&';
RECEIVE   : '<-';

// Number literals

DECIMAL_LIT : ('0' | [1-9] ('_'? [0-9])*);
BINARY_LIT  : '0' [bB] ('_'? BIN_DIGIT)+;
OCTAL_LIT   : '0' [oO]? ('_'? OCTAL_DIGIT)+;
HEX_LIT     : '0' [xX] ('_'? HEX_DIGIT)+;

FLOAT_LIT         : (DECIMAL_FLOAT_LIT | HEX_FLOAT_LIT);
DECIMAL_FLOAT_LIT : DECIMALS ('.' DECIMALS? EXPONENT? | EXPONENT) | '.' DECIMALS EXPONENT?;
HEX_FLOAT_LIT     : '0' [xX] HEX_MANTISSA HEX_EXPONENT;

fragment HEX_MANTISSA:
    ('_'? HEX_DIGIT)+ ('.' ( '_'? HEX_DIGIT)*)?
    | '.' HEX_DIGIT ('_'? HEX_DIGIT)*
;

fragment HEX_EXPONENT: [pP] [+-]? DECIMALS;

IMAGINARY_LIT: (DECIMAL_LIT | BINARY_LIT | OCTAL_LIT | HEX_LIT | FLOAT_LIT) 'i';

// Rune literals
BYTE_VALUE: OCTAL_BYTE_VALUE | HEX_BYTE_VALUE;

OCTAL_BYTE_VALUE: '\\' OCTAL_DIGIT OCTAL_DIGIT OCTAL_DIGIT;

HEX_BYTE_VALUE: '\\' 'x' HEX_DIGIT HEX_DIGIT;

LITTLE_U_VALUE: '\\' 'u' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT;

BIG_U_VALUE:
    '\\' 'U' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
;

// String literals

RAW_STRING_LIT         : '`' (.*?) '`';
INTERPRETED_STRING_LIT : '"' (~["\\] | ESCAPED_VALUE)* '"';
RAW_CHAR_LIT           : '\'' (~['\\] | ESCAPED_VALUE)? '\'';

// Hidden tokens

WSI : [ \t\r]+ -> skip;
EOS : ([\t\r\n ;]+);

COMMENT      : '/*' .*? '*/' -> skip;
LINE_COMMENT : '//' ~[\r\n]* -> skip;

// Fragments

fragment ESCAPED_VALUE:
    '\\' (
        'u' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
        | 'U' HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT HEX_DIGIT
        | [abfnrtv\\'"]
        | OCTAL_DIGIT OCTAL_DIGIT OCTAL_DIGIT
        | 'x' HEX_DIGIT HEX_DIGIT
    )
;

fragment DECIMALS: [0-9] ('_'? [0-9])*;

fragment OCTAL_DIGIT: [0-7];

fragment HEX_DIGIT: [0-9a-fA-F];

fragment BIN_DIGIT: [01];

fragment EXPONENT: [eE] [+-]? DECIMALS;

fragment LETTER: UNICODE_LETTER | '_';

//[\p{Nd}] matches a digit zero through nine in any script except ideographic scripts
fragment UNICODE_DIGIT: [\p{Nd}];
//[\p{L}] matches any kind of letter from any language
fragment UNICODE_LETTER: [\p{L}];

fragment IdentifierStart: [\p{L}] | [$_] | '\\' UnicodeEscapeSequence;

fragment IdentifierPart:
    IdentifierStart
    | [\p{Mn}]
    | [\p{Nd}]
    | [\p{Pc}]
    | '\u4E00' ..'\u9FFF'
    | '\u200C'
    | '\u200D'
;

fragment UnicodeEscapeSequence:
    'u' HexDigit HexDigit HexDigit HexDigit
    | 'u' '{' HexDigit HexDigit+ '}'
;

fragment HexDigit: [_0-9a-fA-F];