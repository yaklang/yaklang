/*
 [The "BSD licence"] Copyright (c) 2013 Sam Harwell All rights reserved.
 
 Redistribution and use
 in source and binary forms, with or without modification, are permitted
 provided that the
 following conditions are met: 1. Redistributions of source code must retain the
 above copyright
 notice, this list of conditions and the following disclaimer. 2. Redistributions in
 binary form
 must reproduce the above copyright notice, this list of conditions and the following
 disclaimer
 in
 the documentation and/or other materials provided with the distribution. 3. The name
 of the
 author
 may not be used to endorse or promote products derived from this software without
 specific
 prior
 written permission.
 
 THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY
 EXPRESS OR
 IMPLIED
 WARRANTIES, INCLUDING,
 BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF
 MERCHANTABILITY AND
 FITNESS
 FOR A PARTICULAR PURPOSE
 ARE DISCLAIMED. IN NO EVENT SHALL THE
 AUTHOR BE LIABLE FOR ANY
 DIRECT,
 INDIRECT, INCIDENTAL,
 SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
 DAMAGES (INCLUDING, BUT NOT
 LIMITED TO,
 PROCUREMENT OF
 SUBSTITUTE GOODS OR SERVICES; LOSS OF
 USE, DATA, OR PROFITS; OR
 BUSINESS
 INTERRUPTION) HOWEVER
 CAUSED AND ON ANY THEORY OF LIABILITY,
 WHETHER IN CONTRACT,
 STRICT
 LIABILITY, OR TORT (INCLUDING
 NEGLIGENCE OR OTHERWISE) ARISING IN
 ANY WAY OUT OF THE USE
 OF THIS
 SOFTWARE, EVEN IF ADVISED OF THE
 POSSIBILITY OF SUCH DAMAGE.
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
    | stringLiteralExpression
    | '(' eos* expression eos* ')'
    | genericSelection
    | '__extension__'? '(' eos* compoundStatement eos* ')'
    | '__builtin_va_arg' '(' eos* unaryExpression eos* ',' eos* typeName eos* ')'
    | '__builtin_offsetof' '(' eos* typeName eos* ',' eos* unaryExpression eos* ')'
    | macroCallExpression  // Support macro calls in expressions (e.g., INTERLEAVE_OUTPUT(16))
    ;

// String literal concatenation: supports both "str1" "str2" and "str" Identifier patterns
// The Identifier pattern handles cases like "%"PRId64 where PRId64 is a macro that expands to a string
// Pattern: StringLiteral followed by zero or more (StringLiteral | Identifier StringLiteral)
// This ensures Identifier is only allowed between two StringLiterals or at the end
stringLiteralExpression
    : StringLiteral (StringLiteral | (Identifier StringLiteral))* Identifier?
    ;

genericSelection
    : '_Generic' '(' eos* assignmentExpression eos* ',' eos* genericAssocList eos* ')'
    ;

genericAssocList
    : genericAssociation (',' eos* genericAssociation)*
    ;

genericAssociation
    : (typeName | 'default') ':' assignmentExpression
    ;

// --- Postfix/Unary/Cast Expressions ---
// Optimized: Use postfixSuffix to support chained operations and reduce recursion depth
postfixExpression
    : (primaryExpression | '__extension__'? '(' typeName ')' '{' initializerList ','? '}') postfixSuffix*
    | leftExpression '++'
    | leftExpression '--'
    ;

// Postfix suffix operations: supports chained operations like func()[0].field
// This reduces recursion depth and improves parsing performance for complex expressions
// Note: Using * quantifier instead of recursion to avoid left recursion issues
postfixSuffix
    : '[' eos* expression eos* ']'
    | '(' eos* argumentExpressionList? eos* ')'
    | ('.' | '->') Identifier
    ;

// Postfix expression that can be used as lvalue (excluding function calls)
postfixExpressionLvalue
    : (primaryExpression | '__extension__'? '(' typeName ')' '{' initializerList ','? '}') postfixSuffixLvalue*
    ;

// Postfix suffix for lvalue expressions (no function calls)
// Note: Using * quantifier instead of recursion to avoid left recursion issues
postfixSuffixLvalue
    : '[' eos* expression eos* ']'
    | ('.' | '->') Identifier
    ;

argumentExpressionList
    : macroArgument (',' eos* macroArgument)*
    ;

// Macro argument: can be an expression, a type name, a single operator, or any token sequence
// This handles cases like:
// - FUN(fmin, double, <) where < is passed as a macro parameter
// - ARRAY_RENAME(3d_array) where 3d_array starts with a digit
// - DECLARE_ALIGNED(SBC_ALIGN, int32_t, name) where int32_t is a type name
macroArgument
    : expression
    | typeName  // Support type names as macro arguments (e.g., int32_t in DECLARE_ALIGNED)
    | DigitSequence Identifier?  // Support identifiers starting with digits (e.g., 3d_array)
    | (Less | Greater | LessEqual | GreaterEqual | Equal | NotEqual | Plus | Minus | Star | Div | Mod | LeftShift | RightShift | And | Or | Caret | AndAnd | OrOr | Tilde | Not | PlusPlus | MinusMinus)
    ;

unaryExpression
    : ('++' | '--') eos* leftExpression
    | '*' eos* unaryExpression
    | '&' eos* leftExpression
    | ('sizeof' | '_Alignof') eos* unaryExpression
    | ('sizeof' | '_Alignof') '(' eos* ('*'* eos* typeName | unaryExpression) eos* ')'
    | '&&' eos* unaryExpression
    | postfixExpression
    ;

castExpression
    : '__extension__'? '(' eos* typeName eos* ')' eos* castExpression
    | unaryExpression
    | DigitSequence
    ;

// --- Binary Expressions ---
assignmentExpression
    : leftExpression eos* assignmentOperator eos* expression
    | castExpression
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
    : expression (',' eos* expression)*
    ;

statementsExpression
    : '(' '{' statement* expression? Semi? '}' ')'
    ;

// --- Left Value Expressions ---
leftExpression
    : '*' eos* unaryExpression
    | '*' eos* castExpression
    | postfixExpressionLvalue
    | '(' eos* leftExpression eos* ')'
    ;

expression
    : unary_op = (Tilde | Plus | Minus | Not | Caret | Star | And) eos* expression
    | expression mul_op = (Star | Div | Mod | LeftShift | RightShift | And) eos* expression
    | expression add_op = (Plus | Minus | Or | Caret) eos* expression
    | expression rel_op = (Equal | NotEqual | Less | LessEqual | Greater | GreaterEqual) eos* expression
    | expression AndAnd eos* expression
    | expression OrOr eos* expression
    | '(' eos* expression eos* ')'
    | expression ('?' eos* expression ':' eos* expression)
    | castExpression
    | assignmentExpression
    | statementsExpression
    | declarationSpecifier
    ;

// --- Declarations ---
declaration
    : declarationSpecifier eos* initDeclaratorList? eos* Semi
    | macroCallExpression declaratorSuffix* eos* ('=' initializer)? eos* Semi  // Support macro calls as declarations, e.g., DECLARE_ALIGNED(...)[8] = {...}
    | staticAssertDeclaration
    ;

declarationSpecifiers
    : declarationSpecifier (',' eos* declarationSpecifier)?
    ;

declarationSpecifiers2
    : declarationSpecifier (',' eos* declarationSpecifier)?
    ;

declarationSpecifier
    : (storageClassSpecifier | typeQualifier | functionSpecifier)* structOrUnion? (
        typeSpecifier
        | Identifier
    ) typeQualifier*
    | alignmentSpecifier
    ;

initDeclaratorList
    : initDeclarator (',' eos* initDeclarator)*
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
    | 'long long'
    | 'float'
    | 'double'
    | 'long double'
    | '_Bool'
    | '_Complex'
    | '__m128'
    | '__m128d'
    | '__m128i'
    | '__extension__' '(' ('__m128' | '__m128d' | '__m128i') ')'
    | 'signed'
    | 'unsigned'
    | atomicTypeSpecifier
    | structOrUnionSpecifier
    | enumSpecifier
    | typedefName
    | '__typeof__' '(' expression ')'
    ;

structOrUnionSpecifier
    : structOrUnion eos* Identifier? '{' eos* structDeclarationList eos* '}' eos* Identifier?
    ;

structOrUnion
    : 'struct'
    | 'union'
    ;

structDeclarationList
    : (structDeclaration ws*)+
    ;

structDeclaration
    : specifierQualifierList eos* structDeclaratorList eos* Semi
    | specifierQualifierList eos* Semi
    | staticAssertDeclaration
    ;

specifierQualifierList
    : (typeSpecifier | typeQualifier) specifierQualifierList?
    ;

structDeclaratorList
    : structDeclarator (',' eos* structDeclarator)*
    ;

structDeclarator
    : declarator
    | declarator? ':' eos* expression
    ;

enumSpecifier
    : 'enum' eos* Identifier? '{' eos* enumeratorList eos* ','? eos* '}'
    | 'enum' eos* Identifier
    ;

enumeratorList
    : enumerator (',' eos* enumerator)*
    ;

enumerator
    : Identifier eos* gccAttributeSpecifier ('=' eos* expression)?
    | Identifier ('=' eos* expression)?
    ;

atomicTypeSpecifier
    : '_Atomic' '(' eos* typeName eos* ')'
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
    : '_Alignas' '(' eos* (typeName | expression) eos* ')'
    ;

declarator
    : pointer? directDeclarator gccDeclaratorExtension*
    ;

// Optimized: Use arraySuffix and functionSuffix to reduce recursion depth
// This significantly improves performance for multi-dimensional arrays like int arr[2][32][32][2]
directDeclarator
    : Identifier declaratorSuffix*
    | macroCallExpression declaratorSuffix*  // Support macro calls as function names, e.g., ARRAY_RENAME(3d_array)(...)
    | '(' eos* declarator eos* ')' declaratorSuffix*
    | Identifier ':' eos* DigitSequence
    | vcSpecificModifer eos* Identifier declaratorSuffix*
    | '(' eos* vcSpecificModifer eos* declarator eos* ')' declaratorSuffix*
    ;

// Declarator suffix: array dimensions or function parameters
// This allows matching multiple array dimensions in one pass, reducing recursion depth
declaratorSuffix
    : arraySuffix
    | functionSuffix
    ;

// Array suffix: matches one or more array dimensions
// Optimized to handle multi-dimensional arrays efficiently
arraySuffix
    : '[' eos* typeQualifierList? eos* expression? eos* ']'
    | '[' eos* 'static' eos* typeQualifierList? eos* expression eos* ']'
    | '[' eos* typeQualifierList eos* 'static' eos* expression eos* ']'
    | '[' eos* typeQualifierList? eos* '*' eos* ']'
    ;

// Function suffix: matches function parameters
functionSuffix
    : '(' eos* parameterTypeList eos* ')'
    | '(' eos* identifierList? eos* ')'
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
    : Attribute__ '(' eos* '(' eos* gccAttributeList? eos* ')' eos* ')'
    ;

gccAttributeList
    : gccAttribute (',' eos* gccAttribute)*
    ;

gccAttribute
    : ~(',' | '(' | ')') ('(' argumentExpressionList? ')')?
    | Identifier ('(' argumentExpressionList? ')')?
    ;

pointer
    : pointerPart+
    ;

pointerPart
    : ('*' | '^') typeQualifierList?
    ;

typeQualifierList
    : typeQualifier+
    ;

parameterTypeList
    : parameterList (',' eos* '...'? eos*)?
    ;

parameterList
    : parameterDeclaration (',' eos* parameterDeclaration)*
    ;

parameterDeclaration
    : declarationSpecifier declarator
    | declarationSpecifier abstractDeclarator?
    ;

identifierList
    : Identifier (',' eos* Identifier)*
    ;

typeName
    : specifierQualifierList abstractDeclarator?
    | typeName ('.' | '->') Identifier
    ;

abstractDeclarator
    : pointer
    | pointer? directAbstractDeclarator gccDeclaratorExtension*
    ;

// Optimized: Use abstractDeclaratorSuffix to reduce recursion depth
directAbstractDeclarator
    : '(' eos* abstractDeclarator eos* ')' gccDeclaratorExtension* abstractDeclaratorSuffix*
    | abstractDeclaratorSuffix+
    ;

// Abstract declarator suffix: array dimensions or function parameters
abstractDeclaratorSuffix
    : abstractArraySuffix
    | abstractFunctionSuffix
    ;

// Abstract array suffix: matches array dimensions in abstract declarators
abstractArraySuffix
    : '[' eos* typeQualifierList? eos* assignmentExpression? eos* ']'
    | '[' eos* 'static' eos* typeQualifierList? eos* assignmentExpression eos* ']'
    | '[' eos* typeQualifierList eos* 'static' eos* assignmentExpression eos* ']'
    | '[' eos* '*' eos* ']'
    ;

// Abstract function suffix: matches function parameters in abstract declarators
abstractFunctionSuffix
    : '(' eos* parameterTypeList? eos* ')' gccDeclaratorExtension*
    ;

typedefName
    : structOrUnion? Identifier
    ;

initializer
    : expression
    | '{' eos* initializerList? eos* ','? eos* '}'
    ;

initializerList
    : designation? initializer (',' eos* designation? initializer)*
    ;

designation
    : designatorList eos* '='
    ;

designatorList
    : designator+
    ;

designator
    : '[' eos* expression eos* ']'
    | '.' eos* Identifier
    ;

staticAssertDeclaration
    : '_Static_assert' eos* '(' eos* expression eos* ',' eos* StringLiteral+ eos* ')' eos* Semi
    ;

// --- Statements ---
statement
    : Identifier ':' eos* statement?  // Labeled statement
    | compoundStatement
    | expressionStatement
    | statementsExpression
    | selectionStatement
    | iterationStatement
    | jumpStatement
    | asmStatement
    | macroCallStatement  // Support macro calls as statements (e.g., FF_DISABLE_DEPRECATION_WARNINGS)
    | Semi
    ;

// Macro call as a statement (identifier without parentheses, possibly followed by semicolon)
// This handles cases like FF_DISABLE_DEPRECATION_WARNINGS which are macros that expand to nothing or pragmas
// Note: These macros typically don't have semicolons and are followed by other statements
macroCallStatement
    : Identifier eos*
    ;

asmStatement
    : Asm eos* ('volatile' | '__volatile__')? eos* '(' eos* asmExprList? eos* (
        ':' eos* asmExprList? eos* (':' eos* asmExprList? eos* (':' eos* asmExprList? eos*)?)? eos*
    )? ')' eos* Semi
    ;

asmExprList
    : expression (',' eos* expression)*
    ;

labeledStatement
    : 'case' expression ':' eos* statement*
    | 'default' ':' eos* statement*
    ;

compoundStatement
    : '{' blockItemList? '}'
    ;

blockItemList
    : (blockItem ws*)+
    ;

blockItem
    : statement
    | declaration
    ;

expressionStatement
    : assignmentExpressions eos* Semi
    ;

selectionStatement
    : 'if' '(' eos* expression eos* ')' eos* statement ('else' eos* statement)?
    | 'switch' '(' eos* expression eos* ')' '{' eos* labeledStatement* eos* '}'
    ;

iterationStatement
    : 'while' '(' eos* expression eos* ')' Semi? eos* statement
    | 'do' eos* statement 'while' '(' eos* expression eos* ')' Semi?
    | 'for' '(' eos* forCondition eos* ')' eos* statement
    ;

forCondition
    : (forDeclarations | assignmentExpressions?) Semi forExpression? Semi forExpression?
    ;

assignmentExpressions
    : assignmentExpression (',' eos* assignmentExpression)*
    ;

forDeclarations
    : forDeclaration (',' eos* forDeclaration)*
    ;

forDeclaration
    : declarationSpecifier initDeclaratorList?
    ;

forExpression
    : expression (',' eos* expression)*
    ;

jumpStatement
    : ('goto' '*'* Identifier | 'continue' | 'break' | 'return' eos* expression?) eos* Semi
    ;

compilationUnit
    : ws* translationUnit? EOF
    ;

translationUnit
    : (externalDeclaration ws*)+
    ;

externalDeclaration
    : declarationSpecifier
    | functionDefinition
    | declaration
    | macroCallExpression  // Allow macro calls like FUN(fmin, double, <) at top level
    | macroCallStatement  // Allow macro calls without parentheses at top level (e.g., FF_DISABLE_DEPRECATION_WARNINGS)
    | Semi
    ;

// Macro call expression: function call that may contain operators as arguments
// This handles cases like FUN(fmin, double, <) where operators are passed as macro parameters
// Optimized: Support macro calls followed by array subscripts, e.g., DECLARE_ALIGNED(32, float, spec1)[256]
macroCallExpression
    : Identifier '(' eos* macroArgumentList? eos* ')' postfixSuffix*
    ;

macroArgumentList
    : macroArgument (',' eos* macroArgument)*
    ;

functionDefinition
    : declarationSpecifier? declarator declarationList? compoundStatement?
    ;

declarationList
    : (declaration ws*)+
    ;

ws
    : EOS+
    | BlockComment
    | LineComment
    ;

// eos: 单个语句结束符
eos
    : Semi
    | EOS
    ;