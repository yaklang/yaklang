/*
PHP grammar.
The MIT License (MIT).
Copyright (c) 2015-2020, Ivan Kochurkin (kvanttt@gmail.com), Positive Technologies.
Copyright (c) 2019-2020, Student Main for php7, php8 support.

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/

// $antlr-format alignTrailingComments true, columnLimit 150, minEmptyLines 1, maxEmptyLinesToKeep 1, reflowComments false, useTab false
// $antlr-format allowShortRulesOnASingleLine false, allowShortBlocksOnASingleLine true, alignSemicolons hanging, alignColons hanging

parser grammar PHPParser;

options {
    tokenVocab = PHPLexer;
}

// HTML
// Also see here: https://github.com/antlr/grammars-v4/tree/master/html

htmlDocument
    : Shebang? htmlDocumentElement* EOF
    ;

htmlDocumentElement
    : inlineHtml | phpBlock
    ;

inlineHtml
    : htmlElement+
    | scriptText
    ;

// TODO: split into html, css and xml elements
htmlElement
    : HtmlDtd
    | HtmlClose
    | HtmlStyleOpen
    | HtmlOpen
    | HtmlName
    | HtmlSlashClose
    | HtmlSlash
    | HtmlText
    | HtmlEquals
    | HtmlStartQuoteString
    | HtmlEndQuoteString
    | HtmlStartDoubleQuoteString
    | HtmlEndDoubleQuoteString
    | HtmlHex
    | HtmlDecimal
    | HtmlQuoteString
    | HtmlDoubleQuoteString
    | StyleBody
    | HtmlScriptOpen
    | HtmlScriptClose
    | XmlStart XmlText* XmlClose
    ;

// Script
// Parse JavaScript with https://github.com/antlr/grammars-v4/tree/master/javascript if necessary.

scriptText
    : ScriptText+
    ;

// PHP

phpBlock
    : importStatement* topStatement+ (PHPEnd|PHPEndSingleLineComment)?
    ;

importStatement
    : Import Namespace namespaceNameList SemiColon
    ;

topStatement
    : statement
    | useDeclaration
    | namespaceDeclaration
    | functionDeclaration
    | classDeclaration
    | globalConstantDeclaration
    | enumDeclaration
    ;

useDeclaration
    : Use (Function_ | Const)? useDeclarationContentList SemiColon
    ;

useDeclarationContentList
    : '\\'? useDeclarationContent (',' '\\'? useDeclarationContent)*
    ;

useDeclarationContent
    : namespaceNameList
    ;

namespaceDeclaration
    : Namespace (
        namespaceNameList? OpenCurlyBracket namespaceStatement* CloseCurlyBracket
        | namespaceNameList SemiColon
    )
    ;

namespaceStatement
    : statement
    | useDeclaration
    | functionDeclaration
    | classDeclaration
    | globalConstantDeclaration
    ;

functionDeclaration
    : attributes? Function_ '&'? identifier /*typeParameterListInBrackets?*/ '(' formalParameterList ')' (
        ':' QuestionMark? typeHint
    )? blockStatement
    ;

classDeclaration
    : attributes? Private? modifier? Partial? (
        classEntryType identifier /*typeParameterListInBrackets?*/ (Extends qualifiedStaticTypeRef)? (
            Implements interfaceList
        )?
        | Interface identifier /*typeParameterListInBrackets?*/ (Extends interfaceList)?
    ) OpenCurlyBracket classStatement* CloseCurlyBracket
    ;

classEntryType
    : Class
    | Trait
    ;

interfaceList
    : qualifiedStaticTypeRef (',' qualifiedStaticTypeRef)*
    ;

//typeParameterListInBrackets
//    : '<:' typeParameterList ':>'
//    | '<:' typeParameterWithDefaultsList ':>'
//    | '<:' typeParameterList ',' typeParameterWithDefaultsList ':>'
//    ;

typeParameterList
    : typeParameterDecl (',' typeParameterDecl)*
    ;

typeParameterWithDefaultsList
    : typeParameterWithDefaultDecl (',' typeParameterWithDefaultDecl)*
    ;

typeParameterDecl
    : attributes? identifier
    ;

typeParameterWithDefaultDecl
    : attributes? identifier Eq (qualifiedStaticTypeRef | primitiveType)
    ;

//genericDynamicArgs
//    : '<:' typeRef (',' typeRef)* ':>'
//    ;

attributes
    : attributeGroup+
    ;

attributeGroup
    : AttributeStart (identifier ':')? attribute (',' attribute)* ']'
    ;

