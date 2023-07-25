package vulinbox

import (
	_ "embed"
	"net/http"
)

//go:embed vul_postmessageiframe.html
var postMessageDemoHtml []byte

func (s *VulinServer) registerPostMessageIframeCase() {
	r := s.router
	iframeGroup := r.Name("JSONP 通信与 iframe postMessage 通信案例").Subrouter()
	iframeRoutes := []*VulInfo{
		{
			Path:  "/iframe/post/message/basic/frame",
			Title: "postMessage 基础案例",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
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
			},
			RiskDetected: true,
		},
		{
			Path: "/iframe/post/message/basic",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "text/html")
				writer.Write(postMessageDemoHtml)
			},
		},
	}
	for _, v := range iframeRoutes {
		addRouteWithVulInfo(iframeGroup, v)
	}
}
