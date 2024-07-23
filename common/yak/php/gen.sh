#!/bin/sh
rm *.tokens
rm *.interp
rm ./parser/*.tokens
rm ./parser/*.interp
  java -jar ../antlr4thirdparty/antlr-4.11.1-complete.jar -Dlanguage=Go -package phpparser ./PHPLexer.g4 ./PHPParser.g4 -o parser -no-listener -visitor