attribute
    : qualifiedNamespaceName arguments?
    ;

innerStatementList
    : innerStatement*
    ;

innerStatement
    : statement
    | functionDeclaration
    | classDeclaration
    ;

// Statements
labelStatement: Label ':';

statement
    : labelStatement
    | blockStatement
    | ifStatement
    | whileStatement
    | doWhileStatement
    | forStatement
    | switchStatement
    | breakStatement
    | continueStatement
    | returnStatement
    | yieldExpression SemiColon
    | globalStatement
    | staticVariableStatement
    | echoStatement
    | expressionStatement
    | unsetStatement
    | foreachStatement
    | tryCatchFinally
    | throwStatement
    | gotoStatement
    | declareStatement
    | emptyStatement_
    | inlineHtmlStatement
    ;

emptyStatement_
    : SemiColon
    ;

blockStatement
    : OpenCurlyBracket innerStatementList CloseCurlyBracket
    ;

ifStatement
    : If parentheses statement elseIfStatement* elseStatement?
    | If parentheses ':' innerStatementList elseIfColonStatement* elseColonStatement? EndIf SemiColon
    ;

elseIfStatement
    : ElseIf parentheses statement
    ;

elseIfColonStatement
    : ElseIf parentheses ':' innerStatementList
    ;

elseStatement
    : Else statement
    ;

elseColonStatement
    : Else ':' innerStatementList
    ;

whileStatement
    : While parentheses (statement | ':' innerStatementList EndWhile SemiColon)
    ;

doWhileStatement
    : Do statement While parentheses SemiColon
    ;

forStatement
    : For '(' forInit? SemiColon expressionList? SemiColon forUpdate? ')' (
        statement
        | ':' innerStatementList EndFor SemiColon
    )
    ;

forInit
    : expressionList
    ;

forUpdate
    : expressionList
    ;

switchStatement
    : Switch parentheses (
        OpenCurlyBracket SemiColon? (switchCaseBlock | switchDefaultBlock)* CloseCurlyBracket
        | ':' SemiColon? (switchCaseBlock | switchDefaultBlock)* EndSwitch SemiColon
    )
    ;
switchCaseBlock
    : Case expression  (':' | SemiColon) SemiColon* innerStatementList
    ;
switchDefaultBlock
    : Default (':' | SemiColon) SemiColon* innerStatementList
    ;


switchBlock
    : (((Case expression) | Default) (':' | SemiColon) SemiColon*)+ innerStatementList
    ;

breakStatement
    : Break expression? SemiColon
    ;

continueStatement
    : Continue expression? SemiColon
    ;

returnStatement
    : Return expression? SemiColon
    ;

expressionStatement
    : expression SemiColon
    ;

unsetStatement
    : Unset '(' chainList ')' SemiColon
    ;

foreachStatement
    : Foreach (
        '(' expression As arrayDestructuring ')'
        | '(' chain As '&'? assignable ('=>' '&'? chain)? ')'
        | '(' expression As assignable ('=>' '&'? chain)? ')'
        | '(' chain As List '(' assignmentList ')' ')'
    ) (statement | ':' innerStatementList EndForeach SemiColon)
    ;

tryCatchFinally
    : Try blockStatement (catchClause+ finallyStatement? | catchClause* finallyStatement)
    ;

catchClause
    : Catch '(' qualifiedStaticTypeRef ('|' qualifiedStaticTypeRef)* VarName? ')' blockStatement
    ;

finallyStatement
    : Finally blockStatement
    ;

throwStatement
    : Throw expression SemiColon
    ;

gotoStatement
    : Goto identifier SemiColon
    ;

declareStatement
    : Declare '(' declareList ')' (statement | ':' innerStatementList EndDeclare SemiColon)
    ;

inlineHtmlStatement
    : inlineHtml+
    ;

declareList
    : directive (',' directive)*
    ;

directive
    : Ticks Eq (numericConstant | Real)
    | Encoding Eq SingleQuoteString
    | StrictTypes Eq numericConstant
    ;

formalParameterList
    : formalParameter? (',' formalParameter)* ','?
    ;

formalParameter
    : attributes? memberModifier* QuestionMark? typeHint? '&'? '...'? variableInitializer
    ;

typeHint
    : Callable
    | primitiveType
    | qualifiedStaticTypeRef
    | typeHint '|' typeHint
    ;

globalStatement
    : Global globalVar (',' globalVar)* SemiColon
    ;

