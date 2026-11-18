#!/bin/sh
# ANTLR 4.13.2 codegen for the Scala frontend.
# Grammar source: antlr/grammars-v4 scala/scala3 @ c306207ac870.
# Renamed to ScalaLexer.g4 / ScalaParser.g4 to match java/php.
set -e
rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package scalaparser ./ScalaLexer.g4 ./ScalaParser.g4 -o parser -no-listener -visitor
