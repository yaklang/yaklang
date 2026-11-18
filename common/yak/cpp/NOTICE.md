# C++ (CPP14) grammar provenance

Source: https://github.com/antlr/grammars-v4
Path: `cpp/CPP14Lexer.g4`, `cpp/CPP14Parser.g4`, `cpp/Go/cpp14_parser_base.go`
Commit: `de38599628d221099c046e92962f70204d852b54` (2026-11-17)

License: MIT. The two grammar files carry this header:

> The MIT License (MIT)
> Copyright (c) 2015 Camilo Sanchez (Camiloasc1) 2020 Martin Mirchev (Marti2203)

The MIT text is in `LICENSE`. The upstream `grammars-v4` tree has no
repository-level LICENSE file; each grammar directory carries its own license.
A previous assumption in the task description said Apache-2.0; that is not what
the upstream `cpp/` files state, so MIT is recorded here as the binding license.

## Local patches

1. `CPP14Parser.g4`: the two `{ this.IsPureSpecifierAllowed() }?` predicates use
   the Go target receiver `{ p.IsPureSpecifierAllowed() }?`. This matches the
   upstream `cpp/Go/transformGrammar.py` step.
2. `parser/cpp14_parser_base.go`: package renamed to `cppparser`, and the ANTLR
   runtime import retargeted from `github.com/antlr4-go/antlr/v4` to
   `github.com/yaklang/antlr/v4`, which is the runtime used by this repository.

3. `CPP14Parser.g4`: `abstractDeclarator` gains one alternative,
   `pointerOperator* LeftBracket constantExpression? RightBracket attributeSpecifierSeq?`.
   Upstream rejects unnamed array types such as the template argument in
   `std::unique_ptr<char[]>` and `std::unique_ptr<const char*[]>`, which appear in
   real projects (cxxopts). The new alternative cannot start with the same token
   as the existing ones, so it adds no ambiguity.

4. `CPP14Parser.g4`, C++17 syntax the task requires (each new alternative has a
   distinct first token or requires `...`/`constexpr`, so none adds ambiguity):
   - `primaryExpression`: `foldExpression` for fold expressions such as
     `(args + ...)`, `(... && flags)` and `(args + ... + more)`;
   - `primaryExpression`: `Noexcept LeftParen expression RightParen`, the
     `noexcept(expr)` operator (the `noexcept` *specifier* was already covered);
   - `selectionStatement`: `If Constexpr? ...` for `if constexpr`;
   - `simpleDeclaration` and `forRangeDeclaration`: structured bindings
     (`auto [a, b] = pair;`, `for (const auto& [k, v] : map)`);
   - `namespaceDefinition`: nested namespace definitions, `namespace a::b { }`.
5. `CPP14Parser.g4`, `literalOperatorId`: accepts `Operator UserDefinedLiteral`,
   so `operator""_km` written without spaces parses.
6. `CPP14Parser.g4` + `CPP14Lexer.g4`: GNU `__attribute__((...))` as an
   `attributeSpecifier`. The lexer gains an `AttributeGnu` token before
   `Identifier`; the parser accepts `balancedTokenSeq` inside the double
   parentheses. System and test headers (glibc, doctest) use this spelling.

No other grammar edits were made. Regenerate with `./gen.sh`.