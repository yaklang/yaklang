// Package crawlertest provides deterministic, loopback-only web fixtures for
// crawler tests. Large JavaScript assets are generated in memory at test time;
// the repository therefore does not need to carry multi-megabyte fixture files.
package crawlertest

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const (
	RootPath           = "/"
	RobotsPath         = "/robots.txt"
	RoutesPath         = "/routes.txt"
	RuntimeConfigPath  = "/.config/runtime.config"
	HeaderRoutesPath   = "/.well-known/service-routes.json"
	ManifestPath       = "/assets/asset-manifest.json"
	ChunkConfigPath    = "/assets/.config/chunks.config"
	RuntimeJSPath      = "/assets/chunks/runtime.js"
	CompiledChunkPath  = "/assets/chunks/713.compiled.js"
	SourceMapPath      = "/assets/chunks/713.compiled.js.map"
	DynamicChunkPath   = "/assets/chunks/quality-ledger.91dbe763.js"
	WorkerPath         = "/assets/workers/quality-ledger.worker.js"
	SmallJSPath        = "/assets/app.small.js"
	MediumJSPath       = "/assets/app.medium.js"
	LargeJSPath        = "/assets/app.large.js"
	HugeJSPath         = "/assets/app.huge.js"
	ExternalScriptPath = "/vendor/external-mixed.js"

	SmallJSSize  = 8 * 1024
	MediumJSSize = 256 * 1024
	LargeJSSize  = 1536 * 1024
	HugeJSSize   = 5 * 1024 * 1024

	// HugePayloadOffset is 4.75 MiB into the 5 MiB asset. The sole request
	// target in that asset starts here, safely beyond the 4.5 MiB boundary.
	HugePayloadOffset = 19 * 256 * 1024
	HugeMinimumOffset = 9 * 512 * 1024

	RoutesTargetPath   = "/service/v1/routes/export"
	ConfigTargetPath   = "/gateway/v1/runtime/bootstrap"
	SmallTargetPath    = "/aPi/V1/Catalog/MixedCase"
	MediumTargetPath   = "/api/v2/escaped/dispatch"
	LargeTargetPath    = "/api/v3/encoded/quote"
	HugeTargetPath     = "/api/v4/internal/reconciliation/commit"
	DynamicTargetPath  = "/api/quality/v2/ledger/snapshot"
	WorkerTargetPath   = "/api/quality/v2/ledger/export"
	ExternalTargetPath = "/partner/v1/telemetry/schema"

	compiledChunkSize = 64 * 1024
	dynamicChunkSize  = 96 * 1024
	workerJSSize      = 32 * 1024
	runtimeJSSize     = 16 * 1024
)

// Scope identifies whether an asset or finding belongs to the seed host or to
// the deliberately different hostname used by the local external-script mock.
type Scope string

const (
	ScopeInternal Scope = "internal"
	ScopeExternal Scope = "external"
)

// FindingKind distinguishes navigable assets from request surfaces.
type FindingKind string

const (
	FindingAsset   FindingKind = "asset"
	FindingConfig  FindingKind = "config"
	FindingRequest FindingKind = "request"
)

// CoverageLayer separates static discovery from actual HTTP observation and
// request-shape recovery. A URL candidate is not proof that it was requested,
// and neither layer preserves a non-GET method/header/body contract.
type CoverageLayer string

const (
	CoverageCandidate  CoverageLayer = "candidate"
	CoverageRequested  CoverageLayer = "requested"
	CoverageStructured CoverageLayer = "structured"
)

// AssetGroundTruth describes one served source asset. Path is an absolute URL
// only for an external asset; internal assets use URL paths.
type AssetGroundTruth struct {
	Path        string
	ContentType string
	Size        int
	Layer       string
	Scope       Scope
}

// ExpectedFinding is the structured oracle consumed by crawler tests. A
// finding with MustRequest=false is a discovery target, not an instruction for
// the test crawler to invoke a business operation.
type ExpectedFinding struct {
	ID            string
	SourceAsset   string
	Value         string
	Method        string
	Headers       map[string]string
	Body          string
	BodyContains  string
	RawQuery      string
	Kind          FindingKind
	Encoding      string
	SourceOffset  int
	Scope         Scope
	MustDiscover  bool
	MustCandidate bool
	MustRequest   bool
	MustStructure bool
	RequiresAI    bool
	// RequiresEvaluation classifies scenarios whose value is assembled rather
	// than present as one literal. It is fixture metadata only: the current
	// crawler proves bounded evidence delivery to the mocked AI contract; it
	// does not claim to execute JavaScript or provide a Goja runtime.
	RequiresEvaluation bool
}

