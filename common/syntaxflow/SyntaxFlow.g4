grammar SyntaxFlow;

/*
yaklang.io
v1ll4n.a5k@gmail.com

build with `antlr -Dlanguage=Go ./SyntaxFlow.g4 -o sf -package sf -no-listener -visitor`

SyntaxFlow is a search expr can handle some structured data
*/

flow: statements EOF;

statements: statement+;

statement
    : checkStatement eos?               # Check
    | descriptionStatement eos?         # Description
    | alertStatement eos?               # Alert
    | filterStatement eos?              # Filter
    | fileFilterContentStatement eos ?  # FileFilterContent
    | comment eos?                      # Command
    | eos                               # Empty
    ;

fileFilterContentStatement
    : '${' fileFilterContentInput '}' lines? '.' fileFilterContentMethod (As refVariable)?
    ;

// match filter content
fileFilterContentInput: ( fileName | regexpLiteral);

// only specific method can be used
// 1. regexp  (re)
// 2. regexp2 (re2)
// 3. xpath
//
// looks like:
// ${/*sqlmap*.xml/}.xpath(select: ...)
// ${application.properties}.re2(jdbc: ...)
fileFilterContentMethod: Identifier '(' fileFilterContentMethodParam? ')'; // do something check for 'forbidden *'
fileFilterContentMethodParam:  fileFilterContentMethodParamItem lines? (',' lines? fileFilterContentMethodParamItem lines? )* ','? lines? ;
fileFilterContentMethodParamItem: fileFilterContentMethodParamKey? fileFilterContentMethodParamValue;
fileFilterContentMethodParamKey: Identifier ':';
fileFilterContentMethodParamValue: nameFilter;
fileName:nameFilter (. nameFilter)*;

filterStatement
    : refVariable filterItem*  (As refVariable)? # RefFilterExpr
    | filterExpr  (As refVariable)?              # PureFilterExpr
    ;

comment: LineComment;


// eos means end of statement
// the ';' can be
eos: ';' | line ;
line: '\n';
lines: line+;

// descriptionStatement will describe the filterExpr with stringLiteral
descriptionStatement: Desc ('(' descriptionItems? ')') | ('{' descriptionItems? '}');
descriptionItems: lines? descriptionItem (',' lines? descriptionItem)* ','? lines?;
descriptionItem
    : stringLiteral lines?
    | stringLiteral ':' stringLiteral lines?
    ;

// echo statement will echo the variable 
alertStatement: Alert refVariable (For stringLiteral)?;

// checkStatement will check the filterExpr($params) is true( .len > 0), if not,
// it will record an error with stringLiteral
// if thenExpr is provided, it will be executed(description) after the assertStatement
checkStatement: Check refVariable thenExpr? elseExpr?;
thenExpr: Then stringLiteral;
elseExpr: Else stringLiteral;

refVariable
    :  '$' (identifier | ('(' identifier ')'));


filterItemFirst
    : nameFilter                                 # NamedFilter
    | '.' lines? nameFilter                      # FieldCallFilter
    | nativeCall                                 # NativeCallFilter
    ;

filterItem
    : filterItemFirst                            # First
    | '...' lines? nameFilter                    # DeepChainFilter
    | '(' lines? actualParam? ')'                # FunctionCallFilter
    | '[' sliceCallItem ']'                      # FieldIndexFilter
    | '?{' conditionExpression '}'               # OptionalFilter
    | '->'                                       # NextFilter
    | '#>'                                       # DefFilter
    | '-->'                                      # DeepNextFilter
    | '-{' (config)? '}->'                       # DeepNextConfigFilter
    | '#->'                                      # TopDefFilter
    | '#{' (config)? '}->'                       # TopDefConfigFilter
    | '+' refVariable                            # MergeRefFilter
    | '-' refVariable                            # RemoveRefFilter
    ;

filterExpr: filterItemFirst filterItem* ;

nativeCall
    : '<' useNativeCall '>'
    ;

useNativeCall
    : identifier useDefCalcParams?
    ;

useDefCalcParams
    : '{' config? '}'
    | '(' config? ')'
    ;

actualParam
    : singleParam    lines?                   # AllParam
    | actualParamFilter+ singleParam? lines?  # EveryParam
    ;

actualParamFilter: singleParam ',' | ',';

singleParam: ( '#>' | '#{' (config)? '}' )? filterStatement ;

config: recursiveConfigItem (',' recursiveConfigItem)* ','?;
recursiveConfigItem: line? identifier ':' recursiveConfigItemValue lines?;
recursiveConfigItemValue
    : (identifier | numberLiteral)
    | '`' filterStatement '`'
    ;

sliceCallItem: nameFilter | numberLiteral;

nameFilter: '*' | identifier | regexpLiteral;

chainFilter
    : '[' ((statements (',' statements)*) | '...') ']'          # Flat
    | '{' ((identifier ':') statements (';' (identifier ':') statements )*)? ';'? '}'  # BuildMap
    ;

stringLiteralWithoutStarGroup: stringLiteralWithoutStar (',' stringLiteralWithoutStar)* ','?;
negativeCondition: (Not | '!');

conditionExpression
    : '(' conditionExpression ')'                                                # ParenCondition
    | filterExpr                                                                 # FilterCondition        // filter dot(.)Member and fields
    |  Opcode ':' opcodes (',' opcodes) * ','?                      # OpcodeTypeCondition    // something like .(call, phi)
    |  Have  ':' stringLiteralWithoutStarGroup       # StringContainHaveCondition // something like .(have: 'a', 'b')
    |  HaveAny ':' stringLiteralWithoutStarGroup       # StringContainAnyCondition // something like .(have: 'a', 'b')
    | negativeCondition conditionExpression                                                    # NotCondition
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
stringLiteralWithoutStar: identifier | regexpLiteral ;
regexpLiteral: RegexpLiteral;
identifier: Identifier | keywords | QuotedStringLiteral;

keywords
    : types
    | opcodes
    | Opcode
    // | As
    | Check
    | Then
    | Desc
    | Else
    | Type
    | In
    | Have
    | HaveAny
    | BoolLiteral
    ;

opcodes: Call | Constant | Phi | FormalParam | Return | Function;

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
SingleQuote: '\'';
DoubleQuote: '"';
LineComment: '//' (~[\r\n])*;
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
Alert : 'alert';
Check: 'check';
Then: 'then';
Desc: 'desc' | 'note';
Else: 'else';
Type: 'type';
In: 'in';
Call: 'call';
Function: 'function';
Constant: 'const' | 'constant';
Phi: 'phi';
FormalParam: 'param' | 'formal_param';
Return: 'return' | 'ret';
Opcode: 'opcode';
Have: 'have';
HaveAny: 'any';
Not: 'not';
For: 'for';

Identifier: IdentifierCharStart IdentifierChar*;
IdentifierChar: [0-9] | IdentifierCharStart;

QuotedStringLiteral
    : SingleQuote ( ~['\\\r\n] | ('\\\'') | '\\\\' | '\\')* SingleQuote
    | DoubleQuote ( ~["\\\r\n] | '\\"' | '\\\\' | '\\' )* DoubleQuote
    ;

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