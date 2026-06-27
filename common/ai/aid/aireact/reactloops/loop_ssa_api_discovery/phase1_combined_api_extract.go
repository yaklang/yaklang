package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const combinedAPICatalogSchemaVersion = 1

// CombinedAPIAuth describes programmatically inferred authentication requirements.
type CombinedAPIAuth struct {
	Required       bool     `json:"required"`
	Realm          string   `json:"auth_realm,omitempty"`
	Mechanisms     []string `json:"mechanisms,omitempty"` // session_cookie, csrf_token, spring_preauthorize
	CredentialHint string   `json:"credential_hint,omitempty"`
	Notes          []string `json:"notes,omitempty"`
}

// CombinedAPIParam is a merged request parameter from backend signature and frontend forms.
type CombinedAPIParam struct {
	Name       string `json:"name"`
	Location   string `json:"location"` // query, post, path, header, body
	Required   bool   `json:"required,omitempty"`
	TypeHint   string `json:"type_hint,omitempty"`
	Source     string `json:"source"` // backend, frontend, both
}

// CombinedAPIRecord is one merged API endpoint from backend + frontend evidence.
type CombinedAPIRecord struct {
	ID              string             `json:"id"`
	Method          string             `json:"method"`
	Path            string             `json:"path"`
	PathRawBackend  string             `json:"path_raw_backend,omitempty"`
	HandlerClass    string             `json:"handler_class,omitempty"`
	HandlerMethod   string             `json:"handler_method,omitempty"`
	BackendFile     string             `json:"backend_file,omitempty"`
	FrontendFiles   []string           `json:"frontend_files,omitempty"`
	Auth            CombinedAPIAuth      `json:"auth"`
	Params          []CombinedAPIParam   `json:"params,omitempty"`
	ContentTypeHint string             `json:"content_type_hint,omitempty"`
	Confidence      string             `json:"confidence"` // high, medium, low
	Sources         []string           `json:"sources"`
}

// CombinedAPICatalog is the programmatic full-stack API catalog.
type CombinedAPICatalog struct {
	SchemaVersion int                 `json:"schema_version"`
	GeneratedAt   time.Time           `json:"generated_at"`
	CodeRoot      string              `json:"code_root"`
	Records       []CombinedAPIRecord `json:"records"`
	Stats         struct {
		Total            int `json:"total"`
		BackendOnly      int `json:"backend_only"`
		FrontendOnly     int `json:"frontend_only"`
		MergedBoth       int `json:"merged_both"`
		WithCsrf         int `json:"with_csrf"`
		WithSessionAuth  int `json:"with_session_auth"`
	} `json:"stats"`
	Warnings []string `json:"warnings,omitempty"`
	FullPath string   `json:"full_report_path,omitempty"`
}

// RunCombinedProgrammaticAPIExtraction merges backend Spring/Servlet harvest with frontend template/JS harvest.
func RunCombinedProgrammaticAPIExtraction(rt *Runtime) (*CombinedAPICatalog, error) {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil, utils.Error("invalid runtime")
	}
	codeRoot := rt.Session.CodeRootPath
	catalog := &CombinedAPICatalog{
		SchemaVersion: combinedAPICatalogSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		CodeRoot:      codeRoot,
	}

	if _, err := RunBuildServletRoutingMap(rt); err != nil {
		catalog.Warnings = append(catalog.Warnings, "servlet_routing_map: "+err.Error())
	}

	backend, err := HarvestSpringEndpoints(codeRoot)
	if err != nil {
		return catalog, err
	}
	backend = enrichBackendEndpointsWithServletPrefix(rt, backend)

	var frontendCalls []FrontendAPICall
	if ok, _ := shouldRunFrontendAPIAnalysis(rt); ok {
		harvest, herr := RunFrontendAPIHarvest(rt)
		if herr != nil {
			catalog.Warnings = append(catalog.Warnings, "frontend_harvest: "+herr.Error())
		} else if harvest != nil {
			frontendCalls = harvest.Calls
		}
	}

	catalog.Records = mergeBackendAndFrontendAPIs(backend, frontendCalls)
	catalog.computeStats()
	catalog.FullPath = store.CombinedAPICatalogPath(rt.WorkDir)

	b, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return catalog, err
	}
	if err := writeJSONFile(catalog.FullPath, b); err != nil {
		return catalog, err
	}
	if rt.Repo != nil && rt.Session != nil {
		_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, store.ArtifactCombinedAPICatalog, string(b))
	}
	log.Infof("ssa_api_discovery: combined_api_catalog records=%d merged=%d csrf=%d",
		catalog.Stats.Total, catalog.Stats.MergedBoth, catalog.Stats.WithCsrf)
	return catalog, nil
}