// ExpectedLayers returns the independently asserted coverage layers for a
// finding. Keeping this derivation on the oracle prevents integration tests
// from silently treating a discovered string as a verified request surface.
func (f ExpectedFinding) ExpectedLayers() []CoverageLayer {
	layers := make([]CoverageLayer, 0, 3)
	if f.MustCandidate {
		layers = append(layers, CoverageCandidate)
	}
	if f.MustRequest {
		layers = append(layers, CoverageRequested)
	}
	if f.MustStructure {
		layers = append(layers, CoverageStructured)
	}
	return layers
}

// GroundTruth contains all deterministic fixture expectations.
type GroundTruth struct {
	Assets            []AssetGroundTruth
	Findings          []ExpectedFinding
	HugeTargetID      string
	HugeTargetOffset  int
	ExternalScriptURL string
}

// Finding returns a ground-truth entry by stable ID.
func (g GroundTruth) Finding(id string) (ExpectedFinding, bool) {
	for _, finding := range g.Findings {
		if finding.ID == id {
			return finding, true
		}
	}
	return ExpectedFinding{}, false
}

// Asset returns an asset description by its internal path or absolute external
// URL.
func (g GroundTruth) Asset(path string) (AssetGroundTruth, bool) {
	for _, asset := range g.Assets {
		if asset.Path == path {
			return asset, true
		}
	}
	return AssetGroundTruth{}, false
}

// CoverageCounts reports the ground-truth denominator for each independent
// layer. Tests should compare their observed numerators with these values.
func (g GroundTruth) CoverageCounts() map[CoverageLayer]int {
	counts := map[CoverageLayer]int{
		CoverageCandidate:  0,
		CoverageRequested:  0,
		CoverageStructured: 0,
	}
	for _, finding := range g.Findings {
		if finding.Scope != ScopeInternal || !finding.MustDiscover {
			continue
		}
		for _, layer := range finding.ExpectedLayers() {
			counts[layer]++
		}
	}
	return counts
}

// RequestRecord is a stable request log without timestamps, making set/count
// assertions deterministic even when the crawler fetches concurrently.
type RequestRecord struct {
	Method   string
	Host     string
	Path     string
	RawQuery string
	Scope    Scope
	Headers  map[string]string
	Body     string
}

type fixtureAsset struct {
	body        []byte
	contentType string
	headers     map[string]string
}

type requestRecorder struct {
	mu       sync.Mutex
	requests []RequestRecord
}

func (r *requestRecorder) add(req *http.Request, scope Scope) {
	body, _ := io.ReadAll(io.LimitReader(req.Body, 64*1024))
	if req.Body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
	}
	headers := make(map[string]string, len(req.Header))
	for key, values := range req.Header {
		headers[http.CanonicalHeaderKey(key)] = strings.Join(values, ", ")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, RequestRecord{
		Method:   req.Method,
		Host:     req.Host,
		Path:     req.URL.Path,
		RawQuery: req.URL.RawQuery,
		Scope:    scope,
		Headers:  headers,
		Body:     string(body),
	})
}

func (r *requestRecorder) snapshot() []RequestRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RequestRecord(nil), r.requests...)
}

// Option customizes a Site.
type Option func(*siteOptions)

type siteOptions struct {
	externalScriptURL *string
}

// WithExternalScriptURL replaces the default loopback-only external script.
// Passing an empty string removes the external script from the fixture.
func WithExternalScriptURL(rawURL string) Option {
	return func(options *siteOptions) {
		value := strings.TrimSpace(rawURL)
		options.externalScriptURL = &value
	}
}

// Site is a deterministic layered mock website. New registers Close with the
// supplied test, but callers may also close it explicitly.
type Site struct {
	Server            *httptest.Server
	URL               string
	ExternalScriptURL string
	ExternalURL       string
	GroundTruth       GroundTruth

	assets           map[string]fixtureAsset
	expectedRequests map[string]ExpectedFinding
	recorder         *requestRecorder
	externalServer   *httptest.Server
	closeOnce        sync.Once
}

