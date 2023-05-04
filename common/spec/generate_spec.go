package spec

import (
	"bytes"
	"fmt"
	"gopkg.in/yaml.v2"
	"html/template"
)

type PalmRpcApiSchema struct {
	PackageName string `json:"package_name" yaml:"package_name"`
	Name        string `json:"name" yaml:"name"`

	Rpcs   []*RpcApi      `json:"rpcs" yaml:"rpcs"`
	Models []*ModelSchema `json:"models"`
}

type ModelSchema struct {
	Name   string       `json:"name" yaml:"name"`
	Fields []*ArgSchema `json:"fields" yaml:"fields"`
}

type RpcApi struct {
	Method   string       `json:"method" yaml:"method"`
	Request  []*ArgSchema `json:"request" yaml:"request"`
	Response []*ArgSchema `json:"response" yaml:"response"`
}

type ArgSchema struct {
	Name string `json:"name" yaml:"name"`
	Type string `json:"type" yaml:"type"`
}

var (
	specRpcAPITempRaw = `package {{ .PackageName }}

import (
	"context"
	"encoding/json"
	"github.com/streadway/amqp"
	"palm/common/mq"
)

{{ $root := . }}{{range $val := .Rpcs }}
type {{ $root.Name }}_{{ $val.Method }}Request struct { {{range $arg := .Request }}
    {{.Name}}	{{.Type}}{{end}}
}

type {{ $root.Name }}_{{ $val.Method }}Response struct { {{range $arg := .Response }}
    {{.Name}}	{{.Type}}{{end}}
}{{end}}

var (
    MethodList = []string{ {{range $val := .Rpcs}}
        "{{ $root.Name }}_{{ $val.Method }}{{end}}",
    }
)



type {{.Name}}ServerHelper struct {
	{{range $val := .Rpcs}}
    do{{ $root.Name }}_{{ $val.Method }} func(ctx context.Context, node string, req *{{ $root.Name }}_{{ $val.Method }}Request, broker *mq.Broker) ({{ $root.Name }}_{{ $val.Method }}Response, error){{end}}
}

func (h *{{.Name}}ServerHelper) Do(broker *mq.Broker, ctx context.Context, f, node string, delivery *amqp.Delivery) (message interface{}, e error) {
	switch f {
{{range $val := .Rpcs}}
	case "{{ $root.Name }}_{{ $val.Method }}":
		var req {{ $root.Name }}_{{ $val.Method }}Request
		err := json.Unmarshal(delivery.Body, &req)
		if err != nil {
			return nil, err
		}
		if h.do{{ $root.Name }}_{{ $val.Method }} == nil {
			return nil, utils.Errorf("not implemented")
		}
		return h.do{{ $root.Name }}_{{ $val.Method }}(ctx, node, &req, broker)
{{end}}
    default:
		return nil, utils.Errorf("unknown: func: %v", f)
	}
}

func New{{.Name}}ServerHelper() *{{.Name}}ServerHelper {
	return &{{.Name}}ServerHelper{}
}



//
type callRpcHandler func(ctx context.Context, funcName, node string, req interface{}) ([]byte, error)
type {{.Name}}ClientHelper struct {
	callRpc callRpcHandler
}
{{range $val := .Rpcs}}
func (h *{{$root.Name}}ClientHelper) {{ $root.Name }}_{{ $val.Method }}(ctx context.Context, node string, req *{{ $root.Name }}_{{ $val.Method }}Request) ({{ $root.Name }}_{{ $val.Method }}Response, error){
	rsp, err := h.callRpc(ctx, {{ $root.Name }}_{{ $val.Method }}, node, req)
	if err != nil {
		return nil, err
	}
	
	var rspIns {{ $root.Name }}_{{ $val.Method }}Response
	err := json.Unmarshal(rsp, &rspIns)
	if err != nil {
		return nil, err
	}
	return &rspIns, nil
}{{end}}

func Generate{{.Name}}ClientHelper(callRpc callRpcHandler) *{{.Name}}ClientHelper {
	return &{{.Name}}ClientHelper{callRpc: callRpc}
}
`
	specRpcAPITemp *template.Template
)

func init() {
	t := template.New("palm rpc api spec")
	var err error
	specRpcAPITemp, err = t.Parse(specRpcAPITempRaw)
	if err != nil {
		panic(fmt.Sprintf("parse palm rpc api spec failed: %s", err))
	}
}

func GeneratePalmRpcByYaml(raw []byte) ([]byte, error) {
	var schema PalmRpcApiSchema
	err := yaml.Unmarshal(raw, &schema)
	if err != nil {
		return nil, err
	}

	return GeneratePalmRpcBySchema(&schema)
}

func GeneratePalmRpcBySchema(schema *PalmRpcApiSchema) ([]byte, error) {
	var buf bytes.Buffer
	err := specRpcAPITemp.Execute(&buf, schema)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
