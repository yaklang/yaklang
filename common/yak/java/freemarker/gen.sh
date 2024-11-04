#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
antlr -Dlanguage=Go -package freemarkerparser ./FreemarkerLexer.g4 ./FreemarkerParser.g4 -o parser -no-listener -visitor