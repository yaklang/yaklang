#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
java -jar ../antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go ./LuaLexer.g4 ./LuaParser.g4 -o parser -no-listener -visitor
