package main

import (
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"strings"

	. "github.com/dave/jennifer/jen"
	"github.com/samber/lo"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
)

type function struct {
	name     string
	params   []*functionParam
	document string
}

type functionParam struct {
	raw  ast.Expr
	name string
	typ  string
}

// 定义一个visitor来遍历AST节点
type visitor struct {
	ast.Visitor
	fset                     *token.FileSet
	funcs                    []*function
	disableGenerateFuncNames []string
}

func ASTGetTypeName(expr ast.Expr, fset *token.FileSet) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + ASTGetTypeName(t.X, fset)
	case *ast.SelectorExpr:
		return ASTGetTypeName(t.X, fset) + "." + t.Sel.Name
	case *ast.InterfaceType:
		return "any"
	case *ast.Ellipsis:
		return "..." + ASTGetTypeName(t.Elt, fset)
	case *ast.ArrayType:
		return "[]" + ASTGetTypeName(t.Elt, fset)
	case *ast.MapType:
		return "map[" + ASTGetTypeName(t.Key, fset) + "]" + ASTGetTypeName(t.Value, fset)
	default:
		var buf strings.Builder
		err := format.Node(&buf, fset, expr)
		if err != nil {
			return ""
		}
		return buf.String()
	}
}

func GetLowhttpHelperFuncs(filepath string, disableGenerateFuncNames []string) (*visitor, []*function) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filepath, nil, parser.ParseComments)
	if err != nil {
		return nil, nil
	}

	visitor := &visitor{
		fset:                     fset,
		disableGenerateFuncNames: disableGenerateFuncNames,
	}
	ast.Walk(visitor, node)
	return visitor, visitor.funcs
}

func specialExportHandle(s string) string {
	if strings.HasPrefix(s, "DNS") {
		return strings.ToLower(s[:3]) + s[3:]
	}

	switch s {
	case "SNI":
		return "sni"
	case "ETCHosts":
		return "etcHosts"
	case "SaveHTTPFlow":
		return "save"
	case "TimeoutFloat":
		return "timeout"
	case "ConnectTimeoutFloat":
		return "connectTimeout"
	case "RetryMaxWaitTimeFloat":
		return "retryMaxWaitTime"
	case "RetryWaitTimeFloat":
		return "retryWaitTime"
	case "BodyStreamReaderHandler":
		return "stream"
	case "ExportedDNSServers":
		return "dnsServer"
	case "RetryNotInStatusCodes":
		return "retryNotInStatusCode"
	case "RetryInStatusCodes":
		return "retryInStatusCode"
	}

	return strings.ToLower(s[:1]) + s[1:]
}

func handleASTParams(params *ast.FieldList, fset *token.FileSet) []*functionParam {
	functionParams := make([]*functionParam, 0, len(params.List))

	for _, field := range params.List {
		if len(field.Names) == 0 {
			functionParams = append(functionParams, &functionParam{
				name: "",
				typ:  ASTGetTypeName(field.Type, fset),
				raw:  field.Type,
			})
		} else {
			for _, name := range field.Names {
				functionParams = append(functionParams, &functionParam{
					name: name.Name,
					typ:  ASTGetTypeName(field.Type, fset),
					raw:  field.Type,
				})
			}
		}
	}

	return functionParams
}

func (v *visitor) FilterFunc(name string) bool {
	return !lo.Contains(v.disableGenerateFuncNames, name)
}

func (v *visitor) Visit(node ast.Node) ast.Visitor {
	switch n := node.(type) {
	case *ast.FuncDecl:
		if n.Name != nil && n.Name.Name != "" && strings.HasPrefix(n.Name.Name, "With") && v.FilterFunc(n.Name.Name) {
			document := ""
			if n.Doc != nil {
				document = strings.TrimSpace(n.Doc.Text())
			}
			v.funcs = append(v.funcs, &function{
				name:     n.Name.Name,
				params:   handleASTParams(n.Type.Params, v.fset),
				document: document,
			})
		}
	}
	return v
}

