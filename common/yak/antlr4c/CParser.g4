/*
 [The "BSD licence"]
 Copyright (c) 2013 Sam Harwell
 All rights reserved.
 
 Redistribution and
 use
 in source and binary forms, with or without
 modification, are permitted provided that the
 following conditions
 are met:
 1. Redistributions of source code must retain the above copyright
 notice, this list of conditions and the following disclaimer.
 2. Redistributions in binary form
 must reproduce the above copyright
 notice, this list of conditions and the following disclaimer
 in
 the
 documentation and/or other materials provided with the distribution.
 3. The name of the
 author may not be used to endorse or promote products
 derived from this software without specific
 prior written permission.
 
 THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY EXPRESS OR
 IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
 OF MERCHANTABILITY AND
 FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
 IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY
 DIRECT, INDIRECT,
 INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
 NOT
 LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 DATA, OR PROFITS; OR
 BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT
 LIABILITY, OR TORT
 (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
 THIS
 SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
 */

/** C 2011 grammar built from the C11 Spec */

// $antlr-format alignTrailingComments true, columnLimit 150, minEmptyLines 1, maxEmptyLinesToKeep 1, reflowComments false, useTab false
// $antlr-format allowShortRulesOnASingleLine false, allowShortBlocksOnASingleLine true, alignSemicolons hanging, alignColons hanging

parser grammar CParser;

// ANTLR-friendly C parser grammar

options {
    tokenVocab = CLexer;
}

// --- Primary Expressions ---
primaryExpression
    : Identifier
    | Constant
    | StringLiteral+
    | '(' expression ')'
    | genericSelection
    | '__extension__'? '(' compoundStatement ')'
    | '__builtin_va_arg' '(' unaryExpression ',' typeName ')'
    | '__builtin_offsetof' '(' typeName ',' unaryExpression ')'
    ;

genericSelection
    : '_Generic' '(' assignmentExpression ',' genericAssocList ')'
    ;

genericAssocList
    : genericAssociation (',' genericAssociation)*
    ;

genericAssociation
    : (typeName | 'default') ':' assignmentExpression
    ;

// --- Postfix/Unary/Cast Expressions ---
postfixExpression
    : (primaryExpression | '__extension__'? '(' typeName ')' '{' initializerList ','? '}')
    | postfixExpression '[' expression ']'
    | postfixExpression '(' argumentExpressionList? ')'
    | postfixExpression ('.' | '->') Identifier
    | postfixExpression '++'
    | postfixExpression '--'
    ;

argumentExpressionList
    : expression (','? expression)*
    ;

unaryExpression
    : ('++' | '--' | '*') unaryExpression
    | ('sizeof' | '_Alignof') '(' '*'* typeName ')'
    | '&&' unaryExpression
    | postfixExpression
    ;

castExpression
    : '__extension__'? '(' typeName ')' castExpression
    | unaryExpression
    | DigitSequence
    ;

// --- Binary Expressions ---
assignmentExpression
    : castExpression (assignmentOperator initializer)?
    | DigitSequence
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
    | '&='
    | '^='
    | '|='
    ;

expressionList
    : expression (',' expression)*
    ;

statementsExpression
    : '(' '{' statement* expression? ';'? '}' ')'
    ;

expression
    : unary_op = (Tilde | Plus | Minus | Not | Caret | Star | And) expression
    | expression mul_op = (Star | Div | Mod | LeftShift | RightShift | And) expression
    | expression add_op = (Plus | Minus | Or | Caret) expression
    | expression rel_op = (Equal | NotEqual | Less | LessEqual | Greater | GreaterEqual) expression
    | expression AndAnd expression
    | expression OrOr expression
    | '(' expression ')'
    | expression ('?' expression ':' expression)
    | castExpression
    | assignmentExpression
    | statementsExpression
    | declarationSpecifier
    ;

// --- Declarations ---
declaration
    : declarationSpecifier initDeclaratorList? ';'
    | staticAssertDeclaration
    ;

declarationSpecifiers
    : declarationSpecifier (',' declarationSpecifier)?
    ;

