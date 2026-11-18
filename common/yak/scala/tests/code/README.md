# Scala syntax fixtures

All files in this directory are original fixtures written for the yaklang Scala
frontend. They are licensed under the repository license.

`regression_import_annotation.scala` records constructs observed while parsing
real projects, but no source code was copied:

- renamed and wildcard import selectors (`{IO => CIO, _}`), observed in
  [ZIO](https://github.com/zio/zio) (Apache-2.0).
- annotations on their own line before a definition, observed in ZIO and
  [Scalatra](https://github.com/scalatra/scalatra) (BSD-3-Clause).
- `private[this]` access qualifiers, observed in Scalatra (BSD-3-Clause).

The frontend uses a single Scala 3 grammar with a compatibility switch for
frequent Scala 2 syntax. `regression_layout.scala` also covers self-type
bodies, unicode character literals, indentation-only `if` bodies and
`for`-comprehension guards, observed in Scalatra and ZIO (same licenses as
above).

Real project checkouts are not committed. The corpus runner is
`scripts/ssa-lang-corpus`.
