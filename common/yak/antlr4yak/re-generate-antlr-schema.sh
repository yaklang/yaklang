#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
rm *.tokens
antlr -Dlanguage=Go ./YaklangLexer.g4 ./YaklangParser.g4 -o parser -no-listener -visitor
