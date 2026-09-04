# Layered crawler fixture

`crawlertest` is a deterministic, loopback-only website used to evaluate crawler asset discovery and JavaScript-analysis plumbing. It does not contain real credentials, make external requests, or commit generated multi-megabyte files.

## Topology

```text
/
├── robots.txt
├── routes.txt
├── .config/runtime.config
├── .well-known/service-routes.json   (HTTP Link header only)
└── assets/
    ├── asset-manifest.json
    ├── app.small.js                 8 KiB
    ├── app.medium.js              256 KiB
    ├── app.large.js               1.5 MiB
    ├── app.huge.js                  5 MiB
    ├── .config/chunks.config
    └── chunks/
        ├── runtime.js               16 KiB
        ├── 713.compiled.js          64 KiB
        ├── 713.compiled.js.map      (SourceMap header only)
        ├── quality-ledger.91dbe763.js  96 KiB
        └── ../workers/quality-ledger.worker.js  32 KiB
```

The root document links the manifest, small entrypoint, runtime, and an external script. The route registry and well-known service description exist only in the root response's `Link` header. Runtime and configuration files lead to the compiled chunk; the compiled chunk contains only an indexed Base64 representation of the 5 MiB asset path. Its `SourceMap` response header leads to a map that names a dynamic quality-ledger chunk. That chunk derives a worker path through a computed `String.fromCharCode` member, and both the chunk and worker contain structured request surfaces that require their headers or bodies.

By default the external script is also local: a second `httptest.Server` is advertised as `localhost`, while the seed server uses `127.0.0.1`. The hostnames therefore exercise domain-scope rules without leaving the loopback interface. `WithExternalScriptURL` can replace that URL, and an empty value disables it.

## Size and encoding matrix

| Asset | Exact size | Construction | Expected request surface |
| --- | ---: | --- | --- |
| `app.small.js` | 8 KiB | mixed-case identifiers and four array fragments | mixed-case GET path |
| `app.medium.js` | 256 KiB | every path byte represented as a JavaScript `\xNN` escape, plus an unresolved template-literal negative | POST, request header, and JSON body |
| `app.large.js` | 1.5 MiB | one-line minified code, Base64, `atob`, and array indexing | PATCH plus request header |
| `app.huge.js` | 5 MiB | one-line source with neutral comment padding and `String.fromCharCode` | PUT, header, and body shape |
| `713.compiled.js` | 64 KiB | minified chunk, indexed Base64, XOR-derived array index | hidden `app.huge.js` asset path |
| dynamic chunk | 96 KiB | Base64 plus computed `String["from"+"CharCode"]` | header-gated GET and hidden Worker asset |
| worker | 32 KiB | Base64 fragment pool plus XOR-derived indexes | POST, header, and JSON body |
| runtime/config | small | `\xNN`, JSON `\u002f`, split route/config fields | next config, chunk, and request surfaces |

Both the small and dynamic-chunk GET surfaces require a custom header. They are intentionally **not** safe URL-only requests: losing the header would turn a useful request shape into a misleading 404. The split `routes.txt` GET has no custom headers or body and therefore provides the positive URL-queue control.

Ordinary asset/config edges use the same ownership rule. References recovered
from the manifest, route registry, well-known description, runtime config, and
source map are recorded as source-attributed, shape-free `GET` findings before
they enter the crawler queue. The deterministic mock calls
`ReportRequestFinding` for these edges; it never depends on a `.js`, `.json`, or
other extension being blindly replayed. This keeps the chain reproducible under
a strict request-shape contract while preserving where every edge came from.

The 5 MiB asset has exactly one `fetch(` sink. Its encoded target begins after 4.5 MiB (`GroundTruth.HugeTargetOffset`) and the cleartext target path is not present anywhere in the asset. Padding is one neutral block comment, which prevents the fixture itself from creating millions of regex candidates or JavaScript AST nodes.

## Usage

```go
func TestCrawlerAgainstLayeredSite(t *testing.T) {
    site := crawlertest.New(t)

    seed := site.URLFor(crawlertest.RootPath)
    truth := site.GroundTruth

    // Run the crawler against seed, then compare discovered assets and request
    // surfaces with truth.Findings as sets rather than relying on fetch order.
    _ = seed
    _ = truth
}
```

Useful fixture observations are available without sharing mutable internals:

```go
requests := site.Requests()
hugeDownloads := site.RequestCount(crawlertest.ScopeInternal, crawlertest.HugeJSPath)
huge, ok := site.GroundTruth.Finding(site.GroundTruth.HugeTargetID)
```

To inject a caller-owned script URL or disable the external script:

```go
site := crawlertest.New(t, crawlertest.WithExternalScriptURL(otherServer.URL+"/vendor.js"))
siteWithoutExternal := crawlertest.New(t, crawlertest.WithExternalScriptURL(""))
```

## Ground-truth semantics