// New starts the fixture entirely on loopback interfaces. By default, the
// external script is served by a second httptest server and referenced through
// the hostname "localhost", while the seed server uses "127.0.0.1". This gives
// crawler scope tests two distinct hostnames without making an external
// network request.
func New(t testing.TB, opts ...Option) *Site {
	t.Helper()

	options := &siteOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(options)
		}
	}

	if options.externalScriptURL != nil && *options.externalScriptURL != "" {
		if err := validateHTTPURL(*options.externalScriptURL); err != nil {
			t.Fatalf("crawlertest: invalid external script URL: %v", err)
		}
	}

	recorder := &requestRecorder{}
	site := &Site{recorder: recorder}

	externalURL := ""
	localExternal := false
	if options.externalScriptURL == nil {
		localExternal = true
		externalBody := buildExternalJS()
		site.externalServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder.add(r, ScopeExternal)
			if r.URL.Path == ExternalTargetPath {
				if r.Method != http.MethodOptions {
					w.Header().Set("Allow", http.MethodOptions)
					http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
					return
				}
				body := []byte(`{"ok":true,"scope":"external","method":"OPTIONS"}`)
				writeResponse(w, r, http.StatusOK, "application/json; charset=utf-8", body, nil)
				return
			}
			if r.URL.Path != ExternalScriptPath {
				http.NotFound(w, r)
				return
			}
			if r.Method != http.MethodGet && r.Method != http.MethodHead {
				w.Header().Set("Allow", "GET, HEAD")
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			writeResponse(w, r, http.StatusOK, "application/javascript; charset=utf-8", externalBody, nil)
		}))

		parsed, err := url.Parse(site.externalServer.URL)
		if err != nil {
			site.externalServer.Close()
			t.Fatalf("crawlertest: parse local external server URL: %v", err)
		}
		site.ExternalURL = parsed.Scheme + "://localhost:" + parsed.Port()
		externalURL = site.ExternalURL + ExternalScriptPath
	} else {
		externalURL = *options.externalScriptURL
		if externalURL != "" {
			parsed, _ := url.Parse(externalURL)
			site.ExternalURL = parsed.Scheme + "://" + parsed.Host
		}
	}

	assets, truth, err := buildAssets(externalURL, localExternal)
	if err != nil {
		if site.externalServer != nil {
			site.externalServer.Close()
		}
		t.Fatalf("crawlertest: build assets: %v", err)
	}

	site.assets = assets
	site.GroundTruth = truth
	site.ExternalScriptURL = externalURL
	site.expectedRequests = make(map[string]ExpectedFinding)
	for _, finding := range truth.Findings {
		if finding.Scope == ScopeInternal && finding.Kind == FindingRequest {
			site.expectedRequests[finding.Value] = finding
		}
	}

	site.Server = httptest.NewServer(site)
	site.URL = site.Server.URL
	t.Cleanup(site.Close)
	return site
}

// Close stops both loopback servers. It is safe to call more than once.
func (s *Site) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.Server != nil {
			s.Server.Close()
		}
		if s.externalServer != nil {
			s.externalServer.Close()
		}
	})
}

// URLFor resolves an internal fixture path against the seed server URL.
func (s *Site) URLFor(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return strings.TrimRight(s.URL, "/") + "/" + strings.TrimLeft(path, "/")
}

// Requests returns a copy of all requests received by both local servers.
func (s *Site) Requests() []RequestRecord {
	if s == nil || s.recorder == nil {
		return nil
	}
	return s.recorder.snapshot()
}

// RequestCount counts requests by scope and URL path.
func (s *Site) RequestCount(scope Scope, path string) int {
	count := 0
	for _, request := range s.Requests() {
		if request.Scope == scope && request.Path == path {
			count++
		}
	}
	return count
}

