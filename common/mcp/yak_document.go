package mcp

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

func init() {
	AddGlobalToolSet("yak_document",
		// library
		WithTool(mcp.NewTool("yakdoc_get_all_library_names",
			mcp.WithDescription("YakDocument: Get all standard library names"),
		), handleYakDocAllLibraryNames),
		WithTool(mcp.NewTool("yakdoc_library_details",
			mcp.WithDescription("YakDocument: Get the standard library details, include function names and variable names"),
			mcp.WithStringArray("library",
				mcp.Description("The library name, if empty, will return global function names and variable names"),
			),
		), handleYakDocLibraryDetails),

		// function
		WithTool(mcp.NewTool("yakdoc_function_details",
			mcp.WithDescription("YakDocument: Get the standard function details, include function name, description and params"),
			mcp.WithString("library",
				mcp.Description("The library name, empty means global function"),
			),
			mcp.WithStringArray("function",
				mcp.Description("The function name"),
				mcp.Required(),
			),
		), handleYakDocFunctionDetails),

		// variable
		WithTool(mcp.NewTool("yakdoc_variable_details",
			mcp.WithDescription("YakDocument: Get the standard variable details, include variable name and value"),
			mcp.WithString("library",
				mcp.Description("The library name, empty means global function"),
			),
			mcp.WithStringArray("variable",
				mcp.Description("The variable name"),
				mcp.Required(),
			),
		), handleYakDocVariableDetails),
	)
}

func handleYakDocAllLibraryNames(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		helper := doc.GetDefaultDocumentHelper()
		return NewCommonCallToolResult(lo.Keys(helper.Libs))
	}
}

func handleYakDocLibraryDetails(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		libNames := utils.MapGetStringSlice(args, "library")
		results := make(map[string]map[string]any, len(libNames))
		for _, name := range libNames {
			result := make(map[string]any, 2)
			result["functions"] = lo.Keys(doc.GetDocumentFunctions(name))
			result["variables"] = lo.Keys(doc.GetDocumentInstances(name))

			results[name] = result
		}
		return NewCommonCallToolResult(results)
	}
}

func handleYakDocFunctionDetails(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		libName := utils.MapGetString(args, "library")
		funcNames := utils.MapGetStringSlice(args, "function")
		if len(funcNames) == 0 {
			return nil, utils.Error("missing argument: function")
		}

		results := make(map[string]*yakdoc.FuncDecl, len(funcNames))
		for _, funcName := range funcNames {
			f := doc.GetDocumentFunction(libName, funcName)
			if f == nil {
				if libName == "" {
					libName = "GLOBAL"
				}
				return nil, utils.Errorf("function[%s.%s] not found", libName, funcName)
			}
			results[funcName] = f
		}
		return NewCommonCallToolResult(results)
	}
}

func handleYakDocVariableDetails(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		libName := utils.MapGetString(args, "library")
		varNames := utils.MapGetStringSlice(args, "variable")
		if len(varNames) == 0 {
			return nil, utils.Error("missing argument: variable")
		}

		results := make(map[string]*yakdoc.LibInstance, len(varNames))
		for _, varName := range varNames {
			i := doc.GetDocumentInstance(libName, varName)
			if i == nil {
				if libName == "" {
					libName = "GLOBAL"
				}
				return nil, utils.Errorf("variable[%s.%s] not found", libName, varName)
			}
			results[varName] = i
		}
		return NewCommonCallToolResult(results)
	}
}
