# Kotlin syntax fixtures

All files in this directory are original fixtures written for the yaklang
Kotlin frontend. They are licensed under the repository license.

`regression_when_range.kt` records constructs observed while parsing real
projects, but no source code was copied:

- `when (val ... = ...)` subjects and `in a..<b` range-until conditions, observed
  in [kotlinx.coroutines](https://github.com/Kotlin/kotlinx.coroutines)
  (Apache-2.0).
- `value class` modifiers, unsigned integer literals, Kotlin 2 multi-dollar
  string interpolation and definitely non-null intersection types (`T & Any`),
  observed in kotlinx.coroutines and [OkHttp](https://github.com/square/okhttp)
  (Apache-2.0).

`regression_gradle.kts` is an original Gradle-style script fixture. It records
`plugins`, `repositories`, `dependencies`, and task configuration blocks
observed in [OkHttp](https://github.com/square/okhttp) and
[kotlinx.coroutines](https://github.com/Kotlin/kotlinx.coroutines), both
Apache-2.0. No project source was copied.

Gradle `.kts` scripts use the grammar's `script` entry rule and are tested
separately from Kotlin source files. Real project checkouts are not committed.
The corpus runner is `scripts/ssa-lang-corpus`.
