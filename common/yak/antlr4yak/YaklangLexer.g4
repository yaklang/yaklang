lexer grammar YaklangLexer;

// antlr 4.11.1

/*
定义关键词与变量名
*/
Panic: 'panic';
Recover: 'recover';
If: 'if';
Elif: 'elif';
Else: 'else';
Switch: 'switch';
Case: 'case';
Default: 'default';
For: 'for';
Continue: 'continue';
Break: 'break';
Return: 'return';
Include: 'include';
Try: 'try';
Catch: 'catch';
Finally: 'finally';
Importmod: 'importmod';
As: 'as';
Export: 'export';
Defer: 'defer';
Go: 'go';
Range: 'range';
Func: 'func' | 'fn' | 'def' | 'function';
Map: 'map';
Chan: 'chan';
Class: 'class';
New: 'new';
Make: 'make';
True: 'true';
False: 'false';
In: 'in';
NotLiteral: 'not';
Assert: 'assert';
Var: 'var';
VarTypeName
    : 'uint' |  'uint8' | 'byte' | 'uint16' | 'uint32' | 'uint64'
    | 'int' | 'int8' | 'int16' | 'int32' | 'int64'
    | 'bool' | 'float' | 'float64' | 'double' | 'string' | Var;
UndefinedLiteral: 'undefined';
Fallthrough: 'fallthrough';
NilLiteral: 'nil';

Identifier
    : [a-zA-Z_][a-zA-Z0-9_]*
    ;
IdentifierWithDollar
    : '$' [a-zA-Z_][a-zA-Z0-9_]*
    ;

/*
    词法部分
*/
// Symbols
// 定义操作符号
Mul: '*';
Div: '/';
Mod: '%';
LtLt: '<<';
Lt: '<';
GtGt: '>>';
Gt: '>';
Amp: '&';
AmpNot: '&^';
Plus: '+';
Sub: '-';
Xor: '^';
Xand: '|';
Eq: '==';
LtEq: '<=';
GtEq: '>=';
Neq: '!=';
ChanIn: '<-';
LogicAnd: '&&';
LogicOr: '||';
Question: '?';
Colon: ':';
LBracket: '[';
RBracket: ']';
LParen: '(';
RParen: ')';
LBrace: '{';
TemplateCloseBrace:             '}' {this.IsInTemplateString()}? -> popMode;
RBrace: '}';
Comma: ',';
AssignEq: '=';
Wavy: '~';
ColonAssignEq: ':=';
PlusPlus: '++';
SubSub: '--';
PlusEq: '+=';
MinusEq: '-=';
MulEq: '*=';
DivEq: '/=';
ModEq: '%=';
BitOrEq: '^=';
LtLtEq: '<<=';
GtGtEq: '>>=';
AmpEq: '&=';
BitAndEq: '|=';
BitAndNotEq: '&^=';
SemiColon: ';';
Ellipsis: '...';
EqGt: '=>';
LtGt: '<>';
Not: '!';
Dot: '.';
// white space
WS: [ \t\r]+ -> skip;
CommontStart: '/*';
CommontEnd: '*/';
BackTickL: '`';
COMMENT: '/*' .*? '*/';

// 定义按行的评论/注释
LINE_COMMENT         : ('//' | '#') ~[\r\n]*;

LF: '\n';

// end of statement
EOS: ( ';' | '/*' .*? '*/' | EOF);
// whitespace with line

/*
定义基本字面量：
1. 整数（十进制，八进制（0 0o0O），十六进制(0x0X)，二进制(0b)）
2. 浮点
3. 字符串（前缀字符串）
4. 字符
*/
IntegerLiteral
    : DecimalIntegerLiteral
    | OctalIntegerLiteral
    | HexIntegerLiteral
    | BinaryIntegerLiteral
    ;
// 浮点数的话，一般就两种表达
// 1. 1.1
// 2. .111
FloatLiteral
    : DecimalIntegerLiteral '.' [0-9]+
    | '.' [0-9]+
    ;
