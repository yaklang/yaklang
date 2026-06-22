package openapi

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type ServerInfo struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

type DocumentInfo struct {
	Title       string       `json:"title"`
	Version     string       `json:"version"`
	SpecVersion string       `json:"specVersion"`
	Servers     []ServerInfo `json:"servers"`
	Domain      string       `json:"domain"`
	IsHttps     bool         `json:"isHttps"`
}

type ParameterSummary struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	Required    bool        `json:"required"`
	Type        string      `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`
	Example     interface{} `json:"example,omitempty"`
	SchemaJSON  string      `json:"schemaJson,omitempty"`
}

type RequestBodySummary struct {
	Required    bool              `json:"required"`
	Description string            `json:"description,omitempty"`
	Content     map[string]string `json:"content"` // contentType -> example json
}

type ResponseSummary struct {
	StatusCode  string `json:"statusCode"`
	Description string `json:"description,omitempty"`
	ExampleJSON string `json:"exampleJson,omitempty"`
}

type OperationInfo struct {
	Path        string             `json:"path"`
	Method      string             `json:"method"`
	OperationId string             `json:"operationId,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	Description string             `json:"description,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	Deprecated  bool               `json:"deprecated"`
	Parameters  []ParameterSummary `json:"parameters,omitempty"`
	RequestBody *RequestBodySummary `json:"requestBody,omitempty"`
	Responses   []ResponseSummary  `json:"responses,omitempty"`
}

type ParsedDocument struct {
	Info        DocumentInfo
	Operations  []OperationInfo
	Content     string
	IsSwaggerV2 bool
	Warnings    []string `json:"warnings,omitempty"`
}

type ParseOptions struct {
	OverrideDomain string
	OverrideHTTPS  *bool
}

type BuildOptions struct {
	OverrideDomain           string
	OverrideHTTPS            *bool
	RequestBodyContentType   string
	ParameterValues          map[string]string // name -> value override
}

func normalizeOpenAPIContent(content string) string {
	jsonT, err := yaml.YAMLToJSON([]byte(content))
	if err == nil {
		return string(jsonT)
	}
	return content
}

func ParseDocument(content string, opts *ParseOptions) (*ParsedDocument, error) {
	content = normalizeOpenAPIContent(content)
	if strings.TrimSpace(content) == "" {
		return nil, utils.Error("openapi document content is empty")
	}
	doc, err := parseSwaggerV2Document(content, opts)
	if err == nil {
		sanitizeParsedDocument(doc)
		return doc, nil
	}
	v2err := err
	doc, err2 := parseOpenAPIV3Document(content, opts)
	if err2 == nil {
		sanitizeParsedDocument(doc)
		return doc, nil
	}
	if doc, _, lerr := tryLenientParseDocument(content, opts); lerr == nil {
		sanitizeParsedDocument(doc)
		return doc, nil
	}
	return nil, describeParseFailure(content, v2err, err2)
}

func BuildOperationRequests(content string, path, method string, opts *BuildOptions) ([][]byte, bool, error) {
	content = normalizeOpenAPIContent(content)
	method = strings.ToUpper(strings.TrimSpace(method))
	path = strings.TrimSpace(path)
	if path == "" || method == "" {
		return nil, false, utils.Error("path and method are required")
	}

	cfg := buildConfigFromOptions(opts)
	reqs, err := buildSwaggerV2OperationRequests(content, path, method, cfg, opts)
	if err == nil && len(reqs) > 0 {
		return reqs, cfg.IsHttps, nil
	}
	reqs, err2 := buildOpenAPIV3OperationRequests(content, path, method, cfg, opts)
	if err2 != nil {
		if err != nil {
			return nil, false, utils.Errorf("build operation request failed: swagger2[%v], openapi3[%v]", err, err2)
		}
		return nil, false, err2
	}
	return reqs, cfg.IsHttps, nil
}