declarationSpecifiers2
    : declarationSpecifier (',' declarationSpecifier)?
    ;

declarationSpecifier
    : (storageClassSpecifier | typeQualifier | functionSpecifier)* structOrUnion? (
        typeSpecifier
        | Identifier
    ) '*'*
    | alignmentSpecifier
    ;

initDeclaratorList
    : initDeclarator (',' initDeclarator)*
    ;

initDeclarator
    : declarator ('=' initializer)?
    ;

storageClassSpecifier
    : 'typedef'
    | 'extern'
    | 'static'
    | '_Thread_local'
    | 'auto'
    | 'register'
    ;

typeSpecifier
    : 'void'
    | 'char'
    | 'short'
    | 'int'
    | 'long'
    | 'float'
    | 'double'
    | '_Bool'
    | '_Complex'
    | '__m128'
    | '__m128d'
    | '__m128i'
    | '__extension__' '(' ('__m128' | '__m128d' | '__m128i') ')'
    | atomicTypeSpecifier
    | structOrUnionSpecifier
    | enumSpecifier
    | typedefName
    | '__typeof__' '(' expression ')'
    ;

structOrUnionSpecifier
    : structOrUnion Identifier? '{' structDeclarationList '}' Identifier?
    ;

structOrUnion
    : 'struct'
    | 'union'
    ;

structDeclarationList
    : structDeclaration+
    ;

structDeclaration
    : specifierQualifierList structDeclaratorList ';'
    | specifierQualifierList ';'
    | staticAssertDeclaration
    ;

specifierQualifierList
    : (typeSpecifier | typeQualifier) specifierQualifierList?
    ;

structDeclaratorList
    : structDeclarator (',' structDeclarator)*
    ;

structDeclarator
    : declarator
    | declarator? ':' expression
    ;

enumSpecifier
    : 'enum' Identifier? '{' enumeratorList ','? '}'
    | 'enum' Identifier
    ;

enumeratorList
    : enumerator (',' enumerator)*
    ;

enumerator
    : Identifier ('=' expression)?
    ;

atomicTypeSpecifier
    : '_Atomic' '(' typeName ')'
    ;

typeQualifier
    : 'const'
    | 'restrict'
    | 'volatile'
    | '_Atomic'
    | 'signed'
    | 'unsigned'
    ;

functionSpecifier
    : 'inline'
    | '_Noreturn'
    | '__inline__'
    | '__stdcall'
    | gccAttributeSpecifier
    | '__declspec' '(' Identifier ')'
    ;

alignmentSpecifier
    : '_Alignas' '(' (typeName | expression) ')'
    ;

declarator
    : pointer? directDeclarator gccDeclaratorExtension*
    ;

directDeclarator
    : Identifier
    | '(' declarator ')'
    | directDeclarator '[' typeQualifierList? assignmentExpression? ']'
    | directDeclarator '[' 'static' typeQualifierList? assignmentExpression ']'
    | directDeclarator '[' typeQualifierList 'static' assignmentExpression ']'
    | directDeclarator '[' typeQualifierList? '*' ']'
    | directDeclarator '(' parameterTypeList ')'
    | directDeclarator '(' identifierList? ')'
    | Identifier ':' DigitSequence
    | vcSpecificModifer Identifier
    | '(' vcSpecificModifer declarator ')'
    ;

vcSpecificModifer
    : '__cdecl'
    | '__clrcall'
    | '__stdcall'
    | '__fastcall'
    | '__thiscall'
    | '__vectorcall'
    ;

gccDeclaratorExtension
    : Asm
    | gccAttributeSpecifier
    | Identifier ('(' gccAttributeList ')')?
    ;

gccAttributeSpecifier
    : '__attribute__' '(' '(' gccAttributeList ')' ')'
    ;

gccAttributeList
    : gccAttribute? (',' gccAttribute?)*
    ;

gccAttribute
    : ~(',' | '(' | ')') ('(' argumentExpressionList? ')')?
    ;