globalVar
    : VarName
    | Dollar chain
    | Dollar OpenCurlyBracket expression CloseCurlyBracket
    ;

echoStatement
    : Echo expressionList SemiColon
    ;

staticVariableStatement
    : Static variableInitializer (',' variableInitializer)* SemiColon
    ;

classStatement
    : Use qualifiedNamespaceNameList traitAdaptations #TraitUse
    | attributes? propertyModifiers typeHint? variableInitializer (',' variableInitializer)* SemiColon  #propertyModifiersVariable
    | attributes? memberModifiers? Const typeHint? identifierInitializer (',' identifierInitializer)* SemiColon #Const
    | attributes? memberModifiers? Function_ '&'? identifier /*typeParameterListInBrackets?*/ '(' formalParameterList ')' (baseCtorCall | returnTypeDecl)? methodBody #Function
    
    ;

traitAdaptations
    : SemiColon
    | OpenCurlyBracket traitAdaptationStatement* CloseCurlyBracket
    ;

traitAdaptationStatement
    : traitPrecedence
    | traitAlias
    ;

traitPrecedence
    : qualifiedNamespaceName '::' identifier InsteadOf qualifiedNamespaceNameList SemiColon
    ;

traitAlias
    : traitMethodReference As (memberModifier | memberModifier? identifier) SemiColon
    ;

traitMethodReference
    : (qualifiedNamespaceName '::')? identifier
    ;

baseCtorCall
    : ':' identifier arguments?
    ;

returnTypeDecl
    : ':' QuestionMark? typeHint
    ;

methodBody
    : SemiColon
    | blockStatement
    ;

propertyModifiers
    : memberModifiers
    | Var
    ;

memberModifiers
    : memberModifier+
    ;

variableInitializer
    : VarName (Eq constantInitializer)?
    ;

identifierInitializer
    : identifier Eq constantInitializer
    ;

globalConstantDeclaration
    : attributes? Const identifierInitializer (',' identifierInitializer)* SemiColon
    ;

enumDeclaration
    : Enum_ identifier (Colon (IntType | StringType))? (Implements interfaceList)? OpenCurlyBracket enumItem* CloseCurlyBracket
    ;

enumItem
    : Case identifier (Eq expression)? SemiColon
    | memberModifiers? functionDeclaration
    | Use qualifiedNamespaceNameList traitAdaptations
    ;

expressionList
    : expression (',' expression)*
    ;

parentheses
    : '(' expression ')'
    ;

fullyQualifiedNamespaceExpr: '\\'? identifier '\\' (identifier '\\')* identifier;

staticClassExpr
    : staticClassExprFunctionMember
    | staticClassExprVariableMember
    ;

staticClassExprFunctionMember
    : fullyQualifiedNamespaceExpr '::' identifier # ClassStaticFunctionMember
    | identifier '::' identifier                  # ClassDirectFunctionMember
    | string '::' identifier                      # StringAsIndirectClassStaticFunctionMember
    | variable '::' identifier                    # VariableAsIndirectClassStaticFunctionMember
    ;

staticClassExprVariableMember
    : fullyQualifiedNamespaceExpr '::' flexiVariable    # ClassStaticVariable
    | identifier '::' flexiVariable                     # ClassDirectStaticVariable
    | string '::' flexiVariable                         # StringAsIndirectClassStaticVariable
    | variable '::' flexiVariable                       # VariableAsIndirectClassStaticVariable
    ;


memberCallKey
    : identifier
    | string
    | variable
    | OpenCurlyBracket expression CloseCurlyBracket
    ;

indexMemberCallKey
    : memberCallKey
    | numericConstant
    | expression
    ;

