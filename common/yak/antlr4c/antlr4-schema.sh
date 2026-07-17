#!/bin/sh

#rm ./parser/*.tokens
#rm ./parser/*.interp

../antlr4util/antlr4 -Dlanguage=Go ./CLexer.g4 ./CParser.g4 -o parser -package c -no-listener -visitor
