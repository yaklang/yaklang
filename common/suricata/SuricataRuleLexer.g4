lexer grammar SuricataRuleLexer;

// Keywords
Any: 'any';

// Symbols
Negative: '!';
Dollar: '$';
Arrow: '->';
BothDirect: '<>';
Mul: '*';
Div: '/';
Mod: '%';
Amp: '&';
Plus: '+';
Sub: '-';
Power: '^';
Lt: '<';
Gt: '>';
LtEq: '<=';
GtEq: '>=';
Colon: ':';
DoubleColon: '::';
LBracket: '[';
RBracket: ']';
ParamStart: '(' -> pushMode(PARAM_MODE);
LBrace: '{';
RBrace: '}';
Comma: ',';
Eq: '=';
NotSymbol: '~';
Dot: '.';

LINE_COMMENT: ('#' | '//') SingleLineInputCharacter* -> skip;

NORMALSTRING
    : '"' ( EscapeSequence | ~('\\'|'"') )* '"'
    ;

INT
    : Digit+
    ;

HEX
    : HexDigit+
    ;

ID
    : [a-zA-Z_][-a-zA-Z_0-9]*
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

HexDigit
    : [a-fA-F0-9]
    ;

fragment
SingleLineInputCharacter
    : ~[\r\n\u0085\u2028\u2029]
    ;

WS
    : [ \t\u000C\r\n]+ -> skip
    ;

NonSemiColon: [^;]+;

SHEBANG
    : '#' Negative SingleLineInputCharacter* -> channel(HIDDEN)
    ;

mode PARAM_MODE;
    ParamWS: [ \t\u000C]+ -> skip;
    ParamEnd: ')' -> popMode;
    ParamColon: ':' -> pushMode(UNTIL_SEP_MODE);
    ParamNegative: '!';
    ParamComma: ',';
    ParamSep: ';';
    ParamCommonStringChar: (~[,;":\n!\r()]) ;
    ParamCommonString: ParamCommonStringChar+ ;

mode UNTIL_SEP_MODE;
    ModeStringStart: '"' -> pushMode(MODE_STRING);
    fragment UntilSettingDataChar: ~[\n\r;"] | '\\"';
    UntilSepSettingEnd: ';' -> popMode;
    UntilSEPParamWS: [ \t\u000C]+ -> skip;
    NonQuotedString: UntilSettingDataChar+;

mode MODE_STRING;
    fragment ModeStringChar: ~[\n\r"] | '\\"';
    ModeString: ModeStringChar+;
    ModeStringEnd: '"' -> popMode;

ANY: .;