#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
antlr -Dlanguage=Go -package phpparser ./PHPLexer.g4 ./PHPParser.g4 -o parser -no-listener -visitor