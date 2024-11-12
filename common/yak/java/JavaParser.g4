/*
 [The "BSD licence"]
 Copyright (c) 2013 Terence Parr, Sam Harwell
 Copyright (c) 2017 Ivan Kochurkin (upgrade to Java 8)
 Copyright (c) 2021 Michał Lorek (upgrade to Java 11)
 Copyright (c) 2022 Michał Lorek (upgrade to Java 17)
 All rights reserved.

 Redistribution and use in source and binary forms, with or without
 modification, are permitted provided that the following conditions
 are met:
 1. Redistributions of source code must retain the above copyright
    notice, this list of conditions and the following disclaimer.
 2. Redistributions in binary form must reproduce the above copyright
    notice, this list of conditions and the following disclaimer in the
    documentation and/or other materials provided with the distribution.
 3. The name of the author may not be used to endorse or promote products
    derived from this software without specific prior written permission.

 THIS SOFTWARE IS PROVIDED BY THE AUTHOR ``AS IS'' AND ANY EXPRESS OR
 IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED WARRANTIES
 OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE DISCLAIMED.
 IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY DIRECT, INDIRECT,
 INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT
 NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
 DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
 THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
 (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE OF
 THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

// $antlr-format alignTrailingComments true, columnLimit 150, minEmptyLines 1, maxEmptyLinesToKeep 1, reflowComments false, useTab false
// $antlr-format allowShortRulesOnASingleLine false, allowShortBlocksOnASingleLine true, alignSemicolons hanging, alignColons hanging

parser grammar JavaParser;

options {
    tokenVocab = JavaLexer;
}

compilationUnit
    : packageDeclaration? (importDeclaration | ';')* (typeDeclaration | ';')*
    | moduleDeclaration EOF
    ;

packageDeclaration
    : annotation* PACKAGE packageName ';'
    ;
packageName
    : qualifiedName
    | Dollar '{' (PACKAGE | identifier) '}' ('.' identifier)*
    ;

importDeclaration
    : IMPORT STATIC? qualifiedName ('.' '*')? ';'
    ;

typeDeclaration
    : classOrInterfaceModifier* (
        classDeclaration
        | enumDeclaration
        | interfaceDeclaration
        | annotationTypeDeclaration
        | recordDeclaration
    )
    ;

modifiers: modifier*;
modifier
    : annotation
    | staticClassModifier
    | staticModifier
    ;

staticModifier
    : NATIVE
    | SYNCHRONIZED
    | TRANSIENT
    | VOLATILE
    ;

classOrInterfaceModifier
    : annotation
    | staticClassModifier
    ;

staticClassModifier
    : PUBLIC
    | PROTECTED
    | PRIVATE
    | STATIC
    | ABSTRACT
    | FINAL // FINAL for class only -- does not apply to interfaces
    | STRICTFP
    | SEALED     // Java17
    | NON_SEALED // Java17
    ;

variableModifier
    : FINAL
    | annotation
    ;

classDeclaration
    : CLASS identifier typeParameters? (EXTENDS typeType)? (IMPLEMENTS typeList)? (
        PERMITS typeList
    )? // Java17
    classBody
    ;

typeParameters
    : '<' typeParameter (',' typeParameter)* '>'
    ;

typeParameter
    : annotation* identifier (EXTENDS annotation* typeBound)?
    ;

typeBound
    : typeType ('&' typeType)*
    ;

enumDeclaration
    : ENUM identifier (IMPLEMENTS typeList)? '{' enumConstants? ','? enumBodyDeclarations? '}'
    ;

enumConstants
    : enumConstant (',' enumConstant)*
    ;

enumConstant
    : annotation* identifier arguments? classBody?
    ;

enumBodyDeclarations
    : ';' classBodyDeclaration*
    ;

interfaceDeclaration
    : '@'? INTERFACE identifier typeParameters? (EXTENDS typeList)? (PERMITS typeList)? interfaceBody
    ;

classBody
    : '{' classBodyDeclaration* '}'
    ;

interfaceBody
    : '{' interfaceBodyDeclaration* '}'
    ;

classBodyDeclaration
    : ';'
    | STATIC? block
    | modifiers memberDeclaration
    ;

memberDeclaration
    : recordDeclaration //Java17
    | methodDeclaration
    | genericMethodDeclaration
    | fieldDeclaration
    | constructorDeclaration
    | genericConstructorDeclaration
    | interfaceDeclaration
    | annotationTypeDeclaration
    | classDeclaration
    | enumDeclaration
    ;

/* We use rule this even for void methods which cannot have [] after parameters.
   This simplifies grammar and we can consider void to be a type, which
   renders the [] matching as a context-sensitive issue or a semantic check
   for invalid return type after parsing.
 */
