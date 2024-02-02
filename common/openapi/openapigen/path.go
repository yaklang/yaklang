package openapigen

import (
	"fmt"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strings"
)

// the 3rd return value is true if before and after is same
func shrinkPath(before, after string) (string, []*openapi3.Parameter, bool, error) {
	if !strings.Contains(before, "/") || !strings.Contains(after, "/") {
		return "", nil, false, utils.Errorf("path [%v %v] not contains /", before, after)
	}

	beforeList := utils.PrettifyListFromStringSplited(before, "/")
	afterList := utils.PrettifyListFromStringSplited(after, "/")

	if len(beforeList) != len(afterList) {
		return "", nil, false, utils.Errorf("path [%v %v] not same length", before, after)
	}

	params := make([]*openapi3.Parameter, 0, len(beforeList))
	part := make([]string, len(beforeList))

	for i := 0; i < len(beforeList); i++ {
		if beforeList[i] != afterList[i] && utils.MatchAllOfRegexp(beforeList[i], `\d+`) {
			if ret := i - 1; ret > 0 {
				if strings.HasPrefix(part[ret], "{") && strings.HasSuffix(part[ret], "}") {
					part[i] = fmt.Sprintf("{%v2}", part[ret][1:len(part[ret])-1])
				} else {
					if len(part[ret]) > 0 {
						part[i] = fmt.Sprintf("{%v}", strings.ToLower(part[ret])+"Id")
					} else {
						part[i] = fmt.Sprintf("{%v}", "id")
					}
				}
			} else {
				part[i] = fmt.Sprintf("{%v}", "id")
			}

			if strings.HasPrefix(part[i], "{") {
				param := openapi3.NewPathParameter(strings.Trim(part[i], `{}`))
				param.Required = true
				param.Schema = &openapi3.SchemaRef{
					Value: &openapi3.Schema{
						Type:    "integer",
						Example: beforeList[i],
					},
				}
				params = append(params, param)
			}
		} else {
			part[i] = beforeList[i]
		}
	}

	result := "/" + strings.Join(part, "/")

	return result, params, result == before, nil
}

func extractQueryParamsFromPath(u string) []*openapi3.Parameter {
	_, query, ok := strings.Cut(u, "?")
	if !ok {
		return nil
	}
	return extractQueryParams(query)
}

func extractQueryParams(query string) []*openapi3.Parameter {
	var params []*openapi3.Parameter
	for _, item := range lowhttp.NewQueryParams(query).Items {
		typeVerbose := "string"
		if utils.MatchAllOfRegexp(item.Key, item.Value) {
			typeVerbose = "integer"
		}
		params = append(params, openapi3.NewQueryParameter(item.Key).WithSchema(&openapi3.Schema{
			Type:    typeVerbose,
			Example: item.Value,
		}))
	}
	return params
}

func mergeParams(params ...[]*openapi3.Parameter) []*openapi3.Parameter {
	var result = omap.NewOrderedMap(map[string]*openapi3.Parameter{})
	for _, paramList := range params {
		for _, param := range paramList {
			hash := codec.Sha256(fmt.Sprintln(param.In, param.Name))
			result.Set(hash, param)
		}
	}
	return result.Values()
}

func mergeRefParams(params ...openapi3.Parameters) openapi3.Parameters {
	var origin [][]*openapi3.Parameter
	for _, pset := range params {
		var ret []*openapi3.Parameter
		for _, ref := range pset {
			if ref.Value != nil {
				ret = append(ret, ref.Value)
			}
		}
		origin = append(origin, ret)
	}
	return paramListToParameters(mergeParams(origin...))
}

func pathToOpenAPIStruct(item *openapi3.PathItem, pathRaw string) (string, *openapi3.PathItem, []*openapi3.Parameter) {
	pathWithoutQuery, query, haveQuery := strings.Cut(pathRaw, "?")
	if !haveQuery {
		query = ""
	}
	if item == nil {
		item = &openapi3.PathItem{
			Connect: nil,
			Delete:  nil,
			Get:     nil,
			Head:    nil,
			Options: nil,
			Patch:   nil,
			Post:    nil,
			Put:     nil,
			Trace:   nil,
		}
	}

	if query != "" {
		return pathWithoutQuery, item, extractQueryParams(query)
	}
	return pathWithoutQuery, item, nil
}

func paramListToParameters(params []*openapi3.Parameter) []*openapi3.ParameterRef {
	var result []*openapi3.ParameterRef
	for _, param := range params {
		result = append(result, &openapi3.ParameterRef{
			Value: param,
		})
	}
	return result
}