// Expressions
// Grouped by priorities: http://php.net/manual/en/language.operators.precedence.php
expression
    : Clone expression                                            # CloneExpression
    | newExpr                                                     # KeywordNewExpression
    | fullyQualifiedNamespaceExpr                                 # FullyQualifiedNamespaceExpression
    | expression ObjectOperator memberCallKey                               #MemerCallExpression
    | expression '[' indexMemberCallKey ']'                       # IndexCallExpression
    | expression ObjectOperator?  OpenCurlyBracket indexMemberCallKey? CloseCurlyBracket  # IndexLegacyCallExpression
    | expression arguments                                        # FunctionCallExpression
    | identifier                                                  # ShortQualifiedNameExpression
    | '\\' identifier                                             # ShortQualifiedNameExpression
    | '\\'? staticClassExpr                                        # StaticClassAccessExpression
    | '&'? flexiVariable                                          # VariableExpression
    | arrayCreation                                               # ArrayCreationExpression
    | constant                                                    # ScalarExpression
    | string                                                      # ScalarExpression
    | defineExpr                                                  # DefinedOrScanDefinedExpression
    | Print expression                                            # PrintExpression
    | Label                                                       # ScalarExpression
    | BackQuoteString                                             # BackQuoteStringExpression
    | '(' expression ')'                                          # ParenthesisExpression
    | include                                                     # IncludeExpression
    | Set_Include_Path expression                                 # IncludeExpression
    | Yield                                                       # SpecialWordExpression
    | List '(' assignmentList ')' Eq expression                   # SpecialWordExpression
    | IsSet '(' chainList ')'                                     # SpecialWordExpression
    | Empty '(' chain ')'                                         # SpecialWordExpression
    | (Exit|Die)  ('(' expression? ')')?                          # SpecialWordExpression
    | (Eval|Assert) expression                                    # CodeExecExpression
    | Throw expression                                            # SpecialWordExpression
    | lambdaFunctionExpr                                          # LambdaFunctionExpression
    | matchExpr                                                   # MatchExpression
    | '(' castOperation ')' expression                            # CastExpression
    | ('~' | '@') expression                                      # UnaryOperatorExpression
    | ('!' | '+' | '-') expression                                # UnaryOperatorExpression
    | ('++' | '--') flexiVariable                                      # PrefixIncDecExpression
    | flexiVariable ('++' | '--')                                      # PostfixIncDecExpression
    | <assoc = right> expression op = '**' expression             # ArithmeticExpression
    | expression InstanceOf typeRef                               # InstanceOfExpression
    | expression op = ('*' | Divide | '%') expression             # ArithmeticExpression
    | expression op = ('+' | '-' | '.') expression                # ArithmeticExpression
    | expression op = ('<<' | '>>') expression                    # ComparisonExpression
    | expression op = (Less | '<=' | Greater | '>=') expression   # ComparisonExpression
    | expression op = ('===' | '!==' | '==' | IsNotEq) expression # ComparisonExpression
    | expression op = '&' expression                              # BitwiseExpression
    | expression op = '^' expression                              # BitwiseExpression
    | expression op = '|' expression                              # BitwiseExpression
    | expression op = '&&' expression                             # BitwiseExpression
    | expression op = '||' expression                             # BitwiseExpression
    | expression op = QuestionMark expression? ':' expression     # ConditionalExpression
    | expression op = '??' expression                             # NullCoalescingExpression
    | expression op = '<=>' expression                            # SpaceshipExpression
    //  assign 
    | leftArrayCreation Eq expression                             # ArrayCreationUnpackExpression
    | staticClassExprVariableMember assignmentOperator expression               # StaticClassMemberCallAssignmentExpression
    | flexiVariable assignmentOperator expression                  # OrdinaryAssignmentExpression
    // logical 
    | expression op = LogicalAnd expression                       # LogicalExpression
    | expression op = LogicalXor expression                       # LogicalExpression
    | expression op = LogicalOr expression                        # LogicalExpression
    | DoubleQuote OpenCurlyBracket expression CloseCurlyBracket DoubleQuote #TemplateExpression
    ;


//即能当左值又能当右值
flexiVariable
    : variable                                    #CustomVariable
    | flexiVariable '[' indexMemberCallKey? ']'    #IndexVariable
    | flexiVariable OpenCurlyBracket indexMemberCallKey? CloseCurlyBracket    # IndexLegacyCallVariable
    | flexiVariable ObjectOperator memberCallKey            #MemberVariable
    ;

defineExpr
    : Define '(' constantString ',' expression ')'
    | Defined '(' constantString ')'
    ;
variable
    : VarName                                               # NormalVariable// $a=3
    | Dollar+ VarName                                       # DynamicVariable// $$a= 1; or $$$a=1;
    | Dollar+ OpenCurlyBracket expression CloseCurlyBracket # MemberCallVariable// ${ expr }=3
    ;

include
    :(Include | IncludeOnce | Require | RequireOnce) expression
    ;

leftArrayCreation // PHP7.1+
    : identifier '(' arrayItemList? ')'
    | arrayDestructuring
    ;

assignable
    : chain
    | arrayCreation
    ;

arrayCreation
    : Array '(' arrayItemList? ')'
    | List '(' arrayItemList? ')'
    | '[' arrayItemList? ']'
    ;

