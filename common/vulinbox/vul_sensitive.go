package vulinbox

import (
	"archive/zip"
	"bytes"
	"embed"

	"github.com/yaklang/yaklang/common/log"

	"io"
	"net/http"
	"path"
	"strings"
)

//go:embed sensitivefs
var _sensitiveFS embed.FS

//go:embed fakegit/website.zip
var _fakeGitWebsite []byte

//go:embed fakegit/website-repository.git.zip
var _fakeGitRepository []byte

//go:embed fakegit/sca-testcase.git.zip
var _fakeGitSCARespos []byte

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

	zipGitFS, err := zip.NewReader(bytes.NewReader(_fakeGitWebsite), int64(len(_fakeGitWebsite)))
	if err != nil {
		log.Errorf("cannot open zip file: %s", err)
	}

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
		{
			Path:  `/website/`,
			Title: "Git Repository 泄漏",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Location", "/git/website/index.html")
				writer.WriteHeader(302)
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
	fakeGitSubrouter := s.router.PathPrefix("/git/")
	fakeGitSubrouter.PathPrefix("/website/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if zipGitFS == nil {
			Failed(writer, request, "Create FAKE GIT Website FAILED")
			return
		}

		request.RequestURI = strings.TrimPrefix(request.RequestURI, `/git/`)
		request.URL.Path = strings.TrimPrefix(request.URL.Path, `/git/`)

		filePath := request.URL.Path
		if filePath == "website/" {
			filePath = "website/index.html"
		}
		if strings.Contains(filePath, "flag.txt") {
			Failed(writer, request, "Cannot found file(%v) in fake git website", request.URL.Path)
		}

		var fp, err = zipGitFS.Open(filePath)
		if err != nil {
			Failed(writer, request, "Cannot found file(%v) in fake git website", request.URL.Path)
			return
		}
		defer fp.Close()
		raw, _ := io.ReadAll(fp)

		if strings.Contains(filePath, ".git/") {
			writer.Header().Set("Content-Type", `application/octet-stream`)
		} else {
			writer.Header().Set("Content-Type", `text/html`)
		}
		writer.Write(raw)
	})

	{
		// "/gitserver/sca-testcase.git"
		router, handler := GeneratorGitHTTPHandler("gitserver", "sca-testcase.git", _fakeGitSCARespos)
		s.router.PathPrefix(router).HandlerFunc(handler)
	}

	{
		// "/gitserver/website-repository.git"
		router, handler := GeneratorGitHTTPHandler("gitserver", "website-repository.git", _fakeGitRepository)
		s.router.PathPrefix(router).HandlerFunc(handler)
	}
}
