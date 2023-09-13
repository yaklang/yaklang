package vulinbox

import (
	"encoding/json"
	"fmt"
	uuid "github.com/satori/go.uuid"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func (s *VulinServer) registerSSRF() {

	s.router.HandleFunc("/redirect/main", func(writer http.ResponseWriter, request *http.Request) {
		DefaultRender(`<h1>Hello, Welcome to Vulinbox!</h1>`, writer, request)
	})

	ssrfGroup := s.router.PathPrefix("/ssrf").Name("SSRF 参数多种情况的测试").Subrouter()

	ssrfGroup.HandleFunc("/flag", func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.Host, "127.0.0.1") {
			uuid := uuid.NewV4().String()
			DefaultRender(fmt.Sprintf("flag{%s}", uuid), writer, request)
			return
		}
		writer.WriteHeader(http.StatusInternalServerError)
		return
	}).Methods(http.MethodGet)
	ssrfRoutes := []*VulInfo{
		{
			DefaultQuery: "json={\"abc\": 123, \"ref\": \"http://www.baidu.com\"}",
			Path:         "/json-in-get",
			Title:        "SSRF JSON Body SSRF",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
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
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "url=http://www.baidu.com/",
			Path:         "/in-get",
			Title:        "SSRF GET 中 URL 参数",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
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
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "",
			Path:         "/in-post",
			Title:        "SSRF POST 中 URL 参数",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == "GET" {
					writer.Header().Set("Content-Type", "text/html; charset=utf8")
					writer.Write([]byte(`
<form action="/ssrf/in-post" method="post">
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
			},
			RiskDetected: true,
		},

		{
			DefaultQuery: "",
			Path:         "/rebinding/in-post",
			Title:        "SSRF POST 中 URL 参数 (DNS Rebinding)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == "GET" {
					writer.Header().Set("Content-Type", "text/html; charset=utf8")
					writer.Write([]byte(`
<form action="/ssrf/rebinding/in-post" method="post">
	<label for="name">姓名:</label>
	<input type="text" id="name" name="name" ><br><br>

	<label for="email">邮箱:</label>
	<input type="email" id="email" name="email" ><br><br>

	<label for="age">年龄:</label>
	<input type="number" id="age" name="age" min="2" max="120" ><br><br>

	
	<label for="url">URL</label>
	<input id='url' name="url"><br><br>

	<label for="timeout">TimeOut</label>
	<input type="number" id='timeout' name="timeout" min="10" max="120"><br><br>

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

				if u == "" || !check(u) {
					writer.Write([]byte("check url failed"))
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}

				c := utils.NewDefaultHTTPClient()
				timeoutSec, err := strconv.Atoi(values.Get("timeout"))
				if err != nil {
					writer.Write([]byte("Invalid timeout value"))
					return
				}

				c.Timeout = time.Duration(timeoutSec) * time.Second
				rsp, err := c.Get(u)
				if err != nil {
					writer.Write([]byte(err.Error()))
					writer.WriteHeader(http.StatusInternalServerError)
					return
				}
				rawResponse, err := utils.HttpDumpWithBody(rsp, true)
				if err != nil {
					writer.Write([]byte(err.Error()))
					return
				}
				writer.Write(rawResponse)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "",
			Path:         "/in-post-multipart",
			Title:        "SSRF POST 中 URL 参数(Multipart)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
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
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "",
			Path:         "/in-json-body",
			Title:        "SSRF JSON Body SSRF",
			Handler: func(writer http.ResponseWriter, request *http.Request) {

				return
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "",
			Path:         "/json-in-post-param",
			Title:        "SSRF POST参数是JSON（包含URL）的情况",
			Handler: func(writer http.ResponseWriter, request *http.Request) {

				return
			},
			RiskDetected: true,
		},

		{
			DefaultQuery: "destUrl=/redirect/main",
			Path:         "/redirect/basic",
			Title:        "完全开放重定向",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "destUrl")
				if strings.Contains(u, `redirect/basic`) {
					DefaultRender("<p>forbidden to "+strconv.Quote(u)+"</p>", writer, request)
					return
				}
				writer.Header().Set("Location", u)
				writer.WriteHeader(302)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "destUrl=/redirect/main",
			Path:         "/redirect/redirect-hell",
			Title:        "完全开放重定向(无限重定向)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "destUrl")
				writer.Header().Set("Location", u)
				writer.WriteHeader(302)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "redUrl=/redirect/main",
			Path:         "/redirect/js/basic",
			Title:        "完全开放重定向（JS location.href）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "redUrl")
				DefaultRender(`
	<h2>Open Redirect With JS</h2>
	<a href=`+strconv.Quote(u)+`>Click ME JUMP NOW (3s)</a>
	<script>
		setTimeout(function() {
	
	window.location.href = `+strconv.Quote(u)+`;
	
	}, 3000)
	</script>
	`, writer, request)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "redirect_to=/redirect/main",
			Path:         "/redirect/js/basic1",
			Title:        "完全开放重定向（JS location.replace）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "redirect_to")
				DefaultRender(`
	<h2>Open Redirect With JS</h2>
	<a href=`+strconv.Quote(u)+`>Click ME JUMP NOW (3s)</a>
	<script>
		setTimeout(function() {
	
	window.location.replace(`+strconv.Quote(u)+`);
	
	}, 3000)
	</script>
	`, writer, request)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "redirect=/redirect/main",
			Path:         "/redirect/js/basic2",
			Title:        "完全开放重定向（JS location.assign）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "redirect")
				DefaultRender(`
	<h2>Open Redirect With JS</h2>
	<a href=`+strconv.Quote(u)+`>Click ME JUMP NOW (3s)</a>
	<script>
		setTimeout(function() {
	
	window.location.assign(`+strconv.Quote(u)+`);
	
	}, 3000)
	</script>
	`, writer, request)
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "redirect=/redirect/main",
			Path:         "/redirect/meta/case1",
			Title:        "完全开放重定向（meta 延迟跳转）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "redirect")
				DefaultRenderEx(true, `<!DOCTYPE html>
	<html>
	 <head>
	   <title>Meta(5s) Refresh Example</title>
	   <meta http-equiv="refresh" content="5;url={{ .url }}">
	 </head>
	</html>
	`, writer, request, map[string]any{
					"url": strings.Trim(strconv.Quote(u), `"`),
				})
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "redirect=/redirect/main",
			Path:         "/redirect/meta/case2",
			Title:        "完全开放重定向（meta）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "redirect")
				DefaultRenderEx(true, `<!DOCTYPE html>
	<html>
	 <head>
	   <title>Meta Refresh Example</title>
	   <meta http-equiv="refresh" content="0;url={{ .url }}">
	 </head>
	</html>
	`, writer, request, map[string]any{
					"url": strings.Trim(strconv.Quote(u), `"`),
				})
			},
			RiskDetected: true,
		},
		{
			DefaultQuery: "redirect=/redirect/main",
			Path:         "/redirect/safe",
			Title:        "安全的重定向(只重定向path)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var u = LoadFromGetParams(request, "redirect")
				DefaultRenderEx(true, `<!DOCTYPE html>
	<html>
	 <head>
	   <title>Meta Refresh Example</title>
	   <meta http-equiv="refresh" content="0;url={{ .url }}">
	 </head>
	</html>
	`, writer, request, map[string]any{
					"url": strings.Trim(getPath(u), `"`),
				})
			},
			RiskDetected: true,
		},
	}

	for _, v := range ssrfRoutes {
		addRouteWithVulInfo(ssrfGroup, v)
	}
}

