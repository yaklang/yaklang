#!/bin/sh
# ANTLR 4.13.2 codegen for the Rust frontend.
# Grammar source: antlr/grammars-v4 rust/ @ c306207ac870.
# Local patch: semantic predicates use the Go target receiver `p.`.
set -e
rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package rustparser ./RustLexer.g4 ./RustParser.g4 -o parser -no-listener -no-visitor
