#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
antlr -Dlanguage=Go -package spelparser ./SpelLexer.g4 ./SpelParser.g4 -o parser -no-listener -visitor