arrayDestructuring
    : '[' ','* indexedDestructItem (','+ indexedDestructItem)* ','* ']'
    | '[' keyedDestructItem (','+ keyedDestructItem)* ','? ']'
    ;

indexedDestructItem
    : '&'? chain
    ;

keyedDestructItem
    : (expression '=>')? '&'? chain
    ;

lambdaFunctionExpr
    : Static? Function_ '&'? '(' formalParameterList ')' lambdaFunctionUseVars? (':' typeHint)? blockStatement
    | LambdaFn '(' formalParameterList ')' '=>' expression
    ;

matchExpr
    : Match_ '(' expression ')' OpenCurlyBracket matchItem (',' matchItem)* ','? CloseCurlyBracket
    ;

matchItem
    : expression (',' expression)* '=>' expression
    ;

newExpr
    : New anonymousClass
    | New typeRef arguments?
    ;

assignmentOperator
    : Eq
    | '+='
    | '-='
    | '*='
    | '**='
    | '/='
    | '.='
    | '%='
    | '&='
    | '|='
    | '^='
    | '<<='
    | '>>='
    | '??=' //高版本引入 7.4
    ;

yieldExpression
    : Yield (expression ('=>' expression)? | From expression)
    ;

arrayItemList
    : arrayItem (',' arrayItem)* ','?
    ;

arrayItem
    : expression ('=>' expression)?
    | (expression '=>')? '&' chain
    ;

lambdaFunctionUseVars
    : Use '(' lambdaFunctionUseVar (',' lambdaFunctionUseVar)* ')'
    ;

lambdaFunctionUseVar
    : '&'? VarName
    ;

qualifiedStaticTypeRef
    : qualifiedNamespaceName // genericDynamicArgs?
    | Static
    ;

typeRef
    : (qualifiedNamespaceName | indirectTypeRef) // genericDynamicArgs?
    | primitiveType
    | Static
    | anonymousClass
    ;

anonymousClass
    : attributes? Private? modifier? Partial? (
        classEntryType /*typeParameterListInBrackets?*/ (Extends qualifiedStaticTypeRef)? (
            Implements interfaceList
        )?
        | Interface identifier /*typeParameterListInBrackets?*/ (Extends interfaceList)?
    ) arguments? OpenCurlyBracket classStatement* CloseCurlyBracket
    ;

indirectTypeRef
    : chainBase (ObjectOperator keyedFieldName)*
    ;

qualifiedNamespaceName
    : Namespace? '\\'? namespaceNameList
    ;

namespaceNameList
    : identifier
    | identifier ('\\' identifier)+ ('\\' namespaceNameTail)?
    ;

namespaceNameTail
    : identifier (As identifier)?
    | OpenCurlyBracket namespaceNameTail (',' namespaceNameTail)* ','? CloseCurlyBracket
    ;

qualifiedNamespaceNameList
    : qualifiedNamespaceName (',' qualifiedNamespaceName)*
    ;

arguments
    : '(' actualArgument? (',' actualArgument)*  ','? ')'
    ;

actualArgument
    : argumentName? '...'? expression
    |  OpenCurlyBracket flexiVariable CloseCurlyBracket
    | '&' chain
    ;

argumentName
    : identifier ':'
    ;

constantInitializer
    : constantString ('.' constantString)*
    | Array '(' (arrayItemList ','?)? ')'
    | '[' (arrayItemList ','?)? ']'
    | ('+' | '-') constantInitializer
    | expression
    ;

constantString: string | constant;


constant
    : Null
    | literalConstant
    | magicConstant
    ;

literalConstant
    : Real
    | BooleanConstant
    | numericConstant
    | stringConstant
    ;

numericConstant
    : Octal
    | Decimal
    | Hex
    | Binary
    ;

classConstant
    : (Class | Parent_) '::' (identifier | Constructor | Get | Set)
    | (qualifiedStaticTypeRef | keyedVariable | string) '::' (
        identifier
        | keyedVariable
    ) // 'foo'::$bar works in php7
    ;

stringConstant
    : Label
    ;

string
    : StartNowDoc HereDocIdentiferName HereDocIdentifierBreak hereDocContent? EndDoc
    | SingleQuoteString
    | DoubleQuote interpolatedStringPart* DoubleQuote
    ;

hereDocContent: HereDocText+;

interpolatedStringPart
    : StringPart
    | UnicodeEscape
    | chain
    ;

chainList
    : chain (',' chain)*
    ;

chain
    : chainOrigin memberAccess*
    | arrayCreation // [$a,$b]=$c
    ;

