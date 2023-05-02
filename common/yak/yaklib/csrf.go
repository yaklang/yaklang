package yaklib

import (
	"html/template"
	"io/ioutil"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
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
var csrfXMLTemplate = template.Must(template.New("csrf-xml").Parse(strings.TrimSpace(`
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
	Type  string
	Name  string
	Value string
}
type _csrfTemplateConfig struct {
	Url         string
	Method      string
	ContentType string
	EncType     string
	Body        string
	Inputs      []_csrfKeyValues
}

type _csrfConfig struct {
	MultipartDefaultValue bool
}

func newDefaultCsrfConfig() *_csrfConfig {
	return &_csrfConfig{
		MultipartDefaultValue: false,
	}
}

type csrfConfig func(c *_csrfConfig)

func GenerateCSRFPoc(raw interface{}, opts ...csrfConfig) (string, error) {
	var (
		packet     []byte
		u          *url.URL
		req        *http.Request
		rawBody    []byte
		method     string
		key, value string
		values     []string
		vals       url.Values
		builder    = &strings.Builder{}

		config         *_csrfConfig
		pocConfig      *_pocConfig
		templateConfig *_csrfTemplateConfig

		isMultipart bool
		template    *template.Template = csrfFormTemplate
		err         error
	)

	switch raw.(type) {
	case string:
		packet = []byte(raw.(string))
	case []byte:
		packet = raw.([]byte)
	default:
		return "", utils.Errorf("poc.CSRFPOC cannot support: %s", reflect.TypeOf(raw))
	}

	config = newDefaultCsrfConfig()
	for _, opt := range opts {
		opt(config)
	}

	pocConfig = newDefaultPoCConfig()
	u, err = lowhttp.ExtractURLFromHTTPRequestRaw(packet, pocConfig.ForceHttps)
	if err != nil {
		return "", utils.Errorf("extract url failed: %s", err)
	}

	req, err = lowhttp.ParseBytesToHttpRequest(packet)
	if err != nil {
		return "", utils.Errorf("parse bytes to http request failed: %s", err)
	}

	method = strings.ToUpper(req.Method)
	templateConfig = &_csrfTemplateConfig{
		Url:     u.String(),
		Method:  method,
		EncType: "application/x-www-form-urlencoded",
		Inputs:  make([]_csrfKeyValues, 0),
	}

	for key, values = range req.Header {
		if strings.ToUpper(key) != "CONTENT-TYPE" {
			continue
		}
		for _, value = range values {
			if strings.Contains(strings.ToLower(value), "multipart/form-data;") {
				isMultipart = true
				templateConfig.ContentType = value
				templateConfig.EncType = "multipart/form-data"
				break
			}
		}
		break
	}

	if method == "POST" {
		if !isMultipart {
			rawBody, err = ioutil.ReadAll(req.Body)
			if err != nil {
				return "", utils.Errorf("parse request body failed: %s", err)
			}

			vals, err = url.ParseQuery(strings.TrimSpace(string(rawBody)))
			if err != nil {
				return "", err
			}
			for key, values = range vals {
				for _, value = range values {
					templateConfig.Inputs = append(templateConfig.Inputs, _csrfKeyValues{"hidden", key, value})
				}
			}
		} else {
			if config.MultipartDefaultValue {
				rawBody, err = ioutil.ReadAll(req.Body)
				if err != nil {
					return "", utils.Errorf("parse request body failed: %s", err)
				}
				template = csrfXMLTemplate
				templateConfig.Body = string(rawBody)
			} else {
				err = req.ParseMultipartForm(81920)
				if err != nil {
					return "", err
				}
				for key, values = range req.MultipartForm.Value {
					for _, value = range values {
						templateConfig.Inputs = append(templateConfig.Inputs, _csrfKeyValues{"hidden", key, value})
					}
				}
				for key = range req.MultipartForm.File {
					templateConfig.Inputs = append(templateConfig.Inputs, _csrfKeyValues{"file", key, ""})
				}
			}
		}

	} else if method == "GET" {
		for key, values := range u.Query() {
			for _, value = range values {
				templateConfig.Inputs = append(templateConfig.Inputs, _csrfKeyValues{"hidden", key, value})
			}
		}
	} else {
		return "", utils.Errorf("not support method: %s", method)
	}

	err = template.Execute(builder, templateConfig)
	if err != nil {
		return "", err
	}

	return builder.String(), nil
}

func _csrfOptWithMultipartDefaultValue(b bool) csrfConfig {
	return func(c *_csrfConfig) {
		c.MultipartDefaultValue = b
	}
}

var CSRFExports = map[string]interface{}{
	"Generate":              GenerateCSRFPoc,
	"multipartDefaultValue": _csrfOptWithMultipartDefaultValue,
}
