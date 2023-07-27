package vulinbox

import (
	"embed"
	"github.com/yaklang/yaklang/common/log"
	"io"
	"net/http"
	"path"
)

//go:embed sensitivefs
var _sensitiveFS embed.FS

func GetSensitiveFile(name string) []byte {
	f, err := _sensitiveFS.Open(path.Join("sensitivefs", name))
	if err != nil {
		log.Errorf("cannot found sensitive file: %s", err)
		return nil
	}
	raw, _ := io.ReadAll(f)
	f.Close()
	return raw
}

func (s *VulinServer) registerSensitive() {
	r := s.router

	_sensitive := func(s string) string {
		return path.Join("/sensitive", s)
	}
	_ = _sensitive

	/*
		swagger demo
		1. v{1-3}/swagger.json
		2. v{1-3}/rest{/}
		3. /api-doc
		4. /swagger/v1/swagger.json

		{path}?/swagger/index.html
	*/

	var sensitiveGroup = r.PathPrefix("/sensitive").Name("敏感信息与敏感文件泄漏").Subrouter()
	var swaggerGroup = r.PathPrefix("/swagger").Name("敏感信息与敏感文件泄漏（Swagger）").Subrouter()
	var vuls = []*VulInfo{
		{
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Write(GetSensitiveFile("openapi-2.json"))
			},
			Path:         `/v1/swagger.json`,
			Title:        "OpenAPI 2.0 Swagger 泄漏",
			RiskDetected: true,
		},
		{
			Path:  `/v2/swagger.json`,
			Title: "OpenAPI 3.0 Swagger 泄漏",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/json")
				writer.Write(GetSensitiveFile("openapi-3.json"))
			},
			RiskDetected: true,
		},
		{
			Path:  `/`,
			Title: "Swagger UI 泄漏",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", `text/html`)
				writer.Write(GetSensitiveFile("swagger-ui.html"))
			},
			RiskDetected: true,
		},
	}
	for _, v := range vuls {
		addRouteWithVulInfo(sensitiveGroup, v)
		addRouteWithVulInfo(swaggerGroup, v)
	}

	addRouteWithVulInfo(swaggerGroup, &VulInfo{
		Path: `/index.html`,
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			writer.Header().Set("Content-Type", `text/html`)
			writer.Write(GetSensitiveFile("swagger-ui.html"))
		},
		RiskDetected: true,
	})
}