Each `ExpectedFinding` records:

- the source asset and expected URL path or absolute URL;
- method, relevant headers, and a body marker where applicable;
- the encoding/derivation used by the source;
- whether discovery needs AI-assisted interpretation or expression evaluation
  (the label classifies the fixture; this test does not execute JavaScript or
  claim that a Goja sandbox is present);
- whether the item is an internal or deliberately external surface;
- whether it must only be discovered or must also be downloaded to continue the asset chain.

The oracle separates three independently scored layers:

| Layer | Meaning | Proof |
| --- | --- | --- |
| `candidate` | A normalized URL/asset was discovered | `onUrlFound` observation |
| `requested` | The crawler actually sent the expected safe HTTP request | loopback request recorder, including query and method |
| `structured` | Method, URL, headers, body, and source provenance were recovered | `AIJSRequestFinding` observation |

`GroundTruth.CoverageCounts()` supplies each denominator, while `ExpectedFinding.ExpectedLayers()` identifies the required layers per edge. A header-gated GET or POST can therefore pass `structured` without being silently counted as `requested`.

The current internal oracle contains 20 candidate edges, 20 safely requested
edges, and 8 business request shapes. Duplicate targets from different source
assets remain separate oracle edges for provenance, while the crawler still
downloads the normalized target at most once.

`MustRequest=false` is intentional for header-dependent GETs and all write operations. The fixture evaluates whether a crawler exposes a request shape; it never requires the crawler to invoke such operations. The mock server returns `405 Method Not Allowed` for a wrong method and `404 Not Found` for an incomplete query/header/body shape, so lossy replay remains observable in focused tests.

The request surface inside the external script has `MustDiscover=false` under the default seed-host scope. It becomes an additional assertion only in a test that explicitly permits and downloads the `localhost` external asset.

## Test contract

A complete crawler integration test should assert all of the following:

1. exact asset sizes and the huge-tail offset match `GroundTruth`;
2. every `MustCandidate`, `MustRequest`, and `MustStructure` entry is recovered in its own set;
3. every `MustRequest` asset is downloaded no more than once;
4. the encoded 5 MiB target is recovered without sending the complete asset to an AI callback;
5. the external script URL is discovered but is not fetched unless its hostname is explicitly allowed;
6. AI callback count, maximum payload, total evidence bytes, HTTP request count, and elapsed time are logged on failure;
7. the functional test uses an 8–9 second context deadline and asserts execution under 10 seconds;
8. the mock returns a finding only when the payload contains source-specific
   evidence, so a source URL alone cannot make the test pass;
9. the matcher contract covers mixed casing, minified calls without spaces,
   escaped member names, Base64 / `fromCharCode` anchors, paired quote and URL
   terminator boundaries, and JavaScript regex-literal negatives;
10. every direct PCRE positive is admitted by its minirehs gate, and every
    trigger-level positive yields a bounded evidence window.

## Redhaze-style acceptance matrix

This local fixture mirrors the discovery mechanisms used by the ColdChain family without making CI depend on a public host:

| Scenario family | Local source of truth | Required observation |
| --- | --- | --- |
| response-only metadata | `Link`, `SourceMap` headers | candidate then requested asset |
| route/config indirection | `.well-known`, `.config`, routes, manifest | reconstructed asset or request edge |
| compiled loaders | runtime, chunk config, source map, dynamic import | complete high-priority asset chain |
| compact/obfuscated code | `\xNN`, `\uNNNN`, Base64, numeric arrays, XOR indexes | bounded evidence reaches the mock AI |
| oversized bundle | 5 MiB single-line tail payload | hidden request shape, payload stays far below full bundle size |
| safety boundary | header-gated GET, POST/PATCH/PUT | structured report exists; no automatic request |
| domain boundary | `127.0.0.1` seed, `localhost` external script | external URL visible but not fetched without explicit scope |

Public `redhaze.top` crawling is a manual, authorized compatibility check, never a CI dependency. CI uses only loopback servers and a context-scoped deterministic AI mock; compilation is excluded from the functional timer, while the crawl itself has a nine-second deadline and a strict ten-second assertion.

The AID tool's `ai-js=auto` (3 calls) and `ai-js=yes` (8 calls) defaults are
budgeted coverage profiles, not completeness guarantees. The full layered-site
integration explicitly uses `ai-js=yes` with `ai-js-max-requests=16`. Its result
is deterministic mocked pipeline coverage—asset scheduling, bounded evidence,
request-shape reporting, and safety disposition—not a claim about live-model
extraction accuracy.

Do not run the 5 MiB integration case with `t.Parallel()`. Although the source itself is only 5 MiB, HTTP packet buffers, strings, reducer windows, and execution engines can hold several copies at once. The generated filler deliberately contains no path-shaped noise so runtime and memory measurements reflect crawler behavior rather than fixture-induced regex explosion.
