package openapi

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
	yaml "github.com/yaklang/yaklang/common/openapi/openapiyaml"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"net/http"
	"regexp"
	"strings"
)

func v2Generator(t string, config *OpenAPIConfig) error {
	var data openapi2.T
	jsonT, err := yaml.YAMLToJSON([]byte(t))
	if err == nil {
		t = string(jsonT)
	}
	err = data.UnmarshalJSON([]byte(t))
	if err != nil {
		return utils.Wrapf(err, "unmarshal openapi2 failed")
	}
	if config == nil {
		config = NewDefaultOpenAPIConfig()
	}

	var root mutate.FuzzHTTPRequestIf
	root, err = mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com
`)
	if err != nil {
		return utils.Wrapf(err, "create http request failed")
	}
	if data.BasePath != "" {
		basePath := strings.TrimRight(data.BasePath, "/")
		root = root.FuzzPath(basePath)
	}

	for pathStr, i := range data.Paths {
		pathRoot := root.FuzzPathAppend(pathStr)
		for op, ins := range i.Operations() {
			methodRoot := pathRoot.FuzzMethod(op)
			pr := methodRoot.FirstFuzzHTTPRequest().GetPath()
			var originPath, _ = codec.PathUnescape(pr)
			if originPath == "" {
				originPath = pr
			}

			if len(ins.Consumes) > 0 {
				methodRoot = methodRoot.FuzzHTTPHeader("Content-Type", ins.Consumes[0])
			}

			for _, parameter := range ins.Parameters {
				switch parameter.In {
				case "path":
					r, err := regexp.Compile(`\{\s*(` + regexp.QuoteMeta(parameter.Name) + `)\s*\}`)
					if err != nil {
						log.Errorf("compile parameters failed: %s", err)
						continue
					}
					originPath = r.ReplaceAllStringFunc(originPath, func(s string) string {
						return fmt.Sprint(ValueViaField(parameter.Name, parameter.Type, parameter.Default))
					})
					methodRoot = methodRoot.FuzzPath(originPath)
				case "query":
					methodRoot = methodRoot.FuzzGetParams(parameter.Name, ValueViaField(parameter.Name, parameter.Type, parameter.Default))
				case "header":
					methodRoot = methodRoot.FuzzHTTPHeader(parameter.Name, ValueViaField(parameter.Name, parameter.Type, parameter.Default))
				case "formData":
					if parameter.Type != "file" {
						methodRoot = methodRoot.FuzzFormEncoded(parameter.Name, ValueViaField(parameter.Name, parameter.Type, parameter.Default))
					} else {
						methodRoot = methodRoot.FuzzUploadFile(parameter.Name, "filename.txt", []byte(`[[file-placeholder]]`))
					}
				case "body":
					if ret := parameter.Schema; ret == nil {
						methodRoot = methodRoot.FuzzPostParams(parameter.Name, ValueViaField(parameter.Name, parameter.Type, parameter.Default))
					} else {
						if ret.Ref == "" && ret.Value == nil {
							methodRoot = methodRoot.FuzzPostRaw("{}")
							continue
						} else if ret.Ref != "" {
							rawObj := v2_SchemeRefToObject(data, ret.Ref)
							raw, err := json.Marshal(rawObj)
							if err != nil {
								log.Errorf("openapi2.0 body(ref) marshal failed: %s", err)
								raw = []byte("{}")
							}
							methodRoot = methodRoot.FuzzPostRaw(string(raw))
						} else if ret.Value != nil {
							ret.Value.AllowEmptyValue = true
							val := schemaValue(data, ret.Value)
							raw, err := json.Marshal(val)
							if err != nil {
								log.Errorf("openapi2.0 body marshal failed: %s", err)
								raw = []byte("{}")
							}
							methodRoot = methodRoot.FuzzPostRaw(string(raw))
						}
					}
				default:
					log.Errorf("unknown parameter type: %s", parameter.In)
				}
			}
			results, err := methodRoot.Results()
			if err != nil {
				log.Warnf("get fuzz results failed: %s", err)
				continue
			}

			for _, request := range results {
				reqBytes, err := utils.DumpHTTPRequest(request, true)
				if err != nil {
					continue
				}
				urlStr, err := lowhttp.ExtractURLStringFromHTTPRequest(reqBytes, config.IsHttps)
				for statusCode, rsp := range ins.Responses {
					if codec.Atoi(statusCode) == 0 {
						statusCode = "200"
					}
					fakeResponse := []byte(`HTTP/1.1 200 OK
Content-Type: application/json
`)
					fakeResponse = lowhttp.ReplaceHTTPPacketFirstLine(fakeResponse, `HTTP/1.1 `+fmt.Sprint(statusCode)+` `+http.StatusText(codec.Atoi(statusCode)))
					if len(ins.Produces) > 0 {
						fakeResponse = lowhttp.ReplaceHTTPPacketHeader(fakeResponse, `Content-Type`, ins.Produces[0])
					}

					body := v2_SchemeRefToBytes(data, rsp.Schema)
					if len(body) > 0 {
						fakeResponse = lowhttp.ReplaceHTTPPacketBody(fakeResponse, body, false)
					}
					record, err := yakit.CreateHTTPFlowFromHTTPWithBodySavedFromRaw(config.IsHttps, reqBytes, fakeResponse, "openapi-2.0", urlStr, "127.0.0.1:80")
					if err != nil {
						continue
					}
					if config.FlowHandler != nil {
						config.FlowHandler(record)
					}
				}

			}
		}
	}
	return nil
}
