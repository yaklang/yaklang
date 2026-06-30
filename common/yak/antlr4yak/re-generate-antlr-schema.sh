#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
rm *.tokens
java -jar ../antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go ./YaklangLexer.g4 ./YaklangParser.g4 -o parser -no-listener -visitor
