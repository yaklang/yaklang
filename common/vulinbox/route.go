package vulinbox

import (
	"net/http"
)

func (s *VulinServer) init() {
	router := s.router

	/*
		SQL注入CASE：http://www.bjski.com.cn/info.php?fid=8&id=1
	*/
	router.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=UTF8")
		writer.Write([]byte(`
<ol>
  <li><a target="_blank" href="/user/by-id-safe?id=1">安全的 SQL 注入</a></li>
  <li><a target="_blank"  href="/user/id?id=1">不安全的 SQL 注入(ID)</a></li>
  <li><a target="_blank"  href="/user/name?name=admin">不安全的 SQL 注入(NAME)</a></li>
  <li><a target="_blank"  href='/ssrf-json-in-get?json={"abc":%20123,%20"ref":%20"http://www.baidu.com"}'>SSRF GET 中 JSON 参数情况</a></li>
  <li><a target="_blank"  href='/ssrf-in-get?url=http://www.baidu.com/'>SSRF GET 中 URL 参数情况</a></li>
  <li><a target="_blank"  href='/ssrf-in-post'>SSRF POST 中 URL 参数情况</a></li>
  <li><a target="_blank"  href='/ssrf-in-json-body'>SSRF JSON Body 中 REF 为 URL</a></li>
  <li><a  target="_blank" href='/ssrf-json-in-post-param'>SSRF POST 中有个 JSON 参数，其中 REF 为 URL 的情况</a></li>
  <li><a  target="_blank" href='/ping/cmd/shlex?ip=127.0.0.1'>Shlex 解析的命令注入</a></li>
  <li><a target="_blank"  href='/ping/cmd/bash?ip=127.0.0.1'>Bash 解析的命令注入</a></li>
</ol>
`))
	})
	s.registerSQLinj()
	s.registerSSRF()
	s.registerPingCMDI()
}