// ServeHTTP implements the seed mock site.
func (s *Site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.recorder.add(r, ScopeInternal)

	if asset, ok := s.assets[r.URL.Path]; ok {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writeResponse(w, r, http.StatusOK, asset.contentType, asset.body, asset.headers)
		return
	}

	if expected, ok := s.expectedRequests[r.URL.Path]; ok {
		if r.Method != expected.Method {
			w.Header().Set("Allow", expected.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if expected.RawQuery != "" && r.URL.RawQuery != expected.RawQuery {
			http.NotFound(w, r)
			return
		}
		for key, value := range expected.Headers {
			if r.Header.Get(key) != value {
				http.NotFound(w, r)
				return
			}
		}
		if expected.BodyContains != "" {
			body, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
			if !bytes.Contains(body, []byte(expected.BodyContains)) {
				http.NotFound(w, r)
				return
			}
		}
		// Business request-surface responses are deliberately neutral. Echoing the
		// request path here would create a second AI-analysis source unrelated to
		// the asset graph and make the fixed crawler-wide model budget depend on
		// scheduler order.
		body := []byte(`{"ok":true}`)
		writeResponse(w, r, http.StatusOK, "application/json; charset=utf-8", body, nil)
		return
	}

	http.NotFound(w, r)
}

func buildAssets(externalScriptURL string, localExternal bool) (map[string]fixtureAsset, GroundTruth, error) {
	smallCode := `(()=>{const PaRtS=["/aPi","/V1","/Catalog","/MixedCase","?region=north"];const TaRgEt=PaRtS.join("");fetch(TaRgEt,{method:["G","E","T"].join(""),headers:{"X-Client-Surface":"catalog-matrix"}});})();`
	small, err := sizedSingleLineJS(smallCode, SmallJSSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}

	mediumEscaped := hexEscapeASCII(MediumTargetPath)
	mediumCode := `(()=>{const _escaped="` + mediumEscaped + `";const _method=["P","O","S","T"].join("");const _noise="/api/${tenant}/unresolved";fetch(_escaped,{method:_method,headers:{"X-Route-Profile":"escaped"},body:JSON.stringify({dispatch:"priority"})});})();`
	medium, err := sizedSingleLineJS(mediumCode, MediumJSSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}

	largeTarget := base64.StdEncoding.EncodeToString([]byte(LargeTargetPath))
	largeMethod := base64.StdEncoding.EncodeToString([]byte(http.MethodPatch))
	largeCode := `(()=>{const _0x91=["` + largeTarget + `","` + largeMethod + `","bm9pc2U="];const _0x2f=i=>atob(_0x91[i]);fetch(_0x2f(0),{method:_0x2f(1),headers:{"X-Chunk-Mode":"indexed"}});})();`
	large, err := sizedSingleLineJS(largeCode, LargeJSSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}

	huge, hugeTargetOffset, err := buildHugeJS()
	if err != nil {
		return nil, GroundTruth{}, err
	}

	runtimeCode := `(()=>{const RuNtImE={cOnFiG:"\x2fassets\x2f.config\x2fchunks.config"};fetch(RuNtImE.cOnFiG).then(r=>r.json()).then(c=>import(c.chunks["713"]));})();`
	runtimeJS, err := sizedSingleLineJS(runtimeCode, runtimeJSSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}

	hugeAssetEncoded := base64.StdEncoding.EncodeToString([]byte(HugeJSPath))
	chunkCode := `(()=>{const _0x8a=["bm9pc2U=","` + hugeAssetEncoded + `"];const _0x4d=i=>atob(_0x8a[i^1]);import(_0x4d(0));})();`
	compiledChunk, err := sizedSingleLineJS(chunkCode, compiledChunkSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}

	if bytes.Contains(small, []byte(SmallTargetPath)) {
		return nil, GroundTruth{}, fmt.Errorf("small target unexpectedly appears in plaintext")
	}
	if bytes.Contains(medium, []byte(MediumTargetPath)) {
		return nil, GroundTruth{}, fmt.Errorf("medium target unexpectedly appears in plaintext")
	}
	if bytes.Contains(large, []byte(LargeTargetPath)) {
		return nil, GroundTruth{}, fmt.Errorf("large target unexpectedly appears in plaintext")
	}
	if bytes.Contains(compiledChunk, []byte(HugeJSPath)) {
		return nil, GroundTruth{}, fmt.Errorf("huge asset path unexpectedly appears in chunk plaintext")
	}

	dynamicWorkerCodes := asciiCharCodes(WorkerPath)
	dynamicTarget := base64.StdEncoding.EncodeToString([]byte(DynamicTargetPath + "?view=delta"))
	dynamicCode := `(()=>{const _worker=String["from"+"CharCode"](` + dynamicWorkerCodes + `);new Worker(_worker);const _endpoint=atob("` + dynamicTarget + `");fetch(_endpoint,{method:"GET",headers:{"X-ColdChain-Module":"quality-ledger"}});})();`
	dynamicChunk, err := sizedSingleLineJS(dynamicCode, dynamicChunkSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}

	workerTarget := base64.StdEncoding.EncodeToString([]byte(WorkerTargetPath))
	workerMethod := base64.StdEncoding.EncodeToString([]byte(http.MethodPost))
	workerCode := `(()=>{const _pool=["` + workerMethod + `","` + workerTarget + `"];const _decode=i=>atob(_pool[i^1]);fetch(_decode(0),{method:_decode(1),headers:{"X-ColdChain-Worker":"ledger-export"},body:JSON.stringify({format:"ndjson"})});})();`
	workerJS, err := sizedSingleLineJS(workerCode, workerJSSize)
	if err != nil {
		return nil, GroundTruth{}, err
	}
	if bytes.Contains(dynamicChunk, []byte(WorkerPath)) || bytes.Contains(dynamicChunk, []byte(DynamicTargetPath)) {
		return nil, GroundTruth{}, fmt.Errorf("dynamic chunk targets unexpectedly appear in plaintext")
	}
	if bytes.Contains(workerJS, []byte(WorkerTargetPath)) {
		return nil, GroundTruth{}, fmt.Errorf("worker target unexpectedly appears in plaintext")
	}

	routesBody := []byte("# generated route registry\nbase=/service\nversion=/v1\nresource=/routes\naction=/export\nquery=format=full\nmethod=GET\nnext=/.config/runtime.config\n")
	runtimeConfigBody := []byte(`{"service_segments":["/gateway","/v1","/runtime","/bootstrap"],"method":["PO","ST"],"headers":{"X-Runtime-Profile":"bootstrap"},"body":{"mode":"hydrate"},"chunk_config":"\u002fassets\u002f.config\u002fchunks.config","asset_manifest":"/assets/asset-manifest.json"}`)
	chunkConfigBody := []byte(`{"chunks":{"713":"\u002fassets\u002fchunks\u002f713.compiled.js"},"load_order":[713]}`)
	manifestBody := []byte(`{"entrypoints":["/assets/app.small.js","/assets/app.medium.js","/assets/app.large.js","/assets/chunks/runtime.js"],"routes":"/routes.txt","runtime_config":"/.config/runtime.config"}`)
	headerRoutesBody := []byte(`{"catalog":"coldchain-operations","runtime":"/.config/runtime.config","manifest":"/assets/asset-manifest.json"}`)
	sourceMapBody := []byte(`{"version":3,"file":"713.compiled.js","sources":["webpack://coldchain/src/quality-ledger.ts"],"names":[],"mappings":"","x_runtime_chunk":"/assets/chunks/quality-ledger.91dbe763.js"}`)
	robotsBody := []byte("User-agent: *\nAllow: /\nSitemap: /routes.txt\n")

	externalTag := ""
	if externalScriptURL != "" {
		externalTag = `<script src="` + html.EscapeString(externalScriptURL) + `"></script>`
	}
	rootBody := []byte(`<!doctype html><html><head><meta charset="utf-8"><link rel="manifest" href="/assets/asset-manifest.json"></head><body><main id="app"></main><script src="/assets/app.small.js"></script><script src="/assets/chunks/runtime.js"></script>` + externalTag + `</body></html>`)

	assets := map[string]fixtureAsset{
		RootPath: {
			body:        rootBody,
			contentType: "text/html; charset=utf-8",
			headers: map[string]string{
				"Link": `<` + HeaderRoutesPath + `>; rel="service-desc", <` + RoutesPath + `>; rel="alternate"`,
			},
		},
		RobotsPath:        {body: robotsBody, contentType: "text/plain; charset=utf-8"},
		RoutesPath:        {body: routesBody, contentType: "text/plain; charset=utf-8"},
		RuntimeConfigPath: {body: runtimeConfigBody, contentType: "application/json; charset=utf-8"},
		HeaderRoutesPath:  {body: headerRoutesBody, contentType: "application/json; charset=utf-8"},
		ManifestPath:      {body: manifestBody, contentType: "application/json; charset=utf-8"},
		ChunkConfigPath:   {body: chunkConfigBody, contentType: "application/json; charset=utf-8"},
		RuntimeJSPath:     {body: runtimeJS, contentType: "application/javascript; charset=utf-8"},
		CompiledChunkPath: {
			body: compiledChunk, contentType: "application/javascript; charset=utf-8",
			headers: map[string]string{"SourceMap": SourceMapPath},
		},
		SourceMapPath:    {body: sourceMapBody, contentType: "application/json; charset=utf-8"},
		DynamicChunkPath: {body: dynamicChunk, contentType: "application/javascript; charset=utf-8"},
		WorkerPath:       {body: workerJS, contentType: "application/javascript; charset=utf-8"},
		SmallJSPath:      {body: small, contentType: "application/javascript; charset=utf-8"},
		MediumJSPath:     {body: medium, contentType: "application/javascript; charset=utf-8"},
		LargeJSPath:      {body: large, contentType: "application/javascript; charset=utf-8"},
		HugeJSPath:       {body: huge, contentType: "application/javascript; charset=utf-8"},
	}

	truth := GroundTruth{
		HugeTargetID:      "huge-from-char-code-request",
		HugeTargetOffset:  hugeTargetOffset,
		ExternalScriptURL: externalScriptURL,
		Assets: []AssetGroundTruth{
			{Path: RootPath, ContentType: assets[RootPath].contentType, Size: len(rootBody), Layer: "document", Scope: ScopeInternal},
			{Path: RobotsPath, ContentType: assets[RobotsPath].contentType, Size: len(robotsBody), Layer: "text", Scope: ScopeInternal},
			{Path: RoutesPath, ContentType: assets[RoutesPath].contentType, Size: len(routesBody), Layer: "routes", Scope: ScopeInternal},
			{Path: RuntimeConfigPath, ContentType: assets[RuntimeConfigPath].contentType, Size: len(runtimeConfigBody), Layer: "config", Scope: ScopeInternal},
			{Path: HeaderRoutesPath, ContentType: assets[HeaderRoutesPath].contentType, Size: len(headerRoutesBody), Layer: "header-linked-config", Scope: ScopeInternal},
			{Path: ManifestPath, ContentType: assets[ManifestPath].contentType, Size: len(manifestBody), Layer: "manifest", Scope: ScopeInternal},
			{Path: ChunkConfigPath, ContentType: assets[ChunkConfigPath].contentType, Size: len(chunkConfigBody), Layer: "chunk-config", Scope: ScopeInternal},
			{Path: RuntimeJSPath, ContentType: assets[RuntimeJSPath].contentType, Size: len(runtimeJS), Layer: "runtime", Scope: ScopeInternal},
			{Path: CompiledChunkPath, ContentType: assets[CompiledChunkPath].contentType, Size: len(compiledChunk), Layer: "compiled-chunk", Scope: ScopeInternal},
			{Path: SourceMapPath, ContentType: assets[SourceMapPath].contentType, Size: len(sourceMapBody), Layer: "header-source-map", Scope: ScopeInternal},
			{Path: DynamicChunkPath, ContentType: assets[DynamicChunkPath].contentType, Size: len(dynamicChunk), Layer: "dynamic-chunk", Scope: ScopeInternal},
			{Path: WorkerPath, ContentType: assets[WorkerPath].contentType, Size: len(workerJS), Layer: "worker", Scope: ScopeInternal},
			{Path: SmallJSPath, ContentType: assets[SmallJSPath].contentType, Size: len(small), Layer: "small-js", Scope: ScopeInternal},
			{Path: MediumJSPath, ContentType: assets[MediumJSPath].contentType, Size: len(medium), Layer: "medium-js", Scope: ScopeInternal},
			{Path: LargeJSPath, ContentType: assets[LargeJSPath].contentType, Size: len(large), Layer: "large-minified-js", Scope: ScopeInternal},
			{Path: HugeJSPath, ContentType: assets[HugeJSPath].contentType, Size: len(huge), Layer: "huge-tail-js", Scope: ScopeInternal},
		},
		Findings: []ExpectedFinding{
			{ID: "header-service-routes", SourceAsset: RootPath, Value: HeaderRoutesPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "http-link-header-only", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true},
			{ID: "header-routes-registry", SourceAsset: RootPath, Value: RoutesPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "http-link-header-only", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true},
			{ID: "compiled-source-map", SourceAsset: CompiledChunkPath, Value: SourceMapPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "http-sourcemap-header-only", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true},
			{ID: "source-map-dynamic-chunk", SourceAsset: SourceMapPath, Value: DynamicChunkPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "source-map-extension", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "service-routes-runtime-config", SourceAsset: HeaderRoutesPath, Value: RuntimeConfigPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "well-known-json-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "service-routes-manifest", SourceAsset: HeaderRoutesPath, Value: ManifestPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "well-known-json-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "manifest-small-entry", SourceAsset: ManifestPath, Value: SmallJSPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "manifest-entrypoint", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "manifest-medium-entry", SourceAsset: ManifestPath, Value: MediumJSPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "manifest-entrypoint", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "manifest-large-entry", SourceAsset: ManifestPath, Value: LargeJSPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "manifest-entrypoint", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "manifest-runtime-entry", SourceAsset: ManifestPath, Value: RuntimeJSPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "manifest-entrypoint", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "manifest-routes", SourceAsset: ManifestPath, Value: RoutesPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "manifest-json-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "manifest-runtime-config", SourceAsset: ManifestPath, Value: RuntimeConfigPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "manifest-json-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "routes-runtime-config", SourceAsset: RoutesPath, Value: RuntimeConfigPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "split-lines-next", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "routes-split-request", SourceAsset: RoutesPath, Value: RoutesTargetPath, RawQuery: "format=full", Method: http.MethodGet, Kind: FindingRequest, Encoding: "split-lines", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, MustStructure: true, RequiresAI: true},
			{ID: "runtime-config-request", SourceAsset: RuntimeConfigPath, Value: ConfigTargetPath, Method: http.MethodPost, Headers: map[string]string{"X-Runtime-Profile": "bootstrap"}, Body: `{"mode":"hydrate"}`, BodyContains: `"mode":"hydrate"`, Kind: FindingRequest, Encoding: "json-array-segments", Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true},
			{ID: "runtime-config-chunk-config", SourceAsset: RuntimeConfigPath, Value: ChunkConfigPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "json-unicode-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "runtime-config-manifest", SourceAsset: RuntimeConfigPath, Value: ManifestPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "json-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "runtime-chunk-config", SourceAsset: RuntimeJSPath, Value: ChunkConfigPath, Method: http.MethodGet, Kind: FindingConfig, Encoding: "javascript-hex-escape", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "chunk-713", SourceAsset: ChunkConfigPath, Value: CompiledChunkPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "json-unicode-escape", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true},
			{ID: "huge-asset", SourceAsset: CompiledChunkPath, Value: HugeJSPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "atob-array-index-xor", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "dynamic-worker", SourceAsset: DynamicChunkPath, Value: WorkerPath, Method: http.MethodGet, Kind: FindingAsset, Encoding: "computed-from-char-code-member", Scope: ScopeInternal, MustDiscover: true, MustCandidate: true, MustRequest: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "small-header-gated-get", SourceAsset: SmallJSPath, Value: SmallTargetPath, RawQuery: "region=north", Method: http.MethodGet, Headers: map[string]string{"X-Client-Surface": "catalog-matrix"}, Kind: FindingRequest, Encoding: "mixed-case-array-join", Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true},
			{ID: "medium-escaped-request", SourceAsset: MediumJSPath, Value: MediumTargetPath, Method: http.MethodPost, Headers: map[string]string{"X-Route-Profile": "escaped"}, Body: `{"dispatch":"priority"}`, BodyContains: `"dispatch":"priority"`, Kind: FindingRequest, Encoding: "javascript-hex-escape", Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "large-atob-request", SourceAsset: LargeJSPath, Value: LargeTargetPath, Method: http.MethodPatch, Headers: map[string]string{"X-Chunk-Mode": "indexed"}, Kind: FindingRequest, Encoding: "atob-array-index", Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "huge-from-char-code-request", SourceAsset: HugeJSPath, Value: HugeTargetPath, Method: http.MethodPut, Headers: map[string]string{"X-Asset-Proof": "huge-tail"}, Body: `{"mode":"reconcile"}`, BodyContains: `"mode":"reconcile"`, Kind: FindingRequest, Encoding: "string-from-char-code", SourceOffset: hugeTargetOffset, Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "dynamic-header-gated-get", SourceAsset: DynamicChunkPath, Value: DynamicTargetPath, RawQuery: "view=delta", Method: http.MethodGet, Headers: map[string]string{"X-ColdChain-Module": "quality-ledger"}, Kind: FindingRequest, Encoding: "atob-and-worker", Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true, RequiresEvaluation: true},
			{ID: "worker-post-request", SourceAsset: WorkerPath, Value: WorkerTargetPath, Method: http.MethodPost, Headers: map[string]string{"X-ColdChain-Worker": "ledger-export"}, Body: `{"format":"ndjson"}`, BodyContains: `"format":"ndjson"`, Kind: FindingRequest, Encoding: "atob-pool-xor", Scope: ScopeInternal, MustDiscover: true, MustStructure: true, RequiresAI: true, RequiresEvaluation: true},
		},
	}

	if externalScriptURL != "" {
		truth.Findings = append(truth.Findings, ExpectedFinding{
			ID: "external-script", SourceAsset: RootPath, Value: externalScriptURL,
			Method: http.MethodGet, Kind: FindingAsset, Encoding: "html-script-src",
			Scope: ScopeExternal, MustDiscover: true, MustCandidate: true,
		})
	}
	if localExternal {
		externalBody := buildExternalJS()
		truth.Assets = append(truth.Assets, AssetGroundTruth{
			Path: externalScriptURL, ContentType: "application/javascript; charset=utf-8",
			Size: len(externalBody), Layer: "external-script", Scope: ScopeExternal,
		})
		truth.Findings = append(truth.Findings, ExpectedFinding{
			ID: "external-script-request", SourceAsset: externalScriptURL,
			Value: ExternalTargetPath, Method: http.MethodOptions, Kind: FindingRequest,
			Encoding: "array-concat", Scope: ScopeExternal, MustDiscover: false, MustStructure: true, RequiresAI: true,
		})
	}

	return assets, truth, nil
}

func buildHugeJS() ([]byte, int, error) {
	if HugePayloadOffset <= HugeMinimumOffset {
		return nil, 0, fmt.Errorf("huge payload offset %d is not beyond 4.5 MiB", HugePayloadOffset)
	}

	prefix := `(()=>{const BuIlD="coldchain-2026.08";globalThis.__fixtureBuild=BuIlD;})();`
	charCodes := asciiCharCodes(HugeTargetPath)
	hiddenCode := `(()=>{const endpoint=String.fromCharCode(` + charCodes + `);const method=String.fromCharCode(80,85,84);fetch(endpoint,{method:method,headers:{"X-Asset-Proof":"huge-tail"},body:JSON.stringify({mode:"reconcile"})});})();`
	if len(prefix) >= HugePayloadOffset || HugePayloadOffset+len(hiddenCode) >= HugeJSSize {
		return nil, 0, fmt.Errorf("huge fixture layout does not fit configured offsets")
	}

	body := make([]byte, HugeJSSize)
	n := copy(body, prefix)
	fillPadding(body[n:HugePayloadOffset])
	copy(body[HugePayloadOffset:], hiddenCode)
	fillPadding(body[HugePayloadOffset+len(hiddenCode):])

	if bytes.Contains(body, []byte(HugeTargetPath)) {
		return nil, 0, fmt.Errorf("huge target unexpectedly appears in plaintext")
	}
	if bytes.Count(body, []byte("fetch(")) != 1 {
		return nil, 0, fmt.Errorf("huge fixture must contain exactly one request sink")
	}
	if bytes.ContainsAny(body, "\r\n") {
		return nil, 0, fmt.Errorf("huge fixture must remain a single line")
	}
	targetOffset := bytes.Index(body, []byte("String.fromCharCode"))
	if targetOffset <= HugeMinimumOffset {
		return nil, 0, fmt.Errorf("huge hidden target offset %d is not beyond 4.5 MiB", targetOffset)
	}
	return body, targetOffset, nil
}

func sizedSingleLineJS(code string, size int) ([]byte, error) {
	if strings.ContainsAny(code, "\r\n") {
		return nil, fmt.Errorf("javascript seed contains a newline")
	}
	if len(code)+4 > size {
		return nil, fmt.Errorf("javascript seed (%d bytes) exceeds target size %d", len(code), size)
	}
	body := make([]byte, size)
	n := copy(body, code)
	fillPadding(body[n:])
	return body, nil
}

func fillPadding(dst []byte) {
	if len(dst) < 4 {
		for i := range dst {
			dst[i] = ';'
		}
		return
	}
	dst[0], dst[1] = '/', '*'
	for i := 2; i < len(dst)-2; i++ {
		dst[i] = 'x'
	}
	dst[len(dst)-2], dst[len(dst)-1] = '*', '/'
}

func hexEscapeASCII(value string) string {
	var builder strings.Builder
	builder.Grow(len(value) * 4)
	for i := 0; i < len(value); i++ {
		builder.WriteString(`\x`)
		builder.WriteString(fmt.Sprintf("%02x", value[i]))
	}
	return builder.String()
}

func asciiCharCodes(value string) string {
	parts := make([]string, 0, len(value))
	for i := 0; i < len(value); i++ {
		parts = append(parts, strconv.Itoa(int(value[i])))
	}
	return strings.Join(parts, ",")
}

func buildExternalJS() []byte {
	return []byte(`(()=>{const ExTeRnAl=["/partner","/v1","/telemetry","/schema"];const endpoint=ExTeRnAl[0]+ExTeRnAl[1]+ExTeRnAl[2]+ExTeRnAl[3];fetch(endpoint,{method:"OPTIONS"});})();`)
}

func validateHTTPURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

func writeResponse(w http.ResponseWriter, r *http.Request, status int, contentType string, body []byte, headers map[string]string) {
	for key, value := range headers {
		w.Header().Set(key, value)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Crawler-Fixture", "layered-site-v1")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(body)
	}
}
