package openapi

import (
	_ "embed"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/openapi/openapi2"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"regexp"
	"strings"
	"testing"
)

//go:embed openapi2/testdata/swagger.json
var openapi2demo string

func TestRequest(t *testing.T) {
	var data openapi2.T
	err := data.UnmarshalJSON([]byte(openapi2demo))
	if err != nil {
		t.Error(err)
		t.Failed()
	}

	var root mutate.FuzzHTTPRequestIf
	root, err = mutate.NewFuzzHTTPRequest(`GET / HTTP/1.1
Host: www.example.com
`)
	if err != nil {
		t.Fatal(err)
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

			for _, parameter := range ins.Parameters {
				switch parameter.In {
				case "path":
					r, err := regexp.Compile(`\{\s*(` + regexp.QuoteMeta(parameter.Name) + `)\s*\}`)
					if err != nil {
						log.Errorf("compile parameters failed: %s", err)
						continue
					}
					originPath = r.ReplaceAllStringFunc(originPath, func(s string) string {
						return fmt.Sprint(OpenAPITypeToMockDataLiteral(parameter.Type, parameter.Default))
					})
					methodRoot = methodRoot.FuzzPath(originPath)
				case "query":
					methodRoot = methodRoot.FuzzGetParams(parameter.Name, OpenAPITypeToMockDataLiteral(parameter.Type, parameter.Default))
				case "header":
					methodRoot = methodRoot.FuzzHTTPHeader(parameter.Name, OpenAPITypeToMockDataLiteral(parameter.Type, parameter.Default))
				case "body":
					if ret := parameter.Schema; ret == nil {
						methodRoot = methodRoot.FuzzPostParams(parameter.Name, OpenAPITypeToMockDataLiteral(parameter.Type, parameter.Default))
					} else {
						if ret.Ref == "" && ret.Value == nil {
							methodRoot = methodRoot.FuzzPostRaw("{}")
							continue
						} else if ret.Ref != "" {
							raw := OpenAPI2RefToObject(data, ret.Ref)
							spew.Dump(raw)
							continue
						} else if ret.Value != nil {
							ret.Value.AllowEmptyValue = true
							raw, err := ret.Value.MarshalJSON()
							if err != nil {
								log.Error(err)
							}
							spew.Dump(raw)
						}
					}
				default:
					log.Errorf("unknown parameter type: %s", parameter.In)
				}
			}
			methodRoot.Show()
			fmt.Println("--------------------------------------")
		}
	}
}