func main() {
	app := cli.NewApp()

	app.Commands = []cli.Command{}

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name: "p,pkgpath",
		},
		cli.StringFlag{
			Name: "f,filename",
		},
		cli.StringFlag{
			Name: "pp,parse_pkgpath",
		},
		cli.StringFlag{
			Name: "pf,parse_filename",
		},
		cli.StringFlag{
			Name: "t,config_type",
		},
		cli.StringFlag{
			Name: "opt,opt_type_name",
		},
		cli.StringFlag{
			Name: "field,config_field_name",
		},
		cli.StringFlag{
			Name: "e, export_map_name",
		},
		cli.BoolFlag{
			Name: "disable_host",
		},
	}
	var handleFuncParams func(p *functionParam, fset *token.FileSet) Code
	handleFuncParams = func(p *functionParam, fset *token.FileSet) Code {
		if _, ok := p.raw.(*ast.SelectorExpr); ok {
			iPkgPath, typ, _ := strings.Cut(p.typ, ".")
			return Id(p.name).Add(Qual(iPkgPath, typ))
		} else if f, ok := p.raw.(*ast.FuncType); ok {
			paramsCode := make([]Code, 0, len(f.Params.List))
			fParams := handleASTParams(f.Params, fset)
			for _, p := range fParams {
				paramsCode = append(paramsCode, handleFuncParams(p, fset))
			}

			resultsCap := 0
			var returnsCode []Code
			if f.Results != nil {
				resultsCap = len(f.Results.List)
				returnsCode = make([]Code, 0, resultsCap)
				if len(f.Results.List) > 0 {
					returns := handleASTParams(f.Results, fset)
					for _, r := range returns {
						returnsCode = append(returnsCode, handleFuncParams(r, fset))
					}
				}
			}

			return Id(p.name).Func().Params(paramsCode...).Params(returnsCode...)
		}

		return Id(p.name).Add(Id(p.typ))
	}
	handleArgs := func(p *functionParam) Code {
		if _, ok := p.raw.(*ast.Ellipsis); ok {
			return Id(p.name).Op("...")
		}
		return Id(p.name)
	}

	app.Action = func(c *cli.Context) error {
		saveFilename := c.String("filename")
		pkgPath := path.Join("github.com/yaklang/yaklang", c.String("pkgpath"))
		parsedPkgPath := path.Join("github.com/yaklang/yaklang", c.String("parse_pkgpath"))
		configType := c.String("config_type")
		optTypeName := c.String("opt_type_name")
		fieldName := c.String("config_field_name")
		exportMapName := c.String("export_map_name")
		disableGenerateFuncNames := []string{"WithNativeHTTPRequestInstance", "WithPacketBytes", "WithResponseCallback", "WithRequest", "WithDefaultBufferSize", "WithBeforeDoRequest", "WithTimeout", "WithConnectTimeout", "WithRetryMaxWaitTime", "WithRetryWaitTime", "WithPayloads", "WithDNSServers", "WithRetryNotInStatusCode", "WithRetryInStatusCode"}
		if c.Bool("disable_host") {
			disableGenerateFuncNames = append(disableGenerateFuncNames, "WithHost")
		}

		file := NewFilePath(pkgPath)

		visitor, funcs := GetLowhttpHelperFuncs(c.String("parse_filename"), disableGenerateFuncNames)

		file.HeaderComment("// Code generated by export-generator. DO NOT EDIT.")
		for _, f := range funcs {
			params := make([]Code, 0, len(f.params))
			args := make([]Code, 0, len(f.params))
			for _, p := range f.params {
				params = append(params, handleFuncParams(p, visitor.fset))
				args = append(args, handleArgs(p))
			}

			file.Comment(f.document)
			file.Func().
				Id("LowhttpOpt" + f.name).
				Params(params...).Params(Qual(pkgPath, optTypeName)).Block(
				Return(Func().Params(Id("config").Op("*").Qual(pkgPath, configType))).
					Block(
						Id("config").Dot(fieldName).Op("=").Append(Id("config").Dot(fieldName), Qual(parsedPkgPath, f.name).Call(args...)),
					),
			)
			file.Empty()
		}

		appendStmts := make([]Code, 0, len(funcs))
		for _, f := range funcs {
			exportName := specialExportHandle(strings.TrimPrefix(f.name, "With"))
			appendStmts = append(appendStmts, Id(exportMapName).Index(Lit(exportName)).Op("=").Id("LowhttpOpt"+f.name))
		}

		file.Func().Id("init").Params().Block(
			appendStmts...,
		)

		log.Infof("\n%#v", file)
		file.Save(saveFilename)

		return nil
	}

	err := app.Run(os.Args)
	if err != nil {
		panic(err)
	}
}