func ImportAllOperationRequests(content string, opts *BuildOptions) ([]*OperationRequest, error) {
	parsed, err := ParseDocument(content, parseOptionsFromBuild(opts))
	if err != nil {
		return nil, err
	}
	var results []*OperationRequest
	var skipped []string
	for _, op := range parsed.Operations {
		reqs, isHttps, err := BuildOperationRequests(content, op.Path, op.Method, opts)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s %s: %v", op.Method, op.Path, err))
			continue
		}
		if len(reqs) == 0 {
			skipped = append(skipped, fmt.Sprintf("%s %s: no requests generated", op.Method, op.Path))
			continue
		}
		for _, raw := range reqs {
			results = append(results, &OperationRequest{
				Path:       op.Path,
				Method:     op.Method,
				RequestRaw: raw,
				IsHTTPS:    isHttps,
			})
		}
	}
	if len(results) == 0 {
		if len(skipped) > 0 {
			return nil, utils.Errorf("no requests generated from openapi document, skipped: %s", strings.Join(skipped, "; "))
		}
		return nil, utils.Error("no requests generated from openapi document")
	}
	if len(skipped) > 0 {
		log.Warnf("openapi import skipped %d operation(s): %s", len(skipped), strings.Join(skipped, "; "))
	}
	return results, nil
}

type OperationRequest struct {
	Path       string
	Method     string
	RequestRaw []byte
	IsHTTPS    bool
}

func parseOptionsFromBuild(opts *BuildOptions) *ParseOptions {
	if opts == nil {
		return nil
	}
	return &ParseOptions{
		OverrideDomain: opts.OverrideDomain,
		OverrideHTTPS:  opts.OverrideHTTPS,
	}
}

func buildConfigFromOptions(opts *BuildOptions) *OpenAPIConfig {
	cfg := NewDefaultOpenAPIConfig()
	cfg.FlowHandler = nil
	if opts == nil {
		return cfg
	}
	if opts.OverrideDomain != "" {
		cfg.Domain = opts.OverrideDomain
	}
	if opts.OverrideHTTPS != nil {
		cfg.IsHttps = *opts.OverrideHTTPS
	}
	return cfg
}

func applyParseOptions(info *DocumentInfo, opts *ParseOptions) {
	if opts == nil {
		return
	}
	if opts.OverrideDomain != "" {
		info.Domain = opts.OverrideDomain
	}
	if opts.OverrideHTTPS != nil {
		info.IsHttps = *opts.OverrideHTTPS
	}
}

