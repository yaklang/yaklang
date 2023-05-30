package vulinbox

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"
)

func (s *VulinServer) registerSSRF() {
	s.router.HandleFunc("/ssrf-json-in-get", func(writer http.ResponseWriter, request *http.Request) {
		raw := request.URL.Query().Get("json")
		if raw == "" {
			writer.Write([]byte(`暂无数据！`))
			return
		}
		var m = make(map[string]interface{})
		err := json.Unmarshal([]byte(raw), &m)
		if err != nil {
			writer.Write([]byte(`JSON Syntax Error: ` + err.Error()))
			return
		}

		ref, ok := m["ref"]
		if !ok {
			writer.Write([]byte(`no ref in json found!`))
			return
		}

		var u = fmt.Sprint(ref)
		c := utils.NewDefaultHTTPClient()
		c.Timeout = 5 * time.Second
		rsp, err := c.Get(u)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		rawResponse, err := utils.HttpDumpWithBody(rsp, true)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		writer.Write(rawResponse)
	})
	s.router.HandleFunc("/ssrf-in-get", func(writer http.ResponseWriter, request *http.Request) {
		ref := request.URL.Query().Get("url")
		var u = fmt.Sprint(ref)
		c := utils.NewDefaultHTTPClient()
		c.Timeout = 5 * time.Second
		rsp, err := c.Get(u)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		rawResponse, err := utils.HttpDumpWithBody(rsp, true)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		writer.Write(rawResponse)
	})
	s.router.HandleFunc("/ssrf-in-post", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "GET" {
			writer.Header().Set("Content-Type", "text/html; charset=utf8")
			writer.Write([]byte(`
<form action="/ssrf-in-post" method="post">
	<label for="name">姓名:</label>
	<input type="text" id="name" name="name" ><br><br>

	<label for="email">邮箱:</label>
	<input type="email" id="email" name="email" ><br><br>

	<label for="age">年龄:</label>
	<input type="number" id="age" name="age" min="2" max="120" ><br><br>

	
	<label for="url">URL</label>
	<input id='url' name="url"><br><br>

	<label for="gender">性别:</label>
	<select id="gender" name="gender" >
		<option value="">请选择</option>
		<option value="male">男</option>
		<option value="female">女</option>
		<option value="other">其他</option>
	</select><br><br>

	<label for="message">留言:</label>
	<textarea id="message" name="message" rows="5" ></textarea><br><br>

	<input type="submit" value="提交">
</form>
`))
			return
		}
		raw, err := ioutil.ReadAll(request.Body)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		values, err := url.ParseQuery(string(raw))
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		var u = fmt.Sprint(values.Get("url"))
		c := utils.NewDefaultHTTPClient()
		c.Timeout = 10 * time.Second
		rsp, err := c.Get(u)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		rawResponse, err := utils.HttpDumpWithBody(rsp, true)
		if err != nil {
			writer.Write([]byte(err.Error()))
			return
		}
		writer.Write(rawResponse)
	})
	s.router.HandleFunc("/ssrf-in-post-multipart", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == "GET" {
			writer.Header().Set("Content-Type", "text/html; charset=utf8")
			writer.Write([]byte(`
<form action="/ssrf-in-post" method="post" enctype="multipart/form-data">
	<label for="name">姓名:</label>
	<input type="text" id="name" name="name" ><br><br>

	<label for="email">邮箱:</label>
	<input type="email" id="email" name="email" ><br><br>

	<label for="age">年龄:</label>
	<input type="number" id="age" name="age" min="2" max="120" ><br><br>

	
	<label for="url">URL</label>
	<input id='url' name="url"><br><br>

	<label for="gender">性别:</label>
	<select id="gender" name="gender" >
		<option value="">请选择</option>
		<option value="male">男</option>
		<option value="female">女</option>
		<option value="other">其他</option>
	</select><br><br>

	<label for="message">留言:</label>
	<textarea id="message" name="message" rows="5" ></textarea><br><br>

	<input type="submit" value="提交">
</form>
`))
			return
		}
	})
}
