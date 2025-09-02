lexer grammar NaslLexer;

SingleLineComment:              '#' ~[\r\n\u2028\u2029]* -> channel(HIDDEN);
OpenBracket:                    '[';
CloseBracket:                   ']';
OpenParen:                      '(';
CloseParen:                     ')';
OpenBrace:                      '{';
CloseBrace:                     '}';
SemiColon:                      ';';
Comma:                          ',';
Assign:                         '=';
Colon:                          ':';
Dot:                            '.';
PlusPlus:                       '++';
MinusMinus:                     '--';
Plus:                           '+';
Minus:                          '-';
BitNot:                         '~';
BitAnd:                         '&';
BitXOr:                         '^';
BitOr:                          '|';
RightShiftArithmetic:           '>>';
LeftShiftArithmetic:            '<<';
LeftShiftLogical:              '<<<';
RightShiftLogical:              '>>>';

Not:                            '!';
Multiply:                       '*';
Pow:                          '**';
Divide:                         '/';
Modulus:                        '%';
LessThan:                       '<';
MoreThan:                       '>';
LessThanEquals:                 '<=';
GreaterThanEquals:              '>=';
Equals_:                        '==';
EqualsRe:                       '=~';
NotEquals:                      '!=';
NotLong:                        '!~';
MTNotLT:                        '>!<';
MTLT:                           '><';
And:                            '&&';
Or:                             '||';
MultiplyAssign:                 '*=';
DivideAssign:                   '/=';
ModulusAssign:                  '%=';
PlusAssign:                     '+=';
MinusAssign:                    '-=';
X: 'x';
RightShiftLogicalAssign:        '>>>=';
LeftShiftLogicalAssign:         '<<<=';
RightShiftArithmeticAssign:     '>>=';
LeftShiftArithmeticAssign:     '<<=';

Break:                          'break';
Var:                            'var';
LocalVar:                       'local_var';
GlobalVar:                      'global_var';
Else:                           'else';
Return:                         'return';
Continue:                       'continue';
For:                            'for';
ForEach:                        'foreach';
If:                             'if';
Function_:                      'function';
Repeat:                         'repeat';
While:                          'while';
Until:                          'until';
StringLiteral:                 ('"' ~["]* '"') | ('\'' SingleStringCharacter* '\'');
fragment SingleStringCharacter
    : ~['\\]
    | '\\' .
    ;
BooleanLiteral:                 'true'
              |                 'false' | 'FALSE' | 'TRUE';

IntegerLiteral: [0-9]+
                ;
FloatLiteral:   IntegerLiteral '.' [0-9]*;
IpLiteral: [0-9]* '.' [0-9]* '.' [0-9]* '.' [0-9]*;
HexLiteral:     '0' [xX] [0-9a-fA-F]+;
NULLLiteral: 'NULL';
Identifier:                     [a-zA-Z_$] [a-zA-Z0-9_$]*;
//CharLiteral:                    '\'' ~['] '\'';
WhiteSpaces:     [\t\u000B\u000C\u0020\u00A0]+ -> channel(HIDDEN);

LineTerminator:  [\r\n\u2028\u2029] -> channel(HIDDEN);
