parser grammar SyntaxFlowParser;

options {
    tokenVocab=SyntaxFlowLexer;
}


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
    : refVariable filterItem*  (As refVariable)?        # RefFilterExpr
    | filterExpr  (As refVariable)?                     # PureFilterExpr
    ;

comment: LineComment;


// eos means end of statement
// the ';' can be
eos: Semicolon | line ;
line: BreakLine;
lines: line+;

// descriptionStatement will describe the filterExpr with stringLiteral
descriptionStatement: Desc (('(' lines? descriptionItems? ')') | ('{' lines? descriptionItems? '}'));
descriptionItems: lines? (descriptionItem descriptionSep)* descriptionItem descriptionSep? ;
descriptionItem
    : comment
    | stringLiteral
    | stringLiteral ':' descriptionItemValue
    ;
descriptionSep
    : ',' lines?
    | lines
    ;

descriptionItemValue: stringLiteral | hereDoc | numberLiteral;
crlfHereDoc: CRLFHereDocIdentifierBreak crlfText? CRLFEndDoc;
lfHereDoc: LFHereDocIdentifierBreak lfText? LFEndDoc;
crlfText: CRLFHereDocText+;
lfText: LFHereDocText+;
hereDoc: '<<<'  HereDocIdentifierName (crlfHereDoc | lfHereDoc);

// echo statement will echo the variable
alertStatement: Alert  refVariable (For ((('{' descriptionItems '}'))|stringLiteral))?;

// checkStatement will check the filterExpr($params) is true( .len > 0), if not,
// it will record an error with stringLitera
// if thenExpr is provided, it will be executed(description) after the assertStatement
checkStatement: Check refVariable thenExpr? elseExpr?;
thenExpr: Then stringLiteral;
elseExpr: Else stringLiteral;

refVariable
    :  '$' (identifier | ('(' identifier ')'));


filterItemFirst
    : constSearchPrefix?(QuotedStringLiteral|hereDoc) # ConstFilter
    | nameFilter                                      # NamedFilter
    | '.' lines? nameFilter                           # FieldCallFilter
    | nativeCall                                      # NativeCallFilter
    ;

constSearchPrefix: ConstSearchModePrefixRegexp | ConstSearchModePrefixGlob | ConstSearchModePrefixExact;

filterItem
    : filterItemFirst                            # First
    | '...' lines? nameFilter                    # DeepChainFilter
    | Question? '(' lines? actualParam? ')'      # FunctionCallFilter
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
    | '&' refVariable                            # IntersectionRefFilter
    | In versionInExpression                     # VersionInFilter
    ;


filterExpr: filterItemFirst filterItem*;

nativeCall
    : '<' useNativeCall '>'
    ;

useNativeCall
    : identifier useDefCalcParams?
    ;

useDefCalcParams
    : '{' nativeCallActualParams? '}'
    | '(' nativeCallActualParams? ')'
    ;
nativeCallActualParams: lines? nativeCallActualParam (',' lines? nativeCallActualParam)* ','? lines?;
nativeCallActualParam
    : (nativeCallActualParamKey (':' | '='))?  nativeCallActualParamValue
    ;
nativeCallActualParamKey: identifier;
nativeCallActualParamValue: identifier | numberLiteral | '`' ~'`'* '`' | '$' identifier | hereDoc;

actualParam
    : singleParam    lines?                   # AllParam
    | actualParamFilter+ singleParam? lines?  # EveryParam
    ;

actualParamFilter: singleParam ',' | ',';

singleParam: ( '#>' | '#{' (config)? '}' )? filterStatement ;

config: recursiveConfigItem (',' recursiveConfigItem)* ','? lines? ;
recursiveConfigItem: lines? identifier ':' recursiveConfigItemValue lines?;
recursiveConfigItemValue
    : (identifier | numberLiteral)
    | '`' filterStatement '`'
    | hereDoc
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
    |  Opcode ':' opcodesCondition (',' opcodesCondition) * ','?                      # OpcodeTypeCondition    // something like .(call, phi)
    |  Have  ':' stringLiteralWithoutStarGroup       # StringContainHaveCondition // something like .(have: 'a', 'b')
    |  HaveAny ':' stringLiteralWithoutStarGroup       # StringContainAnyCondition // something like .(have: 'a', 'b')
    |  VersionIn ':' versionInExpression              # VersionInCondition
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

versionInExpression: versionInterval ('||' versionInterval)*;
versionInterval: ( '[' | '(') vstart? ',' vend? (   ']'| ')' ) ;
vstart: versionString;
vend: versionString;
// unless ',' ']' ')'
versionBlockElement: Number versionSuffix* ;
versionSuffix: '-' | Identifier;
versionBlock:  versionBlockElement ('.' versionBlockElement )*;
versionString
    : stringLiteral
    | versionBlock
    ;

opcodesCondition: opcodes | identifier;

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
    | constSearchPrefix
    ;

opcodes: Call | Constant | Phi | FormalParam | Return | Function;

types: StringType | NumberType | ListType | DictType | BoolType;
boolLiteral: BoolLiteral;