func responseToOpenAPIStruct(response []byte) *openapi3.Response {
	var body []byte
	if funk.IsEmpty(response) {
		response, body, _ = lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Type: application/json

{}`))
	}

	scheme := anyToScheme(body)

	ins := &openapi3.Response{}
	for key, value := range lowhttp.GetHTTPPacketHeaders(response) {
		switch strings.ToLower(key) {
		case "content-length", "content-type", "transfer-encoding", "content-encoding":
			continue
		}
		if ins.Headers == nil {
			ins.Headers = make(map[string]*openapi3.HeaderRef)
		}
		param := openapi3.NewHeaderParameter(key)
		param.Example = value
		ins.Headers[key] = &openapi3.HeaderRef{
			Value: &openapi3.Header{
				Parameter: *param,
			},
		}

	}
	if scheme != nil {
		ins.WithJSONSchema(scheme)
	}
	return ins
}

func mergeResponse(rsps *openapi3.Responses, code int, rsp *openapi3.Response) *openapi3.Responses {
	if rsps == nil {
		rsps = openapi3.NewResponses()
	}
	if rsps.Status(code) == nil {
		rsps.Set(fmt.Sprint(code), &openapi3.ResponseRef{Value: rsp})
	}
	return rsps
}

func HttpFlowToOpenAPIStruct(before string, pathItem *openapi3.PathItem, req []byte, rsp []byte) (string, *openapi3.PathItem, error) {
	if before == "" {
		before = lowhttp.GetHTTPRequestPathWithoutQuery(req)
	}
	pathName, pathItem, operations, err := requestToOpenAPIStruct(before, pathItem, req)
	if err != nil {
		return pathName, pathItem, err
	}

	statusCode := lowhttp.ExtractStatusCodeFromResponse(rsp)
	if statusCode <= 0 {
		statusCode = 200
	}
	operations.Responses = mergeResponse(operations.Responses, statusCode, responseToOpenAPIStruct(rsp))
	return pathName, pathItem, nil
}

func requestToOpenAPIStruct(beforePath string, item *openapi3.PathItem, request []byte) (string, *openapi3.PathItem, *openapi3.Operation, error) {
	p := lowhttp.GetHTTPRequestPath(request)
	if p == "" {
		return "", nil, nil, utils.Error("path is empty")
	}
	pathWithoutQuery, ops, params := pathToOpenAPIStruct(item, p)

	operation := &openapi3.Operation{
		Parameters: paramListToParameters(params),
		Responses:  nil,
	}

	method := lowhttp.GetHTTPRequestMethod(request)
	switch strings.TrimSpace(strings.ToUpper(method)) {
	case "GET":
		if ops.Get == nil {
			ops.Get = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Get.Parameters = mergeRefParams(ops.Get.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Get.Parameters = mergeRefParams(ops.Get.Parameters, operation.Parameters)
		}
	case "POST":
		if ops.Post == nil {
			ops.Post = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Post.Parameters = mergeRefParams(ops.Post.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Post.Parameters = mergeRefParams(ops.Post.Parameters, operation.Parameters)
		}
	case "PUT":
		if ops.Put == nil {
			ops.Put = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Put.Parameters = mergeRefParams(ops.Put.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Put.Parameters = mergeRefParams(ops.Put.Parameters, operation.Parameters)
		}
	case "DELETE":
		if ops.Delete == nil {
			ops.Delete = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Delete.Parameters = mergeRefParams(ops.Delete.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Delete.Parameters = mergeRefParams(ops.Delete.Parameters, operation.Parameters)
		}
	case "CONNECT":
		if ops.Connect == nil {
			ops.Connect = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Connect.Parameters = mergeRefParams(ops.Connect.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Connect.Parameters = mergeRefParams(ops.Connect.Parameters, operation.Parameters)
		}
	case "OPTIONS":
		if ops.Options == nil {
			ops.Options = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Options.Parameters = mergeRefParams(ops.Options.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Options.Parameters = mergeRefParams(ops.Options.Parameters, operation.Parameters)
		}
	case "TRACE":
		if ops.Trace == nil {
			ops.Trace = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Trace.Parameters = mergeRefParams(ops.Trace.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Trace.Parameters = mergeRefParams(ops.Trace.Parameters, operation.Parameters)
		}
	case "PATCH":
		if ops.Patch == nil {
			ops.Patch = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Patch.Parameters = mergeRefParams(ops.Patch.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Patch.Parameters = mergeRefParams(ops.Patch.Parameters, operation.Parameters)
		}
	case "HEAD":
		if ops.Head == nil {
			ops.Head = operation
		} else {
			after, parameters, isSame, _ := shrinkPath(beforePath, pathWithoutQuery)
			if !isSame && len(parameters) > 0 {
				ops.Head.Parameters = mergeRefParams(ops.Head.Parameters, paramListToParameters(parameters))
				pathWithoutQuery = after
			}
			ops.Head.Parameters = mergeRefParams(ops.Head.Parameters, operation.Parameters)
		}
	default:
		return "", nil, nil, utils.Errorf("method [%v] not supported", method)
	}

	body := lowhttp.GetHTTPPacketBody(request)
	if !funk.IsEmpty(body) {
		operation.RequestBody = &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Content: openapi3.NewContentWithJSONSchema(anyToScheme(body)),
			},
		}
	}

	return pathWithoutQuery, ops, operation, nil
}

type BasicHTTPFlow struct {
	Request  []byte
	Response []byte
}

func generate(c chan *BasicHTTPFlow) ([]byte, error) {
	results := omap.NewOrderedMap(map[string]*openapi3.PathItem{})
	for flow := range c {
		pathWithoutQ := lowhttp.GetHTTPRequestPathWithoutQuery(flow.Request)
		isNewPath := true
		var existedPath string
		for _, existed := range results.Keys() {
			_, params, _, _ := shrinkPath(existed, pathWithoutQ)
			if len(params) > 0 {
				existedPath = existed
				isNewPath = false
				break
			}
		}
		if !isNewPath {
			pathWithoutQ = existedPath
		}
		pathItem, ok := results.Get(pathWithoutQ)
		if !ok {
			pathItem = nil
		}
		newPath, pathItem, err := HttpFlowToOpenAPIStruct(pathWithoutQ, pathItem, flow.Request, flow.Response)
		if err != nil {
			continue
		}
		if newPath != pathWithoutQ {
			results.Delete(pathWithoutQ)
			results.Set(newPath, pathItem)
		} else {
			results.Set(pathWithoutQ, pathItem)
		}
	}

	var t openapi3.T
	t.Info = &openapi3.Info{
		Title:          "Yakit Generated API for www.example.com",
		Description:    "",
		TermsOfService: "",
		Version:        "",
	}

	t.Paths = openapi3.NewPaths()
	results.ForEach(func(i string, v *openapi3.PathItem) bool {
		t.Paths.Set(i, v)
		return true
	})
	return t.MarshalJSON()
}
