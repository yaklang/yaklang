# Adaptive JavaScript asset analysis

The crawler can optionally discover request candidates from JavaScript and
configuration assets without launching a browser or executing site code. The
adaptive path is additive: existing `crawler.aiJSExtract()` callers retain the
legacy behavior unless they also pass `crawler.aiJSAdaptive(true)`.

## Pipeline

```text
HTML / text / config / JavaScript asset
  -> deterministic URL and file-path extraction
  -> lexical masking and local trigger scoring
  -> minirehs literal prefilter
  -> Go PCRE Lite precise matching
  -> bounded, source-attributed evidence windows
  -> optional AI analysis
  -> URL validation and seed-host scope check
  -> normal crawler deduplication and scheduling
```

Every AI round is bounded by a crawler-wide call budget and deadline; empty or
duplicate model output is not scheduled. Model output is never treated as
authority: malformed, non-HTTP, out-of-scope, fragment-only, and non-path
values are dropped before scheduling.

Assets remain separate. Each model payload identifies its source URL and byte
offsets; the crawler never concatenates a 5 MiB bundle with every other page
asset. Candidate quotas are distributed across signal families and file
regions so dense paths near the beginning cannot starve an encoded request at
the end of a one-line bundle.

## Lexical and matcher layers

The analyzer does not run one Go `regexp` expression after another over a
multi-megabyte bundle. Matching is split into four bounded layers:

1. a lightweight lexer masks comments, documentation strings, and JavaScript
   regex literals, decodes `\xNN` / `\uNNNN` member names, and canonicalizes
   bracket calls such as `globalThis["fetch"](endpoint)`;
2. minirehs performs one case-insensitive existence scan for the mandatory
   literal gates of each rule;
3. only admitted rules run through the low-level Go PCRE Lite API, which gives
   exact byte offsets and supports non-consuming identifier boundaries,
   lookaround, and paired-quote backreferences;
4. fixed source regions receive independent quotas and bounded overlap, so a
   no-hit region cannot rescan the rest of a 5 MiB file and front-loaded noise
   cannot starve a tail signal.

The PCRE rules use byte mode rather than UTF validation. This preserves source
offsets and permits malformed response bytes while the rules themselves remain
ASCII-oriented. Every segmented extraction rule has an explicit maximum width;
overlap is larger than that width, and matches touching an artificial right edge
are rejected. Match and recursion depth are capped. A PCRE resource-limit error
fails open into bounded literal-centered evidence instead of becoming a silent
no-match. A minirehs match may be a false positive, but it must never be a false
negative for its associated PCRE rule; the CI parity corpus checks that invariant.

## Trigger signals

Literal paths are emitted locally and do not need AI by themselves. The default
adaptive threshold is three points:

| Signal | Score |
| --- | ---: |
| dynamic request expression | 3 |
| request sink | 1 |
| route/config metadata | 1 |
| string assembly | 2 |
| encoded or obfuscated value | 2 |
| compiled chunk runtime | 1 |

Matching is boundary-aware and covers mixed casing, minified calls without
spaces, multiline expressions, property/bracket calls, dynamic imports,
JavaScript hex and Unicode escapes, Base64 decoders, `String.fromCharCode`,
array joins, and Webpack-style chunk runtimes. Comments, documentation strings,
`prefetch`, and identifiers such as `myfetch` are negative controls and must
remain below the trigger threshold.

## Budgets and privacy

Adaptive defaults in the Go API cap one crawler run at eight AI calls, 256
candidate windows, 512 KiB of evidence per asset, and four seconds per call.
The AID `simple_crawler` uses a smaller low-cost profile: three calls, 96
windows, 192 KiB evidence, two seconds per call, and no whole-small-file fast
path.

The request body is never included in model context. Sensitive query values,
the request line, Referer/Origin URLs, asset URLs, and credential-like headers
are redacted. JavaScript, comments, and configuration text are explicitly
treated as untrusted data rather than model instructions.

Tests inject an `AIJSInvoker` with `WithAIJSInvokerContext`; production falls
back to LiteForge. The context-scoped seam avoids process-global mock mutation
and carries parent cancellation and deadlines through AID's
`crawler.context(CTX)` option.

## Hermetic acceptance test

`crawlertest.New(t)` builds a loopback-only layered website in memory. It serves
8 KiB, 256 KiB, 1.5 MiB, and 5 MiB JavaScript assets plus route, manifest, and
`.config` layers. The 5 MiB target is encoded after 4.5 MiB and does not appear
in cleartext.

Run the essential contract with:

```bash
go test ./common/crawler \
  -run '^(TestMUSTPASS_AIJSLayeredSite|TestMUSTPASS_AIJSPCREPrefilter)$' \
  -count=1 -timeout=20s
```

The test itself uses a nine-second deadline and requires functional crawl time
below ten seconds. It also proves that:

- no real model or non-loopback network is used;
- the complete 5 MiB asset is never placed in one model payload;
- model calls and evidence bytes remain bounded;
- all expected internal request surfaces are recovered;
- the different-host script is reported but not downloaded;
- source assets are not downloaded twice;
- credentials do not appear in model payloads.

The PCRE contract additionally proves mixed-case and no-space calls,
identifier boundaries (`fetch` versus `prefetch` / `fetcher`), paired quote
termination, escaped quotes, JavaScript regex-literal negatives, malformed
UTF-8 byte offsets, minirehs gate parity, and the invariant that every covered
dynamic trigger produces at least one bounded source-evidence window.

## Known boundary

The compatibility callback is still `func(string)`, so AI findings are fed back
as URL candidates and scheduled as GET requests. Method, headers, and body shape
can be observed in the fixture ground truth, but are not yet preserved by this
legacy callback. Browser-only state, rendered DOM, post-click traffic, and
runtime-only XHR remain outside this static analyzer. External script downloads
also still use the historical direct fetch path, but they now reserve a
crawler-wide request-budget slot before I/O and share an in-flight dedup key
with scheduled requests.