func paramValueOverride(opts *BuildOptions, name string, fallback interface{}) interface{} {
	if opts != nil && opts.ParameterValues != nil {
		if v, ok := opts.ParameterValues[name]; ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	return fallback
}

func joinOpenAPIPath(base string, apiPath string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	apiPath = strings.TrimLeft(strings.TrimSpace(apiPath), "/")
	if apiPath == "" {
		if base == "" {
			return "/"
		}
		return base
	}
	if base == "" {
		return "/" + apiPath
	}
	return base + "/" + apiPath
}

func appendOpenAPIPath(root mutate.FuzzHTTPRequestIf, apiPath string) mutate.FuzzHTTPRequestIf {
	currentPath := root.FirstFuzzHTTPRequest().GetPath()
	return root.FuzzPath(joinOpenAPIPath(currentPath, apiPath))
}

func parseSwaggerV2Document(content string, opts *ParseOptions) (*ParsedDocument, error) {
	var data openapi2.T
	if err := data.UnmarshalJSON([]byte(content)); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(data.Swagger, "2") {
		return nil, utils.Errorf("not swagger 2.x")
	}

	info := DocumentInfo{
		Title:       data.Info.Title,
		Version:     data.Info.Version,
		SpecVersion: data.Swagger,
		Domain:      data.Host,
	}
	if strings.Contains(strings.Join(data.Schemes, ","), "https") {
		info.IsHttps = true
	}
	if info.Domain != "" {
		if host, _, err := utils.ParseStringToHostPort(info.Domain); err == nil {
			info.Domain = host
		}
	}
	info.Servers = []ServerInfo{{URL: data.Host + data.BasePath, Description: "swagger 2 host/basePath"}}
	applyParseOptions(&info, opts)

	var operations []OperationInfo
	for pathStr, pathItem := range data.Paths {
		for method, op := range pathItem.Operations() {
			operations = append(operations, swaggerV2OperationSummary(data, pathStr, method, op))
		}
	}
	return &ParsedDocument{
		Info:        info,
		Operations:  operations,
		Content:     content,
		IsSwaggerV2: true,
	}, nil
}

func swaggerV2OperationSummary(data openapi2.T, path, method string, op *openapi2.Operation) OperationInfo {
	item := OperationInfo{
		Path:        path,
		Method:      strings.ToUpper(method),
		OperationId: op.OperationID,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        append([]string{}, op.Tags...),
		Deprecated:  op.Deprecated,
	}
	for _, parameter := range op.Parameters {
		p := parameter
		if p != nil && p.Ref != "" {
			resolved, err := v2_parameterToValue(data, p.Ref)
			if err == nil && resolved != nil {
				p = resolved
			}
		}
		if p == nil {
			continue
		}
		if p.In == "body" {
			body := &RequestBodySummary{Required: p.Required, Description: p.Description}
			body.Content = map[string]string{}
			contentType := "application/json"
			if len(op.Consumes) > 0 {
				contentType = op.Consumes[0]
			}
			if p.Schema != nil {
				if p.Schema.Ref != "" {
					body.Content[contentType] = string(v2_SchemeRefToBytes(data, p.Schema))
				} else if p.Schema.Value != nil {
					val := schemaValue(data, p.Schema.Value)
					raw, _ := json.Marshal(val)
					body.Content[contentType] = string(raw)
				}
			}
			item.RequestBody = body
			continue
		}
		example := p.Default
		if example == nil {
			example = ValueViaField(p.Name, p.Type, p.Default)
		}
		item.Parameters = append(item.Parameters, ParameterSummary{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required,
			Type:        p.Type,
			Description: p.Description,
			Example:     example,
		})
	}
	for code, rsp := range op.Responses {
		item.Responses = append(item.Responses, ResponseSummary{
			StatusCode:  code,
			Description: rsp.Description,
			ExampleJSON: string(v2_SchemeRefToBytes(data, rsp.Schema)),
		})
	}
	return item
}

func parseOpenAPIV3Document(content string, opts *ParseOptions) (*ParsedDocument, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromData([]byte(content))
	if err != nil {
		return nil, err
	}
	if doc.OpenAPI == "" || (!strings.HasPrefix(doc.OpenAPI, "3") && !strings.HasPrefix(doc.OpenAPI, "v3")) {
		return nil, utils.Errorf("not openapi 3.x")
	}

	info := DocumentInfo{
		Title:       doc.Info.Title,
		Version:     doc.Info.Version,
		SpecVersion: doc.OpenAPI,
	}
	for _, server := range doc.Servers {
		info.Servers = append(info.Servers, ServerInfo{URL: server.URL, Description: server.Description})
		if info.Domain == "" {
			urlStr := utils.ExtractHostPort(server.URL)
			domain, _, err := utils.ParseStringToHostPort(urlStr)
			if err != nil {
				domain = urlStr
			}
			info.Domain = domain
		}
		if !info.IsHttps && strings.HasPrefix(strings.ToLower(server.URL), "https://") {
			info.IsHttps = true
		}
	}
	if info.Domain == "" {
		info.Domain = "www.example.com"
	}
	applyParseOptions(&info, opts)

	var operations []OperationInfo
	for _, pathStr := range doc.Paths.InMatchingOrder() {
		pathItem := doc.Paths.Value(pathStr)
		for method, op := range pathItem.Operations() {
			operations = append(operations, openAPIV3OperationSummary(*doc, pathStr, method, op))
		}
	}
	return &ParsedDocument{
		Info:       info,
		Operations: operations,
		Content:    content,
	}, nil
}

func openAPIV3OperationSummary(data openapi3.T, path, method string, op *openapi3.Operation) OperationInfo {
	item := OperationInfo{
		Path:        path,
		Method:      strings.ToUpper(method),
		OperationId: op.OperationID,
		Summary:     op.Summary,
		Description: op.Description,
		Tags:        append([]string{}, op.Tags...),
		Deprecated:  op.Deprecated,
	}
	pathItem := data.Paths.Find(path)
	if pathItem == nil {
		return item
	}
	allParams := append([]*openapi3.ParameterRef{}, pathItem.Parameters...)
	allParams = append(allParams, op.Parameters...)
	for _, paramRef := range allParams {
		param, err := v3_parameterToValue(data, paramRef)
		if err != nil {
			continue
		}
		ps := ParameterSummary{
			Name:        param.Name,
			In:          param.In,
			Required:    param.Required,
			Description: param.Description,
			Example:     param.Example,
		}
		if param.Schema != nil {
			if scheme, err := v3_schemaToValue(data, param.Schema); err == nil {
				ps.Type = scheme.Type
				if ps.Example == nil {
					ps.Example = ValueViaField(param.Name, scheme.Type, scheme.Default)
				}
				if raw, err := json.Marshal(scheme); err == nil {
					ps.SchemaJSON = string(raw)
				}
			}
		}
		if ps.Example == nil {
			ps.Example = ValueViaField(param.Name, ps.Type, nil)
		}
		if ps.Type == "" {
			ps.Type = "string"
		}
		item.Parameters = append(item.Parameters, ps)
	}
	if op.RequestBody != nil {
		if body, err := v3_requestBodyToValue(data, op.RequestBody); err == nil && body != nil {
			rb := &RequestBodySummary{Required: body.Required, Description: body.Description, Content: map[string]string{}}
			for contentType, media := range body.Content {
				if media.Schema == nil {
					continue
				}
				if scheme, err := v3_schemaToValue(data, media.Schema); err == nil {
					rb.Content[contentType] = string(v3_mockSchemaJson(data, scheme))
				}
			}
			item.RequestBody = rb
		}
	}
	for code, responseRef := range op.Responses.Map() {
		rs := ResponseSummary{StatusCode: code}
		if response, err := v3_responseToValue(data, responseRef); err == nil && response != nil {
			if response.Description != nil {
				rs.Description = *response.Description
			}
			for contentType, media := range response.Content {
				if media.Schema == nil {
					continue
				}
				if scheme, err := v3_schemaToValue(data, media.Schema); err == nil {
					rs.ExampleJSON = string(v3_mockSchemaJson(data, scheme))
					_ = contentType
					break
				}
			}
		}
		item.Responses = append(item.Responses, rs)
	}
	return item
}

func buildSwaggerV2OperationRequests(content, path, method string, cfg *OpenAPIConfig, opts *BuildOptions) ([][]byte, error) {
	var data openapi2.T
	if err := data.UnmarshalJSON([]byte(content)); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(data.Swagger, "2") {
		return nil, utils.Errorf("not swagger 2.x")
	}
	applyV2Config(&data, cfg)
	root, err := mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com
`)
	if err != nil {
		return nil, err
	}
	var methodRoot mutate.FuzzHTTPRequestIf = root.FuzzHTTPHeader("Host", cfg.Domain)
	if trimmedBasePath := strings.TrimRight(strings.TrimSpace(data.BasePath), "/"); trimmedBasePath != "" {
		methodRoot = methodRoot.FuzzPath(trimmedBasePath)
	}

	pathItem, ok := data.Paths[path]
	if !ok {
		return nil, utils.Errorf("path %q not found", path)
	}
	op := pathItem.GetOperation(strings.ToUpper(method))
	if op == nil {
		return nil, utils.Errorf("method %q not found for path %q", method, path)
	}

	methodRoot = appendOpenAPIPath(methodRoot, path).FuzzMethod(strings.ToLower(method))
	if len(op.Consumes) > 0 {
		methodRoot = methodRoot.FuzzHTTPHeader("Content-Type", op.Consumes[0])
	}
	pr := methodRoot.FirstFuzzHTTPRequest().GetPath()
	originPath, _ := codec.PathUnescape(pr)
	if originPath == "" {
		originPath = pr
	}
	for _, parameter := range op.Parameters {
		p := parameter
		if p != nil && p.Ref != "" {
			resolved, err := v2_parameterToValue(data, p.Ref)
			if err != nil {
				continue
			}
			if resolved != nil {
				p = resolved
			}
		}
		if p == nil {
			continue
		}
		switch p.In {
		case "path":
			value := paramValueOverride(opts, p.Name, ValueViaField(p.Name, p.Type, p.Default))
			r, err := regexp.Compile(`\{\s*(` + regexp.QuoteMeta(p.Name) + `)\s*\}`)
			if err != nil {
				continue
			}
			originPath = r.ReplaceAllString(originPath, fmt.Sprint(value))
			methodRoot = methodRoot.FuzzPath(originPath)
		case "query":
			methodRoot = methodRoot.FuzzGetParams(p.Name, paramValueOverride(opts, p.Name, ValueViaField(p.Name, p.Type, p.Default)))
		case "header":
			methodRoot = methodRoot.FuzzHTTPHeader(p.Name, fmt.Sprint(paramValueOverride(opts, p.Name, ValueViaField(p.Name, p.Type, p.Default))))
		case "body":
			if p.Schema != nil {
				if p.Schema.Ref != "" {
					methodRoot = methodRoot.FuzzPostRaw(string(v2_SchemeRefToBytes(data, p.Schema)))
				} else if p.Schema.Value != nil {
					val := schemaValue(data, p.Schema.Value)
					raw, _ := json.Marshal(val)
					methodRoot = methodRoot.FuzzPostRaw(string(raw))
				}
			}
		}
	}
	return dumpFuzzRequests(methodRoot, cfg.IsHttps)
}

func buildOpenAPIV3OperationRequests(content, path, method string, cfg *OpenAPIConfig, opts *BuildOptions) ([][]byte, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	data, err := loader.LoadFromData([]byte(content))
	if err != nil {
		return nil, err
	}
	applyV3Config(data, cfg)

	root, err := mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com
`, mutate.OptHTTPS(cfg.IsHttps))
	if err != nil {
		return nil, err
	}
	var pathRoot mutate.FuzzHTTPRequestIf = root.FuzzHTTPHeader("Host", cfg.Domain)
	if baseURL, _ := data.Servers.BasePath(); baseURL != "" {
		baseURL = strings.TrimRight(baseURL, "/")
		if baseURL != "" {
			pathRoot = pathRoot.FuzzPath(baseURL)
		}
	}

	pathItem := data.Paths.Find(path)
	if pathItem == nil {
		return nil, utils.Errorf("path %q not found", path)
	}
	op := pathItem.GetOperation(strings.ToUpper(method))
	if op == nil {
		return nil, utils.Errorf("method %q not found for path %q", method, path)
	}

	pathRoot = appendOpenAPIPath(pathRoot, path)
	if len(pathItem.Parameters) > 0 {
		pr := pathRoot.FirstFuzzHTTPRequest().GetPath()
		originPath, _ := codec.PathUnescape(pr)
		if originPath == "" {
			originPath = pr
		}
		for _, paramIns := range pathItem.Parameters {
			param, err := v3_parameterToValue(*data, paramIns)
			if err != nil {
				continue
			}
			pathRoot, originPath = applyParametersWithOverrides(*data, param, pathRoot, originPath, opts)
		}
	}

	methodRoot := pathRoot.FuzzMethod(strings.ToLower(method))
	pr := methodRoot.FirstFuzzHTTPRequest().GetPath()
	originPath, _ := codec.PathUnescape(pr)
	if originPath == "" {
		originPath = pr
	}
	for _, parameter := range op.Parameters {
		param, err := v3_parameterToValue(*data, parameter)
		if err != nil {
			continue
		}
		methodRoot, originPath = applyParametersWithOverrides(*data, param, methodRoot, originPath, opts)
	}

	var bodyRoots []mutate.FuzzHTTPRequestIf
	if ret, _ := v3_requestBodyToValue(*data, op.RequestBody); ret != nil {
		for contentType, scheme := range ret.Content {
			if opts != nil && opts.RequestBodyContentType != "" && contentType != opts.RequestBodyContentType {
				continue
			}
			bodyRoot := methodRoot.FuzzHTTPHeader("Content-Type", contentType)
			sIns, err := v3_schemaToValue(*data, scheme.Schema)
			if err != nil {
				continue
			}
			bytes := v3_mockSchemaJson(*data, sIns)
			if len(bytes) > 0 {
				bodyRoot = bodyRoot.FuzzPostRaw(string(bytes))
			}
			bodyRoots = append(bodyRoots, bodyRoot)
		}
	}
	if len(bodyRoots) == 0 {
		bodyRoots = append(bodyRoots, methodRoot)
	}

	var results [][]byte
	for _, bodyRoot := range bodyRoots {
		reqs, err := dumpFuzzRequests(bodyRoot, cfg.IsHttps)
		if err != nil {
			return nil, err
		}
		results = append(results, reqs...)
	}
	return results, nil
}

