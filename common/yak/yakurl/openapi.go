package yakurl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/openapi"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	openAPIUploadLocation     = "upload"
	openAPIQueryOp            = "op"
	openAPIQueryMethod        = "method"
	openAPIQueryPath          = "path"
	openAPIQueryOperationID   = "operationId"
	openAPIQueryOverrideDomain = "overrideDomain"
	openAPIQueryOverrideHTTPS = "overrideIsHttps"
	openAPIQueryContentType   = "requestBodyContentType"

	openAPIOpBuild     = "build"
	openAPIOpImportAll = "import-all"
	openAPIOpDetail    = "detail"

	openAPIResourceDocument = "openapi-document"
	openAPIResourceOperation = "openapi-operation"
	openAPIResourceRequest  = "fuzzer-request"
)

type cachedOpenAPIDocument struct {
	Content string
	Parsed  *openapi.ParsedDocument
}

var (
	openAPIDocumentStore sync.Map
)

type openapiAction struct{}

func newOpenAPIAction() *openapiAction {
	return &openapiAction{}
}

func (a *openapiAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	location := strings.TrimSpace(u.GetLocation())
	if location == "" || location == openAPIUploadLocation {
		return nil, utils.Error("openapi document id is required")
	}

	doc, err := loadCachedOpenAPIDocument(location)
	if err != nil {
		return nil, err
	}

	query := openAPIQueryValues(u.GetQuery())
	if query.Get(openAPIQueryOp) == openAPIOpDetail || (query.Get(openAPIQueryMethod) != "" && query.Get(openAPIQueryPath) != "") {
		op, err := resolveOpenAPIOperation(doc.Parsed, query)
		if err != nil {
			return nil, err
		}
		return &ypb.RequestYakURLResponse{
			Resources: []*ypb.YakURLResource{openAPIOperationDetailResource(location, op)},
		}, nil
	}

	return listOpenAPIDocumentResources(location, doc.Parsed)
}

func (a *openapiAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	location := strings.TrimSpace(u.GetLocation())
	query := openAPIQueryValues(u.GetQuery())

	if location == "" || location == openAPIUploadLocation {
		return uploadOpenAPIDocument(params)
	}

	doc, err := loadCachedOpenAPIDocument(location)
	if err != nil {
		return nil, err
	}

	switch strings.TrimSpace(query.Get(openAPIQueryOp)) {
	case openAPIOpBuild:
		return buildOpenAPIOperationRequests(location, doc, params)
	case openAPIOpImportAll:
		return importAllOpenAPIRequests(location, doc, params)
	default:
		return nil, utils.Errorf("unsupported openapi op %q, want %q or %q", query.Get(openAPIQueryOp), openAPIOpBuild, openAPIOpImportAll)
	}
}

