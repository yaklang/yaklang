lexer grammar LuaLexer;




//定义关键字

Function: 'function';
Nil: 'nil';
False: 'false';
True: 'true';
Return: 'return';
Break: 'break';
Continue: 'continue';
Goto: 'goto';
Repeat: 'repeat';
Until: 'until';
While: 'while';
Do: 'do';
End: 'end';
If: 'if';
Then: 'then';
Else: 'else';
ElseIf: 'elseif';
For: 'for';
In: 'in';
Local: 'local';
Not: 'not';
Or: 'or';
And: 'and';



// Symbols
// 定义操作符号
Mul: '*';
Div: '/';
IntegralDiv: '//';
Mod: '%';
Amp: '&';
Xand: '|';
LtLt: '<<';
GtGt: '>>';
Plus: '+';
Sub: '-';
Power: '^';
Lt: '<';
Gt: '>';
Eq: '==';
LtEq: '<=';
GtEq: '>=';
Neq: '~=';
Colon: ':';
DoubleColon: '::';
LBracket: '[';
RBracket: ']';
LParen: '(';
RParen: ')';
LBrace: '{';
RBrace: '}';
Comma: ',';
Pound: '#';
AssignEq: '=';
PlusPlus: '++';
SubSub: '--';
PlusEq: '+=';
MinusEq: '-=';
MulEq: '*=';
DivEq: '/=';
ModEq: '%=';
SemiColon: ';';
Ellipsis: '...';
NotSymbol: '~';
Dot: '.';
Strcat: '..';









NAME
    : [a-zA-Z_][a-zA-Z_0-9]*
    ;

NORMALSTRING
    : '"' ( EscapeSequence | ~('\\'|'"') )* '"'
    ;

CHARSTRING
    : '\'' ( EscapeSequence | ~('\''|'\\') )* '\''
    ;

LONGSTRING
    : '[' NESTED_STR ']'
    ;

fragment
NESTED_STR
    : '=' NESTED_STR '='
    | '[' .*? ']'
    ;

INT
    : Digit+
    ;

HEX
    : '0' [xX] HexDigit+
    ;

FLOAT
    : Digit+ '.' Digit* ExponentPart?
    | '.' Digit+ ExponentPart?
    | Digit+ ExponentPart
    ;

HEX_FLOAT
    : '0' [xX] HexDigit+ '.' HexDigit* HexExponentPart?
    | '0' [xX] '.' HexDigit+ HexExponentPart?
    | '0' [xX] HexDigit+ HexExponentPart
    ;

fragment
ExponentPart
    : [eE] [+-]? Digit+
    ;

fragment
HexExponentPart
    : [pP] [+-]? Digit+
    ;

fragment
EscapeSequence
    : '\\' [abfnrtvz"'|$#\\]   // World of Warcraft Lua additionally escapes |$#
    | '\\' '\r'? '\n'
    | DecimalEscape
    | HexEscape
    | UtfEscape
    ;

fragment
DecimalEscape
    : '\\' Digit
    | '\\' Digit Digit
    | '\\' [0-2] Digit Digit
    ;

fragment
HexEscape
    : '\\' 'x' HexDigit HexDigit
    ;

fragment
UtfEscape
    : '\\' 'u{' HexDigit+ '}'
    ;

fragment
Digit
    : [0-9]
    ;

fragment
HexDigit
    : [0-9a-fA-F]
    ;

fragment
SingleLineInputCharacter
    : ~[\r\n\u0085\u2028\u2029]
    ;

COMMENT
    : '--[' NESTED_STR ']' -> channel(HIDDEN)
    ;

LINE_COMMENT
    : '--' SingleLineInputCharacter* -> channel(HIDDEN)
    ;

WS
    : [ \t\u000C\r\n]+ -> skip
    ;

SHEBANG
    : '#' '!' SingleLineInputCharacter* -> channel(HIDDEN)
    ;