func enrichBackendEndpointsWithServletPrefix(rt *Runtime, eps []APIEndpoint) []APIEndpoint {
	out := make([]APIEndpoint, len(eps))
	for i, ep := range eps {
		job := FeatureWorkJob{
			EntryFile: ep.FilePath,
			PackagePatterns: []string{ep.PackageName + ".*"},
		}
		prefix := resolvePathPrefixForHandler(rt, ep.ClassName, ep.FilePath, job.PackagePatterns)
		if prefix != "" {
			ep.HTTPPath = joinMountAndRelativePath(prefix, ep.HTTPPath)
		}
		out[i] = ep
	}
	return out
}

func mergeBackendAndFrontendAPIs(backend []APIEndpoint, frontend []FrontendAPICall) []CombinedAPIRecord {
	byKey := map[string]*CombinedAPIRecord{}

	for _, ep := range backend {
		for _, method := range ep.HTTPMethods {
			method = normalizeHTTPMethod(method)
			method = inferBackendHTTPMethod(ep, method)
			path := normURLPath(ep.HTTPPath)
			if path == "" {
				continue
			}
			rec := newCombinedFromBackend(ep, method, path)
			key := combinedAPIKey(method, path)
			byKey[key] = rec
		}
	}

	for _, fe := range frontend {
		method := normalizeHTTPMethod(fe.Method)
		path := normURLPath(firstNonEmpty(fe.PathResolved, fe.PathRaw))
		if path == "" {
			continue
		}
		key := combinedAPIKey(method, path)
		if rec, ok := byKey[key]; ok {
			mergeFrontendIntoCombined(rec, fe)
			continue
		}
		if matched := matchFrontendToBackendRecord(byKey, fe, method, path); matched {
			continue
		}
		byKey[key] = newCombinedFromFrontend(fe, method, path)
	}

	out := make([]CombinedAPIRecord, 0, len(byKey))
	for _, rec := range byKey {
		finalizeCombinedRecord(rec)
		out = append(out, *rec)
	}
	return out
}

func newCombinedFromBackend(ep APIEndpoint, method, path string) *CombinedAPIRecord {
	auth := inferCombinedAuthFromBackend(ep)
	params := backendParamsToCombined(ep, method)
	contentType := "application/x-www-form-urlencoded"
	if ep.RequestBody != nil && ep.RequestBody.ContentType != "" {
		contentType = ep.RequestBody.ContentType
	}
	return &CombinedAPIRecord{
		ID:             ep.ID,
		Method:         method,
		Path:           path,
		PathRawBackend: normURLPath(strings.TrimPrefix(ep.HTTPPath, "/")),
		HandlerClass:   ep.ClassName,
		HandlerMethod:  ep.MethodName,
		BackendFile:    ep.FilePath,
		Auth:           auth,
		Params:         params,
		ContentTypeHint: contentType,
		Confidence:     backendConfidence(ep),
		Sources:        []string{"backend"},
	}
}

func newCombinedFromFrontend(fe FrontendAPICall, method, path string) *CombinedAPIRecord {
	auth := CombinedAPIAuth{
		Required:   fe.AuthRealmHint == AuthRealmAdmin || fe.AuthRealmHint == "api",
		Realm:      fe.AuthRealmHint,
		Mechanisms: []string{},
	}
	if auth.Required {
		auth.Mechanisms = append(auth.Mechanisms, "session_cookie")
		auth.CredentialHint = "auth_credential_id for realm " + fe.AuthRealmHint
	}
	params := frontendParamsToCombined(fe.Params)
	if method == "POST" && hasFrontendCsrfParam(fe.Params) {
		auth.Mechanisms = append(auth.Mechanisms, "csrf_token")
		auth.Notes = append(auth.Notes, "frontend POST form includes _csrf")
	}
	return &CombinedAPIRecord{
		ID:            "fe|" + routeKey(method, path),
		Method:        method,
		Path:          path,
		FrontendFiles: []string{fe.SourceFile},
		Auth:          auth,
		Params:        params,
		ContentTypeHint: "application/x-www-form-urlencoded",
		Confidence:    fe.Confidence,
		Sources:       []string{"frontend"},
	}
}

func mergeFrontendIntoCombined(rec *CombinedAPIRecord, fe FrontendAPICall) {
	if rec == nil {
		return
	}
	rec.Sources = appendUniqueString(rec.Sources, "frontend")
	if fe.SourceFile != "" {
		rec.FrontendFiles = appendUniqueString(rec.FrontendFiles, fe.SourceFile)
	}
	if fe.AuthRealmHint != "" && rec.Auth.Realm == "" {
		rec.Auth.Realm = fe.AuthRealmHint
	}
	rec.Params = mergeCombinedParams(rec.Params, frontendParamsToCombined(fe.Params))
	if strings.EqualFold(fe.Method, "POST") && hasFrontendCsrfParam(fe.Params) {
		rec.Auth.Mechanisms = appendUniqueString(rec.Auth.Mechanisms, "csrf_token")
	}
	if fe.Confidence == "high" {
		rec.Confidence = "high"
	} else if rec.Confidence != "high" && fe.Confidence == "medium" {
		rec.Confidence = "medium"
	}
}