TemplateDoubleQuoteStringStart
    : 'f"'  {this.IncreaseTemplateDepth();} -> pushMode(TEMPLATE_DOUBLE_QUOTE_MODE)
    ;
TemplateBackTickStringStart
    : 'f`'  {this.IncreaseTemplateDepth();} -> pushMode(TEMPLATE_BACKTICK_MODE)
    ;

StringLiteral
    : DoubleQuoteStringLiteral
    | BackTickStringLiteral
    | HexStringLiteral
    | SingleQuoteStringLiteral
    ;



// 字符，一般定义为 char
CharacterLiteral
    : '\'' SingleStringCharacter '\''
    ;

mode TEMPLATE_DOUBLE_QUOTE_MODE;
    TemplateDoubleQuoteStringCharacterStringEnd:                 '"' {this.DecreaseTemplateDepth();} -> popMode;
    TemplateDoubleQuoteStringCharacter
        : ~["\\\r\n$]
        | '\\' EscapeSequence
        | '\\$'
        ;
    TemplateDoubleQuoteStringStartExpression:  '${' -> pushMode(DEFAULT_MODE);

mode TEMPLATE_BACKTICK_MODE;
    TemplateBackTickStringCharacterStringEnd:                 '`' {this.DecreaseTemplateDepth();} -> popMode;
    TemplateBackTickStringCharacter
        : ~[`\\$]
        | '\\' EscapeSequence
        | '\\$'
        | '\\`'
        ;
    TemplateBackTickStringStartExpression:  '${' -> pushMode(DEFAULT_MODE);


// Fragment rules
fragment HexIntegerLiteral:              '0' [xX] HexDigit+;
fragment OctalIntegerLiteral
    : '0' [0-7]+
    | '0' [oO] [0-7]+
    ;
fragment BinaryIntegerLiteral:           '0' [bB] [01]+;
fragment HexDigit
    : [0-9a-fA-F]
    ;
fragment DecimalIntegerLiteral
    : '0'
    | [1-9] [0-9]*
    ;

// 字符串字面量
// 设置什么时候应该被转义
fragment EscapeSequence
    : CharacterEscapeSequence
    | '0' // no digit ahead! TODO
    | HexEscapeSequence

    // 这两个是针对 unicode 的，暂时就不要了
    // | UnicodeEscapeSequence
    // | ExtendedUnicodeEscapeSequence
    ;
fragment CharacterEscapeSequence
    : SingleEscapeCharacter
    | NonEscapeCharacter
    ;
fragment HexEscapeSequence
    : 'x' HexDigit HexDigit
    ;

fragment SingleEscapeCharacter
    : ['"\\bfnrtv]
    ;

fragment NonEscapeCharacter
    : ~['"\\bfnrtv0-9xu\r\n]
    ;
fragment EscapeCharacter
    : SingleEscapeCharacter
    | [0-9]
    | [xu]
    ;

fragment DoubleStringCharacter
    : ~["\\\r\n]
    | '\\' EscapeSequence
    ;
fragment SingleStringCharacter
    : ~['\\\r\n]
    | '\\' EscapeSequence
    ;
fragment BackTickStringCharacter
    : ~[`]
    ;

fragment DoubleQuoteTemplateStringLiteral
    : StringLiteralPrefix  ? '"'
    ;
fragment BackTickTemplateStringLiteral
    : StringLiteralPrefix? '`'
    ;

fragment DoubleQuoteStringLiteral
    : StringLiteralPrefix  ? '"' DoubleStringCharacter* '"'
    ;

fragment SingleQuoteStringLiteral
    : StringLiteralPrefix  ? '\''  '\''
    | StringLiteralPrefix  ? '\'' SingleStringCharacter SingleStringCharacter+  '\''
    ;

fragment BackTickStringLiteral
    : StringLiteralPrefix? '`' BackTickStringCharacter* '`'
    ;
fragment HexStringLiteral
    : '0h' (HexDigit HexDigit) +
    ;
fragment StringLiteralPrefix: [a-eg-zA-Z];
