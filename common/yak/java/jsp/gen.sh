#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
antlr -Dlanguage=Go -package jspparser ./JSPLexer.g4 ./JSPParser.g4 -o parser -no-listener -visitor