func (a *openapiAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (a *openapiAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	location := strings.TrimSpace(u.GetLocation())
	if location == "" || location == openAPIUploadLocation {
		return nil, utils.Error("openapi document id is required")
	}
	openAPIDocumentStore.Delete(location)
	return &ypb.RequestYakURLResponse{}, nil
}

func (a *openapiAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (a *openapiAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.Post(params)
}

func uploadOpenAPIDocument(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	content := strings.TrimSpace(string(params.GetBody()))
	if content == "" {
		return nil, utils.Error("openapi document content is empty")
	}

	u := params.GetUrl()
	query := openAPIQueryValues(u.GetQuery())
	parseOpts := &openapi.ParseOptions{OverrideDomain: query.Get(openAPIQueryOverrideDomain)}
	if parseBoolQuery(query.Get(openAPIQueryOverrideHTTPS)) {
		v := true
		parseOpts.OverrideHTTPS = &v
	}

	parsed, err := openapi.ParseDocument(content, parseOpts)
	if err != nil {
		return nil, err
	}

	docID := uuid.NewString()
	openAPIDocumentStore.Store(docID, &cachedOpenAPIDocument{
		Content: content,
		Parsed:  parsed,
	})

	root, err := listOpenAPIDocumentResources(docID, parsed)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func listOpenAPIDocumentResources(docID string, parsed *openapi.ParsedDocument) (*ypb.RequestYakURLResponse, error) {
	if parsed == nil {
		return nil, utils.Error("parsed openapi document is nil")
	}

	resources := []*ypb.YakURLResource{
		openAPIDocumentRootResource(docID, parsed),
	}
	for _, op := range parsed.Operations {
		resources = append(resources, openAPIOperationListResource(docID, op))
	}
	sort.SliceStable(resources[1:], func(i, j int) bool {
		left := resources[i+1]
		right := resources[j+1]
		leftMethod := GetQueryParam(left.GetExtra(), "method")
		rightMethod := GetQueryParam(right.GetExtra(), "method")
		if leftMethod != rightMethod {
			return leftMethod < rightMethod
		}
		return GetQueryParam(left.GetExtra(), "path") < GetQueryParam(right.GetExtra(), "path")
	})
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  int64(len(resources)),
		Total:     int64(len(resources)),
		Resources: resources,
	}, nil
}

func buildOpenAPIOperationRequests(docID string, doc *cachedOpenAPIDocument, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	query := openAPIQueryValues(params.GetUrl().GetQuery())
	op, err := resolveOpenAPIOperation(doc.Parsed, query)
	if err != nil {
		return nil, err
	}

	buildOpts, err := parseOpenAPIBuildOptions(params, query)
	if err != nil {
		return nil, err
	}

	rawRequests, isHTTPS, err := openapi.BuildOperationRequests(doc.Content, op.Path, op.Method, buildOpts)
	if err != nil {
		return nil, err
	}
	if len(rawRequests) == 0 {
		return nil, utils.Error("no requests generated from openapi operation")
	}

	resources := make([]*ypb.YakURLResource, 0, len(rawRequests))
	for _, raw := range rawRequests {
		resources = append(resources, fuzzerRequestResource(op.Path, op.Method, raw, isHTTPS))
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  int64(len(resources)),
		Total:     int64(len(resources)),
		Resources: resources,
	}, nil
}

func importAllOpenAPIRequests(docID string, doc *cachedOpenAPIDocument, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	query := openAPIQueryValues(params.GetUrl().GetQuery())
	buildOpts, err := parseOpenAPIBuildOptions(params, query)
	if err != nil {
		return nil, err
	}

	all, err := openapi.ImportAllOperationRequests(doc.Content, buildOpts)
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, utils.Error("no requests generated from openapi document")
	}

	resources := make([]*ypb.YakURLResource, 0, len(all))
	for _, item := range all {
		resources = append(resources, fuzzerRequestResource(item.Path, item.Method, item.RequestRaw, item.IsHTTPS))
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  int64(len(resources)),
		Total:     int64(len(resources)),
		Resources: resources,
	}, nil
}

func loadCachedOpenAPIDocument(docID string) (*cachedOpenAPIDocument, error) {
	raw, ok := openAPIDocumentStore.Load(docID)
	if !ok {
		return nil, utils.Errorf("openapi document %q not found", docID)
	}
	doc, ok := raw.(*cachedOpenAPIDocument)
	if !ok || doc == nil {
		return nil, utils.Errorf("invalid openapi document cache for %q", docID)
	}
	return doc, nil
}

func resolveOpenAPIOperation(parsed *openapi.ParsedDocument, query url.Values) (openapi.OperationInfo, error) {
	method := strings.ToUpper(strings.TrimSpace(query.Get(openAPIQueryMethod)))
	path := strings.TrimSpace(query.Get(openAPIQueryPath))
	operationID := strings.TrimSpace(query.Get(openAPIQueryOperationID))

	if method != "" && path != "" {
		for _, op := range parsed.Operations {
			if strings.EqualFold(op.Method, method) && op.Path == path {
				return op, nil
			}
		}
		return openapi.OperationInfo{}, utils.Errorf("operation not found: %s %s", method, path)
	}

	if operationID != "" {
		for _, op := range parsed.Operations {
			if op.OperationId == operationID {
				return op, nil
			}
		}
		return openapi.OperationInfo{}, utils.Errorf("operationId %q not found", operationID)
	}

	return openapi.OperationInfo{}, utils.Error("method/path or operationId is required")
}

func parseOpenAPIBuildOptions(params *ypb.RequestYakURLParams, query url.Values) (*openapi.BuildOptions, error) {
	opts := &openapi.BuildOptions{
		OverrideDomain:         query.Get(openAPIQueryOverrideDomain),
		RequestBodyContentType: query.Get(openAPIQueryContentType),
		ParameterValues:        map[string]string{},
	}
	if parseBoolQuery(query.Get(openAPIQueryOverrideHTTPS)) {
		v := true
		opts.OverrideHTTPS = &v
	}

	body := strings.TrimSpace(string(params.GetBody()))
	if body == "" {
		for _, kv := range params.GetUrl().GetQuery() {
			if strings.HasPrefix(kv.GetKey(), "param.") {
				opts.ParameterValues[strings.TrimPrefix(kv.GetKey(), "param.")] = kv.GetValue()
			}
		}
		return opts, nil
	}

	var payload struct {
		OverrideDomain         string            `json:"overrideDomain"`
		OverrideIsHttps        *bool             `json:"overrideIsHttps"`
		RequestBodyContentType string            `json:"requestBodyContentType"`
		ParameterValues        map[string]string `json:"parameterValues"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return nil, utils.Errorf("invalid build options json: %v", err)
	}
	if payload.OverrideDomain != "" {
		opts.OverrideDomain = payload.OverrideDomain
	}
	if payload.OverrideIsHttps != nil {
		opts.OverrideHTTPS = payload.OverrideIsHttps
	}
	if payload.RequestBodyContentType != "" {
		opts.RequestBodyContentType = payload.RequestBodyContentType
	}
	for k, v := range payload.ParameterValues {
		opts.ParameterValues[k] = v
	}
	return opts, nil
}

func openAPIDocumentRootResource(docID string, parsed *openapi.ParsedDocument) *ypb.YakURLResource {
	info := parsed.Info
	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = docID
	}
	return &ypb.YakURLResource{
		ResourceType:      openAPIResourceDocument,
		VerboseType:       "OpenAPI Document",
		ResourceName:      docID,
		VerboseName:       title,
		Path:              "/",
		YakURLVerbose:     fmt.Sprintf("openapi://%s/", docID),
		HaveChildrenNodes: len(parsed.Operations) > 0,
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
		},
		Extra: openAPIDocumentInfoExtras(info, len(parsed.Operations), parsed.Warnings),
	}
}

func openAPIOperationListResource(docID string, op openapi.OperationInfo) *ypb.YakURLResource {
	verbose := strings.TrimSpace(op.Summary)
	if verbose == "" {
		verbose = fmt.Sprintf("%s %s", strings.ToUpper(op.Method), op.Path)
	}
	return &ypb.YakURLResource{
		ResourceType:      openAPIResourceOperation,
		VerboseType:       "OpenAPI Operation",
		ResourceName:      fmt.Sprintf("%s %s", strings.ToUpper(op.Method), op.Path),
		VerboseName:       verbose,
		Path:              op.Path,
		YakURLVerbose:     fmt.Sprintf("openapi://%s/?method=%s&path=%s", docID, strings.ToUpper(op.Method), url.QueryEscape(op.Path)),
		HaveChildrenNodes: false,
		Url: openAPIOperationURL(docID, op),
		Extra: append(openAPIOperationSummaryExtras(op), &ypb.KVPair{
			Key:   "doc_id",
			Value: docID,
		}),
	}
}

func openAPIOperationDetailResource(docID string, op openapi.OperationInfo) *ypb.YakURLResource {
	resource := openAPIOperationListResource(docID, op)
	detailJSON, _ := json.Marshal(op)
	resource.Extra = append(resource.GetExtra(), &ypb.KVPair{
		Key:   "detail_json",
		Value: string(detailJSON),
	})
	return resource
}

func openAPIOperationURL(docID string, op openapi.OperationInfo) *ypb.YakURL {
	return &ypb.YakURL{
		Schema:   "openapi",
		Location: docID,
		Path:     "/",
		Query: []*ypb.KVPair{
			{Key: openAPIQueryMethod, Value: strings.ToUpper(op.Method)},
			{Key: openAPIQueryPath, Value: op.Path},
			{Key: openAPIQueryOp, Value: openAPIOpDetail},
		},
	}
}

func fuzzerRequestResource(path, method string, raw []byte, isHTTPS bool) *ypb.YakURLResource {
	return &ypb.YakURLResource{
		ResourceType:  openAPIResourceRequest,
		VerboseType:   "Fuzzer Request",
		ResourceName:  fmt.Sprintf("%s %s", strings.ToUpper(method), path),
		VerboseName:   fmt.Sprintf("%s %s", strings.ToUpper(method), path),
		Path:          path,
		HaveChildrenNodes: false,
		Extra: []*ypb.KVPair{
			{Key: "method", Value: strings.ToUpper(method)},
			{Key: "path", Value: path},
			{Key: "request", Value: string(raw)},
			{Key: "is_https", Value: fmt.Sprint(isHTTPS)},
		},
	}
}

func openAPIDocumentInfoExtras(info openapi.DocumentInfo, operationCount int, warnings []string) []*ypb.KVPair {
	extras := []*ypb.KVPair{
		{Key: "title", Value: info.Title},
		{Key: "version", Value: info.Version},
		{Key: "specVersion", Value: info.SpecVersion},
		{Key: "domain", Value: info.Domain},
		{Key: "is_https", Value: fmt.Sprint(info.IsHttps)},
		{Key: "operation_count", Value: fmt.Sprint(operationCount)},
	}
	if len(warnings) > 0 {
		if raw, err := json.Marshal(warnings); err == nil {
			extras = append(extras, &ypb.KVPair{Key: "parse_warnings", Value: string(raw)})
		}
	}
	for idx, server := range info.Servers {
		extras = append(extras, &ypb.KVPair{
			Key:   fmt.Sprintf("server_%d_url", idx),
			Value: server.URL,
		})
		if server.Description != "" {
			extras = append(extras, &ypb.KVPair{
				Key:   fmt.Sprintf("server_%d_description", idx),
				Value: server.Description,
			})
		}
	}
	return extras
}

func openAPIOperationSummaryExtras(op openapi.OperationInfo) []*ypb.KVPair {
	extras := []*ypb.KVPair{
		{Key: "method", Value: strings.ToUpper(op.Method)},
		{Key: "path", Value: op.Path},
		{Key: "operationId", Value: op.OperationId},
		{Key: "summary", Value: op.Summary},
		{Key: "description", Value: op.Description},
		{Key: "deprecated", Value: fmt.Sprint(op.Deprecated)},
	}
	if len(op.Tags) > 0 {
		extras = append(extras, &ypb.KVPair{Key: "tags", Value: strings.Join(op.Tags, ",")})
	}
	return extras
}

func openAPIQueryValues(query []*ypb.KVPair) url.Values {
	values := make(url.Values)
	for _, kv := range query {
		if kv == nil {
			continue
		}
		values.Add(kv.GetKey(), kv.GetValue())
	}
	return values
}

func parseBoolQuery(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}