func inferCombinedAuthFromBackend(ep APIEndpoint) CombinedAPIAuth {
	auth := CombinedAPIAuth{
		Required: ep.AuthRequired,
		Mechanisms: []string{},
	}
	if ep.AuthRequired {
		auth.Mechanisms = append(auth.Mechanisms, "session_cookie")
	}
	for _, rule := range ep.AuthRequirements {
		auth.Mechanisms = appendUniqueString(auth.Mechanisms, "spring_"+rule.Type)
		if len(rule.Roles) > 0 {
			auth.Notes = append(auth.Notes, "roles="+strings.Join(rule.Roles, ","))
		}
	}
	if hasCsrfAnnotation(ep.RawAnnotations) {
		auth.Required = true
		auth.Mechanisms = appendUniqueString(auth.Mechanisms, "csrf_token")
		auth.Notes = append(auth.Notes, "@Csrf annotation on handler")
	}
	if strings.Contains(strings.ToLower(ep.FilePath), "/controller/admin/") ||
		strings.Contains(ep.ClassName, ".controller.admin.") {
		auth.Realm = AuthRealmAdmin
		auth.Required = true
		auth.CredentialHint = "admin session cookie (JSESSIONID + PUBLICCMS_ADMIN); _csrf only on @Csrf mutations"
	} else if strings.Contains(strings.ToLower(ep.FilePath), "/controller/api/") {
		auth.Realm = "api"
	}
	return auth
}

func inferBackendHTTPMethod(ep APIEndpoint, defaultMethod string) string {
	if hasCsrfAnnotation(ep.RawAnnotations) {
		if ep.SessionAttributeMethod {
			return "POST"
		}
		return defaultMethod
	}
	raw := strings.ToLower(strings.Join(ep.RawAnnotations, " "))
	if ep.SessionAttributeMethod && defaultMethod == "GET" &&
		!strings.Contains(raw, "method=requestmethod.get") {
		return "POST"
	}
	return defaultMethod
}

func hasCsrfAnnotation(annos []string) bool {
	for _, a := range annos {
		if strings.Contains(a, "@Csrf") {
			return true
		}
	}
	return false
}

// RequiresCsrf reports whether the endpoint needs a _csrf parameter (@Csrf or explicit form).
func (rec *CombinedAPIRecord) RequiresCsrf() bool {
	return rec.hasCsrf()
}

func (rec *CombinedAPIRecord) hasCsrf() bool {
	for _, m := range rec.Auth.Mechanisms {
		if m == "csrf_token" {
			return true
		}
	}
	for _, p := range rec.Params {
		if p.Name == "_csrf" {
			return true
		}
	}
	return false
}

func csrfParamLocationForMethod(method string) string {
	switch normalizeHTTPMethod(method) {
	case "POST", "PUT", "PATCH", "DELETE":
		return "post"
	default:
		return "query"
	}
}

func backendParamsToCombined(ep APIEndpoint, method string) []CombinedAPIParam {
	var out []CombinedAPIParam
	for _, pv := range ep.PathVariables {
		out = append(out, CombinedAPIParam{Name: pv.Name, Location: "path", TypeHint: pv.Type, Source: "backend"})
	}
	for _, qp := range ep.QueryParams {
		out = append(out, CombinedAPIParam{Name: qp.Name, Location: "query", Required: qp.Required, TypeHint: qp.Type, Source: "backend"})
	}
	if hasCsrfAnnotation(ep.RawAnnotations) {
		out = append(out, CombinedAPIParam{Name: "_csrf", Location: csrfParamLocationForMethod(method), Required: true, Source: "backend"})
	}
	return out
}

func frontendParamsToCombined(in []FrontendAPIParam) []CombinedAPIParam {
	out := make([]CombinedAPIParam, 0, len(in))
	for _, p := range in {
		loc := p.Location
		if loc == "" {
			loc = "post"
		}
		out = append(out, CombinedAPIParam{
			Name:     p.Name,
			Location: loc,
			Required: p.Required,
			Source:   "frontend",
		})
	}
	return out
}

func mergeCombinedParams(base, extra []CombinedAPIParam) []CombinedAPIParam {
	index := map[string]int{}
	for i, p := range base {
		index[p.Name+"|"+p.Location] = i
	}
	for _, p := range extra {
		k := p.Name + "|" + p.Location
		if i, ok := index[k]; ok {
			if base[i].Source != p.Source {
				base[i].Source = "both"
			}
			base[i].Required = base[i].Required || p.Required
			continue
		}
		base = append(base, p)
		index[k] = len(base) - 1
	}
	return base
}

