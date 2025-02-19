package mcp

import (
	"context"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
)

func (s *MCPServer) registerYakDocumentTool() {
	// library
	s.server.AddTool(mcp.NewTool("yakdoc_get_all_library_names",
		mcp.WithDescription("YakDocument: Get all standard library names"),
	), s.handleYakDocAllLibraryNames)
	s.server.AddTool(mcp.NewTool("yakdoc_library_details",
		mcp.WithDescription("YakDocument: Get the standard library details, include function names and variable names"),
		mcp.WithString("library",
			mcp.Description("The library name, if empty, will return global function names and variable names"),
		),
	), s.handleYakDocLibraryDetails)

	// function
	s.server.AddTool(mcp.NewTool("yakdoc_function_details",
		mcp.WithDescription("YakDocument: Get the standard function details, include function name, description and params"),
		mcp.WithString("library",
			mcp.Description("The library name, empty means global function"),
		),
		mcp.WithString("function",
			mcp.Description("The function name"),
			mcp.Required(),
		),
	), s.handleYakDocFunctionDetails)

	// variable
	s.server.AddTool(mcp.NewTool("yakdoc_variable_details",
		mcp.WithDescription("YakDocument: Get the standard variable details, include variable name and value"),
		mcp.WithString("library",
			mcp.Description("The library name, empty means global function"),
		),
		mcp.WithString("variable",
			mcp.Description("The variable name"),
			mcp.Required(),
		),
	), s.handleYakDocVariableDetails)

}

func (s *MCPServer) handleYakDocAllLibraryNames(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	helper := doc.GetDefaultDocumentHelper()
	return NewCommonCallToolResult(lo.Keys(helper.Libs))
}

func (s *MCPServer) handleYakDocLibraryDetails(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	libName := utils.MapGetString(args, "library")
	results := map[string]any{
		"functions": lo.Keys(doc.GetDocumentFunctions(libName)),
		"variables": lo.Keys(doc.GetDocumentInstances(libName)),
	}
	return NewCommonCallToolResult(results)
}

func (s *MCPServer) handleYakDocFunctionDetails(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	libName := utils.MapGetString(args, "library")
	funcName := utils.MapGetString(args, "function")
	if funcName == "" {
		return nil, utils.Error("missing argument: function")
	}

	f := doc.GetDocumentFunction(libName, funcName)
	if f == nil {
		if libName == "" {
			libName = "GLOBAL"
		}
		return nil, utils.Errorf("function[%s.%s] not found", libName, funcName)
	}
	return NewCommonCallToolResult(f)
}

func (s *MCPServer) handleYakDocVariableDetails(
	ctx context.Context,
	request mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	args := request.Params.Arguments
	libName := utils.MapGetString(args, "library")
	varName := utils.MapGetString(args, "variable")
	if varName == "" {
		return nil, utils.Error("missing argument: variable")
	}

	i := doc.GetDocumentInstance(libName, varName)
	if i == nil {
		if libName == "" {
			libName = "GLOBAL"
		}
		return nil, utils.Errorf("variable[%s.%s] not found", libName, varName)
	}
	return NewCommonCallToolResult(i)
}
