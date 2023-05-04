parser grammar YaklangParser;

options {
    tokenVocab=YaklangLexer;
}

/*
    语法部分
*/
// 定义整个程序都是从语句出发构成的
// 因为 EOS 的特殊构造，所以，本语法要求最好为 '\n' 结尾
program: ws* statementList EOF;

statementList: (statement )+;

// 语句的构成也并不复杂
statement
    // 基本语句
    : lineCommentStmt eos

    // 声明变量的优先级比表达式高，这个规则匹配应该是 var a,d,b,c 只能支持 Var，特殊语法
    | declearVariableExpressionStmt eos

    // var(...) 或者 var 单独使用，作为类型，expression 是右值
    | assignExpressionStmt eos
    | expressionStmt eos
    | block eos
    | tryStmt eos

    | empty

    // 流程控制
    | ifStmt /* if expr {} elif {} */
    | switchStmt
    | forRangeStmt
    | forStmt
    | breakStmt eos
    | returnStmt eos
    | continueStmt eos
    | fallthroughStmt eos
    | includeStmt eos
    | deferStmt eos
    | goStmt eos
    | assertStmt eos
    ;

tryStmt: 'try' block 'catch' Identifier? block ('finally' block)?;

expressionStmt: expression;
assignExpressionStmt: assignExpression;
lineCommentStmt: (LINE_COMMENT | COMMENT);

includeStmt: 'include' StringLiteral;
deferStmt: 'defer' expression;
goStmt: 'go' ((expression functionCall) | instanceCode);
assertStmt: 'assert' expression (',' expression)*;
fallthroughStmt: 'fallthrough';
breakStmt: 'break';
continueStmt: 'continue';
returnStmt: 'return' expressionList?;

/*
for statement
*/
forStmt: 'for' (forStmtCond | '(' forStmtCond ')' | expression)? block;
forStmtCond:  forFirstExpr? ';' expression? ';' forThirdExpr?;
forFirstExpr: assignExpression | expression;
forThirdExpr: assignExpression | expression;

/*
for range/in statement
for-in 的行为和 python 的行为很像
*/
forRangeStmt: 'for' (((leftExpressionList (':=' | '='))? 'range') | ((leftExpressionList)? 'in')) expression block;


/*
switch statement syntax
*/
switchStmt: 'switch' expression? '{' (ws* 'case' expressionList ':' statementList?)* ( ws* 'default' ':' statementList?)? ws* '}';

/*
if statement syntax
*/
ifStmt: 'if' expression block ('elif' expression block)* elseBlock?;
elseBlock: 'else' (ifStmt|block);

block: '{' ws* statementList? ws* '}';

empty: EOS | ';' | ws;

inplaceAssignOperator
    : '+=' | '-=' | '*='
    | '/=' | '%=' | '&='
    | '|=' | '^=' | '<<='
    | '>>=' | '&^='
    ;

assignExpression
    : leftExpressionList ('=' | ':=') expressionList
    | leftExpression ('++' | '--')
    | leftExpression inplaceAssignOperator expression
    ;

// 变量声明语句，和赋值不一样
declearVariableExpressionStmt: declearVariableExpression;
declearVariableExpression: declearVariableOnly | declearAndAssignExpression;
declearVariableOnly: Var Identifier (',' Identifier) *;
declearAndAssignExpression: Var leftExpressionList ('=' | ':=') expressionList;

leftExpressionList
    : leftExpression (',' leftExpression) *
    ;

/*
一元操作：

前缀op + expression
*/
unaryOperator
    : '!' | '-' | '+'
    | '^' | '&' | '*'
    | '<-'
    ;

bitBinaryOperator
    : '<<' | '>>'
    | '&' | '&^'
    | '|'
    | '^'
    ;

additiveBinaryOperator
    : '+' | '-'
    ;

multiplicativeBinaryOperator
    : '*' | '/' | '%'
    ;

comparisonBinaryOperator
    : '>' | '<'
    | '<=' | '>='
    | '!='
    | '<>'
    | '=='
    ;
// 定义左值，一般就三种情况
/*
a = 1
a[1] = 1
a.abc = 1
a.$a = 1
*/
leftExpression
    : expression (leftMemberCall | leftSliceCall)
    | Identifier
    ;
leftMemberCall: '.' (Identifier | IdentifierWithDollar);
leftSliceCall: '[' expression ']';

