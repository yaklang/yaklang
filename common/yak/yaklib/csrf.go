package yaklib

import (
	"errors"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var csrfFormTemplate = template.Must(template.New("csrf-form").Parse(strings.TrimSpace(`
<html>
<body>
<form action="{{.Url}}" method="{{.Method}}" name="form1" {{if eq .Method "POST"}}enctype="{{.EncType}}"{{end}} >
{{- range .Inputs }}
<input type="{{.Type}}" name="{{.Name}}" value="{{.Value}}"/>
{{- end }}
<input type="submit" value="Submit request" />
</form>
<script>history.pushState('', '', '/');</script>
{{- if .AutoSubmit }}
<script>(function(){var f=document.forms['form1'];if(f)HTMLFormElement.prototype.submit.call(f);})();</script>
{{- end }}
</body>
</html>
`)))

var csrfJSTemplate = template.Must(template.New("csrf-js").Parse(strings.TrimSpace(`
<html>
<body>
<script>history.pushState('', '', '/')</script>
<script>
function submitRequest()
{
var xhr = new XMLHttpRequest();
xhr.open("{{.Method}}", "{{.Url}}", true);
xhr.setRequestHeader("Accept", "*\/*");
xhr.setRequestHeader("Content-Type", "{{.ContentType}}");
xhr.withCredentials = true;
var body = {{.Body}};
var aBody = new Uint8Array(body.length);
for (var i = 0; i < aBody.length; i++)
aBody[i] = body.charCodeAt(i); 
xhr.send(new Blob([aBody]));
}
</script>
<form action="#">
<input type="button" value="Submit request" onclick="submitRequest();" />
</form>
{{- if .AutoSubmit }}
<script>(function(){var f=document.forms['form1'];if(f)HTMLFormElement.prototype.submit.call(f);})();</script>
{{- end }}
</body>
</html>
`)))

type _csrfKeyValues struct {
	Type  template.HTMLAttr
	Name  template.HTMLAttr
	Value template.HTMLAttr
}
type _csrfTemplateConfig struct {
	Url         any
	Method      any
	ContentType any
	EncType     any
	Body        any
	AutoSubmit  bool
	Inputs      []_csrfKeyValues
}

type _csrfConfig struct {
	MultipartDefaultValue bool
	https                 bool
	autoSubmit            bool
}

func newDefaultCsrfConfig() *_csrfConfig {
	return &_csrfConfig{
		MultipartDefaultValue: false,
	}
}

type csrfConfig func(c *_csrfConfig)

// Generate 根据传入的原始请求报文生成跨站请求伪造(CSRF)类型的漏洞验证(POC) HTML 页面
// 在 yak 中通过 csrf.Generate 调用，自动从请求中提取 URL、方法与表单参数构造自动提交表单
// 参数:
//   - raw: 原始 HTTP 请求报文，可以是字符串或字节数组
//   - opts: 可选配置项，如 csrf.https、csrf.multipartDefaultValue、csrf.autoSubmit
//
// 返回值:
//   - 生成的 CSRF POC HTML 字符串
//   - 错误信息，成功时为 nil
//
// Example:
// ```
// raw = "POST /submit HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\nname=admin&age=2"
// poc, err = csrf.Generate(raw)
// assert err == nil, "generate should succeed"
// // 生成的 HTML 中应包含目标地址与表单字段
// assert str.Contains(poc, `action="http://example.com/submit"`), "poc should target the request url"
// assert str.Contains(poc, `name="name"`), "poc should carry the form field"
// ```
func GenerateCSRFPoc(raw interface{}, opts ...csrfConfig) (string, error) {
	var (
		packet     []byte
		u          *url.URL
		req        *http.Request
		rawBody    []byte
		method     string
		key, value string
		values     []string
		builder    = &strings.Builder{}

		config         *_csrfConfig
		templateConfig *_csrfTemplateConfig

		tmpl *template.Template = csrfFormTemplate
		err  error
	)

	switch raw.(type) {
	case string:
		packet = []byte(raw.(string))
	case []byte:
		packet = raw.([]byte)
	default:
		return "", utils.Errorf("raw type cannot support: %s", reflect.TypeOf(raw))
	}

	config = newDefaultCsrfConfig()
	for _, opt := range opts {
		opt(config)
	}

	u, err = lowhttp.ExtractURLFromHTTPRequestRaw(packet, config.https)
	if err != nil {
		return "", utils.Wrap(err, "extract url failed")
	}

	req, err = lowhttp.ParseBytesToHttpRequest(packet)
	if err != nil {
		return "", utils.Wrap(err, "parse request failed")
	}

	method = strings.ToUpper(req.Method)

	if config.MultipartDefaultValue {
		tmpl = csrfJSTemplate
		templateConfig = &_csrfTemplateConfig{
			Url:        template.JSStr(u.String()),
			Method:     template.JSStr(method),
			AutoSubmit: config.autoSubmit,
			Inputs:     make([]_csrfKeyValues, 0),
		}
	} else {
		templateConfig = &_csrfTemplateConfig{
			Url:        template.HTMLAttr(u.String()),
			Method:     template.HTMLAttr(method),
			EncType:    template.HTMLAttr("application/x-www-form-urlencoded"),
			AutoSubmit: config.autoSubmit,
			Inputs:     make([]_csrfKeyValues, 0),
		}
	}

	for key, values = range req.Header {
		if strings.ToUpper(key) != "CONTENT-TYPE" {
			continue
		}
		for _, value = range values {
			templateConfig.ContentType = value
			if strings.Contains(strings.ToLower(value), "multipart/form-data;") {
				if tmpl == csrfFormTemplate {
					templateConfig.EncType = template.HTMLAttr(value)
				} else if tmpl == csrfJSTemplate {
					templateConfig.ContentType = template.JSStr(value)
				}
				break
			} else if strings.Contains(strings.ToLower(value), "application/json") {
				break
			}
		}
		break
	}

	rawBody, err = ioutil.ReadAll(req.Body)
	if tmpl == csrfJSTemplate {
		templateConfig.Body = template.JS(strconv.Quote(string(rawBody)))
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", utils.Wrap(err, "read body failed")
	}
	if len(rawBody) > 0 {
		params, _, err := lowhttp.GetParamsFromBody(utils.InterfaceToString(templateConfig.ContentType), rawBody)
		if err != nil {
			return "", utils.Wrap(err, "get params from body failed")
		}
		for _, param := range params.Items {
			for _, value := range param.Values {
				templateConfig.Inputs = append(templateConfig.Inputs, _csrfKeyValues{"hidden", template.HTMLAttr(param.Key), template.HTMLAttr(value)})
			}
		}
	} else {
		vals := lowhttp.ParseQueryParams(strings.TrimSpace(string(u.RawQuery)))
		for _, item := range vals.Items {
			templateConfig.Inputs = append(templateConfig.Inputs, _csrfKeyValues{"hidden", template.HTMLAttr(item.Key), template.HTMLAttr(item.Value)})
		}
	}

	err = tmpl.Execute(builder, templateConfig)
	if err != nil {
		return "", err
	}

	return builder.String(), nil
}

// multipartDefaultValue 设置请求报文是否按 multipart/form-data 类型处理
// 当设置为 true 时，会改用基于 JavaScript(XHR) 提交的 POC 模板
// 在 yak 中通过 csrf.multipartDefaultValue 调用
// 参数:
//   - b: 是否启用 multipart/form-data 提交模式
//
// 返回值:
//   - 一个 csrf.Generate 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：生成基于 JS 提交的 CSRF POC
// raw = "POST / HTTP/1.1\r\nHost: example.com\r\nContent-Type: multipart/form-data; boundary=x\r\n\r\n"
// poc = csrf.Generate(raw, csrf.multipartDefaultValue(true))~
// ```
func CsrfOptWithMultipartDefaultValue(b bool) csrfConfig {
	return func(c *_csrfConfig) {
		c.MultipartDefaultValue = b
	}
}

// https 设置目标请求报文是否使用 HTTPS，从而决定生成 POC 中目标 URL 的协议
// 在 yak 中通过 csrf.https 调用
// 参数:
//   - b: 是否使用 HTTPS 协议
//
// 返回值:
//   - 一个 csrf.Generate 可接收的配置选项
//
// Example:
// ```
// raw = "POST /submit HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\nname=admin"
// // 启用 https 后，生成的目标地址应为 https 协议
// poc = csrf.Generate(raw, csrf.https(true))~
// assert str.Contains(poc, "https://example.com/submit"), "poc should use https scheme"
// ```
func CsrfOptWithHTTPS(b bool) csrfConfig {
	return func(c *_csrfConfig) {
		c.https = b
	}
}

// autoSubmit 设置是否在生成的 HTML 中注入自动提交脚本，使页面加载后自动提交表单
// 在 yak 中通过 csrf.autoSubmit 调用
// 参数:
//   - b: 是否注入自动提交脚本
//
// 返回值:
//   - 一个 csrf.Generate 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：生成加载后自动提交的 CSRF POC
// raw = "POST /submit HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/x-www-form-urlencoded\r\n\r\nname=admin"
// poc = csrf.Generate(raw, csrf.autoSubmit(true))~
// ```
func CsrfOptWithAutoSubmit(b bool) csrfConfig {
	return func(c *_csrfConfig) {
		c.autoSubmit = b
	}
}

var CSRFExports = map[string]interface{}{
	"Generate":              GenerateCSRFPoc,
	"multipartDefaultValue": CsrfOptWithMultipartDefaultValue,
	"https":                 CsrfOptWithHTTPS,
	"autoSubmit":            CsrfOptWithAutoSubmit,
}
