package yakgrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// AddMCPServer 添加MCP服务器
func (s *Server) AddMCPServer(ctx context.Context, req *ypb.AddMCPServerRequest) (*ypb.GeneralResponse, error) {
	if req.GetName() == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "服务器名称不能为空",
		}, nil
	}

	if req.GetType() == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "服务器类型不能为空",
		}, nil
	}

	// 验证服务器类型
	if req.GetType() != "stdio" && req.GetType() != "sse" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "服务器类型必须是 stdio 或 sse",
		}, nil
	}

	// 根据类型验证必要字段
	if req.GetType() == "stdio" && req.GetCommand() == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "stdio 类型服务器必须提供启动命令",
		}, nil
	}

	if req.GetType() == "sse" && req.GetURL() == "" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "sse 类型服务器必须提供 URL",
		}, nil
	}

	envs := make(schema.MapStringAny)
	for _, env := range req.GetEnvs() {
		envs[env.GetKey()] = env.GetValue()
	}
	server := &schema.MCPServer{
		Name:    req.GetName(),
		Type:    req.GetType(),
		URL:     req.GetURL(),
		Command: req.GetCommand(),
		Enable:  req.GetEnable(),
		Envs:    envs,
	}

	err := yakit.CreateMCPServer(s.GetProfileDatabase(), server)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: err.Error(),
		}, nil
	}

	return &ypb.GeneralResponse{
		Ok:     true,
		Reason: "MCP服务器添加成功",
	}, nil
}

// DeleteMCPServer 删除MCP服务器
func (s *Server) DeleteMCPServer(ctx context.Context, req *ypb.DeleteMCPServerRequest) (*ypb.GeneralResponse, error) {
	if req.GetID() <= 0 {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "服务器ID无效",
		}, nil
	}

	err := yakit.DeleteMCPServer(s.GetProfileDatabase(), req.GetID())
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: err.Error(),
		}, nil
	}

	return &ypb.GeneralResponse{
		Ok:     true,
		Reason: "MCP服务器删除成功",
	}, nil
}

// UpdateMCPServer 更新MCP服务器
func (s *Server) UpdateMCPServer(ctx context.Context, req *ypb.UpdateMCPServerRequest) (*ypb.GeneralResponse, error) {
	if req.GetID() <= 0 {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "服务器ID无效",
		}, nil
	}

	// 验证服务器类型
	if req.GetType() != "" && req.GetType() != "stdio" && req.GetType() != "sse" {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: "服务器类型必须是 stdio 或 sse",
		}, nil
	}

	envs := make(schema.MapStringAny)
	for _, env := range req.GetEnvs() {
		envs[env.GetKey()] = env.GetValue()
	}
	server := &schema.MCPServer{
		Name:    req.GetName(),
		Type:    req.GetType(),
		URL:     req.GetURL(),
		Command: req.GetCommand(),
		Enable:  req.GetEnable(),
		Envs:    envs,
	}

	err := yakit.UpdateMCPServer(s.GetProfileDatabase(), req.GetID(), server)
	if err != nil {
		return &ypb.GeneralResponse{
			Ok:     false,
			Reason: err.Error(),
		}, nil
	}

	return &ypb.GeneralResponse{
		Ok:     true,
		Reason: "MCP服务器更新成功",
	}, nil
}

// GetAllMCPServers 获取所有MCP服务器（支持分页和搜索）
func (s *Server) GetAllMCPServers(ctx context.Context, req *ypb.GetAllMCPServersRequest) (*ypb.GetAllMCPServersResponse, error) {
	paginator, servers, err := yakit.QueryMCPServers(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}

	var mcpServers []*ypb.MCPServer
	for _, server := range servers {
		mcpServer := server.ToGRPC()

		// 如果需要显示工具列表且服务器已启用，尝试连接服务器获取工具信息
		if req.GetIsShowToolList() && server.Enable {
			tools, err := s.getMCPServerTools(ctx, server)
			if err != nil {
				mcpServer.ErrorMsg = err.Error()
			} else {
				mcpServer.Tools = tools
			}
		}

		mcpServers = append(mcpServers, mcpServer)
	}

	return &ypb.GetAllMCPServersResponse{
		MCPServers: mcpServers,
		Pagination: &ypb.Paging{
			Page:  int64(paginator.Page),
			Limit: int64(paginator.Limit),
		},
		Total: int64(paginator.TotalRecord),
	}, nil
}

// getMCPServerTools 获取MCP服务器的工具列表
func (s *Server) getMCPServerTools(ctx context.Context, server *schema.MCPServer) ([]*ypb.MCPServerTool, error) {
	var mcpClient client.MCPClient
	var err error

	// 根据服务器类型创建客户端
	switch server.Type {
	case "stdio":
		// 解析命令和参数 - 简单的空格分割
		commandParts := utils.PrettifyListFromStringSplited(server.Command, " ")
		if len(commandParts) == 0 {
			return nil, utils.Errorf("invalid command: %s", server.Command)
		}
		command := commandParts[0]
		args := commandParts[1:]
		envs := make([]string, 0, len(server.Envs))
		for k, v := range server.Envs {
			envs = append(envs, fmt.Sprintf("%s=%v", k, v))
		}
		mcpClient, err = client.NewStdioMCPClient(command, envs, args...)
		if err != nil {
			return nil, err
		}
	case "sse":
		sseMcpClient, err := client.NewSSEMCPClient(server.URL)
		if err != nil {
			return nil, utils.Errorf("create sse mcp client failed: %s", err)
		}
		err = sseMcpClient.Start(ctx)
		if err != nil {
			return nil, utils.Errorf("start sse mcp client failed: %s", err)
		}
		mcpClient = sseMcpClient
	default:
		return nil, utils.Errorf("unsupported server type: %s", server.Type)
	}

	defer mcpClient.Close()

	// 初始化连接
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "yaklang-mcp-client",
		Version: "1.0.0",
	}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		return nil, err
	}

	// 获取工具列表
	toolsRequest := mcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		return nil, err
	}

	// 转换为gRPC格式
	var tools []*ypb.MCPServerTool
	for _, tool := range toolsResult.Tools {
		params, err := parseMCPToolInputSchema(&tool.InputSchema)
		if err != nil {
			return nil, utils.Errorf("parse mcp tool input schema failed: %s", err)
		}
		mcpTool := &ypb.MCPServerTool{
			Name:        tool.Name,
			Description: tool.Description,
			Params:      params,
		}
		tools = append(tools, mcpTool)
	}

	return tools, nil
}

