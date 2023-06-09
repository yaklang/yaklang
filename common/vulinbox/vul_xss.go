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
	router.HandleFunc("/xss/safe", func(writer http.ResponseWriter, request *http.Request) {
		var name = request.URL.Query().Get("name")
		safeName := template.HTMLEscapeString(name)
		writer.Write([]byte(fmt.Sprintf(`<html>
Hello %v
</html>`, safeName)))
		writer.Header().Set("Content-Type", "text/html")
		writer.WriteHeader(200)
		return
	})
	router.HandleFunc("/xss/echo", func(writer http.ResponseWriter, request *http.Request) {
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
	router.HandleFunc("/xss/safe/nosniff/jpeg", func(writer http.ResponseWriter, request *http.Request) {
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
	})
	router.HandleFunc("/xss/attr/onclick", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/attr/alt", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/attr/alt/json", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/attr/alt/b64/json", func(writer http.ResponseWriter, request *http.Request) {
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

	})

	router.HandleFunc("/xss/attr/src", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/attr/href", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/attr/onclick2", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/attr/script", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/js/in-str", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/js/in-str2", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/js/in-str-temp", func(writer http.ResponseWriter, request *http.Request) {
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

	})
	router.HandleFunc("/xss/cookie/name", func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := utils.HttpDumpWithBody(request, true)
		xCname := lowhttp.GetHTTPPacketCookieFirst(raw, "xCname")
		if xCname == "" && lowhttp.GetHTTPRequestQueryParam(raw, "skip") != "1" {
			http.SetCookie(writer, &http.Cookie{
				Name:  "xCname",
				Value: "UserAdmin",
			})
			writer.Header().Set("Location", "/xss/cookie/name?skip=1")
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
	})
	router.HandleFunc("/xss/cookie/b64/name", func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := utils.HttpDumpWithBody(request, true)
		xCname := lowhttp.GetHTTPPacketCookieFirst(raw, "xCnameB64")
		xCnameRaw, _ := codec.DecodeBase64(xCname)
		xCname = string(xCnameRaw)
		if xCname == "" && lowhttp.GetHTTPRequestQueryParam(raw, "skip") != "1" {
			http.SetCookie(writer, &http.Cookie{
				Name:  "xCnameB64",
				Value: codec.EncodeBase64("OrdinaryUser"),
			})
			writer.Header().Set("Location", "/xss/cookie/b64/name?skip=1")
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
	})
	router.HandleFunc("/xss/cookie/b64/json/name", func(writer http.ResponseWriter, request *http.Request) {
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
			writer.Header().Set("Location", "/xss/cookie/b64/json/name?skip=1")
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
	})

}
