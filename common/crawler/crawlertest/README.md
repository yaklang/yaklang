# Layered crawler fixture

`crawlertest` is a deterministic, loopback-only website used to evaluate crawler asset discovery and JavaScript-analysis plumbing. It does not contain real credentials, make external requests, or commit generated multi-megabyte files.

## Topology

```text
/
├── robots.txt
├── routes.txt
├── .config/runtime.config
└── assets/
    ├── asset-manifest.json
    ├── app.small.js                 8 KiB
    ├── app.medium.js              256 KiB
    ├── app.large.js               1.5 MiB
    ├── app.huge.js                  5 MiB
    ├── .config/chunks.config
    └── chunks/
        ├── runtime.js               16 KiB
        └── 713.compiled.js          64 KiB
```

The root document links the manifest, route registry, small entrypoint, runtime, and an external script. The manifest exposes the ordinary entrypoints. Runtime and configuration files lead to the compiled chunk; the compiled chunk contains only an indexed Base64 representation of the 5 MiB asset path.

By default the external script is also local: a second `httptest.Server` is advertised as `localhost`, while the seed server uses `127.0.0.1`. The hostnames therefore exercise domain-scope rules without leaving the loopback interface. `WithExternalScriptURL` can replace that URL, and an empty value disables it.

## Size and encoding matrix

| Asset | Exact size | Construction | Expected request surface |
| --- | ---: | --- | --- |
| `app.small.js` | 8 KiB | mixed-case identifiers and four array fragments | mixed-case GET path |
| `app.medium.js` | 256 KiB | every path byte represented as a JavaScript `\xNN` escape | POST plus request header |
| `app.large.js` | 1.5 MiB | one-line minified code, Base64, `atob`, and array indexing | PATCH plus request header |
| `app.huge.js` | 5 MiB | one-line source with neutral comment padding and `String.fromCharCode` | PUT, header, and body shape |
| `713.compiled.js` | 64 KiB | minified chunk, indexed Base64, XOR-derived array index | hidden `app.huge.js` asset path |
| runtime/config | small | `\xNN`, JSON `\u002f`, split route/config fields | next config, chunk, and request surfaces |

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
- whether discovery needs AI-assisted interpretation or Goja-style execution;
- whether the item is an internal or deliberately external surface;
- whether it must only be discovered or must also be downloaded to continue the asset chain.

`MustRequest=false` is intentional for business request surfaces. The fixture evaluates whether a crawler exposes a request shape; it does not require the crawler to invoke every discovered operation. The mock server returns `405 Method Not Allowed` when a request surface is called with the wrong method so method-loss is visible in integration tests.

The request surface inside the external script has `MustDiscover=false` under the default seed-host scope. It becomes an additional assertion only in a test that explicitly permits and downloads the `localhost` external asset.

## Test contract

A complete crawler integration test should assert all of the following:

1. exact asset sizes and the huge-tail offset match `GroundTruth`;
2. every `MustDiscover` entry is recovered, with results compared as sets;
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

Do not run the 5 MiB integration case with `t.Parallel()`. Although the source itself is only 5 MiB, HTTP packet buffers, strings, reducer windows, and execution engines can hold several copies at once. The generated filler deliberately contains no path-shaped noise so runtime and memory measurements reflect crawler behavior rather than fixture-induced regex explosion.
