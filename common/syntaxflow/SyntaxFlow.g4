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
    | '[' numberLiteral ']'                   # ListIndexFilter
    | filterExpr '=>' chainFilter             # AheadChainFilter
    | filterExpr '==>' chainFilter            # DeepChainFilter
    | filterExpr '.' filterFieldMember        # FieldChainFilter
    ;

chainFilter
    : '[' ((conditionExpression (',' conditionExpression)*) | '...') ']'          # Flat
    | '{' ((identifier ':') filters (';' (identifier ':') filters )*)? ';'? '}'  # BuildMap
    ;

filterFieldMember
    : identifier | numberLiteral | typeCast
    | ( '(' conditionExpression ')')
    ;
conditionExpression
    : numberLiteral                               # FilterExpressionNumber
    | stringLiteral                               # FilterExpressionString
    | regexpLiteral                               # FilterExpressionRegexp
    | '(' conditionExpression ')'                 # FilterExpressionParen
    | '!' conditionExpression                     # FilterExpressionNot
    | op = (
        '>' | '<' | '=' | '==' | '>='
         | '<=' | '!='
        ) (
            numberLiteral | identifier | boolLiteral
        ) # FilterExpressionCompare
    | op = ( '=~' | '!~') (stringLiteral | regexpLiteral) # FilterExpressionRegexpMatch
    | conditionExpression '&&' conditionExpression      # FilterExpressionAnd
    | conditionExpression '||' conditionExpression      # FilterExpressionOr
    ;

numberLiteral: Number | OctalNumber | BinaryNumber | HexNumber;
stringLiteral: identifier;
regexpLiteral: RegexpLiteral;
typeCast: '(' types ')';
identifier: Identifier | types;
types: StringType | NumberType | ListType | DictType | BoolType;
boolLiteral: BoolLiteral;

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
RegexpMatch: '=~';
NotRegexpMatch: '!~';
And: '&&';
Or: '||';
NotEq: '!=';

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
Bang: '!';


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
BoolType: 'bool';
BoolLiteral: 'true' | 'false';

Identifier: IdentifierCharStart IdentifierChar*;
fragment IdentifierCharStart: '%' | '_' | [a-z] | [A-Z] | '%%';
fragment IdentifierChar: [0-9] | IdentifierCharStart;
fragment HexDigit: [a-fA-F0-9];
fragment Digit: [0-9];
fragment OctalDigit: [0-7];

RegexpLiteral: '/' RegexpLiteralChar+ '/';
fragment RegexpLiteralChar
    : '\\' '/'
    | ~[/]
    ;