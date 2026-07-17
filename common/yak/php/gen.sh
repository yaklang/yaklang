#!/bin/sh
rm *.tokens
rm *.interp
rm ./parser/*.tokens
rm ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package phpparser ./PHPLexer.g4 ./PHPParser.g4 -o parser -no-listener -visitor