func getPath(u string) string {
	urlInfo, err := url.Parse(u)
	if err != nil {
		return ""
	}
	return urlInfo.Path
}

func checkIP(value string) bool {
	pattern := `^\d{1,3}(\.\d{1,3}){3}$`
	if matched, _ := regexp.MatchString(pattern, value); !matched {
		return false
	}

	ary := strings.Split(value, ".")
	for _, v := range ary {
		num, err := strconv.Atoi(v)
		if err != nil || num > 255 {
			return false
		}
	}
	return true
}

func dnsLookup(s string) string {
	if matched, _ := regexp.MatchString(`^[a-zA-Z0-9-.]*$`, s); matched {
		resp, err := http.Get(fmt.Sprintf("http://ip-api.com/json/%s", s))
		if err != nil {
			return "wrong"
		}
		defer resp.Body.Close()
		body, _ := ioutil.ReadAll(resp.Body)
		query := gjson.Get(string(body), "query").String()

		if checkIP(query) {
			return query
		}
		return "wrong"
	}
	return "wrong"
}

func check(s string) bool {
	if !strings.HasPrefix(s, "http://") {
		return false
	}

	blacklist := []string{"wrong", "127.", "local", "@", "flag"}

	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	host := u.Hostname()
	port := u.Port()

	if host == "" || port == "" {
		return false
	}
	dns := netx.LookupFirst(host,
		netx.WithDNSNoCache(true),
		netx.WithDNSDisableSystemResolver(true),
	)
	//dns := dnsLookup(host)

	dockerIP := "110.110.2.1"
	if isPrivate(dns) || dns != dockerIP || port == "80" || port == "8080" {
		return false
	}

	for _, blItem := range blacklist {
		r := regexp.MustCompile(blItem)

		if r.MatchString(s) { // Simplified from ip.fromLong logic for demonstration
			return false
		}
	}

	return true
}

func isPrivate(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	privateIPBlocks := []*net.IPNet{}
	for _, cidr := range []string{
		"127.0.0.0/8",    // IPv4 loopback
		"10.0.0.0/8",     // RFC1918
		"172.16.0.0/12",  // RFC1918
		"192.168.0.0/16", // RFC1918
		"169.254.0.0/16", // Link-local
		"::1/128",        // IPv6 loopback
		"fe80::/10",      // IPv6 link-local
		"fc00::/7",       // IPv6 unique local addr
	} {
		_, block, _ := net.ParseCIDR(cidr)
		privateIPBlocks = append(privateIPBlocks, block)
	}

	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
