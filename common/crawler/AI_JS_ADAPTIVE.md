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
  -> structured request validation and credential redaction
  -> request-surface reporting
  -> local request-shape conflict veto
  -> shape-free GET URL scope check, deduplication, and scheduling
```

Every AI round is bounded by a crawler-wide call budget and deadline; empty or
duplicate model output is not scheduled. Model output is untrusted extraction
input and must pass the same URL, scope, request-shape, credential, and quota
policy as deterministic findings. A validated shape-free GET may enter the
compatibility scheduler; malformed, non-HTTP, out-of-scope, fragment-only, and
non-path values are dropped. In adaptive mode, explicit local source evidence
takes precedence over model output: when an owning call for the same canonical
host and decoded path carries a non-GET method, headers, body, or opaque request
options, a model-reported GET is retained as a finding but cannot authorize a
request. Query differences and equivalent default-port, IP, or IDNA host forms
do not bypass this veto. An unowned endpoint/API-like literal remains
report-only because its sink may be an alias elsewhere in the source. The only
unowned scheduling exception is a narrow static/config asset class (JavaScript,
source maps, WASM, CSS, config/web-manifest files, route registries, and
resource-semantic JSON names), for which a model-correlated shape-free GET may
establish a recursive asset edge. Arbitrary `/api/*.json` names are not in that
exception.

The structured finding contract preserves URL, method, headers, bounded body,
and source asset. A non-GET request, HEAD request, credential-bearing query, or
request that depends on any header/body is reported as a request surface but is
not automatically replayed. This prevents a required-header GET or POST from
being silently degraded into a misleading plain GET. Sensitive headers are
removed, sensitive query and JSON fields are redacted, and an oversized body is
represented only by omission metadata. Target and provenance URLs have hard
length caps; target URL userinfo is rejected and provenance userinfo is
removed; provenance fragments are discarded because they are never sent in an
HTTP request. Harmless query bytes are preserved exactly, while control bytes in
displayable headers, bodies, provenance, and bounded Content-Type metadata are
escaped so findings cannot forge output sections or terminal control sequences.

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

Raw URL candidates also receive a bounded JavaScript-escape pass for
`\uNNNN`, `\xNN`, and `\/`. Unresolved templates and scheme-less host-shaped
pseudo paths are rejected. A decoded extensionless endpoint is not replayed as
GET because its method/headers/body may only be recoverable through structured
analysis. A decoded file-like value remains eligible for call-site evaluation,
but its suffix alone never authorizes deterministic crawling.
Before any raw candidate enters the GET-only compatibility scheduler, its own
call site must positively prove a one-argument default-GET helper such as
`fetch(...)` / `axios.get(...)`, or an explicit asset loader such as
`import(...)` / `new Worker(...)`. A bare constant is never assumed to be GET,
even when it ends in `.json` or `.js`: it may be consumed by a distant
POST/DELETE outside any bounded local window. Explicit methods, headers,
bodies, opaque request options, aliases, and unknown/custom request helpers are
therefore deferred to structured analysis. The deferred candidate itself
raises the adaptive score to the analysis threshold so this conservative
safety gate cannot become a coverage loss.

GET proof uses a finite known-root list rather than method-name suffixes.
Standard `fetch` globals and explicit Axios/Ky/Got/request/jQuery GET entry
points are recognized; an arbitrary `api.get(...)`, `api.getJSON(...)`, or
`api.fetch(...)` remains untrusted and is deferred even when it has one
argument.

## Budgets and privacy

Adaptive defaults in the Go API cap one crawler run at eight AI calls, 256
candidate windows, 512 KiB of evidence per asset, and four seconds per call.
Adaptive mode also raises the default local half-window from the legacy 120
bytes to 512 bytes, enough to retain compact method/header/body definitions a
few hundred bytes from their request sink; an explicit context setting still
wins. Total evidence and token caps do not increase.
The AID `simple_crawler` uses a smaller low-cost profile: three calls, 96
windows, 192 KiB evidence, and no whole-small-file fast path. It inherits the
caller's context/deadline instead of imposing an additional two-second model
cutoff.

The request body is never included in model context. Sensitive query values,
the request line, Referer/Origin URLs, asset URLs, and credential-like headers
are redacted. Obsolete folded header continuation lines are conservatively
redacted in full rather than associated with a possibly misparsed header name.
Within JavaScript evidence, quoted values assigned to recognized credential
properties and credential fields inside quoted `headers`/`body` blocks are also
redacted while harmless sibling fields remain available for request-shape
analysis.
JavaScript, comments, and configuration text are explicitly treated as
untrusted data rather than model instructions.

Tests inject an `AIJSInvoker` with `WithAIJSInvokerContext`; production falls
back to LiteForge. The context-scoped seam avoids process-global mock mutation
and carries parent cancellation and deadlines through AID's
`crawler.context(CTX)` option. A Go-only mock reports production-equivalent
structured output through `AIJSExtractConfig.ReportRequestFinding`, which uses
the same sanitizer and safe-scheduling gate as LiteForge output. The legacy
URL-only callback is also gated: credential-query candidates are reported in
redacted form but are never scheduled as altered GET requests.

Repeated semantic analysis is deduplicated only for the exact canonical source
URL and SHA-256 content fingerprint. Scheme, port, `www` alias, path, query, and
subdomain boundaries are deliberately preserved because identical bytes can
depend on relative resolution, `import.meta.url`, or `currentScript`. Both the
source identity and body are hashed in the runtime key.

## Hermetic acceptance test

`crawlertest.New(t)` builds a loopback-only layered website in memory. It serves
8 KiB, 256 KiB, 1.5 MiB, and 5 MiB JavaScript assets plus route, manifest, and
`.config` layers. The 5 MiB target is encoded after 4.5 MiB and does not appear
in cleartext.

Run the essential contract with:

```bash
go test ./common/crawler \
  -run '^(TestMUSTPASS_JSHandle|TestMUSTPASS_AIJSLayeredSite|TestMUSTPASS_AIJSPCREPrefilter|TestMUSTPASS_AIJSExternalAssetBudget|TestAIJSContract|TestCoreAsset|TestCoreURL|TestRedactURLForDisplay|TestSanitizeTextForDisplay|TestDomainBlackList|TestDomainWhiteListExactPattern|TestExactOriginSeedMatcher)' \
  -count=1 -timeout=40s
```

The test itself uses a nine-second deadline and requires functional crawl time
below ten seconds. It also proves that:

- no real model or non-loopback network is used;
- the complete 5 MiB asset is never placed in one model payload;
- model calls and evidence bytes remain bounded;
- all expected internal request surfaces are recovered;
- the different-host script is reported but not downloaded;
- source assets are not downloaded twice;
- seed/request-context credentials, recognized credential-like URL-query
  values, and obvious quoted credential/header/body property values do not
  appear in model payloads. Other JavaScript source is intentionally supplied
  as bounded evidence and may itself contain secrets; callers must select an
  appropriate model/provider for that data boundary.
- structured methods, headers, bounded bodies, and source provenance survive;
- covered source-backed non-GET and header/body-dependent requests, including
  bounded `const`/`let`/`var` URL aliases, are not downgraded to GET;
- an exact duplicate of the same source URL and content does not spend another
  AI call, while same-directory filenames and query variants remain distinct.

The PCRE contract additionally proves mixed-case and no-space calls,
identifier boundaries (`fetch` versus `prefetch` / `fetcher`), paired quote
termination, escaped quotes, JavaScript regex-literal negatives, malformed
UTF-8 byte offsets, minirehs gate parity, and the invariant that every covered
dynamic trigger produces at least one bounded source-evidence window.

## Known boundary

Browser-only state, rendered DOM, post-click traffic, and values created only by
runtime-only XHR remain outside this static analyzer. The analyzer exposes
source-backed request shapes; it does not execute non-GET operations or guess
missing credentials/state. The compatibility `func(string)` path remains for
safe, shape-free GET candidates, while structured findings travel through the
separate request-surface callback. A model finding remains a bounded candidate,
not absolute proof of the source method: it is still constrained by local
conflict veto, scope, redaction, request budget, and scheduler deduplication.
