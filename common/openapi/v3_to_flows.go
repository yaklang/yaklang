package openapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/openapi/openapi3"
	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"net/http"
	"regexp"
	"strings"
)

func applyParameters(data openapi3.T, param *openapi3.Parameter, methodRoot mutate.FuzzHTTPRequestIf, originPath string) (mutate.FuzzHTTPRequestIf, string) {
	scheme, err := v3_schemaToValue(data, param.Schema)
	if err != nil {
		log.Errorf("v3_schemaToValue [%v] failed: %v", param.Name, err)
		return methodRoot, originPath
	}
	switch param.In {
	case openapi3.ParameterInQuery:
		methodRoot = methodRoot.FuzzGetParams(param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
	case openapi3.ParameterInHeader:
		methodRoot = methodRoot.FuzzHTTPHeader(param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
	case openapi3.ParameterInPath:
		r, err := regexp.Compile(`\{\s*(` + regexp.QuoteMeta(param.Name) + `)\s*\}`)
		if err != nil {
			log.Errorf("compile parameters failed: %s", err)
			return methodRoot, originPath
		}
		originPath = r.ReplaceAllStringFunc(originPath, func(s string) string {
			return fmt.Sprint(ValueViaField(param.Name, scheme.Type, scheme.Default))
		})
		methodRoot = methodRoot.FuzzPath(originPath)
	case openapi3.ParameterInCookie:
		methodRoot = methodRoot.FuzzCookie(param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
	}
	return methodRoot, originPath
}

func v3Generator(t string, config *OpenAPIConfig) error {
	var data openapi3.T
	jsonT, err := yaml.YAMLToJSON([]byte(t))
	if err == nil {
		t = string(jsonT)
	}
	err = data.UnmarshalJSON([]byte(t))
	if err != nil {
		return utils.Wrapf(err, "unmarshal openapi3 failed")
	}
	if config == nil {
		config = NewDefaultOpenAPIConfig()
	}

	if !strings.HasPrefix(data.OpenAPI, "3") && !strings.HasPrefix(data.OpenAPI, "v3") {
		return utils.Errorf("openapi is not v3, got (%v)", data.OpenAPI)
	}

	for _, server := range data.Servers {
		var originHttps = strings.HasPrefix(strings.ToLower(server.URL), "https://")
		urlStr := utils.ExtractHostPort(server.URL)
		domian, _, err := utils.ParseStringToHostPort(urlStr)
		if err != nil {
			domian = urlStr
		}
		if config.Domain == "" {
			config.Domain = domian
		}
		if !config.IsHttps {
			config.IsHttps = originHttps
		}
	}

	if config.Domain == "" {
		config.Domain = "www.example.com"
	}

	var root mutate.FuzzHTTPRequestIf
	root, err = mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com
`, mutate.OptHTTPS(config.IsHttps))
	if err != nil {
		return utils.Wrapf(err, "create http request failed")
	}

	root = root.FuzzHTTPHeader("Host", config.Domain)

	baseUrl, _ := data.Servers.BasePath()
	if baseUrl != "" {
		baseUrl = strings.TrimRight(baseUrl, "/")
		root = root.FuzzPath(baseUrl)
	}

	for _, pathStr := range data.Paths.InMatchingOrder() {
		pathIns := data.Paths.Value(pathStr)
		log.Debugf("path: %v, ops: %v", pathStr, len(pathIns.Operations()))

		if strings.Contains(pathStr, `/whitelabel/links/{link_id}/subuser`) {
			log.Debugf("path: %v, ops: %v", pathStr, len(pathIns.Operations()))
		}

		pathRoot := root.FuzzPathAppend(pathStr)

		if len(pathIns.Parameters) > 0 {
			pr := pathRoot.FirstFuzzHTTPRequest().GetPath()
			var originPath, _ = codec.PathUnescape(pr)
			if originPath == "" {
				originPath = pr
			}
			for _, paramIns := range pathIns.Parameters {
				param, err := v3_parameterToValue(data, paramIns)
				if err != nil {
					log.Errorf("v3_parameterToValue [%v] failed: %v", param.Name, err)
					continue
				}
				pathRoot, originPath = applyParameters(data, param, pathRoot, originPath)
			}
		}

		for op, ins := range pathIns.Operations() {
			methodRoot := pathRoot.FuzzMethod(op)
			pr := methodRoot.FirstFuzzHTTPRequest().GetPath()
			var originPath, _ = codec.PathUnescape(pr)
			if originPath == "" {
				originPath = pr
			}

			for _, parameter := range ins.Parameters {
				param, err := v3_parameterToValue(data, parameter)
				if err != nil {
					log.Errorf("v3_parameterToValue [%v] failed: %v", param.Name, err)
					continue
				}
				scheme, err := v3_schemaToValue(data, param.Schema)
				if err != nil {
					log.Errorf("v3_schemaToValue [%v] failed: %v", param.Name, err)
					continue
				}
				switch param.In {
				case openapi3.ParameterInQuery:
					methodRoot = methodRoot.FuzzGetParams(param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
				case openapi3.ParameterInHeader:
					methodRoot = methodRoot.FuzzHTTPHeader(param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
				case openapi3.ParameterInPath:
					r, err := regexp.Compile(`\{\s*(` + regexp.QuoteMeta(param.Name) + `)\s*\}`)
					if err != nil {
						log.Errorf("compile parameters failed: %s", err)
						continue
					}
					originPath = r.ReplaceAllStringFunc(originPath, func(s string) string {
						return fmt.Sprint(ValueViaField(param.Name, scheme.Type, scheme.Default))
					})
					methodRoot = methodRoot.FuzzPath(originPath)
				case openapi3.ParameterInCookie:
					methodRoot = methodRoot.FuzzCookie(param.Name, ValueViaField(param.Name, scheme.Type, scheme.Default))
				}
			}

			var bodyRoots []mutate.FuzzHTTPRequestIf
			if ret, _ := v3_requestBodyToValue(data, ins.RequestBody); ret != nil {
				for contentType, scheme := range ret.Content {
					bodyRoot := methodRoot.FuzzHTTPHeader("Content-Type", contentType)
					sIns, err := v3_schemaToValue(data, scheme.Schema)
					if err != nil {
						log.Errorf("v3_schemaToValue [%v] failed: %v", scheme.Schema, err)
						continue
					}
					bytes := v3_mockSchemaJson(data, sIns)
					if len(bytes) > 0 {
						bodyRoot = bodyRoot.FuzzPostRaw(string(bytes))
					}
					bodyRoots = append(bodyRoots, bodyRoot)
				}
			}

			//pr := methodRoot.FirstFuzzHTTPRequest().GetPath()
			//var originPath, _ = codec.PathUnescape(pr)
			//if originPath == "" {
			//	originPath = pr
			//}

			if len(bodyRoots) == 0 {
				bodyRoots = append(bodyRoots, methodRoot)
			}

			for _, forkedBody := range bodyRoots {
				var responses [][]byte
				for code, responseRef := range ins.Responses.Map() {
					fakeResponse := []byte(`HTTP/1.1 200 OK
Content-Type: application/json

{}`)
					codeInt := codec.Atoi(code)
					if codeInt > 0 {
						fakeResponse = lowhttp.ReplaceHTTPPacketFirstLine(fakeResponse, fmt.Sprintf("HTTP/1.1 %s %v", code, http.StatusText(codeInt)))
					}

					response, err := v3_responseToValue(data, responseRef)
					if err != nil {
						log.Errorf("v3_responseToValue [%v] failed: %v", responseRef, err)
						responses = append(responses, fakeResponse)
						continue
					}

					//for h, k := range response.Headers {
					//	headerValue, err := v3_headerToValue(data, k)
					//	if err != nil {
					//		log.Errorf("v3_headerToValue [%v] failed: %v", k, err)
					//		continue
					//	}
					//	headerValue.Content
					//	forkedBody.FuzzHTTPHeader(headerValue.Name, v3_mockMediaType(data, headerValue.))
					//}

					for contentType, schemeRef := range response.Content {
						scheme, err := v3_schemaToValue(data, schemeRef.Schema)
						if err != nil {
							log.Errorf("v3_schemaToValue [%v] failed: %v", schemeRef.Schema, err)
							responses = append(responses, fakeResponse)
							continue
						}
						bytes := v3_mockSchemaJson(data, scheme)
						if len(bytes) > 0 {
							fakeResponse = lowhttp.ReplaceHTTPPacketHeader(fakeResponse, "Content-Type", contentType)
							fakeResponse = lowhttp.ReplaceHTTPPacketBodyFast(fakeResponse, bytes)
						}
						responses = append(responses, fakeResponse)
					}
				}

				if config.FlowHandler != nil {
					req := forkedBody.FirstHTTPRequestBytes()
					urlStr, _ := lowhttp.ExtractURLStringFromHTTPRequest(req, config.IsHttps)
					for _, rspRaw := range responses {
						record, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(config.IsHttps, req, rspRaw, "openapi", urlStr, "127.0.0.1:80")
						if err != nil {
							continue
						}
						config.FlowHandler(record)
					}

				}
			}
		}
	}
	return nil
}
