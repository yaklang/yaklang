# DFA legacy fixtures

These are unmodified outputs of the production Compile/MarshalImage/blob writer
at 702a2b79f0f7d4ec133706b182f742a3f7ad237a, Go 1.22.12, CGO=1,
clang 22.1.8. The test-only writer used the same two rules as TestProgramDFAFixture:
ID 0 `[ab][cd]`, ID 1 `[ab]{2}`, MVS existence mode, default options.
No production traffic or external rule data is included.

| Fixture | Version | Bytes | SHA256 |
| --- | --- | ---: | --- |
| compile-image-v5-dfa.rehs | image5 / blob3 | 236 | d2042655323afe67d3c85139e6a29726201e09de7f6f704e57ce240502d4f87e |
| native-blob-v3-dfa.bin | blob3 | 6128 | f6192917e3e16fdc86e4c6d1780f982b9006ab9ebbae28032404789fdda5ef3b |

Before the fix, native individual execution of ID 0 matches `c`, and ID 1 matches
`b`; native execution after LoadImage repeats the errors. The merged NFA path is
correct. New tests require safe NFA fallback on these exact old tables, retained
byte-identical image export, and a single disk-cache refresh. Do not regenerate
these legacy files with the corrected producer.
