#!/bin/sh
# ANTLR 4.13.2 codegen for the Kotlin frontend.
# Grammar source: antlr/grammars-v4 kotlin/kotlin-formal @ c306207ac870.
# Unicode letters and digits use ANTLR \p{...} properties; no UnicodeClasses.g4.
set -e
rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package kotlinparser ./KotlinLexer.g4 ./KotlinParser.g4 -o parser -no-listener -visitor
