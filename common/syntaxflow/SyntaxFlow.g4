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
    : filterExpr (As refVariable)? eos? # FilterExecution
    | assertStatement eos?              # FilterAssert
    | descriptionStatement eos?         # Description
    | eos                               # EmptyStatement
    ;

// eos means end of statement
// the ';' can be
eos: ';';

// descriptionStatement will describe the filterExpr with stringLiteral
descriptionStatement: Desc ('(' descriptionItems? ')') | ('{' descriptionItems? '}');
descriptionItems: descriptionItem (',' descriptionItem)*;
descriptionItem
    : stringLiteral
    | stringLiteral ':' stringLiteral
    ;

// assertStatement will assert the filterExpr is true, if not, it will throw an error with stringLiteral
// if thenExpr is provided, it will be executed(description) after the assertStatement
assertStatement: Assert refVariable thenExpr? elseExpr?;
thenExpr: Then stringLiteral;
elseExpr: Else stringLiteral;


refVariable
    :  '$' (identifier | ('(' identifier ')'));

filterExpr
    : '$'    identifier?                                    # CurrentRootFilter
    | nameFilter                                            # PrimaryFilter
    | '.' nameFilter                                        # FieldFilter
    | filterExpr '.' nameFilter                             # FieldCallFilter
    | filterExpr '(' actualParam? ')'                       # FunctionCallFilter
    | filterExpr '[' sliceCallItem ']'                      # FieldIndexFilter
    | filterExpr '?{' conditionExpression '}'               # OptionalFilter
    | filterExpr '->' filterExpr                            # NextFilter
    | filterExpr '->'                                       # NextSingleFilter
    | filterExpr '#>' filterExpr                            # DefFilter
    | filterExpr '#>'                                       # DefSingleFilter
    | filterExpr '-->' filterExpr                           # DeepNextFilter
    | filterExpr '-->'                                      # DeepNextSingleFilter
    | filterExpr '#->' filterExpr                           # TopDefFilter
    | filterExpr '#->'                                      # TopDefSingleFilter
    | filterExpr '-{' (recursiveConfig)? '}->' filterExpr   # ConfiggedDeepNextFilter
    | filterExpr '-{' (recursiveConfig)? '}->'              # ConfiggedDeepNextSingleFilter
    | filterExpr '#{' (recursiveConfig)? '}->' filterExpr   # ConfiggedTopDefFilter
    | filterExpr '#{' (recursiveConfig)? '}->'              # ConfiggedTopDefSingleFilter
    | filterExpr '-<' useDefCalcDescription '>-' filterExpr # UseDefCalcFilter
    ;

useDefCalcDescription
    : identifier useDefCalcParams?
    ;

useDefCalcParams
    : '{' recursiveConfig? '}'
    | '(' recursiveConfig? ')'
    ;

actualParam
    : singleParam                      # AllParam
    | actualParamFilter+ singleParam?  # EveryParam
    ;

actualParamFilter: singleParam ',' | ',';

singleParam: ( '#>' | '#{' (recursiveConfig)? '}' )? filterStatement ;

recursiveConfig: recursiveConfigItem (',' recursiveConfigItem)* ','? ;
recursiveConfigItem: identifier ':' recursiveConfigItemValue;
recursiveConfigItemValue
    : (identifier | numberLiteral)
    | '`' filterStatement '`'
    ;

sliceCallItem: nameFilter | numberLiteral;

nameFilter: '*' | '$' | identifier | regexpLiteral;

chainFilter
    : '[' ((filters (',' filters)*) | '...') ']'          # Flat
    | '{' ((identifier ':') filters (';' (identifier ':') filters )*)? ';'? '}'  # BuildMap
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
stringLiteral: identifier | '*';
regexpLiteral: RegexpLiteral;
identifier: Identifier | types | As | Assert | Then | Desc;

types: StringType | NumberType | ListType | DictType | BoolType;
boolLiteral: BoolLiteral;

DeepFilter: '==>';
Deep: '...';

Percent: '%%';
DeepDot: '..';
LtEq: '<=';
GtEq: '>=';
DoubleGt: '>>';
Filter: '=>';
EqEq: '==';
RegexpMatch: '=~';
NotRegexpMatch: '!~';
And: '&&';
Or: '||';
NotEq: '!=';
ConditionStart: '?{';
DeepNextStart: '-{';
DeepNextEnd: '}->';
TopDefStart: '#{';
DefStart: '#>';
TopDef: '#->';
Gt: '>';
Dot: '.';
Lt: '<';
Eq: '=';
Question: '?';
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
Star: '*';
Minus: '-';
As: 'as';
Backtick: '`';

WhiteSpace: [ \r\n] -> skip;
Number: Digit+;
OctalNumber: '0o' OctalDigit+;
BinaryNumber: '0b' ('0' | '1')+;
HexNumber: '0x' HexDigit+;
//StringLiteral: '`' (~[`])* '`';
StringType: 'str';
ListType: 'list';
DictType: 'dict';
NumberType: 'int' | 'float';
BoolType: 'bool';
BoolLiteral: 'true' | 'false';
Assert: 'assert';
Then: 'then';
Desc: 'desc' | 'note';
Else: 'else';

Identifier: IdentifierCharStart IdentifierChar*;
IdentifierChar: [0-9] | IdentifierCharStart;

fragment IdentifierCharStart: '*' | '_' | [a-z] | [A-Z];
fragment HexDigit: [a-fA-F0-9];
fragment Digit: [0-9];
fragment OctalDigit: [0-7];

RegexpLiteral: '/' RegexpLiteralChar+ '/';
fragment RegexpLiteralChar
    : '\\' '/'
    | ~[/]
    ;

WS: [ \t\r]+ -> skip;