chainOrigin
    : chainBase
    | functionCall
    | '(' newExpr ')'
    ;

memberAccess
    : ObjectOperator keyedFieldName actualArguments?
    ;

functionCall
    : functionCallName actualArguments
    ;

functionCallName
    : qualifiedNamespaceName
    | classConstant
    | chainBase
    | parentheses
    | Label
    ;

actualArguments
//    : genericDynamicArgs? arguments+ squareCurlyExpression*
    : arguments+ squareCurlyExpression*
    ;

chainBase
    : keyedVariable ('::' keyedVariable)?
    | qualifiedStaticTypeRef '::' keyedVariable
    ;

keyedFieldName
    : keyedSimpleFieldName
    | keyedVariable
    ;

keyedSimpleFieldName
    : (identifier | OpenCurlyBracket expression CloseCurlyBracket) squareCurlyExpression*
    ;

keyedVariable
    : Dollar* (VarName | Dollar OpenCurlyBracket expression CloseCurlyBracket) squareCurlyExpression*
    ;

squareCurlyExpression
    : '[' expression? ']'
    | OpenCurlyBracket expression CloseCurlyBracket
    ;

assignmentList
    : assignmentListElement? (',' assignmentListElement?)*
    ;

assignmentListElement
    : chain
    | List '(' assignmentList ')'
    | arrayItem
    ;

modifier
    : Abstract
    | Final
    ;

identifier
    : Label
    | key
    ;

key
    : Abstract
    | Array
    | As
    | BinaryCast
    | BoolType
    | BooleanConstant
    | Break
    | Callable
    | Case
    | Catch
    | Class
    | Clone
    | Const
    | Continue
    | Declare
    | Default
    | Do
    | DoubleCast
    | DoubleType
    | Echo
    | Else
    | ElseIf
    | Empty
    | EndDeclare
    | EndFor
    | EndForeach
    | EndIf
    | EndSwitch
    | EndWhile
//    | Eval
//    | Exit
    | Extends
    | Final
    | Finally
    | FloatCast
    | For
    | Foreach
    | Function_
    | Global
    | Goto
    | If
    | Implements
    | Import
//    | Include
//    | IncludeOnce
    | InstanceOf
    | InsteadOf
    | Int16Cast
    | Int64Type
    | Int8Cast
    | Interface
    | IntType
//    | IsSet
    | LambdaFn
    | List
    | LogicalAnd
    | LogicalOr
    | LogicalXor
    | Namespace
    | New
    | Null
    | ObjectType
    | Parent_
    | Partial
    | Print
    | Private
    | Protected
    | Public
    | Readonly
//    | Require
//    | RequireOnce
    | Resource
    | Return
    | Static
    | StringType
    | Switch
//    | Throw
    | Trait
    | Try
    | Typeof
    | UintCast
    | UnicodeCast
//    | Unset
    | Use
    | Var
    | While
    | Yield
    | From
    | Enum_
    | Match_
    | Ticks
    | Encoding
    | StrictTypes
    | Get
    | Set
    | Call
    | CallStatic
    | Constructor
    | Destruct
    | Wakeup
    | Sleep
    | Autoload
    | IsSet__
    | Unset__
    | ToString__
    | Invoke
    | SetState
    | Clone__
    | DebugInfo
    | Namespace__
    | Class__
    | Traic__
    | Function__
    | Method__
    | Line__
    | File__
    | Dir__
    ;

memberModifier
    : Public
    | Protected
    | Private
    | Static
    | Abstract
    | Final
    | Readonly
    ;

magicConstant
    : Namespace__
    | Class__
    | Traic__
    | Function__
    | Method__
    | Line__
    | File__
    | Dir__
    ;

magicMethod
    : Get
    | Set
    | Call
    | CallStatic
    | Constructor
    | Destruct
    | Wakeup
    | Sleep
    | Autoload
    | IsSet__
    | Unset__
    | ToString__
    | Invoke
    | SetState
    | Clone__
    | DebugInfo
    ;

primitiveType
    : BoolType
    | IntType
    | Int64Type
    | DoubleType
    | StringType
    | Resource
    | ObjectType
    | Array
    ;

castOperation
    : BoolType
    | Int8Cast
    | Int16Cast
    | IntType
    | Int64Type
    | UintCast
    | DoubleCast
    | DoubleType
    | FloatCast
    | StringType
    | BinaryCast
    | UnicodeCast
    | Array
    | ObjectType
    | Resource
    | Unset
    ;