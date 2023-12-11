/*
 * The MIT License (MIT)
 *
 * Copyright (c) 2014 by Bart Kiers (original author) and Alexandre Vitorelli (contributor -> ported to CSharp)
 * Copyright (c) 2017-2020 by Ivan Kochurkin (Positive Technologies):
    added ECMAScript 6 support, cleared and transformed to the universal grammar.
 * Copyright (c) 2018 by Juan Alvarez (contributor -> ported to Go)
 * Copyright (c) 2019 by Student Main (contributor -> ES2020)
 *
 * Permission is hereby granted, free of charge, to any person
 * obtaining a copy of this software and associated documentation
 * files (the "Software"), to deal in the Software without
 * restriction, including without limitation the rights to use,
 * copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the
 * Software is furnished to do so, subject to the following
 * conditions:
 *
 * The above copyright notice and this permission notice shall be
 * included in all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
 * EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
 * OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
 * NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
 * HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
 * WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
 * FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
 * OTHER DEALINGS IN THE SOFTWARE.
 */
parser grammar JavaScriptParser;

// Insert here @header for C++ parser.

options {
    tokenVocab=JavaScriptLexer;
    superClass=JavaScriptParserBase;
}

program
    : HashBangLine? statements? EOF
    ;
statements: statement+;
statement
    : block
    | variableStatement
    | importStatement
    | exportStatement
    | ';'
    | classDeclaration
    | functionDeclaration
    | expressionSequence eos
    | ifStatement
    | iterationStatement
    | continueStatement
    | breakStatement
    | returnStatement
    | yieldStatement
    | withStatement
    | labelledStatement
    | switchStatement
    | throwStatement
    | tryStatement
    | debuggerStatement
    ;

block: '{' statements? '}';

importStatement
    : Import importFromBlock
    ;

importFromBlock
    : importDefault? (importNamespace | importModuleItems) importFrom eos
    | StringLiteral eos
    ;

importModuleItems
    : '{' (importAliasName ',')* (importAliasName ','?)? '}'
    ;

importAliasName
    : moduleExportName (As importedBinding)?
    ;

moduleExportName
    : identifierName
    | StringLiteral
    ;

// yield and await are permitted as BindingIdentifier in the grammar
importedBinding
    : Identifier
    | Yield
    | Await
    ;

importDefault
    : aliasName ','
    ;

importNamespace
    : ('*' | identifierName) (As identifierName)?
    ;

importFrom
    : From StringLiteral
    ;

aliasName
    : identifierName (As identifierName)?
    ;

exportStatement
    : Export Default? (exportFromBlock | declaration) eos    # ExportDeclaration
    | Export Default singleExpression eos                    # ExportDefaultDeclaration
    ;

exportFromBlock
    : importNamespace importFrom eos
    | exportModuleItems importFrom? eos
    ;

exportModuleItems
    : '{' (exportAliasName ',')* (exportAliasName ','?)? '}'
    ;

exportAliasName
    : moduleExportName (As moduleExportName)?
    ;

declaration
    : variableStatement
    | classDeclaration
    | functionDeclaration
    ;

variableStatement
    : variableDeclarationList eos
    ;

variableDeclarationList
    : modifier = (Var | Const | NonStrictLet | StrictLet)  variableDeclaration (',' variableDeclaration)*
    ;

variableDeclaration
    : assignable ('=' singleExpression)? // ECMAScript 6: Array & Object Matching
    ;

emptyStatement_
    : SemiColon
    ;

ifStatement
    : If '(' expressionSequence ')' statement (Else If '(' expressionSequence ')' statement)* elseBlock?
    ;

elseBlock
    : Else statement
    ;

forFirst
    : expressionSequence | variableDeclarationList
    ;

forSecond
    : expressionSequence
    ;

forThird
    : expressionSequence
    ;

iterationStatement
    : Do statement While '(' expressionSequence ')' eos                                                                       # DoStatement
    | While '(' expressionSequence ')' statement                                                                              # WhileStatement
    | For '(' forFirst? ';' forSecond? ';' forThird? ')' statement   # ForStatement
    | For '(' (singleExpression | variableDeclarationList) In expressionSequence ')' statement                                # ForInStatement
    // strange, 'of' is an identifier. and p.p("of") not work in sometime.
    | For Await? '(' (singleExpression | variableDeclarationList) identifier{p.p("of")}? expressionSequence ')' statement  # ForOfStatement
    ;

continueStatement
    : Continue ({p.notLineTerminator()}? identifier)? eos
    ;

breakStatement
    : Break ({p.notLineTerminator()}? identifier)? eos
    ;

returnStatement
    : Return ({p.notLineTerminator()}? expressionSequence)? eos
    ;

