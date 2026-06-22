package openapi

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

func appendResponseSummaries(
	item *OperationInfo,
	responses map[string]struct {
		Description string
		ExampleJSON string
	},
	warnings *[]string,
	opRef string,
) {
	for code, rsp := range responses {
		if !IsValidResponseStatusCode(code) {
			if warnings != nil {
				*warnings = append(*warnings, fmt.Sprintf(
					"%s: skipped non-standard response key %q (expected HTTP status code or \"default\")",
					opRef, code,
				))
			}
			continue
		}
		item.Responses = append(item.Responses, ResponseSummary{
			StatusCode:  code,
			Description: rsp.Description,
			ExampleJSON: rsp.ExampleJSON,
		})
	}
}

func appendParameterSummary(
	item *OperationInfo,
	ps ParameterSummary,
	warnings *[]string,
	opRef string,
	isSwaggerV2 bool,
) {
	if !IsValidParameterIn(ps.In, isSwaggerV2) {
		if warnings != nil {
			*warnings = append(*warnings, fmt.Sprintf(
				"%s: skipped parameter %q with non-standard location %q",
				opRef, ps.Name, ps.In,
			))
		}
		return
	}
	item.Parameters = append(item.Parameters, ps)
}

func describeParseFailure(content string, v2err, v3err error) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return utils.Error("openapi document content is empty")
	}

	var raw json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return utils.Errorf("invalid JSON/YAML syntax: %v", err)
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return utils.Errorf("parse openapi document failed: swagger2[%v], openapi3[%v]", v2err, v3err)
	}

	var hints []string
	if _, ok := root["swagger"]; !ok {
		if _, ok3 := root["openapi"]; !ok3 {
			hints = append(hints, "missing required field \"swagger\" (2.x) or \"openapi\" (3.x)")
		}
	}
	if _, ok := root["paths"]; !ok {
		hints = append(hints, "missing required field \"paths\"")
	}
	if len(hints) == 0 {
		return utils.Errorf("document looks like OpenAPI but failed strict parsing: swagger2[%v], openapi3[%v]", v2err, v3err)
	}
	return utils.Errorf("not a valid OpenAPI document (%s); swagger2[%v], openapi3[%v]", strings.Join(hints, "; "), v2err, v3err)
}

func tryLenientParseDocument(content string, opts *ParseOptions) (*ParsedDocument, []string, error) {
	content = normalizeOpenAPIContent(content)
	var root map[string]any
	if err := json.Unmarshal([]byte(content), &root); err != nil {
		return nil, nil, err
	}

	pathsRaw, ok := root["paths"]
	if !ok {
		return nil, nil, utils.Error("missing paths")
	}
	paths, ok := pathsRaw.(map[string]any)
	if !ok {
		return nil, nil, utils.Error("paths is not an object")
	}

	info := DocumentInfo{
		Title:       "Untitled API",
		Version:     "unknown",
		SpecVersion: "lenient",
		Domain:      "www.example.com",
	}
	if infoRaw, ok := root["info"].(map[string]any); ok {
		if title, ok := infoRaw["title"].(string); ok && title != "" {
			info.Title = title
		}
		if version, ok := infoRaw["version"].(string); ok && version != "" {
			info.Version = version
		}
	}
	if host, ok := root["host"].(string); ok && host != "" {
		info.Domain = host
	}
	if schemes, ok := root["schemes"].([]any); ok {
		for _, scheme := range schemes {
			if s, ok := scheme.(string); ok && strings.EqualFold(s, "https") {
				info.IsHttps = true
				break
			}
		}
	}
	if swagger, ok := root["swagger"].(string); ok && swagger != "" {
		info.SpecVersion = swagger
	}
	if openapi, ok := root["openapi"].(string); ok && openapi != "" {
		info.SpecVersion = openapi
	}
	applyParseOptions(&info, opts)

	var operations []OperationInfo
	var warnings []string
	warnings = append(warnings, "document parsed in lenient mode; non-standard fields were ignored")

	for pathStr, pathItemRaw := range paths {
		pathItem, ok := pathItemRaw.(map[string]any)
		if !ok {
			warnings = append(warnings, fmt.Sprintf("skipped path %q: invalid path item", pathStr))
			continue
		}
		for method, opRaw := range pathItem {
			if !IsValidHTTPMethod(method) {
				continue
			}
			opMap, ok := opRaw.(map[string]any)
			if !ok {
				continue
			}
			item := OperationInfo{
				Path:   pathStr,
				Method: strings.ToUpper(method),
			}
			if summary, ok := opMap["summary"].(string); ok {
				item.Summary = summary
			}
			if desc, ok := opMap["description"].(string); ok {
				item.Description = desc
			}
			if opID, ok := opMap["operationId"].(string); ok {
				item.OperationId = opID
			}
			if tagsRaw, ok := opMap["tags"].([]any); ok {
				for _, tag := range tagsRaw {
					if s, ok := tag.(string); ok {
						item.Tags = append(item.Tags, s)
					}
				}
			}
			if paramsRaw, ok := opMap["parameters"].([]any); ok {
				for _, paramRaw := range paramsRaw {
					paramMap, ok := paramRaw.(map[string]any)
					if !ok {
						continue
					}
					ps := ParameterSummary{
						Name:        fmt.Sprint(paramMap["name"]),
						In:          fmt.Sprint(paramMap["in"]),
						Description: fmt.Sprint(paramMap["description"]),
					}
					if ps.Name == "" || ps.In == "" {
						continue
					}
					if required, ok := paramMap["required"].(bool); ok {
						ps.Required = required
					}
					if typ, ok := paramMap["type"].(string); ok {
						ps.Type = typ
					}
					if ps.Example == nil {
						ps.Example = ValueViaField(ps.Name, ps.Type, paramMap["default"])
					}
					appendParameterSummary(&item, ps, &warnings, formatOperationRef(item.Method, item.Path), strings.HasPrefix(info.SpecVersion, "2"))
				}
			}
			if responsesRaw, ok := opMap["responses"].(map[string]any); ok {
				respMap := make(map[string]struct {
					Description string
					ExampleJSON string
				})
				for code, rspRaw := range responsesRaw {
					rspMap, ok := rspRaw.(map[string]any)
					if !ok {
						continue
					}
					respMap[code] = struct {
						Description string
						ExampleJSON string
					}{
						Description: fmt.Sprint(rspMap["description"]),
					}
				}
				appendResponseSummaries(&item, respMap, &warnings, formatOperationRef(item.Method, item.Path))
			}
			operations = append(operations, item)
		}
	}

	if len(operations) == 0 {
		return nil, warnings, utils.Error("no valid operations found in lenient parse")
	}

	return &ParsedDocument{
		Info:       info,
		Operations: operations,
		Content:    content,
		Warnings:   warnings,
	}, warnings, nil
}
