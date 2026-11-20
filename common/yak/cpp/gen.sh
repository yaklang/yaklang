#!/bin/sh
# ANTLR 4.13.2 codegen for the C++ frontend.
# The grammar is maintained in-tree; MIT attribution is in LICENSE and the
# grammar file headers.
# Local patches:
#   1. semantic predicates use the Go receiver `p.` instead of `this.`;
#   2. parser/ holds CPPParserBase, retargeted to github.com/yaklang/antlr/v4
#      so it shares the generated parser package;
#   3. abstractDeclarator accepts unnamed array types (`unique_ptr<char[]>`),
#      which upstream rejects;
#   4. C++17 additions used by the corpus: fold expressions, if constexpr,
#      structured bindings, nested namespace definitions, noexcept operator;
#   5. literalOperatorId accepts `operator""_suffix` without spaces;
#   6. GNU __attribute__((...)) (lexer token AttributeGnu + parser attribute).
set -e
rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package cppparser ./CPPLexer.g4 ./CPPParser.g4 -o parser -no-listener -no-visitor