expression
    // 单目运算
    : typeLiteral '(' ws* expression? ws* ')'
    | literal
    | anonymousFunctionDecl
    | Panic '(' ws* expression ws* ')'
    | Recover '(' ')'
    | Identifier
    | expression (memberCall | sliceCall | functionCall)
    | parenExpression
    | instanceCode // 闭包，快速执行代码 fn{...}
    | makeExpression // make 特定语法
    | unaryOperator expression



    // 二元运算（位运算全面优先于数字运算，数字运算全面优先于高级逻辑运算）
    | expression bitBinaryOperator ws* expression

    // 普通数学运算
    | expression multiplicativeBinaryOperator ws* expression
    | expression additiveBinaryOperator ws* expression
    | expression comparisonBinaryOperator ws* expression

    // 高级逻辑
    | expression '&&' ws* expression
    | expression '||' ws* expression
    | expression 'not'? 'in' expression
    | expression '<-' expression
    | expression '?' ws* expression ws* ':' ws* expression
    ;

parenExpression: '(' expression? ')' ;

// 定义 make 语法，有点特殊，因为涉及到类型声明
makeExpression: 'make' '(' ws* typeLiteral (',' ws* expressionListMultiline )?')';
typeLiteral
    : VarTypeName
    | Var
    | sliceTypeLiteral
    | mapTypeLiteral
    | 'chan' typeLiteral
    ;
sliceTypeLiteral: '[' ']' typeLiteral;
mapTypeLiteral: 'map' '[' typeLiteral ']' typeLiteral;


instanceCode: Func block;

/*
定义函数
fn(p1,p2,p3){}
fn abc(p1,p2,p3){}
fn abc(p1,p2,p3...){}
*/
anonymousFunctionDecl
    : Func functionNameDecl? '(' functionParamDecl? ')'  block
    | ('('  functionParamDecl? ')' | Identifier ) '=>' (expression | block)
    ;

functionNameDecl: Identifier;
functionParamDecl: ws* Identifier (ws* ',' ws* Identifier)* '...'? ws* ','? ws*;

functionCall: '(' ordinaryArguments? ')' '~'?;
ordinaryArguments: ws* expression (ws* ',' ws* expression)* '...'? ws* ','? ws*;

// call member id.abc
memberCall: '.' (Identifier | IdentifierWithDollar) ;

// call slice [start:end]
sliceCall
    : '[' expression? ':' expression? ':' expression? ']'
    | '[' expression? ':' expression? ']'
    | '[' expression ']'    // [key] -- [0]
    ;

literal
    : templateStringLiteral
    | stringLiteral
    | numericLiteral
    | charaterLiteral
    | UndefinedLiteral
    | NilLiteral
    | boolLiteral
    | mapLiteral
    | sliceTypedLiteral
    | typeLiteral
    | sliceLiteral
    ;

numericLiteral
    : IntegerLiteral
    | FloatLiteral
    ;
stringLiteral
    : StringLiteral
    ;
templateDoubleQuoteStringLiteral
    : TemplateDoubleQuoteStringStart (templateDoubleQupteStringAtom)* TemplateDoubleQuoteStringCharacterStringEnd
    ;
templateBackTickStringLiteral
    : TemplateBackTickStringStart (templateBackTickStringAtom)* TemplateBackTickStringCharacterStringEnd
    ;
templateStringLiteral
    : templateDoubleQuoteStringLiteral | templateBackTickStringLiteral
    ;
templateDoubleQupteStringAtom
    :TemplateDoubleQuoteStringCharacter+
    | TemplateDoubleQuoteStringStartExpression expression TemplateCloseBrace
    ;
templateBackTickStringAtom
    :TemplateBackTickStringCharacter+
    | TemplateBackTickStringStartExpression expression TemplateCloseBrace
    ;
boolLiteral
    : ('true' | 'false')
    ;
charaterLiteral: CharacterLiteral;

sliceLiteral: '[' ws* expressionListMultiline? ws* ']';

 sliceTypedLiteral
     : sliceTypeLiteral '{' ws* expressionListMultiline? ws* '}'
     ;

// 表达式列表
expressionList: expression (',' expression)* ','?;
expressionListMultiline: expression (',' ws* expression)* ','?;

/* map literal */
mapLiteral
    : mapTypedLiteral
    |'{' ws* mapPairs? ws* '}'
    ;
mapTypedLiteral
    : mapTypeLiteral '{' ws* mapPairs? ws* '}'
    ;
mapPairs: mapPair (',' ws* mapPair)* ','?;
mapPair: expression ':' expression;

ws: (LF | COMMENT | LINE_COMMENT) +;

// end of statement
eos
    : ';'
    | LF +
    | COMMENT
    | LINE_COMMENT
    | { this.closingBracket() }?
    ;