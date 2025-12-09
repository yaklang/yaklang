/*
 [The "BSD licence"] Copyright (c) 2017 Sasa Coh, Michał Błotniak
 Copyright (c) 2019 Ivan
 Kochurkin, kvanttt@gmail.com, Positive Technologies 
 Copyright (c) 2019 Dmitry Rassadin,
 flipparassa@gmail.com,Positive Technologies All rights reserved. 
 Copyright (c) 2021 Martin
 Mirchev, mirchevmartin2203@gmail.com
 Copyright (c) 2023 Dmitry Litovchenko, i@dlitovchenko.ru
 
 Redistribution and use in source and binary forms, with or without modification, are permitted
 provided that the following conditions are met: 1. Redistributions of source code must retain the
 above copyright notice, this list of conditions and the following disclaimer. 2. Redistributions
 in
 binary form must reproduce the above copyright notice, this list of conditions and the
 following
 disclaimer in the documentation and/or other materials provided with the distribution.
 3. The name
 of the author may not be used to endorse or promote products derived from this
 software without
 specific prior written permission.
 
 THIS SOFTWARE IS PROVIDED BY THE AUTHOR
 ``AS IS'' AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING,
 BUT NOT LIMITED TO, THE IMPLIED
 WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
 ARE DISCLAIMED. IN NO EVENT
 SHALL THE AUTHOR BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
 SPECIAL, EXEMPLARY, OR
 CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
 SUBSTITUTE GOODS OR
 SERVICES;
 LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
 CAUSED AND ON ANY
 THEORY OF
 LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING
 NEGLIGENCE OR
 OTHERWISE)
 ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
 POSSIBILITY
 OF SUCH
 DAMAGE.
 */

/*
 * A Go grammar for ANTLR 4 derived from the Go Language Specification https://golang.org/ref/spec
 */

// $antlr-format alignTrailingComments true, columnLimit 150, minEmptyLines 1, maxEmptyLinesToKeep 1, reflowComments false, useTab false
// $antlr-format allowShortRulesOnASingleLine false, allowShortBlocksOnASingleLine true, alignSemicolons hanging, alignColons hanging

parser grammar GoParser;

// Insert here @header for C++ parser.

options {
    tokenVocab = GoLexer;
    superClass = GoParserBase;
}

sourceFile
    : eos* packageClause (eos* importDecl eos*)* (( methodDecl | functionDecl | declaration) eos*)* EOF
    ;

packageClause
    : PACKAGE packageName eos*
    ;

packageName
    : IDENTIFIER
    ;

importDecl
    : IMPORT (importSpec | L_PAREN eos* (importSpec eos*)* R_PAREN)
    ;

importSpec
    : alias = (DOT | IDENTIFIER)? importPath
    ;

importPath
    : string_
    ;

declaration
    : constDecl
    | typeDecl
    | varDecl
    ;

constDecl
    : CONST eos* (constSpec eos* | L_PAREN eos* (constSpec eos*)* R_PAREN)
    ;

constSpec
    : identifierList (type_? ASSIGN expressionList)?
    ;

identifierList
    : IDENTIFIER (COMMA IDENTIFIER)*
    ;

expressionList
    : eos* expression eos* (COMMA eos* expression eos*)*
    ;

typeDecl
    : TYPE (typeSpec | L_PAREN eos* (typeSpec eos*)* eos* R_PAREN)
    ;

typeSpec
    : aliasDecl
    | typeDef
    ;

aliasDecl
    : IDENTIFIER ASSIGN type_
    ;

typeDef
    : IDENTIFIER typeParameters? type_
    ;

typeParameters
    : L_BRACKET typeParameterDecl (COMMA typeParameterDecl)* R_BRACKET
    ;

typeParameterDecl
    : identifierList typeElement
    ;

typeElement
    : typeTerm (OR typeTerm)*
    ;

typeTerm
    : UNDERLYING? type_
    ;

// Function declarations

functionDecl
    : FUNC IDENTIFIER typeParameters? signature eos? block?
    ;

methodDecl
    : FUNC receiver IDENTIFIER signature eos? block?
    ;

receiver
    : parameters
    ;

varDecl
    : VAR eos* (varSpec | L_PAREN eos* (varSpec eos*)* R_PAREN)
    ;

