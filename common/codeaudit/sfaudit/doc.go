// Package sfaudit runs codeaudit's file-content rules through the SyntaxFlow
// source-mode engine ("pointer matching").
//
// Rules are authored as embedded .sf files (mode: "source", language:
// "general") under rules/. Each rule is identified by the codeaudit finding
// ID it produces (e.g. secret.aws_access_key), so callers keep their existing
// severity/title metadata and JSON contract while the matching itself is
// delegated to SyntaxFlow:
//
//   - regexes run on the PCRE2-capable engine (lookaheads and backreferences
//     are supported, unlike Go's RE2),
//   - hits carry real file/line positions from the source-mode editors,
//   - placeholder suppression is expressed inside the rules as negative
//     lookaheads instead of Go-side post-filtering.
//
// The engine is intentionally thin: files in, structured hits out.
package sfaudit