methodDeclaration
    : typeTypeOrVoid identifier formalParameters ('[' ']')* (THROWS qualifiedNameList)? methodBody
    ;

methodBody
    : block
    | ';'
    ;

typeTypeOrVoid
    : typeType
    | VOID
    ;

genericMethodDeclaration
    : typeParameters methodDeclaration
    ;

genericConstructorDeclaration
    : typeParameters constructorDeclaration
    ;

constructorDeclaration
    : identifier formalParameters (THROWS qualifiedNameList)? constructorBody = block
    ;

compactConstructorDeclaration
    : modifiers identifier constructorBody = block
    ;

fieldDeclaration
    : typeType variableDeclarators ';'
    ;

interfaceBodyDeclaration
    : modifiers interfaceMemberDeclaration
    | ';'
    ;

interfaceMemberDeclaration
    : recordDeclaration // Java17
    | constDeclaration
    | interfaceMethodDeclaration
    | genericInterfaceMethodDeclaration
    | interfaceDeclaration
    | annotationTypeDeclaration
    | classDeclaration
    | enumDeclaration
    ;

constDeclaration
    : typeType constantDeclarator (',' constantDeclarator)* ';'
    ;

constantDeclarator
    : identifier ('[' ']')* '=' variableInitializer
    ;

// Early versions of Java allows brackets after the method name, eg.
// public int[] return2DArray() [] { ... }
// is the same as
// public int[][] return2DArray() { ... }
interfaceMethodDeclaration
    : interfaceMethodModifier* interfaceCommonBodyDeclaration
    ;

// Java8
interfaceMethodModifier
    : annotation
    | PUBLIC
    | ABSTRACT
    | DEFAULT
    | STATIC
    | STRICTFP
    ;

genericInterfaceMethodDeclaration
    : interfaceMethodModifier* typeParameters interfaceCommonBodyDeclaration
    ;

interfaceCommonBodyDeclaration
    : annotation* typeTypeOrVoid identifier formalParameters ('[' ']')* (THROWS qualifiedNameList)? methodBody
    ;

variableDeclarators
    : variableDeclarator (',' variableDeclarator)*
    ;

variableDeclarator
    : variableDeclaratorId ('=' variableInitializer)?
    ;

variableDeclaratorId
    : identifier ('[' ']')*
    ;

variableInitializer
    : expression
    | arrayInitializer
    ;

arrayInitializer
    : '{' (variableInitializer (',' variableInitializer)* ','?)? '}'
    ;

classOrInterfaceType
    : (identifier typeArguments? '.')* typeIdentifier typeArguments?
    ;

typeArgument
    : typeType
    | annotation* '?' ((EXTENDS | SUPER) typeType)?
    ;

qualifiedNameList
    : qualifiedName (',' qualifiedName)*
    ;

formalParameters
    : '(' (
        receiverParameter?
        | receiverParameter (',' formalParameterList)?
        | formalParameterList?
    ) ')'
    ;

receiverParameter
    : typeType (identifier '.')* THIS
    ;

formalParameterList
    : formalParameter (',' formalParameter)* (',' lastFormalParameter)?
    | lastFormalParameter
    ;

formalParameter
    : variableModifier* typeType variableDeclaratorId
    ;

lastFormalParameter
    : variableModifier* typeType annotation* '...' variableDeclaratorId
    ;

// local variable type inference
lambdaLVTIList
    : lambdaLVTIParameter (',' lambdaLVTIParameter)*
    ;

lambdaLVTIParameter
    : variableModifier* VAR identifier
    ;

qualifiedName
    : identifier ('.' identifier)*
    ;

literal
    : integerLiteral
    | floatLiteral
    | CHAR_LITERAL
    | STRING_LITERAL
    | BOOL_LITERAL
    | NULL_LITERAL
    | TEXT_BLOCK // Java17
    ;

integerLiteral
    : DECIMAL_LITERAL
    | HEX_LITERAL
    | OCT_LITERAL
    | BINARY_LITERAL
    ;

floatLiteral
    : FLOAT_LITERAL
    | HEX_FLOAT_LITERAL
    ;

// ANNOTATIONS
altAnnotationQualifiedName
    : (identifier DOT)* '@' identifier
    ;

annotation
    : ('@' qualifiedName | altAnnotationQualifiedName) (
        '(' ( elementValuePairs | elementValue)? ')'
    )?
    ;

elementValuePairs
    : elementValuePair (',' elementValuePair)*
    ;