varSpec
    : identifierList (type_ (ASSIGN expressionList)? | ASSIGN expressionList)
    ;

block
    : L_CURLY eos* statementList? R_CURLY
    ;

statementList
    : (statement eos*)+
    ;

statement
    : declaration
    | labeledStmt
    | simpleStmt
    | goStmt
    | returnStmt
    | breakStmt
    | continueStmt
    | gotoStmt
    | fallthroughStmt
    | block
    | ifStmt
    | switchStmt
    | selectStmt
    | forStmt
    | deferStmt
    ;

simpleStmt
    : sendStmt
    | incDecStmt
    | assignment
    | expressionStmt
    | shortVarDecl
    ;

assignment
    : expressionList assign_op expressionList
    ;

assign_op
    : (PLUS | MINUS | OR | CARET | STAR | DIV | MOD | LSHIFT | RSHIFT | AMPERSAND | BIT_CLEAR)? ASSIGN
    ;

expressionStmt
    : expression
    ;

sendStmt
    : channel = expression RECEIVE data = expression
    ;

incDecStmt
    : expression (PLUS_PLUS | MINUS_MINUS)
    ;

shortVarDecl
    : identifierList DECLARE_ASSIGN expressionList
    ;

labeledStmt
    : IDENTIFIER COLON eos* forStmt?
    ;

returnStmt
    : RETURN expressionList?
    ;

breakStmt
    : BREAK IDENTIFIER?
    ;

continueStmt
    : CONTINUE IDENTIFIER?
    ;

gotoStmt
    : GOTO IDENTIFIER
    ;

fallthroughStmt
    : FALLTHROUGH
    ;

deferStmt
    : DEFER expression
    ;

ifStmt
    : IF eos* (expression | simpleStmt eos* expression) eos* block (ELSE eos* (ifStmt | block))?
    ;

switchStmt
    : exprSwitchStmt
    | typeSwitchStmt
    ;

exprSwitchStmt
    : SWITCH eos* (expression? | simpleStmt? eos* expression?) L_CURLY eos* exprCaseClause* eos* R_CURLY
    ;

exprCaseClause
    : exprSwitchCase COLON eos* statementList?
    ;

exprSwitchCase
    : CASE expressionList
    | DEFAULT
    ;

typeSwitchStmt
    : SWITCH eos* (typeSwitchGuard | simpleStmt eos* typeSwitchGuard) eos* L_CURLY eos* typeCaseClause* eos* R_CURLY
    ;

typeSwitchGuard
    : (IDENTIFIER DECLARE_ASSIGN)? primaryExpr DOT L_PAREN TYPE R_PAREN
    ;

typeCaseClause
    : typeSwitchCase COLON eos* statementList?
    ;

typeSwitchCase
    : CASE typeList
    | DEFAULT
    ;

typeList
    : eos* (type_ | NIL_LIT) (COMMA eos* (type_ | NIL_LIT))*
    ;

selectStmt
    : SELECT L_CURLY eos* commClause* eos* R_CURLY
    ;

commClause
    : commCase COLON eos* statementList?
    ;

commCase
    : CASE (sendStmt | recvStmt)
    | DEFAULT
    ;

recvStmt
    : (expressionList ASSIGN | identifierList DECLARE_ASSIGN)? recvExpr = expression
    ;

forStmt
    : FOR eos* (expression? | forClause | rangeClause?) eos* block
    ;

forClause
    : initStmt = simpleStmt? eos? expression? eos? postStmt = simpleStmt?
    ;

rangeClause
    : (expressionList ASSIGN | identifierList DECLARE_ASSIGN)? RANGE expression
    ;

goStmt
    : GO expression
    ;

type_
    : typeName typeArgs?
    | typeLit
    | L_PAREN eos? type_ eos? R_PAREN
    ;

typeArgs
    : L_BRACKET typeList COMMA? R_BRACKET
    ;

typeName
    : qualifiedIdent
    | IDENTIFIER
    ;

typeLit
    : arrayType
    | structType
    | pointerType
    | functionType
    | interfaceType
    | sliceType
    | mapType
    | channelType
    ;

arrayType
    : L_BRACKET arrayLength R_BRACKET elementType
    ;

arrayLength
    : expression
    ;

elementType
    : type_
    ;

