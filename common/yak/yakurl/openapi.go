package yakurl

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openapi"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	openAPIUploadLocation      = "upload"
	openAPIHistoryLocation     = "history"
	openAPIQueryOp             = "op"
	openAPIQueryMethod         = "method"
	openAPIQueryPath           = "path"
	openAPIQueryOperationID    = "operationId"
	openAPIQueryOverrideDomain = "overrideDomain"
	openAPIQueryOverrideHTTPS  = "overrideIsHttps"
	openAPIQueryContentType    = "requestBodyContentType"
	openAPIQueryParseTaskID    = "parse_task_id"

	openAPIOpBuild     = "build"
	openAPIOpImportAll = "import-all"
	openAPIOpDetail    = "detail"

	openAPIResourceDocument  = "openapi-document"
	openAPIResourceOperation = "openapi-operation"
	openAPIResourceRequest   = "fuzzer-request"
)

type cachedOpenAPIDocument struct {
	Content string
	Parsed  *openapi.ParsedDocument
	Session openAPIDocumentSession

	// lazy parse: Parsed is populated on first access via EnsureParsed.
	parseOnce sync.Once
	parseErr  error
}

// EnsureParsed lazily parses the document content on first use and caches the
// result. Subsequent calls return the cached ParsedDocument without re-parsing.
// This keeps startup / history listing cheap — only spec files + session.json
// are read from disk, and full ParseDocument (including schema mock expansion)
// is deferred until an operation list / detail / build request actually needs it.
func (d *cachedOpenAPIDocument) EnsureParsed() (*openapi.ParsedDocument, error) {
	if d == nil {
		return nil, utils.Error("nil openapi document cache")
	}
	if d.Parsed != nil {
		return d.Parsed, nil
	}
	d.parseOnce.Do(func() {
		d.Parsed, d.parseErr = openapi.ParseDocument(d.Content, nil)
		if d.Parsed != nil {
			title := strings.TrimSpace(d.Session.Title)
			if title == "" || title == d.Session.SessionID {
				d.Session.Title = strings.TrimSpace(d.Parsed.Info.Title)
				if d.Session.Title == "" {
					d.Session.Title = d.Session.SessionID
				}
			}
		}
	})
	return d.Parsed, d.parseErr
}

var (
	openAPIDocumentStore sync.Map
)

type openapiAction struct{}

func newOpenAPIAction() *openapiAction {
	return &openapiAction{}
}

func (a *openapiAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.GetWithContext(context.Background(), params)
}

func (a *openapiAction) GetWithContext(ctx context.Context, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	_ = ctx
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	location := strings.TrimSpace(u.GetLocation())
	query := openAPIQueryValues(u.GetQuery())
	if location == openAPIHistoryLocation || query.Get(openAPIQueryOp) == openAPIHistoryLocation {
		ensureOpenAPIDocumentStoreLoaded()
		return listOpenAPIDocumentHistory()
	}
	if location == "" || location == openAPIUploadLocation {
		return nil, utils.Error("openapi document id is required")
	}

	doc, err := loadCachedOpenAPIDocument(location)
	if err != nil {
		return nil, err
	}

	if query.Get(openAPIQueryOp) == openAPIOpDetail || (query.Get(openAPIQueryMethod) != "" && query.Get(openAPIQueryPath) != "") {
		parsed, err := doc.EnsureParsed()
		if err != nil {
			return nil, err
		}
		op, err := resolveOpenAPIOperation(parsed, query)
		if err != nil {
			return nil, err
		}
		return &ypb.RequestYakURLResponse{
			Resources: []*ypb.YakURLResource{openAPIOperationDetailResource(location, op)},
		}, nil
	}

	parsed, err := doc.EnsureParsed()
	if err != nil {
		return nil, err
	}
	return listOpenAPIDocumentResources(location, parsed)
}

func (a *openapiAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.PostWithContext(context.Background(), params)
}

