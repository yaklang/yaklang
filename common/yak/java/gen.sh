#!/bin/sh

rm ./parser/*.tokens
rm ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package javaparser ./JavaLexer.g4 ./JavaParser.g4 -o parser -no-listener -visitor
