#!/bin/sh
rm *.tokens
rm *.interp
rm ./parser/*.tokens
rm ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package pythonparser ./PythonLexer.g4 ./PythonParser.g4 -o parser -no-listener -visitor
