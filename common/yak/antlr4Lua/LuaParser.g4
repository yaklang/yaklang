/*
BSD License

Copyright (c) 2013, Kazunori Sakamoto
Copyright (c) 2016, Alexander Alexeev
All rights reserved.

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions
are met:

1. Redistributions of source code must retain the above copyright
   notice, this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright
   notice, this list of conditions and the following disclaimer in the
   documentation and/or other materials provided with the distribution.
3. Neither the NAME of Rainer Schuster nor the NAMEs of its contributors
   may be used to endorse or promote products derived from this software
   without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS
"AS IS" AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT
LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
A PARTICULAR PURPOSE ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT
HOLDER OR CONTRIBUTORS BE LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL,
SPECIAL, EXEMPLARY, OR CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT
LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR SERVICES; LOSS OF USE,
DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER CAUSED AND ON ANY
THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY, OR TORT
(INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

This grammar file derived from:

    Luau 0.537 Grammar Documentation
    https://github.com/Roblox/luau/blob/0.537/docs/_pages/grammar.md

    Lua 5.4 Reference Manual
    http://www.lua.org/manual/5.4/manual.html

    Lua 5.3 Reference Manual
    http://www.lua.org/manual/5.3/manual.html

    Lua 5.2 Reference Manual
    http://www.lua.org/manual/5.2/manual.html

    Lua 5.1 grammar written by Nicolai Mainiero
    http://www.antlr3.org/grammar/1178608849736/Lua.g

Tested by Kazunori Sakamoto with Test suite for Lua 5.2 (http://www.lua.org/tests/5.2/)

Tested by Alexander Alexeev with Test suite for Lua 5.3 http://www.lua.org/tests/lua-5.3.2-tests.tar.gz

Tested by Matt Hargett with:
    - Test suite for Lua 5.4.4: http://www.lua.org/tests/lua-5.4.4-tests.tar.gz
    - Test suite for Selene Lua lint tool v0.20.0: https://github.com/Kampfkarren/selene/tree/0.20.0/selene-lib/tests
    - Test suite for full-moon Lua parsing library v0.15.1: https://github.com/Kampfkarren/full-moon/tree/main/full-moon/tests
    - Test suite for IntelliJ-Luanalysis IDE plug-in v1.3.0: https://github.com/Benjamin-Dobell/IntelliJ-Luanalysis/tree/v1.3.0/src/test
    - Test suite for StyLua formatting tool v.14.1: https://github.com/JohnnyMorganz/StyLua/tree/v0.14.1/tests
    - Entire codebase for luvit: https://github.com/luvit/luvit/
    - Entire codebase for lit: https://github.com/luvit/lit/
    - Entire codebase and test suite for neovim v0.7.2: https://github.com/neovim/neovim/tree/v0.7.2
    - Entire codebase for World of Warcraft Interface: https://github.com/tomrus88/BlizzardInterfaceCode
    - Benchmarks and conformance test suite for Luau 0.537: https://github.com/Roblox/luau/tree/0.537
*/

parser grammar LuaParser;

options {
    tokenVocab=LuaLexer;
}

chunk
    : block EOF
    ;

block
    : stat* laststat?
    ;

stat
    : SemiColon
    | varlist AssignEq explist
    | functioncall
    | label
    | Break
    | Goto NAME
    | Do block End
    | While exp Do block End
    | Repeat block Until exp
    | If exp Then block (ElseIf exp Then block)* (Else block)? End
    | For NAME AssignEq exp Comma exp (Comma exp)? Do block End
    | For namelist In explist Do block End
    | Function funcname funcbody
    | Local Function NAME funcbody
    | Local attnamelist (AssignEq explist)?
    ;

attnamelist
    : NAME attrib (',' NAME attrib)*
    ;

attrib
    : (Lt NAME Gt)?
    ;

laststat
    : Return explist? | Break | Continue SemiColon?
    ;

label
    : DoubleColon NAME DoubleColon
    ;

funcname
    : NAME (Dot NAME)* (Colon NAME)?
    ;

varlist
    : var (Comma var)*
    ;

namelist
    : NAME (Comma NAME)*
    ;

explist
    : (exp Comma)* exp
    ;

exp
    : Nil | False | True
    | number
    | string
    | Ellipsis
    | functiondef
    | prefixexp
    | tableconstructor
    | <assoc=right> exp operatorPower exp
    | operatorUnary exp
    | exp operatorMulDivMod exp
    | exp operatorAddSub exp
    | <assoc=right> exp operatorStrcat exp
    | exp operatorComparison exp
    | exp operatorAnd exp
    | exp operatorOr exp
    | exp operatorBitwise exp
    ;

prefixexp
    : varOrExp nameAndArgs*
    ;

functioncall
    : varOrExp nameAndArgs+
    ;

varOrExp
    : var | LParen exp RParen
    ;

var
    : (NAME | LParen exp RParen varSuffix) varSuffix*
    ;

varSuffix
    : nameAndArgs* (LBracket exp RBracket | Dot NAME)
    ;

nameAndArgs
    : (Colon NAME)? args
    ;

args
    : LParen explist? RParen | tableconstructor | string
    ;

functiondef
    : Function funcbody
    ;

funcbody
    : LParen parlist? RParen block End
    ;

parlist
    : namelist (Comma Ellipsis)? | Ellipsis
    ;

tableconstructor
    : LBrace fieldlist? RBrace
    ;

fieldlist
    : field (fieldsep field)* fieldsep?
    ;

field
    : LBracket exp RBracket AssignEq exp | NAME AssignEq exp | exp
    ;

fieldsep
    : Comma | SemiColon
    ;

operatorOr
	: Or;

operatorAnd
	: And;

operatorComparison
	: Lt | Gt | LtEq | GtEq | Neq | Eq;

operatorStrcat
	: Strcat;

operatorAddSub
	: Plus | Sub;

operatorMulDivMod
	: Mul | Div | Mod | IntegralDiv;

operatorBitwise
	: Amp | Xand | NotSymbol | LtLt | GtGt;

operatorUnary
    : Not | Pound | Sub | NotSymbol;

operatorPower
    : Power;

number
    : INT | HEX | FLOAT | HEX_FLOAT
    ;

string
    : NORMALSTRING | CHARSTRING | LONGSTRING
    ;