yieldStatement
    : Yield ({p.notLineTerminator()}? expressionSequence)? eos
    ;

withStatement
    : With '(' expressionSequence ')' statement
    ;

switchStatement
    : Switch '(' expressionSequence ')' caseBlock
    ;

caseBlock
    : '{' caseClauses? (defaultClause caseClauses?)? '}'
    ;

caseClauses
    : caseClause+
    ;

caseClause
    : Case expressionSequence ':' statements?
    ;

defaultClause
    : Default ':' statements?
    ;

labelledStatement
    : identifier ':' statement
    ;

throwStatement
    : Throw {p.notLineTerminator()}? expressionSequence eos
    ;

tryStatement
    : Try block (catchProduction finallyProduction? | finallyProduction)
    ;

catchProduction
    : Catch ('(' assignable? ')')? block
    ;

finallyProduction
    : Finally block
    ;

debuggerStatement
    : Debugger eos
    ;

functionDeclaration
    : Async? Function_ '*'? identifier '(' formalParameterList? ')' functionBody
    ;

classDeclaration
    : Class identifier classTail
    ;

classTail
    : (Extends singleExpression)? '{' classElement* '}'
    ;

classElement
    : Static? methodDefinition
    | Static? fieldDefinition
    | Static block
    | ';'
    ;

methodDefinition
    : (Async {p.notLineTerminator()}?)? '*'? classElementName '(' formalParameterList? ')' functionBody
    | '*'? getter '(' ')' functionBody
    | '*'? setter '(' formalParameterList? ')' functionBody
    ;

fieldDefinition
    : classElementName initializer?
    ;

classElementName
    : propertyName
    | privateIdentifier
    ;

privateIdentifier
    : '#' identifierName
    ;

formalParameterList
    : formalParameterArg (',' formalParameterArg)* (',' lastFormalParameterArg)?
    | lastFormalParameterArg
    ;

formalParameterArg
    : assignable ('=' singleExpression)?      // ECMAScript 6: Initialization
    ;

lastFormalParameterArg                        // ECMAScript 6: Rest Parameter
    : Ellipsis singleExpression
    ;

functionBody
    : '{' statements? '}'
    ;

arrayLiteral
    : ('[' elementList ']')
    ;

elementList
    : ','* arrayElement? (','+ arrayElement)* ','* // Yes, everything is optional
    ;

arrayElement
    : Ellipsis? singleExpression
    ;

propertyAssignment
    : propertyName ':' singleExpression                                             # PropertyExpressionAssignment
    | '[' singleExpression ']' ':' singleExpression                                 # ComputedPropertyExpressionAssignment
    | Async? '*'? propertyName '(' formalParameterList?  ')'  functionBody  # FunctionProperty
    | getter '(' ')' functionBody                                           # PropertyGetter
    | setter '(' formalParameterArg ')' functionBody                        # PropertySetter
    | Ellipsis? singleExpression                                                    # PropertyShorthand
    ;

propertyName
    : identifierName
    | StringLiteral
    | numericLiteral
    | '[' singleExpression ']'
    ;

arguments
    : '('(argument (',' argument)* ','?)?')'
    ;

argument
    : Ellipsis? (singleExpression | identifier)
    ;

expressionSequence
//    : singleExpression  ({p.n(",")}? ',' expressionSequence)* // 1.59
    // : singleExpression  (',' expressionSequence)*               // 2.16
    : singleExpression  (',' singleExpression)*               // 0.12
    ;

specificExpression
    : identifierName
    | templateStringLiteral
    | arguments
    ;

questionDot: '?' '.';

keywordSingleExpression
    : Import '(' singleExpression ')'                                       # ImportExpression
    | New singleExpression ('(' (argument (',' argument)* ',')? ')')?       # NewExpression
    | New '.' identifier                                                    # MetaExpression // new.target
    | Await singleExpression                                                # AwaitExpression
    ;

