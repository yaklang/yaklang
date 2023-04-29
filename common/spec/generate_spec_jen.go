package spec

import (
	"fmt"
	"github.com/dave/jennifer/jen"
	"gopkg.in/yaml.v2"
	"strings"
)

func JenGeneratePalmRpcByYaml(raw []byte) ([]byte, error) {
	var schema PalmRpcApiSchema
	err := yaml.Unmarshal(raw, &schema)
	if err != nil {
		return nil, err
	}

	return JenGeneratePalmRpcBySchema(&schema)
}

func stringForJenState(s string, state *jen.Statement) *jen.Statement {
	switch strings.ToLower(s) {
	case "string", "str":
		state.String()
	case "[]string", "string[]", "strs", "strings":
		state.Index().String()
	case "int":
		state.Int()
	case "int64":
		state.Int64()
	case "int[]", "[]int", "ints":
		state.Index().Int()
	case "int64[]", "[]int64", "int64s":
		state.Index().Int64()
	case "boolean", "bool":
		state.Bool()
	case "byte":
		state.Byte()
	case "bytes", "[]byte", "byte[]":
		state.Index().Byte()
	case "float", "float64":
		state.Float64()
	case "floats", "[]float", "float[]", "float64s", "[]float64", "float64[]":
		state.Index().Float64()
	default:
		state.Id(s)
	}
	return state
}

func generateServerFunc(schema *PalmRpcApiSchema, f *jen.File) *jen.File {
	var cases []jen.Code
	for _, m := range schema.Rpcs {
		methodName := fmt.Sprintf("%v_%v", schema.Name, m.Method)
		cases = append(cases, jen.Case(jen.Lit(methodName)).Block(
			jen.Var().Id("req").Id(fmt.Sprintf("%vRequest", methodName)),
			jen.Id("err").Op(":=").Qual("encoding/json", "Unmarshal").Parens(jen.List(
				jen.Id("delivery").Dot("Body"), jen.Add(jen.Op("&")).Id("req"),
			)),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.List(jen.Nil(), jen.Id("err"))),
			),
			jen.If(jen.Id("h").Dot(fmt.Sprintf("Do%v", methodName)).Op("==").Nil()).Block(
				jen.Return(jen.Nil(), jen.Qual("palm/common/utils", "Errorf").Call(jen.Lit("not implemented"))),
			),
			jen.Return(jen.Id("h").Dot(fmt.Sprintf("Do%v", methodName))).Call(
				jen.Id("ctx"), jen.Id("node"), jen.Op("&").Id("req"), jen.Id("broker"),
			),
		))
	}
	cases = append(cases, jen.Default().Block(jen.Return(jen.Nil(), jen.Qual("palm/common/utils", "Errorf").Call(
		jen.Lit("unknown func: %v"), jen.Id("f"),
	))))

	f.Func().Parens(jen.Id("h").Add(jen.Op("*").Id(fmt.Sprintf("%vServerHelper", schema.Name)))).Id("Do").Parens(jen.List(
		jen.Id("broker").Add(jen.Op("*")).Qual("palm/common/mq", "Broker"),
		jen.Id("ctx").Qual("context", "Context"),
		jen.Id("f"), jen.Id("node").String(),
		jen.Id("delivery").Add(jen.Op("*")).Qual("github.com/streadway/amqp", "Delivery"),
	)).Parens(jen.List(
		jen.Id("message").Interface(), jen.Id("e").Error(),
	)).Block(jen.Switch(jen.Id("f")).Block(
		cases...,
	))

	f.Func().Id(fmt.Sprintf("New%vServerHelper", schema.Name)).Params().Op("*").Id(
		fmt.Sprintf("%vServerHelper", schema.Name),
	).Block(
		jen.Return(jen.Op("&").Id(fmt.Sprintf("%vServerHelper", schema.Name)).Block()),
	)

	return f
}