func (a *openapiAction) PostWithContext(ctx context.Context, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	location := strings.TrimSpace(u.GetLocation())
	query := openAPIQueryValues(u.GetQuery())

	if location == "" || location == openAPIUploadLocation {
		return uploadOpenAPIDocument(ctx, params)
	}

	doc, err := loadCachedOpenAPIDocument(location)
	if err != nil {
		return nil, err
	}

	switch strings.TrimSpace(query.Get(openAPIQueryOp)) {
	case openAPIOpBuild:
		return buildOpenAPIOperationRequests(ctx, location, doc, params)
	case openAPIOpImportAll:
		return importAllOpenAPIRequests(ctx, location, doc, params)
	default:
		return nil, utils.Errorf("unsupported openapi op %q, want %q or %q", query.Get(openAPIQueryOp), openAPIOpBuild, openAPIOpImportAll)
	}
}

func (a *openapiAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.PutWithContext(context.Background(), params)
}

func (a *openapiAction) PutWithContext(ctx context.Context, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	_ = ctx
	return nil, utils.Error("not implemented")
}

func (a *openapiAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.DeleteWithContext(context.Background(), params)
}

func (a *openapiAction) DeleteWithContext(ctx context.Context, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	_ = ctx
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	location := strings.TrimSpace(u.GetLocation())
	if location == "" || location == openAPIUploadLocation {
		return nil, utils.Error("openapi document id is required")
	}
	if err := validateOpenAPIDocumentID(location); err != nil {
		return nil, err
	}
	if err := removeOpenAPIDocument(location); err != nil {
		return nil, err
	}
	return &ypb.RequestYakURLResponse{}, nil
}

func (a *openapiAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.HeadWithContext(context.Background(), params)
}

func (a *openapiAction) HeadWithContext(ctx context.Context, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	_ = ctx
	return nil, utils.Error("not implemented")
}

func (a *openapiAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return a.Post(params)
}

func (a *openapiAction) Handle(ctx context.Context, method string, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	switch strings.ToUpper(method) {
	case http.MethodGet:
		return a.GetWithContext(ctx, params)
	case http.MethodPost:
		return a.PostWithContext(ctx, params)
	case http.MethodPut:
		return a.PutWithContext(ctx, params)
	case http.MethodDelete:
		return a.DeleteWithContext(ctx, params)
	case http.MethodHead:
		return a.HeadWithContext(ctx, params)
	default:
		return nil, utils.Errorf("not implemented method: %v", method)
	}
}

func uploadOpenAPIDocument(ctx context.Context, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	content := strings.TrimSpace(string(params.GetBody()))
	if content == "" {
		return nil, utils.Error("openapi document content is empty")
	}

	u := params.GetUrl()
	query := openAPIQueryValues(u.GetQuery())
	parseOpts := &openapi.ParseOptions{
		OverrideDomain: query.Get(openAPIQueryOverrideDomain),
		Context:        ctx,
		OnProgress:     openAPIParseProgressBroadcaster(query.Get(openAPIQueryParseTaskID)),
	}
	if parseBoolQuery(query.Get(openAPIQueryOverrideHTTPS)) {
		v := true
		parseOpts.OverrideHTTPS = &v
	}

	parsed, err := openapi.ParseDocument(content, parseOpts)
	if err != nil {
		return nil, err
	}

	docID := uuid.NewString()
	now := time.Now().Unix()
	specFile := openAPISpecFileName(strings.TrimSpace(query.Get("fileName")), content)
	title := strings.TrimSpace(parsed.Info.Title)
	doc := &cachedOpenAPIDocument{
		Content: content,
		Parsed:  parsed,
		Session: newOpenAPIDocumentSession(docID, title, query.Get("fileName"), specFile, now),
	}
	if err := storeOpenAPIDocument(docID, doc); err != nil {
		return nil, err
	}

	root, err := listOpenAPIDocumentResources(docID, parsed)
	if err != nil {
		return nil, err
	}
	if taskID := strings.TrimSpace(query.Get(openAPIQueryParseTaskID)); taskID != "" && len(root.GetResources()) > 0 {
		root.Resources[0].Extra = append(root.Resources[0].GetExtra(), &ypb.KVPair{
			Key:   openAPIQueryParseTaskID,
			Value: taskID,
		})
	}
	return root, nil
}

