package vulinbox

import (
	_ "embed"
	"net/http"
)

//go:embed vul_postmessageiframe.html
var postMessageDemoHtml []byte

func (s *VulinServer) registerPostMessageIframeCase() {
	r := s.router
	r.HandleFunc("/iframe/post/message/basic", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write(postMessageDemoHtml)
	})
	r.HandleFunc("/iframe/post/message/basic/frame", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.Write([]byte(`<!DOCTYPE html>
<html>
<head>
  <title>Iframe Page</title>
  <script>
    function receiveMessage(event) {
      document.getElementById("output").innerHTML = "inside iframe recv Message: " + event.data;
      event.source.postMessage("Hello from iframe!", event.origin);
    }
    window.addEventListener("message", receiveMessage, false);
  </script>
</head>
<body>
  <h3 style='color: red;'>Iframe PAGE inside</h3>
  <p id="output"></p>
</body>
</html>`))
	}).Name("postMessage 基础案例")
}
