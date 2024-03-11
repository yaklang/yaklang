package vulinbox

import "net/http"

var expect100handle = func(writer http.ResponseWriter, request *http.Request) {
	if request.Header.Get("Expect") == "100-continue" {
		writer.WriteHeader(100)
		return
	}
	writer.WriteHeader(200)
	writer.Write([]byte(`This Message is Behind 100-Continue`))
}