pointerType
    : STAR type_
    ;

interfaceType
    : INTERFACE L_CURLY eos* ((methodSpec | typeElement) eos*)* eos* R_CURLY
    ;

sliceType
    : L_BRACKET R_BRACKET elementType
    ;

// It's possible to replace `type` with more restricted typeLit list and also pay attention to nil maps
mapType
    : MAP L_BRACKET type_ R_BRACKET elementType
    ;

channelType
    : (CHAN | CHAN RECEIVE | RECEIVE CHAN) elementType
    ;

methodSpec
    : IDENTIFIER parameters result
    | IDENTIFIER parameters
    ;

functionType
    : FUNC signature
    ;

signature
    : parameters result?
    ;

result
    : parameters
    | type_
    ;

parameters
    : L_PAREN eos* (eos* parameterDecl eos* (eos* COMMA eos* parameterDecl eos*)* COMMA?)? eos* R_PAREN
    ;

parameterDecl
    : identifierList? ELLIPSIS? type_
    ;

expression
    : unary_op = (PLUS | MINUS | EXCLAMATION | CARET | STAR | AMPERSAND | RECEIVE) eos* expression
    | expression mul_op = (STAR | DIV | MOD | LSHIFT | RSHIFT | AMPERSAND | BIT_CLEAR) eos* expression
    | expression add_op = (PLUS | MINUS | OR | CARET) eos* expression
    | expression rel_op = (
        EQUALS
        | NOT_EQUALS
        | LESS
        | LESS_OR_EQUALS
        | GREATER
        | GREATER_OR_EQUALS
    ) eos* expression
    | expression LOGICAL_AND eos* expression
    | expression LOGICAL_OR eos* expression
    | primaryExpr
    ;

primaryExpr
    : operand
    | conversion
    | methodExpr
    | primaryExpr (DOT eos* IDENTIFIER typeArgs? | index | slice_ | typeAssertion | arguments)
    ;

conversion
    : type_ L_PAREN eos* expression eos* COMMA? eos* R_PAREN
    ;

operand
    : literal
    | operandName typeArgs?
    | L_PAREN expression R_PAREN
    ;

literal
    : basicLit
    | compositeLit
    | functionLit
    ;

basicLit
    : NIL_LIT
    | integer
    | string_
    | char_
    | FLOAT_LIT
    ;

integer
    : DECIMAL_LIT
    | BINARY_LIT
    | OCTAL_LIT
    | HEX_LIT
    | IMAGINARY_LIT
    ;

operandName
    : IDENTIFIER
    ;

qualifiedIdent
    : IDENTIFIER DOT IDENTIFIER
    ;

compositeLit
    : literalType literalValue
    ;

literalType
    : structType
    | arrayType
    | L_BRACKET ELLIPSIS R_BRACKET elementType
    | sliceType
    | mapType
    | typeName typeArgs?
    ;

literalValue
    : L_CURLY eos* (elementList eos* COMMA?)? eos* R_CURLY
    ;

elementList
    : keyedElement (COMMA keyedElement)*
    ;

keyedElement
    : eos* (key eos* COLON eos*)? element eos*
    ;

key
    : expression
    ;

element
    : expression
    | literalValue
    ;

structType
    : STRUCT L_CURLY eos* (fieldDecl eos*)* R_CURLY
    ;

fieldDecl
    : (identifierList type_ | embeddedField) tag = string_?
    ;

string_
    : RAW_STRING_LIT
    | INTERPRETED_STRING_LIT
    ;

char_
    : RAW_CHAR_LIT
    ;

embeddedField
    : STAR? typeName typeArgs?
    ;

functionLit
    : FUNC signature block
    ; // function

index
    : L_BRACKET expression R_BRACKET
    ;

slice_
    : L_BRACKET (
        low = expression? COLON high = expression?
        | low = expression? COLON high = expression COLON max = expression
    ) R_BRACKET
    ;

typeAssertion
    : DOT L_PAREN type_ R_PAREN
    ;

arguments
    : L_PAREN eos* ((expressionList | type_ (COMMA expressionList)?) ELLIPSIS? eos* COMMA? eos*)? R_PAREN
    ;

methodExpr
    : type_ DOT IDENTIFIER
    ;

eos
    : SEMI
    | EOS
    ;