func generateClientFunc(schema *PalmRpcApiSchema, f *jen.File) *jen.File {
	callRpcHandlerName := "callRpcHandler"
	f.Type().Id(callRpcHandlerName).Func().Params(
		jen.Id("ctx").Qual("context", "Context"),
		jen.Id("funcName"), jen.Id("node").String(),
		jen.Id("gen").Interface(),
	).Parens(jen.List(jen.Index().Byte(), jen.Error()))

	clientName := fmt.Sprintf("%vClientHelper", schema.Name)
	f.Type().Id(clientName).Struct(jen.Id("callRpc").Id(callRpcHandlerName))

	for _, r := range schema.Rpcs {
		methodName := fmt.Sprintf("%v_%v", schema.Name, r.Method)
		requestName := fmt.Sprintf("%vRequest", methodName)
		responseName := fmt.Sprintf("%vResponse", methodName)
		f.Func().Parens(jen.Id("h").Add(jen.Op("*")).Id(clientName)).Id(methodName).Params(
			jen.Id("ctx").Qual("context", "Context"),
			jen.Id("node").String(),
			jen.Id("req").Add(jen.Op("*")).Id(requestName),
		).Parens(jen.List(jen.Op("*").Id(responseName), jen.Error())).Block(
			jen.List(jen.Id("rsp"), jen.Id("err")).Op(":=").Id(
				"h").Dot("callRpc").Call(jen.Id("ctx"), jen.Lit(methodName), jen.Id("node"), jen.Id("req")),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Id("err")),
			),

			jen.Var().Id("rspIns").Id(responseName),
			jen.Id("err").Op("=").Qual("encoding/json", "Unmarshal").Call(jen.Id("rsp"), jen.Op("&").Id("rspIns")),
			jen.If(jen.Id("err").Op("!=").Nil()).Block(
				jen.Return(jen.Nil(), jen.Id("err")),
			),
			jen.Return(jen.Op("&").Id("rspIns"), jen.Nil()),
		)
	}

	f.Func().Id(fmt.Sprintf("Generate%v", clientName)).Params(
		jen.Id("callRpc").Id(callRpcHandlerName)).Op("*").Id(clientName).Block(
		jen.Return(jen.Op("&").Id(clientName).Values(
			jen.Dict{
				jen.Id("callRpc"): jen.Id("callRpc"),
			}),
		),
	)
	return f
}

func generateModels(schema *PalmRpcApiSchema, f *jen.File) *jen.File {
	for _, m := range schema.Models {
		var fields []jen.Code
		for _, rsp := range m.Fields {
			state := jen.Id(rsp.Name)
			state = stringForJenState(rsp.Type, state)
			fields = append(fields, state)
		}
		f.Type().Id(m.Name).Struct(fields...)
	}
	return f
}

func JenGeneratePalmRpcBySchema(schema *PalmRpcApiSchema) ([]byte, error) {
	f := jen.NewFile(schema.PackageName)
	//f.ImportNames(map[string]string{
	//	"context":                   "context",
	//	"encoding/json":             "json",
	//	"github.com/streadway/amqp": "amqp",
	//	"palm/common/mq":            "mq",
	//})

	// generate request and response
	var (
		methodLits []jen.Code
		methods    []string
	)
	for _, r := range schema.Rpcs {
		methodName := fmt.Sprintf("%v_%v", schema.Name, r.Method)
		methodLits = append(methodLits, jen.Lit(methodName))
		methods = append(methods, methodName)
		var requestArgs []jen.Code
		for _, req := range r.Request {
			state := jen.Id(req.Name)
			state = stringForJenState(req.Type, state)
			requestArgs = append(requestArgs, state)
		}

		var responseArgs []jen.Code
		for _, rsp := range r.Response {
			state := jen.Id(rsp.Name)
			state = stringForJenState(rsp.Type, state)
			responseArgs = append(responseArgs, state)
		}

		f.Type().Id(fmt.Sprintf("%vRequest", methodName)).Struct(
			requestArgs...,
		)
		f.Type().Id(fmt.Sprintf("%vResponse", methodName)).Struct(
			responseArgs...,
		)
	}

	// var lists
	f.Var().Parens(
		jen.Id("MethodList").Op("=").Index().String().Values(
			methodLits...,
		),
	)

	// 生成服务器端代码
	var codes []jen.Code
	for _, m := range methods {
		codes = append(codes,
			jen.Id(fmt.Sprintf("Do%v", m)).Func().Parens(
				jen.List(
					jen.Id("ctx").Qual("context", "Context"),
					jen.Id("node").String(),
					jen.Id("req").Add(jen.Op("*")).Id(m+"Request"),
					jen.Id("broker").Add(jen.Op("*")).Qual("palm/common/mq", "Broker"),
				),
			).Parens(jen.List(
				jen.Add(jen.Op("*")).Id(fmt.Sprintf("%vResponse", m)), jen.Error()),
			),
		)
	}
	f.Type().Id(schema.Name + "ServerHelper").Struct(codes...)
	f = generateServerFunc(schema, f)

	// 生成客户端代码
	f = generateClientFunc(schema, f)

	// 生成数据模型代码
	f = generateModels(schema, f)

	return []byte(f.GoString()), nil
}
