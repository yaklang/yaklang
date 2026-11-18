#!/bin/sh
# ANTLR 4.13.2 codegen for the C++ frontend.
# Grammar source: antlr/grammars-v4 cpp/CPP14Lexer.g4 + cpp/CPP14Parser.g4
# pinned at de38599628d221099c046e92962f70204d852b54 (MIT, see NOTICE.md).
# Local patches:
#   1. semantic predicates use the Go receiver `p.` instead of `this.`;
#   2. parser/ holds CPP14ParserBase from grammars-v4/cpp/Go, retargeted to
#      github.com/yaklang/antlr/v4 so it shares the generated parser package;
#   3. abstractDeclarator accepts unnamed array types (`unique_ptr<char[]>`),
#      which upstream rejects;
#   4. C++17 additions required by the task: fold expressions, if constexpr,
#      structured bindings, nested namespace definitions, noexcept operator;
#   5. literalOperatorId accepts `operator""_suffix` without spaces;
#   6. GNU __attribute__((...)) (lexer token AttributeGnu + parser attribute).
set -e
rm -f ./parser/*.tokens
rm -f ./parser/*.interp
../antlr4util/antlr4 -Dlanguage=Go -package cppparser ./CPP14Lexer.g4 ./CPP14Parser.g4 -o parser -no-listener -visitor