elementValuePair
    : identifier '=' elementValue
    ;

elementValue
    : expression
    | annotation
    | elementValueArrayInitializer
    ;

elementValueArrayInitializer
    : '{' (elementValue (',' elementValue)*)? ','? '}'
    ;

annotationTypeDeclaration
    : '@' INTERFACE identifier annotationTypeBody
    ;

annotationTypeBody
    : '{' annotationTypeElementDeclaration* '}'
    ;

annotationTypeElementDeclaration
    : modifiers annotationTypeElementRest
    | ';' // this is not allowed by the grammar, but apparently allowed by the actual compiler
    ;

annotationTypeElementRest
    : typeType annotationMethodOrConstantRest ';'
    | classDeclaration ';'?
    | interfaceDeclaration ';'?
    | enumDeclaration ';'?
    | annotationTypeDeclaration ';'?
    | recordDeclaration ';'? // Java17
    ;

annotationMethodOrConstantRest
    : annotationMethodRest
    | annotationConstantRest
    ;

annotationMethodRest
    : identifier '(' ')' defaultValue?
    ;

annotationConstantRest
    : variableDeclarators
    ;

defaultValue
    : DEFAULT elementValue
    ;

// MODULES - Java9

moduleDeclaration
    : OPEN? MODULE qualifiedName moduleBody
    ;

moduleBody
    : '{' moduleDirective* '}'
    ;

moduleDirective
    : REQUIRES requiresModifier* qualifiedName ';'
    | EXPORTS qualifiedName (TO qualifiedName)? ';'
    | OPENS qualifiedName (TO qualifiedName)? ';'
    | USES qualifiedName ';'
    | PROVIDES qualifiedName WITH qualifiedName ';'
    ;

requiresModifier
    : TRANSITIVE
    | STATIC
    ;

// RECORDS - Java 17

recordDeclaration
    : RECORD identifier typeParameters? recordHeader (IMPLEMENTS typeList)? recordBody
    ;

recordHeader
    : '(' recordComponentList? ')'
    ;

recordComponentList
    : recordComponent (',' recordComponent)*
    ;

recordComponent
    : typeType identifier
    ;

recordBody
    : '{' (classBodyDeclaration | compactConstructorDeclaration)* '}'
    ;

// STATEMENTS / BLOCKS
blockOrState
    : block 
    | statement
    ;

block
    : '{' blockStatementList? '}'
    ;

elseBlock
    :ELSE blockOrState
    ;
elseIfBlock
    :ELSE IF parExpression blockOrState
    ;

blockStatementList
    : blockStatement +
    ;

blockStatement
    : localVariableDeclaration ';'
    | localTypeDeclaration
    | statement
    ;

localVariableDeclaration
    : variableModifier* (VAR identifier '=' expression | typeType variableDeclarators)
    ;

identifier
    : IDENTIFIER
    | MODULE
    | OPEN
    | REQUIRES
    | EXPORTS
    | OPENS
    | TO
    | USES
    | PROVIDES
    | WITH
    | TRANSITIVE
    | YIELD
    | SEALED
    | PERMITS
    | RECORD
    | VAR
    ;

typeIdentifier // Identifiers that are not restricted for type declarations
    : IDENTIFIER
    | MODULE
    | OPEN
    | REQUIRES
    | EXPORTS
    | OPENS
    | TO
    | USES
    | PROVIDES
    | WITH
    | TRANSITIVE
    | SEALED
    | PERMITS
    | RECORD
    ;

localTypeDeclaration
    : classOrInterfaceModifier* (classDeclaration | interfaceDeclaration | recordDeclaration)
    ;

statement
    : blockLabel = block                                                        # BlockLabelStatement
    | ASSERT expression (':' expression)? ';'                                   # AssertStatement
    | ifstmt                                                                    # IfStatement
    | FOR '(' forControl ')' blockOrState                                       # ForStatement
    | WHILE parExpression blockOrState                                          # WhileStatement
    | DO block WHILE parExpressionList ';'                                      # DoWhileStatement
    | TRY block (catchClause+ finallyBlock? | finallyBlock)                     # TryStatement
    | TRY resourceSpecification block catchClause* finallyBlock?                # TryWithResourcesStatement
    | switchStatement                                                           # PureSwitchStatement
    | SYNCHRONIZED parExpression block                                          # SynchronizedStatement
    | RETURN expression? ';'                                                    # ReturnStatement
    | THROW expression ';'                                                      # ThrowStatement
    | BREAK identifier? ';'                                                     # BreakStatement
    | CONTINUE identifier? ';'                                                  # ContinueStatement
    | YIELD expression ';'                                                      # YieldStatement// Java17
    | SEMI                                                                      # SemiStatement
    | statementExpression = expression ';'                                      # ExpressionStatement
    | switchExpression ';'?                                                     # SwitchArrowExpression // Java17
    | identifierLabel = identifier ':' statement                                # IdentifierLabelStatement
    ;

