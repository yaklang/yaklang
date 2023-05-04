#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
antlr4 -Dlanguage=Go ./LuaLexer.g4 ./LuaParser.g4 -o parser -no-listener -visitor