singleExpression
    : keywordSingleExpression                                               # KeywordExpression
    | literal                                                               # LiteralExpression
    | Class identifier? classTail                                           # ClassExpression
    | anonymousFunction                                                     # FunctionExpression
    | singleExpression templateStringLiteral                                # TemplateStringExpression  // ECMAScript 6
    | yieldStatement                                                        # YieldExpression // ECMAScript 6
    | This                                                                  # ThisExpression
    | identifier                                                            # IdentifierExpression
    | Super                                                                 # SuperExpression
    | arrayLiteral                                                          # ArrayLiteralExpression
    | objectLiteral                                                         # ObjectLiteralExpression
    | '(' expressionSequence ')'                                            # ParenthesizedExpression
    | singleExpression questionDot optionalChainMember         # OptionalChainExpression
    | singleExpression '.' '#'? identifierName         # ChainExpression
    | singleExpression '[' singleExpression ']'  # MemberIndexExpression
    // | singleExpression '?'? '.' '#'? identifierName                         # MemberDotExpression
    | singleExpression arguments                                            # ArgumentsExpression

    // suffix self add dec
    | singleExpression {p.notLineTerminator()}? op = ('++' | '--')          # PostUnaryExpression

    // prefix unary
    | preUnaryOperator singleExpression                                     # PreUnaryExpression

    // 二元运算
    | <assoc=right> singleExpression '**' singleExpression                                  # PowerExpression
    | singleExpression op = ('*' | '/' | '%') singleExpression                              # MultiplicativeExpression
    | singleExpression op = ('+' | '-') singleExpression                                    # AdditiveExpression
    | singleExpression op = ('<<' | '>>' | '>>>') singleExpression                          # BitShiftExpression
    | singleExpression op = ('<' | '>' | '<=' | '>=' | In | Instanceof) singleExpression    # RelationalExpression
    | singleExpression op = ('==' | '!=' | '===' | '!==') singleExpression                  # EqExpression
    | singleExpression ('&' | '^' | '|') singleExpression                                   # BitExpression
    | singleExpression '&&' singleExpression                            # LogicalAndExpression
    | singleExpression '||' singleExpression                            # LogicalOrExpression
    | singleExpression '??' singleExpression                            # CoalesceExpression
    | singleExpression '?' singleExpression ':' singleExpression        # TernaryExpression

    // assign
    | <assoc=right> singleExpression assignmentOperator singleExpression    # AssignmentOperatorExpression
    ;


preUnaryOperator
    : ('++' | '--' )
    | ('+' | '-' | '!' | '~')
    | Typeof
    | Void
    | Delete
    ;

initializer
// TODO: must be `= AssignmentExpression` and we have such label alredy but it doesn't respect the specification.
//  See https://tc39.es/ecma262/multipage/ecmascript-language-expressions.html#prod-Initializer
    : '=' singleExpression
    ;

assignable
    : identifier
    | arrayLiteral
    | objectLiteral
    ;

objectLiteral
    : '{' (propertyAssignment (',' propertyAssignment)* ','?)? '}'
    ;

anonymousFunction
    : Async? Function_ '*'? identifier? '(' formalParameterList? ')' functionBody    # AnonymousFunctionDecl
    | Async? arrowFunctionParameters '=>' arrowFunctionBody                    # ArrowFunction
    ;

arrowFunctionParameters
    : identifier
    | '(' formalParameterList? ')'
    ;

arrowFunctionBody
    : functionBody
    | singleExpression
    ;

assignmentOperator
    : '='
    | '*='
    | '/='
    | '%='
    | '+='
    | '-='
    | '<<='
    | '>>='
    | '>>>='
    | '&='
    | '^='
    | '|='
    | '**='
    ;

literal
    : NullLiteral
    | BooleanLiteral
    | StringLiteral
    | templateStringLiteral
    | RegularExpressionLiteral
    | numericLiteral
    | bigintLiteral
    ;

templateStringLiteral
    : BackTick templateStringAtom* BackTick
    ;

templateStringAtom
    : TemplateStringAtom
    | TemplateStringStartExpression singleExpression TemplateCloseBrace
    ;

numericLiteral
    : DecimalLiteral
    | HexIntegerLiteral
    | OctalIntegerLiteral
    | OctalIntegerLiteral2
    | BinaryIntegerLiteral
    ;

bigintLiteral
    : BigDecimalIntegerLiteral
    | BigHexIntegerLiteral
    | BigOctalIntegerLiteral
    | BigBinaryIntegerLiteral
    ;

getter
    : Get classElementName
    ;

setter
    : Set classElementName
    ;

identifierName
    : identifier
    | NullLiteral
    | BooleanLiteral
    | Break
    | Do
    | Instanceof
    | Typeof
    | Case
    | Else
    | New
    | Var
    | Catch
    | Finally
    | Return
    | Void
    | Continue
    | For
    | Switch
    | While
    | Debugger
    | Function_
    | This
    | With
    | Default
    | If
    | Throw
    | Delete
    | In
    | Try
    | Class
    | Enum
    | Extends
    | Super
    | Const
    | Export
    | Import
    | Implements
    | NonStrictLet
    | StrictLet
    | Private
    | Public
    | Interface
    | Package
    | Protected
    | Static
    | Yield
    | Async
    | Await
    | From
    | As
    ;

identifier
    : Identifier
    | NonStrictLet
    | Async
    | As
    | From
    | Get
    | Set
    | Static
    ;

optionalChainMember
    : '#'? identifierName
    | '[' singleExpression ']'
    ;

eos
    : SemiColon
    | {p.isEOS()}?
    | EOF
    ;