func finalizeCombinedRecord(rec *CombinedAPIRecord) {
	if rec.Auth.Required && rec.Auth.Realm == AuthRealmAdmin && rec.Auth.CredentialHint == "" {
		rec.Auth.CredentialHint = "admin session cookie via auth_credential_id"
	}
	if len(rec.Sources) > 1 || (len(rec.Sources) == 1 && rec.Sources[0] == "frontend" && rec.HandlerClass != "") {
		// merged
	}
	if len(rec.Sources) >= 2 ||
		(len(rec.FrontendFiles) > 0 && rec.HandlerClass != "") {
		rec.Confidence = "high"
	}
}

func backendConfidence(ep APIEndpoint) string {
	if ep.Confidence >= 0.85 {
		return "high"
	}
	if ep.Confidence >= 0.6 {
		return "medium"
	}
	return "low"
}

func combinedAPIKey(method, path string) string {
	return normalizeHTTPMethod(method) + "|" + normURLPath(path)
}

func matchFrontendToBackendRecord(byKey map[string]*CombinedAPIRecord, fe FrontendAPICall, method, path string) bool {
	for _, rec := range byKey {
		if rec.Path != path {
			continue
		}
		mergeFrontendIntoCombined(rec, fe)
		return true
	}
	stem := strings.ToLower(controllerStemFromEntryFile(fe.LinkedHandlerHint + ".java"))
	if stem == "" {
		stem = pathSegmentAfterAdmin(path)
	}
	for _, rec := range byKey {
		if rec.HandlerClass == "" {
			continue
		}
		classStem := controllerStemFromEntryFile(filepath.Base(rec.BackendFile))
		pathStem := pathSegmentAfterAdmin(rec.Path)
		if stem != "" && (classStem == stem || pathStem == stem) && strings.HasSuffix(rec.Path, "/"+strings.TrimPrefix(path, "/admin/")) {
			mergeFrontendIntoCombined(rec, fe)
			return true
		}
	}
	return false
}

func pathSegmentAfterAdmin(path string) string {
	path = strings.Trim(normURLPath(path), "/")
	parts := strings.Split(path, "/")
	if len(parts) >= 2 && parts[0] == "admin" {
		return strings.ToLower(parts[1])
	}
	if len(parts) > 0 {
		return strings.ToLower(parts[0])
	}
	return ""
}

func hasFrontendCsrfParam(params []FrontendAPIParam) bool {
	for _, p := range params {
		if p.Name == "_csrf" {
			return true
		}
	}
	return false
}

func (c *CombinedAPICatalog) computeStats() {
	c.Stats.Total = len(c.Records)
	for _, rec := range c.Records {
		hasBE := false
		hasFE := false
		for _, s := range rec.Sources {
			if s == "backend" {
				hasBE = true
			}
			if s == "frontend" {
				hasFE = true
			}
		}
		switch {
		case hasBE && hasFE:
			c.Stats.MergedBoth++
		case hasBE:
			c.Stats.BackendOnly++
		case hasFE:
			c.Stats.FrontendOnly++
		}
		if rec.hasCsrf() {
			c.Stats.WithCsrf++
		}
		for _, m := range rec.Auth.Mechanisms {
			if m == "session_cookie" {
				c.Stats.WithSessionAuth++
				break
			}
		}
	}
}

func loadCombinedAPICatalog(workDir string) (*CombinedAPICatalog, error) {
	b, err := os.ReadFile(store.CombinedAPICatalogPath(workDir))
	if err != nil {
		return nil, err
	}
	var c CombinedAPICatalog
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func findCombinedAPIRecordExact(catalog *CombinedAPICatalog, method, path string) *CombinedAPIRecord {
	if catalog == nil {
		return nil
	}
	path = normURLPath(path)
	method = normalizeHTTPMethod(method)
	for i := range catalog.Records {
		rec := &catalog.Records[i]
		if rec.Path != path {
			continue
		}
		if method != "" && !strings.EqualFold(rec.Method, method) {
			continue
		}
		return rec
	}
	return nil
}

func findCombinedAPIRecord(catalog *CombinedAPICatalog, method, pathContains string) *CombinedAPIRecord {
	if catalog == nil {
		return nil
	}
	method = normalizeHTTPMethod(method)
	for i := range catalog.Records {
		rec := &catalog.Records[i]
		if method != "" && !strings.EqualFold(rec.Method, method) {
			continue
		}
		if strings.Contains(rec.Path, pathContains) {
			return rec
		}
	}
	return nil
}