# Rust syntax fixtures

All files in this directory are original fixtures written for the yaklang Rust
frontend. They are licensed under the repository license.

`regression_let_else.rs` records constructs observed while parsing real
projects, but no source code was copied:

- inner block documentation (`//!`) and multiline inner attributes
  (`#![allow(\n ...)]`), observed in [ripgrep](https://github.com/BurntSushi/ripgrep)
  (MIT/Unlicense).
- `let ... else` statements, observed in ripgrep and
  [serde](https://github.com/serde-rs/serde) (MIT/Apache-2.0).
- associated-type bounds (`type Error: std::error::Error;`), observed in serde
  (MIT/Apache-2.0).

Real project checkouts are not committed. The corpus runner is
`scripts/ssa-lang-corpus`.
