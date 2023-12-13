grammar SyntaxFlow;

/*
yaklang.io
v1ll4n.a5k@gmail.com

build with `antlr -Dlanguage=Go ./SyntaxFlow.g4 -o sf -package sf -no-listener -visitor`

SyntaxFlow is a search expr can handle some structured data
*/

flow: filters EOF;

filters: filterStatement+;

filterStatement
    : existedRef? (direction = ('>>' | '<<'))? filterExpr ('=>' refVariable)?
    ;

existedRef: refVariable;

refVariable
    :  '$' (identifier | ('(' identifier ')'));

filterExpr
    : identifier                              # PrimaryFilter
    | numberLiteral                           # NumberIndexFilter
    | op = ('>>' | '<<') filterExpr           # DirectionFilter
    | '(' filterExpr ')'                      # ParenFilter
    | '.' filterFieldMember                   # FieldFilter
    | filterExpr '=>' chainFilter             # AheadChainFilter
    | filterExpr '==>' chainFilter            # DeepChainFilter
    | filterExpr '.' filterFieldMember        # FieldChainFilter
    ;

chainFilter
    : '[' ((filterExpression (',' filterExpression)*) | '...') ']'          # Flat
    | '{' ((identifier ':') filters (';' (identifier ':') filters )*)? ';'? '}'  # BuildMap
    ;

filterFieldMember: identifier | numberLiteral | typeCast;
filterExpression
    : numberLiteral

    | stringLiteral
    | boolLiteral
    | op = ('>' | '<' | '=' | '==' | '>=' | '<=' | '~=' /*for string regex*/) filterExpression
    | filterExpression '&&' filterExpression
    | filterExpression '||' filterExpression
    ;

numberLiteral: Number | OctalNumber | BinaryNumber | HexNumber;
stringLiteral: identifier;
typeCast: '(' types ')';
identifier: Identifier | types;
types: StringType | NumberType | ListType | DictType | BoolType;
boolLiteral: BoolType;

DeepFilter: '==>';
Deep: '...';

Percent: '%%';
DeepDot: '..';
LtEq: '<=';
GtEq: '>=';
DoubleLt: '<<';
DoubleGt: '>>';
Filter: '=>';
EqEq: '==';
RegexpMatch: '~=';
And: '&&';
Or: '||';
Gt: '>';
Dot: '.';
Lt: '<';
Eq: '=';
OpenParen: '(';
Comma: ',';
CloseParen: ')';
ListSelectOpen: '[';
ListSelectClose: ']';
MapBuilderOpen: '{';
MapBuilderClose: '}';
ListStart: '#';
DollarOutput: '$';
Colon: ':';
Search: '%';

WhiteSpace: [ \r\n] -> skip;
Number: Digit+;
OctalNumber: '0o' OctalDigit+;
BinaryNumber: '0b' ('0' | '1')+;
HexNumber: '0x' HexDigit+;
StringLiteral: '`' (~[`])* '`';
StringType: 'str';
ListType: 'list';
DictType: 'dict';
NumberType: 'int' | 'float';
BoolType: 'true' | 'false';

Identifier: IdentifierCharStart IdentifierChar*;
fragment IdentifierCharStart: '%' | '_' | [a-z] | [A-Z] | '%%';
fragment IdentifierChar: [0-9] | IdentifierCharStart;
fragment HexDigit: [a-fA-F0-9];
fragment Digit: [0-9];
fragment OctalDigit: [0-7];