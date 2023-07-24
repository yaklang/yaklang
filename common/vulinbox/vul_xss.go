package vulinbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"html/template"
	"net/http"
	"net/http/httputil"
	"regexp"
	textTemp "text/template"
)

func unsafeTemplate(html string, params map[string]any) ([]byte, error) {
	temp, err := textTemp.New("TEST").Parse(html)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	err = temp.Execute(&buf, params)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unsafeTemplateRender(writer http.ResponseWriter, req *http.Request, html string, params map[string]any) {
	data, err := unsafeTemplate(html, params)
	if err != nil {
		writer.WriteHeader(500)
		writer.Header().Set("Content-Type", "text/plain; charset=UTF8")
		writer.Write([]byte(fmt.Sprintf("Request ERROR: %v\n\n", err)))
		raw, err := httputil.DumpRequest(req, true)
		if err != nil {
			writer.Write([]byte(fmt.Sprintf("DUMP REQUEST ERROR: %v\n\n", err)))
			return
		}
		writer.Write([]byte("TRACE REQUEST: \n" + string(raw)))
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=UTF8")
	writer.WriteHeader(200)
	writer.Write(data)
}

func (s *VulinServer) registerXSS() {
	var router = s.router

	xssGroup := router.PathPrefix("/xss").Name("XSS 多场景").Subrouter()
	xssRoutes := []*VulInfo{
		{
			DefaultQuery: "name=admin",
			Path:         "/safe",
			Title:        "安全实体转义",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var name = request.URL.Query().Get("name")
				safeName := template.HTMLEscapeString(name)
				writer.Write([]byte(fmt.Sprintf(`<html>
Hello %v
</html>`, safeName)))
				writer.Header().Set("Content-Type", "text/html")
				writer.WriteHeader(200)
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/echo",
			Title:        "直接拼接导致XSS注入",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				var name = request.URL.Query().Get("name")
				writer.Header().Set("Content-Type", "text/html")
				writer.Write([]byte(fmt.Sprintf(`
<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello %v
</div>
</body>
</html>
`, name)))
				writer.WriteHeader(200)
				return
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/replace/nocase",
			Title:        "不安全的过滤导致XSS",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
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
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/js/in-str",
			Title:        "XSS: 存在于 JS 代码中(字符串中)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Here are photo for U! <br>
	<script>console.info("Hello" + '{{ .name }}')</script>
</div>
</body>
</html>`, map[string]any{
					"name": request.URL.Query().Get("name"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/js/in-str2",
			Title:        "XSS: 存在于 JS 代码中(字符串中2)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Here are photo for U! <br>
	<script>const name = "{{ .name }}";

console.info("Hello" + `+"`${name}`"+`);</script>
</div>
</body>
</html>`, map[string]any{
					"name": request.URL.Query().Get("name"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=admin",
			Path:         "/js/in-str-temp",
			Title:        "XSS: 存在于 JS 代码中(字符串模版中)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Here are photo for U! <br>
	<script>const name = "Admin";

console.info("Hello" + `+"`{{ .name }}: ${name}`"+`);</script>
</div>
</body>
</html>`, map[string]any{
					"name": request.URL.Query().Get("name"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "",
			Path:         "/safe/nosniff/jpeg",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("X-Content-Type-Options", "nosniff")
				writer.Header().Set("Content-Type", "image/jpeg")
				writer.Write([]byte(
					fmt.Sprintf(`<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello %v
</div>
</body>
</html>`, request.URL.Query().Get("name"))))
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "code=2-1",
			Path:         "/attr/onclick",
			Title:        "输出存在于HTML节点on...属性中",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br>
	<img style='width: 100px' src="/static/logo.png" onclick='{{ .code }}'/>
</div>
</body>
</html>`, map[string]any{
					"code": request.URL.Query().Get("code"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "value=visitor-name",
			Path:         "/attr/alt",
			Title:        "输出存在于HTML节点属性中，但是不再on属性中(IMG ALT)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				// %27onmousemove=%27javascript:alert(1)
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br>
	<img style='width: 100px' alt='{{.value}}' src="/static/logo.png" onclick='javascript:alert("Welcome CLICK ME!")'/>
</div>
</body>
</html>`, map[string]any{
					"value": request.URL.Query().Get("value"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "json={\"value\":\"value=visitor-name\"}",
			Path:         "/attr/alt/json",
			Title:        "进阶1：输出存在于HTML节点属性中，但是不再on属性中(IMG ALT)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				// %27onmousemove=%27javascript:alert(1)
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br>
	<img style='width: 100px' alt='{{.value}}' src="/static/logo.png" onclick='javascript:alert("Welcome CLICK ME!")'/>
</div>
</body>
</html>`, map[string]any{
					"value": LoadFromGetJSONParam(request, "json", "value"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "b64json=eyJ2YWx1ZSI6InZhbHVlPXZpc2l0b3ItbmFtZSJ9",
			Path:         "/attr/alt/b64/json",
			Title:        "进阶2：输出存在于HTML节点属性中，但是不再on属性中(IMG ALT)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				// %27onmousemove=%27javascript:alert(1)
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br>
	<img style='width: 100px' alt='{{.value}}' src="/static/logo.png" onclick='javascript:alert("Welcome CLICK ME!")'/>
</div>
</body>
</html>`, map[string]any{
					"value": LoadFromGetBase64JSONParam(request, "b64json", "value"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "src=/static/logo.png",
			Path:         "/attr/src",
			Title:        "输出存在于HTML节点属性中，但是不再on属性中(IMG SRC)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				// %27onmousemove=%27javascript:alert(1)
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br> <br>
	<img style='width: 100px' alt='value' src="{{ .value }}" onclick='javascript:alert("Welcome CLICK ME!")'/>
</div>
</body>
</html>`, map[string]any{
					"value": request.URL.Query().Get("src"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "href=/static/logo.png",
			Path:         "/attr/href",
			Title:        "输出存在于HTML节点属性中，但是不再on属性中(HREF)",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				// %27onmousemove=%27javascript:alert(1)
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br> <br>
	<a href='{{ .value }}' target='_blank'>Click ME to load IMG LOGO!</a>
	<img style='width: 100px' alt='value' src="/static/logo.png" onclick='javascript:alert("Welcome CLICK ME!")'/>
</div>
</body>
</html>`, map[string]any{
					"value": request.URL.Query().Get("href"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "code=2-1",
			Path:         "/attr/onclick2",
			Title:        "输出存在于HTML节点on...属性中的部分代码属性",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello Visitor!
	<br>
	Here are photo for U! <br>
	<img style='width: 100px' src="/static/logo.png" onclick='javascript:alert({{ .code }})'/>
</div>
</body>
</html>`, map[string]any{
					"code": request.URL.Query().Get("code"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "name=OrdinaryVisitor",
			Path:         "/attr/script",
			Title:        "script标签的某些属性中",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Hello <p id='name'></p>
	<br>
	Here are photo for U! <br>
	<script>document.getElementById('name').innerHTML = '{{ .name }}'</script>
</div>
</body>
</html>`, map[string]any{
					"name": request.URL.Query().Get("name"),
				})
				writer.Header().Set("Content-Type", "text/html")

			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "",
			Path:         "/cookie/name",
			Title:        "Cookie 中的 XSS",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, _ := utils.HttpDumpWithBody(request, true)
				xCname := lowhttp.GetHTTPPacketCookieFirst(raw, "xCname")
				if xCname == "" && lowhttp.GetHTTPRequestQueryParam(raw, "skip") != "1" {
					http.SetCookie(writer, &http.Cookie{
						Name:  "xCname",
						Value: "UserAdmin",
					})
					writer.Header().Set("Location", "/cookie/name?skip=1")
					writer.WriteHeader(302)
					return
				}

				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Here are photo for U! <br>
	<img style='width: 100px' src="/static/logo.png" onclick='{{ .code }}'/>	
	<script>const name = "Admin";

console.info("Hello" + `+"`{{ .name }}: ${name}`"+`);</script>
</div>
</body>
</html>`, map[string]any{
					"name": xCname,
				})
				writer.Header().Set("Content-Type", "text/html")
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "",
			Path:         "/cookie/b64/name",
			Title:        "Cookie 中的 XSS（Base64）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, _ := utils.HttpDumpWithBody(request, true)
				xCname := lowhttp.GetHTTPPacketCookieFirst(raw, "xCnameB64")
				xCnameRaw, _ := codec.DecodeBase64(xCname)
				xCname = string(xCnameRaw)
				if xCname == "" && lowhttp.GetHTTPRequestQueryParam(raw, "skip") != "1" {
					http.SetCookie(writer, &http.Cookie{
						Name:  "xCnameB64",
						Value: codec.EncodeBase64("OrdinaryUser"),
					})
					writer.Header().Set("Location", "/cookie/b64/name?skip=1")
					writer.WriteHeader(302)
					return
				}

				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Here are photo for U! <br>
	<img style='width: 100px' src="/static/logo.png" onclick='{{ .name }}'/>	
	<script>const name = "Admin";

console.info("Hello" + `+"`{{ .name }}: ${name}`"+`);</script>
</div>
</body>
</html>`, map[string]any{
					"name": xCname,
				})
				writer.Header().Set("Content-Type", "text/html")
			},
			Detected:      true,
			ExpectedValue: "1",
		},
		{
			DefaultQuery: "",
			Path:         "/cookie/b64/json/name",
			Title:        "Cookie 中的 XSS（Base64-JSON）",
			Handler: func(writer http.ResponseWriter, request *http.Request) {
				raw, _ := utils.HttpDumpWithBody(request, true)
				xCname := lowhttp.GetHTTPPacketCookieFirst(raw, "xCnameB64J")
				xCnameRaw, _ := codec.DecodeBase64(xCname)
				xCname = string(xCnameRaw)
				var MC = make(map[string]any)
				_ = json.Unmarshal(xCnameRaw, &MC)
				xCname = utils.MapGetString(MC, "name")
				if xCname == "" && lowhttp.GetHTTPRequestQueryParam(raw, "skip") != "1" {
					http.SetCookie(writer, &http.Cookie{
						Name:  "xCnameB64J",
						Value: codec.EncodeBase64(utils.Jsonify(map[string]any{"name": "xCnameB64J-OrdinaryUser"})),
					})
					writer.Header().Set("Location", "/cookie/b64/json/name?skip=1")
					writer.WriteHeader(302)
					return
				}

				unsafeTemplateRender(writer, request, `<!doctype html>
<html>
<head>
    <title>Example DEMO</title>

    <meta charset="utf-8" />
    <meta http-equiv="Content-type" content="text/html; charset=utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <style type="text/css">
    body {
        background-color: #f0f0f2;
        margin: 0;
        padding: 0;
        font-family: -apple-system, system-ui, BlinkMacSystemFont, "Segoe UI", "Open Sans", "Helvetica Neue", Helvetica, Arial, sans-serif;
        
    }
    div {
        width: 600px;
        margin: 5em auto;
        padding: 2em;
        background-color: #fdfdff;
        border-radius: 0.5em;
        box-shadow: 2px 3px 7px 2px rgba(0,0,0,0.02);
    }
    </style>    
</head>

<body>
<div>
	Here are photo for U! <br>
	<img style='width: 100px' src="/static/logo.png" onclick='{{ .name }}'/>	
	<script>const name = "Admin";

console.info("Hello" + `+"`{{ .name }}: ${name}`"+`);</script>
</div>
</body>
</html>`, map[string]any{
					"name": xCname,
				})
				writer.Header().Set("Content-Type", "text/html")
			},
			Detected:      true,
			ExpectedValue: "1",
		},
	}

	for _, v := range xssRoutes {
		addRouteWithVulInfo(xssGroup, v)
	}

}
