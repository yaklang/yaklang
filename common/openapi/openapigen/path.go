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

func pathToOpenAPIStruct(pathRaw string) (string, *openapi3.PathItem, []*openapi3.Parameter) {
	pathWithoutQuery, query, haveQuery := strings.Cut(pathRaw, "?")
	if !haveQuery {
		query = ""
	}
	item := &openapi3.PathItem{
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
	if funk.IsEmpty(response) {
		response, _, _ = lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 OK
Content-Type: application/json

{}`))
	}

	ins := &openapi3.Response{}
	for key, value := range lowhttp.GetHTTPPacketHeaders(response) {
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
	return ins
}

func requestToOpenAPIStruct(request []byte) (string, *openapi3.PathItem, *openapi3.Operation, error) {
	p := lowhttp.GetHTTPRequestPath(request)
	if p == "" {
		return "", nil, nil, utils.Error("path is empty")
	}
	pathWithoutQuery, ops, params := pathToOpenAPIStruct(p)

	operation := &openapi3.Operation{
		Parameters: paramListToParameters(params),
		Responses:  nil,
	}

	method := lowhttp.GetHTTPRequestMethod(request)
	switch strings.TrimSpace(strings.ToUpper(method)) {
	case "GET":
		ops.Get = operation
	case "POST":
		ops.Post = operation
	case "PUT":
		ops.Put = operation
	case "DELETE":
		ops.Delete = operation
	case "CONNECT":
		ops.Connect = operation
	case "OPTIONS":
		ops.Options = operation
	case "TRACE":
		ops.Trace = operation
	case "PATCH":
		ops.Patch = operation
	case "HEAD":
		ops.Head = operation
	default:
		return "", nil, nil, utils.Errorf("method [%v] not supported", method)
	}
	return pathWithoutQuery, ops, operation, nil
}
