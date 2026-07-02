#!/bin/sh
rm *.tokens
rm *.interp
rm ./parser/*.tokens
rm ./parser/*.interp
java -jar ../antlr4thirdparty/antlr-4.13.2-complete.jar -Dlanguage=Go -package pythonparser ./PythonLexer.g4 ./PythonParser.g4 -o parser -no-listener -visitor
