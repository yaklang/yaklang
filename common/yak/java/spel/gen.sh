#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
java -jar ../../antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go -package spelparser ./SpelLexer.g4 ./SpelParser.g4 -o parser -no-listener -visitor