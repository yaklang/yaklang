#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
../../antlr4util/antlr4 -Dlanguage=Go -package jspparser ./JSPLexer.g4 ./JSPParser.g4 -o parser -no-listener -visitor