func listOpenAPIDocumentHistory() (*ypb.RequestYakURLResponse, error) {
	ensureOpenAPIDocumentStoreLoaded()
	type historyItem struct {
		docID string
		doc   *cachedOpenAPIDocument
	}
	items := make([]historyItem, 0)
	openAPIDocumentStore.Range(func(key, value any) bool {
		docID, ok := key.(string)
		if !ok || docID == "" {
			return true
		}
		doc, ok := value.(*cachedOpenAPIDocument)
		if !ok || doc == nil {
			return true
		}
		items = append(items, historyItem{docID: docID, doc: doc})
		return true
	})
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].doc.Session.LastUsedAt == items[j].doc.Session.LastUsedAt {
			return items[i].docID > items[j].docID
		}
		return items[i].doc.Session.LastUsedAt > items[j].doc.Session.LastUsedAt
	})

	resources := make([]*ypb.YakURLResource, 0, len(items))
	for _, item := range items {
		// title 已存盘时零解析；仅 title 缺失（空或等于 docID）时才解析回填。
		// 展示优先用 Session.Title，避免 Parsed==nil 时 VerboseName/Extra.title 退化成 docID。
		parsed := item.doc.Parsed
		titleBefore := strings.TrimSpace(item.doc.Session.Title)
		if parsed == nil && (titleBefore == "" || titleBefore == item.docID) {
			parsed, _ = item.doc.EnsureParsed()
			titleAfter := strings.TrimSpace(item.doc.Session.Title)
			if titleAfter != "" && titleAfter != titleBefore {
				if err := saveOpenAPIDocumentSessionToDisk(item.docID, item.doc); err != nil {
					log.Warnf("persist openapi document title backfill for %q failed: %v", item.docID, err)
				}
			}
		}
		title := strings.TrimSpace(item.doc.Session.Title)
		resource := openAPIDocumentRootResource(item.docID, parsed, title)
		resource.Extra = append(resource.GetExtra(), openAPIDocumentHistoryExtras(item.docID, item.doc)...)
		resources = append(resources, resource)
	}
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  int64(len(resources)),
		Total:     int64(len(resources)),
		Resources: resources,
	}, nil
}

func openAPIDocumentHistoryExtras(docID string, doc *cachedOpenAPIDocument) []*ypb.KVPair {
	sess := doc.Session
	if sess.SessionID == "" {
		sess.SessionID = docID
	}
	title := strings.TrimSpace(sess.Title)
	if title == "" && doc.Parsed != nil {
		title = strings.TrimSpace(doc.Parsed.Info.Title)
	}
	extras := []*ypb.KVPair{
		{Key: "session_id", Value: sess.SessionID},
		{Key: "title", Value: title},
		{Key: "source", Value: sess.Source},
		{Key: "created_at", Value: fmt.Sprint(sess.CreatedAt)},
		{Key: "updated_at", Value: fmt.Sprint(sess.UpdatedAt)},
		{Key: "last_used_at", Value: fmt.Sprint(sess.LastUsedAt)},
		{Key: "uploaded_at", Value: fmt.Sprint(sess.CreatedAt)},
	}
	if sess.FileName != "" {
		extras = append(extras, &ypb.KVPair{Key: "file_name", Value: sess.FileName})
	}
	if doc.Parsed != nil {
		extras = append(extras, &ypb.KVPair{Key: "operation_count", Value: fmt.Sprint(len(doc.Parsed.Operations))})
	}
	return extras
}