statementList: statement+;

switchStatement
    : 'switch' parExpression '{' switchBlockStatementGroup* '}'
    ;

switchBlockStatementGroup
    : switchLabel statementList?
    ;

switchLabel
    : 'case' expressionList ':'
    | 'default' ':'
    ;

ifstmt
    :IF parExpression blockOrState? elseIfBlock* elseBlock?
    ;
catchClause
    : CATCH '(' variableModifier* catchType identifier ')' block
    ;

catchType
    : qualifiedName ('|' qualifiedName)*
    ;

finallyBlock
    : FINALLY block
    ;

resourceSpecification
    : '(' resources ';'? ')'
    ;

resources
    : resource (';' resource)*
    ;

resource
    : variableModifier* (classOrInterfaceType variableDeclaratorId | VAR identifier) '=' expression
    | qualifiedName
    ;

/** Matches cases then statements, both of which are mandatory.
 *  To handle empty cases at the end, we add switchLabel* to statement.
 */


forControl
    : enhancedForControl
    | forInit? ';' expression? ';' forUpdate = expressionList?
    ;

forInit
    : localVariableDeclaration
    | expressionList
    ;

enhancedForControl
    : variableModifier* (typeType | VAR) variableDeclaratorId ':' expression
    ;

// EXPRESSIONS

parExpression
    : '(' expression ')'
    ;

parExpressionList
    : '(' expressionList ')'
    ;

expressionList
    : expression (',' expression)* ','?
    ;

methodCall
    : (identifier | THIS | SUPER) arguments
    ;

expression
    // Expression order in accordance with https://introcs.cs.princeton.edu/java/11precedence/
    // Level 16, Primary, array and member access
    : primary                                                       # PrimaryExpression
    | expression '[' expression ']'                                 # SliceCallExpression
    | expression bop = '.' (
        identifier
        | methodCall
        | THIS
        | NEW nonWildcardTypeArguments? innerCreator
        | SUPER superSuffix
        | explicitGenericInvocation
    )                                                               # MemberCallExpression
    // Method calls and method references are part of primary, and hence level 16 precedence
    | methodCall                                                    # FunctionCallExpression
    | expression '::' typeArguments? identifier                     # MethodReferenceExpression
    | typeType '::' (typeArguments? identifier | NEW)               # ConstructorReferenceExpression
    | classType '::' typeArguments? NEW                             # ConstructorReferenceExpression

    // Java17
    | switchExpression                                              # Java17SwitchExpression

    // Level 15 Post-increment/decrement operators
    | leftExpression= expression (leftMemberCall | leftSliceCall)  postfix = ('++' | '--')                            # PostfixExpression1
    | identifier  postfix = ('++' | '--')                            # PostfixExpression2
    // Level 14, Unary operators
    | prefix = ('+' | '-'  | '~' | '!') expression                   # PrefixUnaryExpression
    | prefix = ('++' | '--') leftExpression= expression (leftMemberCall | leftSliceCall)                     # PrefixBinayExpression1
    | prefix = ('++' | '--') identifier                              # PrefixBinayExpression2
    // Level 13 Cast and object creation
    | '(' annotation* typeType ('&' typeType)* ')' expression       # CastExpression
    | NEW creator                                                   # NewCreatorExpression

    // Level 12 to 1, Remaining operators
    // Level 12, Multiplicative operators
    | expression bop = ('*' | '/' | '%') expression                 # MultiplicativeExpression
    // Level 11, Additive operators
    | expression bop = ('+' | '-') expression                       # AdditiveExpression
    // Level 10, Shift operators
    | expression   ('<''<'| '>''>''>'| '>''>' ) expression       # ShiftExpression
    // Level 9, Relational operators
    | expression bop = ('<=' | '>=' | '>' | '<') expression         # RelationalExpression
    | expression bop = INSTANCEOF (typeType | pattern)              # InstanceofExpression
    // Level 8, Equality Operators
    | expression bop = ('==' | '!=') expression                     # EqualityExpression
    // Level 7, Bitwise AND
    | expression bop = '&' expression                               # BitwiseAndExpression
    // Level 6, Bitwise XOR
    | expression bop = '^' expression                               # BitwiseXORExpression
    // Level 5, Bitwise OR
    | expression bop = '|' expression                               # BitwiseORExpression
    // Level 4, Logic AND (Short-Curity AND)
    | expression bop = '&&' expression                              # LogicANDExpression
    // Level 3, Logic OR (Short-Curity OR)
    | expression bop = '||' expression                              # LogicORExpression
    // Level 2, Ternary (Conditional) operator
    | <assoc = right> expression bop = '?' expression ':' expression# TernaryExpression
    // Level 1, Assignment
    | <assoc = right>leftExpression= expression (leftMemberCall | leftSliceCall)  bop = (
         '+='
        | '-='
        | '*='
        | '/='
        | '&='
        | '|='
        | '^='
        | '>>='
        | '>>>='
        | '<<='
        | '%='
    ) expression                                                    # AssignmentExpression1
    |<assoc = right>identifier  bop = (
              '+='
             | '-='
             | '*='
             | '/='
             | '&='
             | '|='
             | '^='
             | '>>='
             | '>>>='
             | '<<='
             | '%='
         ) expression                                              # AssignmentExpression2
    | <assoc = right>leftExpression= expression (leftMemberCall | leftSliceCall)  bop = '=' (expression|identifier)            # AssignmentEqExpression1
    | <assoc = right>identifier bop = '=' (expression|identifier)          # AssignmentEqExpression2
    // Level 0, Lambda Expression Java8
    | lambdaExpression                                              # Java8LambdaExpression
    ;