func parseMCPToolInputSchema(inputSchema *mcp.ToolInputSchema) ([]*ypb.MCPServerToolParamInfo, error) {
	if inputSchema == nil {
		return []*ypb.MCPServerToolParamInfo{}, nil
	}

	var params []*ypb.MCPServerToolParamInfo

	// 如果没有 Properties，返回空列表
	if inputSchema.Properties == nil || inputSchema.Properties.Len() == 0 {
		return params, nil
	}

	// 创建必需字段的映射，便于快速查找
	requiredMap := make(map[string]bool)
	for _, req := range inputSchema.Required {
		requiredMap[req] = true
	}

	// 遍历所有属性
	inputSchema.Properties.ForEach(func(paramName string, paramValue any) bool {
		param := &ypb.MCPServerToolParamInfo{
			Name: paramName,
		}

		// 检查是否为必需参数
		param.Required = requiredMap[paramName]

		// 解析参数详细信息
		if paramMap, ok := paramValue.(map[string]interface{}); ok {
			// 参数类型
			if paramType, exists := paramMap["type"]; exists {
				param.Type = utils.InterfaceToString(paramType)
			}

			// 参数描述
			if description, exists := paramMap["description"]; exists {
				param.Description = utils.InterfaceToString(description)
			}

			// 默认值
			if defaultVal, exists := paramMap["default"]; exists {
				param.Default = utils.InterfaceToString(defaultVal)
			}

			// 处理枚举值
			if enum, exists := paramMap["enum"]; exists {
				if enumArray, ok := enum.([]interface{}); ok {
					enumStrings := make([]string, len(enumArray))
					for i, v := range enumArray {
						enumStrings[i] = utils.InterfaceToString(v)
					}
					// 将枚举值添加到描述中
					if param.Description != "" {
						param.Description += " (可选值: " + strings.Join(enumStrings, ", ") + ")"
					} else {
						param.Description = "可选值: " + strings.Join(enumStrings, ", ")
					}
				}
			}

			// 处理数组类型的额外信息
			if param.Type == "array" {
				if items, exists := paramMap["items"]; exists {
					if itemsMap, ok := items.(map[string]interface{}); ok {
						if itemType, exists := itemsMap["type"]; exists {
							param.Type = "array[" + utils.InterfaceToString(itemType) + "]"
						}
					}
				}
			}

			// 处理对象类型的额外信息
			if param.Type == "object" {
				if properties, exists := paramMap["properties"]; exists {
					if propsMap, ok := properties.(map[string]interface{}); ok {
						propNames := make([]string, 0, len(propsMap))
						for propName := range propsMap {
							propNames = append(propNames, propName)
						}
						if len(propNames) > 0 {
							if param.Description != "" {
								param.Description += " (属性: " + strings.Join(propNames, ", ") + ")"
							} else {
								param.Description = "属性: " + strings.Join(propNames, ", ")
							}
						}
					}
				}
			}

			// 处理数值类型的范围限制
			if param.Type == "number" || param.Type == "integer" {
				var constraints []string
				if minimum, exists := paramMap["minimum"]; exists {
					constraints = append(constraints, "最小值: "+utils.InterfaceToString(minimum))
				}
				if maximum, exists := paramMap["maximum"]; exists {
					constraints = append(constraints, "最大值: "+utils.InterfaceToString(maximum))
				}
				if len(constraints) > 0 {
					if param.Description != "" {
						param.Description += " (" + strings.Join(constraints, ", ") + ")"
					} else {
						param.Description = strings.Join(constraints, ", ")
					}
				}
			}

			// 处理字符串类型的长度限制
			if param.Type == "string" {
				var constraints []string
				if minLength, exists := paramMap["minLength"]; exists {
					constraints = append(constraints, "最小长度: "+utils.InterfaceToString(minLength))
				}
				if maxLength, exists := paramMap["maxLength"]; exists {
					constraints = append(constraints, "最大长度: "+utils.InterfaceToString(maxLength))
				}
				if pattern, exists := paramMap["pattern"]; exists {
					constraints = append(constraints, "模式: "+utils.InterfaceToString(pattern))
				}
				if len(constraints) > 0 {
					if param.Description != "" {
						param.Description += " (" + strings.Join(constraints, ", ") + ")"
					} else {
						param.Description = strings.Join(constraints, ", ")
					}
				}
			}
		} else {
			// 如果不是 map 类型，尝试直接转换为字符串作为类型
			param.Type = utils.InterfaceToString(paramValue)
		}

		// 如果类型为空，设置默认类型
		if param.Type == "" {
			param.Type = "string"
		}

		params = append(params, param)
		return true // 继续遍历
	})

	return params, nil
}