func listOpenAPIDocumentResources(docID string, parsed *openapi.ParsedDocument) (*ypb.RequestYakURLResponse, error) {
	if parsed == nil {
		return nil, utils.Error("parsed openapi document is nil")
	}

	resources := []*ypb.YakURLResource{
		openAPIDocumentRootResource(docID, parsed, ""),
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

func buildOpenAPIOperationRequests(ctx context.Context, docID string, doc *cachedOpenAPIDocument, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	_ = docID
	query := openAPIQueryValues(params.GetUrl().GetQuery())
	parsed, err := doc.EnsureParsed()
	if err != nil {
		return nil, err
	}
	op, err := resolveOpenAPIOperation(parsed, query)
	if err != nil {
		return nil, err
	}

	buildOpts, err := parseOpenAPIBuildOptions(params, query)
	if err != nil {
		return nil, err
	}
	buildOpts.Context = ctx

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

func importAllOpenAPIRequests(ctx context.Context, docID string, doc *cachedOpenAPIDocument, params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	_ = docID
	query := openAPIQueryValues(params.GetUrl().GetQuery())
	buildOpts, err := parseOpenAPIBuildOptions(params, query)
	if err != nil {
		return nil, err
	}
	buildOpts.Context = ctx
	buildOpts.OnProgress = openAPIParseProgressBroadcaster(query.Get(openAPIQueryParseTaskID))

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
	ensureOpenAPIDocumentStoreLoaded()
	if err := validateOpenAPIDocumentID(docID); err != nil {
		return nil, err
	}
	raw, ok := openAPIDocumentStore.Load(docID)
	if ok {
		doc, ok := raw.(*cachedOpenAPIDocument)
		if !ok || doc == nil {
			return nil, utils.Errorf("invalid openapi document cache for %q", docID)
		}
		touchOpenAPIDocumentLastUsed(docID, doc)
		return doc, nil
	}
	doc, err := loadOpenAPIDocumentFromDisk(docID)
	if err != nil {
		return nil, utils.Errorf("openapi document %q not found", docID)
	}
	touchOpenAPIDocumentLastUsed(docID, doc)
	openAPIDocumentStore.Store(docID, doc)
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

func openAPIDocumentRootResource(docID string, parsed *openapi.ParsedDocument, sessionTitle string) *ypb.YakURLResource {
	var info openapi.DocumentInfo
	var operationCount int
	var warnings []string
	if parsed != nil {
		info = parsed.Info
		operationCount = len(parsed.Operations)
		warnings = parsed.Warnings
	}
	title := strings.TrimSpace(info.Title)
	if title == "" {
		title = strings.TrimSpace(sessionTitle)
	}
	if title == "" {
		title = docID
	}
	// Extra.title 与 VerboseName 保持一致，避免 Parsed==nil 时 Extra 先写出空 title，
	// 前端 GetQueryParam 取到第一个空值。
	if strings.TrimSpace(info.Title) == "" {
		info.Title = title
	}
	return &ypb.YakURLResource{
		ResourceType:      openAPIResourceDocument,
		VerboseType:       "OpenAPI Document",
		ResourceName:      docID,
		VerboseName:       title,
		Path:              "/",
		YakURLVerbose:     fmt.Sprintf("openapi://%s/", docID),
		HaveChildrenNodes: operationCount > 0,
		Url: &ypb.YakURL{
			Schema:   "openapi",
			Location: docID,
			Path:     "/",
		},
		Extra: openAPIDocumentInfoExtras(info, operationCount, warnings),
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

// openAPIParseProgressPush is pushed via DuplexConnection for parse/import progress.
type openAPIParseProgressPush struct {
	TaskID  string  `json:"task_id"`
	Percent float64 `json:"percent"`
	Stage   string  `json:"stage"`
	Message string  `json:"message"`
	Current int     `json:"current"`
	Total   int     `json:"total"`
}

func openAPIParseProgressBroadcaster(taskID string) func(openapi.ParseProgress) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil
	}
	return func(p openapi.ParseProgress) {
		yakit.BroadcastData(yakit.ServerPushType_OpenAPIParse, &openAPIParseProgressPush{
			TaskID:  taskID,
			Percent: p.Percent,
			Stage:   p.Stage,
			Message: p.Message,
			Current: p.Current,
			Total:   p.Total,
		})
	}
}