pointer
    : (('*' | '^') typeQualifierList?)+
    ;

typeQualifierList
    : typeQualifier+
    ;

parameterTypeList
    : parameterList (',' '...'?)?
    ;

parameterList
    : parameterDeclaration (',' parameterDeclaration)*
    ;

parameterDeclaration
    : declarationSpecifier declarator
    | declarationSpecifier abstractDeclarator?
    ;

identifierList
    : Identifier (',' Identifier)*
    ;

typeName
    : specifierQualifierList abstractDeclarator?
    ;

abstractDeclarator
    : pointer
    | pointer? directAbstractDeclarator gccDeclaratorExtension*
    ;

directAbstractDeclarator
    : '(' abstractDeclarator ')' gccDeclaratorExtension*
    | '[' typeQualifierList? assignmentExpression? ']'
    | '[' 'static' typeQualifierList? assignmentExpression ']'
    | '[' typeQualifierList 'static' assignmentExpression ']'
    | '[' '*' ']'
    | '(' parameterTypeList? ')' gccDeclaratorExtension*
    | directAbstractDeclarator '[' typeQualifierList? assignmentExpression? ']'
    | directAbstractDeclarator '[' 'static' typeQualifierList? assignmentExpression ']'
    | directAbstractDeclarator '[' typeQualifierList 'static' assignmentExpression ']'
    | directAbstractDeclarator '[' '*' ']'
    | directAbstractDeclarator '(' parameterTypeList? ')' gccDeclaratorExtension*
    ;

typedefName
    : structOrUnion? Identifier
    ;

initializer
    : expression
    | '{' initializerList? ','? '}'
    ;

initializerList
    : designation? initializer (',' designation? initializer)*
    ;

designation
    : designatorList '='
    ;

designatorList
    : designator+
    ;

designator
    : '[' expression ']'
    | '.' Identifier
    ;

staticAssertDeclaration
    : '_Static_assert' '(' expression ',' StringLiteral+ ')' ';'
    ;

// --- Statements ---
statement
    : Identifier ':' statement?
    | compoundStatement
    | expressionStatement
    | statementsExpression
    | selectionStatement
    | iterationStatement
    | jumpStatement
    | asmStatement
    | ';'
    ;

asmStatement
    : Asm ('volatile' | '__volatile__')? '(' asmExprList? (
        ':' asmExprList? (':' asmExprList? (':' asmExprList?)?)?
    )? ')' ';'
    ;

asmExprList
    : expression (',' expression)*
    ;

labeledStatement
    : 'case' expression ':' statement*
    | 'default' ':' statement*
    ;

compoundStatement
    : '{' blockItemList? '}'
    ;

blockItemList
    : blockItem+
    ;

blockItem
    : statement
    | declaration
    ;

expressionStatement
    : assignmentExpressions ';'
    ;

selectionStatement
    : 'if' '(' expression ')' statement ('else' statement)?
    | 'switch' '(' expression ')' '{' labeledStatement* '}'
    ;

iterationStatement
    : 'while' '(' expression ')' ';'? statement
    | 'do' statement 'while' '(' expression ')' ';'?
    | 'for' '(' forCondition ')' statement
    ;

forCondition
    : (forDeclarations | assignmentExpressions?) ';' forExpression? ';' forExpression?
    ;

assignmentExpressions
    : assignmentExpression (',' assignmentExpression)*
    ;

forDeclarations
    : forDeclaration (',' forDeclaration)*
    ;

forDeclaration
    : declarationSpecifier initDeclaratorList?
    ;

forExpression
    : expression (',' expression)*
    ;

jumpStatement
    : ('goto' '*'* Identifier | 'continue' | 'break' | 'return' expression?) ';'
    ;

compilationUnit
    : translationUnit? EOF
    ;

translationUnit
    : externalDeclaration+
    ;

externalDeclaration
    : declarationSpecifier
    | functionDefinition
    | declaration
    | ';'
    ;

functionDefinition
    : declarationSpecifier? declarator declarationList? compoundStatement?
    ;

declarationList
    : declaration+
    ;