func applyParametersWithOverrides(data openapi3.T, param *openapi3.Parameter, methodRoot mutate.FuzzHTTPRequestIf, originPath string, opts *BuildOptions) (mutate.FuzzHTTPRequestIf, string) {
	scheme, err := v3_schemaToValue(data, param.Schema)
	if err != nil {
		log.Errorf("v3_schemaToValue [%v] failed: %v", param.Name, err)
		return methodRoot, originPath
	}
	value := paramValueOverride(opts, param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
	switch param.In {
	case openapi3.ParameterInQuery:
		methodRoot = methodRoot.FuzzGetParams(param.Name, value)
	case openapi3.ParameterInHeader:
		methodRoot = methodRoot.FuzzHTTPHeader(param.Name, fmt.Sprint(value))
	case openapi3.ParameterInPath:
		r, err := regexp.Compile(`\{\s*(` + regexp.QuoteMeta(param.Name) + `)\s*\}`)
		if err != nil {
			return methodRoot, originPath
		}
		originPath = r.ReplaceAllString(originPath, fmt.Sprint(value))
		methodRoot = methodRoot.FuzzPath(originPath)
	case openapi3.ParameterInCookie:
		methodRoot = methodRoot.FuzzCookie(param.Name, fmt.Sprint(value))
	}
	return methodRoot, originPath
}

func dumpFuzzRequests(root mutate.FuzzHTTPRequestIf, isHttps bool) ([][]byte, error) {
	results, err := root.Results()
	if err != nil {
		return nil, err
	}
	var reqs [][]byte
	for _, request := range results {
		reqBytes, err := utils.DumpHTTPRequest(request, true)
		if err != nil {
			continue
		}
		if isHttps {
			_ = isHttps
		}
		reqs = append(reqs, reqBytes)
	}
	return reqs, nil
}

func applyV2Config(data *openapi2.T, cfg *OpenAPIConfig) {
	if cfg.Domain == "" {
		host := data.Host
		if host != "" {
			if h, _, err := utils.ParseStringToHostPort(host); err == nil {
				host = h
			}
			cfg.Domain = host
		}
	}
	if cfg.Domain == "" {
		cfg.Domain = "www.example.com"
	}
	if !cfg.IsHttps && strings.Contains(strings.Join(data.Schemes, ","), "https") {
		cfg.IsHttps = true
	}
}

func applyV3Config(data *openapi3.T, cfg *OpenAPIConfig) {
	if cfg.Domain == "" {
		for _, server := range data.Servers {
			urlStr := utils.ExtractHostPort(server.URL)
			domain, _, err := utils.ParseStringToHostPort(urlStr)
			if err != nil {
				domain = urlStr
			}
			if domain != "" {
				cfg.Domain = domain
				break
			}
		}
	}
	if cfg.Domain == "" {
		cfg.Domain = "www.example.com"
	}
	if !cfg.IsHttps {
		for _, server := range data.Servers {
			if strings.HasPrefix(strings.ToLower(server.URL), "https://") {
				cfg.IsHttps = true
				break
			}
		}
	}
}
