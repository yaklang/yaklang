package vulinbox

import (
	"fmt"
	"html/template"
	"net/http"
	"regexp"
)

func (s *VulinServer) registerXSS() {
	var router = s.router
	router.HandleFunc("/xss/safe", func(writer http.ResponseWriter, request *http.Request) {
		var name = request.URL.Query().Get("name")
		safeName := template.HTMLEscapeString(name)
		writer.Write([]byte(fmt.Sprintf(`<html>
Hello %v
</html>`, safeName)))
		writer.WriteHeader(200)
		return
	})
	router.HandleFunc("/xss/echo", func(writer http.ResponseWriter, request *http.Request) {
		var name = request.URL.Query().Get("name")
		writer.Write([]byte(fmt.Sprintf(`<html>
Hello %v
</html>`, name)))
		writer.WriteHeader(200)
		return
	})
	router.HandleFunc("/xss/replace/nocase", func(writer http.ResponseWriter, request *http.Request) {
		var name = request.URL.Query().Get("name")
		scriptRegex := regexp.MustCompile("(?i)<script>")
		name = scriptRegex.ReplaceAllString(name, "")

		scriptEndRegex := regexp.MustCompile("(?i)</script>")
		name = scriptEndRegex.ReplaceAllString(name, "")
		writer.Write([]byte(fmt.Sprintf(`<html>
Hello %v
</html>`, name)))
		writer.WriteHeader(200)
		return
	})
}