leftMemberCall
    :'.' identifier
    ;

leftSliceCall
    : '[' expression ']'
    ;

// Java17
pattern
    : variableModifier* typeType annotation* identifier
    ;

// Java8
lambdaExpression
    : lambdaParameters '->' lambdaBody
    ;

// Java8
lambdaParameters
    : identifier
    | '(' formalParameterList? ')'
    | '(' identifier (',' identifier)* ')'
    | '(' lambdaLVTIList? ')'
    ;

// Java8
lambdaBody
    : expression
    | block
    ;

primary
    : '(' expression ')'
    | THIS
    | SUPER
    | literal
    | identifier
    | typeTypeOrVoid '.' CLASS
    | nonWildcardTypeArguments (explicitGenericInvocationSuffix | THIS arguments)
    ;

// Java17
switchExpression
    :SWITCH  parExpression '{' switchLabeledRule* defaultLabeledRule? '}'
    ;

// Java17
switchLabeledRule
    : CASE (expressionList | NULL_LITERAL | guardedPattern) (ARROW | COLON) switchRuleOutcome
    ;

// Java17
defaultLabeledRule
    : DEFAULT (ARROW | COLON) switchRuleOutcome
    ;

// Java17
guardedPattern
    : '(' guardedPattern ')'
    | variableModifier* typeType annotation* identifier ('&&' expression)*
    | guardedPattern '&&' expression
    ;

// Java17
switchRuleOutcome
    : block
    | blockStatement*
    ;

classType
    : (classOrInterfaceType '.')? annotation* identifier typeArguments?
    ;

creator
    : nonWildcardTypeArguments? createdName classCreatorRest
    | createdName arrayCreatorRest
    ;

createdName
    : identifier typeArgumentsOrDiamond? ('.' identifier typeArgumentsOrDiamond?)*
    | primitiveType
    ;

innerCreator
    : identifier nonWildcardTypeArgumentsOrDiamond? classCreatorRest
    ;

arrayCreatorRest
    : ('[' ']')+ arrayInitializer
    | ('[' expression ']')+ ('[' ']')*
    ;

classCreatorRest
    : arguments classBody?
    ;

explicitGenericInvocation
    : nonWildcardTypeArguments explicitGenericInvocationSuffix
    ;

typeArgumentsOrDiamond
    : '<' '>'
    | typeArguments
    ;

nonWildcardTypeArgumentsOrDiamond
    : '<' '>'
    | nonWildcardTypeArguments
    ;

nonWildcardTypeArguments
    : '<' typeList '>'
    ;

typeList
    : typeType (',' typeType)*
    ;

typeType
    : annotation* (classOrInterfaceType | primitiveType) (annotation* '[' ']')*
    ;

primitiveType
    : BOOLEAN
    | CHAR
    | BYTE
    | SHORT
    | INT
    | LONG
    | FLOAT
    | DOUBLE
    ;

typeArguments
    : '<' typeArgument (',' typeArgument)* '>'
    ;

superSuffix
    : arguments
    | '.' typeArguments? identifier arguments?
    ;

explicitGenericInvocationSuffix
    : SUPER superSuffix
    | identifier arguments
    ;

arguments
    : '(' expressionList? ')'
    ;