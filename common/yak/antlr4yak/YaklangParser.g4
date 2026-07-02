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
    | declareVariableExpressionStmt eos

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
deferStmt: 'defer' (recoverStmt | panicStmt | callExpr);
goStmt: 'go' callExpr;
assertStmt: 'assert' expression (',' expression)*;
fallthroughStmt: 'fallthrough';
breakStmt: 'break';
continueStmt: 'continue';
returnStmt: 'return' expressionList?;

callExpr: functionCallExpr | instanceCode;
/*
functionCallExpr 消歧说明：

历史写法 `expression functionCall` 与 expression 自身的 `expression functionCall` 后缀
重叠：对 `f()`，expression 既能整体吞掉 `f()`（表达式后缀调用），又能只取 `f` 再由外层
functionCall 取 `()`，二者共享前缀，SLL 无法在合并上下文中判定，导致 `go f()`/`defer f()`
这类语句 bail 回退 LL。

现在直接复用通用 `expression`（其顶层通常为 `expression functionCall` 或 instanceCode），
判别完全交给 expression 自身的左递归消解，SLL 即可命中。是否为合法调用（顶层为 functionCall
或 instanceCode）由 visitor 判定。
*/
functionCallExpr: expression;

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
panic statement
*/
panicStmt: Panic '(' ws* expression ws* ')';

/*
recover statement
*/
recoverStmt: Recover '(' ')';

/*
if statement syntax

支持 Go 风格的初始化语句：if <init>; <cond> { ... }
初始化语句可以是赋值、变量声明或普通表达式，其声明的变量作用域覆盖整个 if/elif/else 链，
但不会泄漏到外部作用域。
*/
ifStmt: 'if' (ifStmtInit ';')? expression block ('elif' expression block)* elseBlock?;
ifStmtInit: assignExpression | declareVariableExpression | expression;
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
declareVariableExpressionStmt: declareVariableExpression;
declareVariableExpression: declareVariableOnly | declareAndAssignExpression;
declareVariableOnly: Var Identifier (',' Identifier) *;
declareAndAssignExpression: Var leftExpressionList ('=' | ':=') expressionList;

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

// `%` 作为二元运算符同时充当取余和格式化功能
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
/*
leftExpression 消歧说明：

历史写法 `expression (leftMemberCall | leftSliceCall) | Identifier` 会与
`expressionStmt` 中的 `expression sliceCall/memberCall` 共享前缀，且 `leftSliceCall`
的 `'[' expression ']'` 与 `sliceCall` 的单下标形式完全重叠，导致 SLL（单一合并上下文）
无法在消费完 `a[0]` 后判定其后是赋值 `=` 还是普通表达式语句，从而 bail 回退到 LL。

现在直接复用通用 `expression`：左值前缀与右值前缀走完全相同的 ATN 路径，判别点后移到
`expression` 结束后的单个 token（`=`/`:=`/`++`/`--`/复合赋值 或 eos），SLL 即可命中。
`Identifier` 作为首选备选，保证裸标识符（如 `for i in x`）不会被 `expression` 贪婪吞掉
`in`/后缀，从而与 for-range 等规则保持既有行为。

左值是否可赋值（成员/下标/标识符）改由 visitor 依据 expression 子树判定，非法左值（如
`1 = 2`、`f() = 2`）在语义阶段报错，而非语法阶段。
*/
leftExpression
    : Identifier
    | expression
    ;

expression
    // 单目运算
    : typeLiteral '(' ws* expression? ws* ')'
    | literal
    | anonymousFunctionDecl
    | panicStmt
    | recoverStmt
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

    // 包含运算仍然是初级逻辑
    | expression 'not'? 'in' expression

    // 高级逻辑
    | expression '&&' ws* expression
    | expression '||' ws* expression
    | expression '?' ws* expression ws* ':' ws* expression

    // 管道操作符
    | expression '<-' expression
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
    | ('('  functionParamDecl? ')' | Identifier ) '=>' (block | expression)
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
    | characterLiteral
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
    | StartNowDoc HereDocIdentifierName HereDocIdentifierBreak crlfHereDoc? CRLFEndDoc
    | StartNowDoc HereDocIdentifierName HereDocIdentifierBreak lfHereDoc? LFEndDoc
    ;
crlfHereDoc: CRLFHereDocText+;
lfHereDoc: LFHereDocText+;
templateSingleQuoteStringLiteral
    : TemplateSingleQuoteStringStart (templateSingleQuoteStringAtom)* TemplateSingleQuoteStringCharacterStringEnd
    ;
templateDoubleQuoteStringLiteral
    : TemplateDoubleQuoteStringStart (templateDoubleQuoteStringAtom)* TemplateDoubleQuoteStringCharacterStringEnd
    ;
templateBackTickStringLiteral
    : TemplateBackTickStringStart (templateBackTickStringAtom)* TemplateBackTickStringCharacterStringEnd
    ;
templateStringLiteral
    : templateSingleQuoteStringLiteral | templateDoubleQuoteStringLiteral | templateBackTickStringLiteral 
    ;
templateSingleQuoteStringAtom
    :TemplateSingleQuoteStringCharacter+
    | TemplateSingleQuoteStringStartExpression expression TemplateCloseBrace
    ;
templateDoubleQuoteStringAtom
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
characterLiteral: CharacterLiteral;

sliceLiteral: '[' ws* expressionListMultiline? ws* ']';

 sliceTypedLiteral
     : sliceTypeLiteral '{' ws* expressionListMultiline? ws* ';'?'}'
     ;

// 表达式列表
expressionList: expression (',' expression)* ','?;
expressionListMultiline: expression (',' ws* expression)* ','?;

/* map literal */
mapLiteral
    : mapTypedLiteral
    |'{' ws* mapPairs? ws* ';'? '}'
    ;
mapTypedLiteral
    : mapTypeLiteral '{' ws* mapPairs? ws* ';'?'}'
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
    ;