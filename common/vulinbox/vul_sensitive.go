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

	/*
		swagger demo
		1. v{1-3}/swagger.json
		2. v{1-3}/rest{/}
		3. /api-doc
		4. /swagger/v1/swagger.json

		{path}?/swagger/index.html
	*/
	r.HandleFunc(_sensitive("v1/swagger.json"), func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(GetSensitiveFile("openapi-2.json"))
	})
	r.HandleFunc(_sensitive("v2/swagger.json"), func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.Write(GetSensitiveFile("openapi-3.json"))
	})
	r.HandleFunc(`/swagger/v1/swagger.json`, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", `application/json`)
		writer.Write(GetSensitiveFile("openapi-2.json"))
	})
	r.HandleFunc(`/swagger/v2/swagger.json`, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", `application/json`)
		writer.Write(GetSensitiveFile("openapi-3.json"))
	})
	r.HandleFunc(`/swagger/`, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", `text/html`)
		writer.Write(GetSensitiveFile("swagger-ui.html"))
	})
	r.HandleFunc(`/swagger/index.html`, func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", `text/html`)
		writer.Write(GetSensitiveFile("swagger-ui.html"))
	})
}
