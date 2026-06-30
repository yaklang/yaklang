package scannode

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func (b *legionJobBridge) handleAIMCPServersList(ctx context.Context, raw []byte) error {
	var command aiv1.ListAIMCPServersCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai mcp servers list command: %w", err)
	}

	ref := aiMCPRefFromListCommand(&command)
	if err := validateAIMCPServersListCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMCPServersListFailed(
			ctx,
			ref,
			"invalid_ai_mcp_servers_list_command",
			err.Error(),
		)
	}

	items, pagination, total, err := listAIMCPServers(ctx, &command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMCPServersListFailed(
			ctx,
			ref,
			"ai_mcp_servers_list_failed",
			err.Error(),
		)
	}
	return b.ensureAIPublisher().PublishAIMCPServersListed(ctx, ref, items, pagination, total)
}

func (b *legionJobBridge) handleAIMCPServerCreate(ctx context.Context, raw []byte) error {
	var command aiv1.CreateAIMCPServerCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai mcp server create command: %w", err)
	}

	ref := aiMCPRefFromCreateCommand(&command)
	if err := validateAIMCPServerCreateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMCPServerCreateFailed(
			ctx,
			ref,
			"invalid_ai_mcp_server_create_command",
			err.Error(),
		)
	}

	record, errorCode, err := createAIMCPServer(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMCPServerCreateFailed(ctx, ref, errorCode, err.Error())
	}
	return b.ensureAIPublisher().PublishAIMCPServerCreated(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIMCPServerUpdate(ctx context.Context, raw []byte) error {
	var command aiv1.UpdateAIMCPServerCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai mcp server update command: %w", err)
	}

	ref := aiMCPRefFromUpdateCommand(&command)
	if err := validateAIMCPServerUpdateCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMCPServerUpdateFailed(
			ctx,
			ref,
			"invalid_ai_mcp_server_update_command",
			err.Error(),
		)
	}

	record, errorCode, err := updateAIMCPServer(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMCPServerUpdateFailed(ctx, ref, errorCode, err.Error())
	}
	return b.ensureAIPublisher().PublishAIMCPServerUpdated(ctx, ref, record, "ok")
}

func (b *legionJobBridge) handleAIMCPServerDelete(ctx context.Context, raw []byte) error {
	var command aiv1.DeleteAIMCPServerCommand
	if err := proto.Unmarshal(raw, &command); err != nil {
		return fmt.Errorf("unmarshal ai mcp server delete command: %w", err)
	}

	ref := aiMCPRefFromDeleteCommand(&command)
	if err := validateAIMCPServerDeleteCommand(b.agent.node.CurrentNodeID(), &command); err != nil {
		return b.ensureAIPublisher().PublishAIMCPServerDeleteFailed(
			ctx,
			ref,
			"invalid_ai_mcp_server_delete_command",
			err.Error(),
		)
	}

	record, errorCode, err := deleteAIMCPServer(&command)
	if err != nil {
		return b.ensureAIPublisher().PublishAIMCPServerDeleteFailed(ctx, ref, errorCode, err.Error())
	}
	return b.ensureAIPublisher().PublishAIMCPServerDeleted(ctx, ref, record, "ok")
}

func validateAIMCPServersListCommand(nodeID string, command *aiv1.ListAIMCPServersCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai mcp servers list metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai mcp servers list command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai mcp servers list target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai mcp servers list target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai mcp servers list owner_user_id is required")
	default:
		return nil
	}
}

func validateAIMCPServerCreateCommand(nodeID string, command *aiv1.CreateAIMCPServerCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai mcp server create metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai mcp server create command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai mcp server create target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai mcp server create target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai mcp server create owner_user_id is required")
	case strings.TrimSpace(command.GetName()) == "":
		return fmt.Errorf("ai mcp server create name is required")
	case strings.TrimSpace(command.GetTransportType()) == "":
		return fmt.Errorf("ai mcp server create transport_type is required")
	default:
		return nil
	}
}

func validateAIMCPServerUpdateCommand(nodeID string, command *aiv1.UpdateAIMCPServerCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai mcp server update metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai mcp server update command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai mcp server update target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai mcp server update target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai mcp server update owner_user_id is required")
	case command.GetServerId() <= 0:
		return fmt.Errorf("ai mcp server update server_id must be greater than 0")
	case strings.TrimSpace(command.GetName()) == "":
		return fmt.Errorf("ai mcp server update name is required")
	case strings.TrimSpace(command.GetTransportType()) == "":
		return fmt.Errorf("ai mcp server update transport_type is required")
	default:
		return nil
	}
}

func validateAIMCPServerDeleteCommand(nodeID string, command *aiv1.DeleteAIMCPServerCommand) error {
	switch {
	case command.GetMetadata() == nil:
		return fmt.Errorf("ai mcp server delete metadata is required")
	case strings.TrimSpace(command.GetMetadata().GetCommandId()) == "":
		return fmt.Errorf("ai mcp server delete command_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) == "":
		return fmt.Errorf("ai mcp server delete target_node_id is required")
	case strings.TrimSpace(command.GetTargetNodeId()) != nodeID:
		return fmt.Errorf("ai mcp server delete target_node_id mismatch: %s", command.GetTargetNodeId())
	case strings.TrimSpace(command.GetOwnerUserId()) == "":
		return fmt.Errorf("ai mcp server delete owner_user_id is required")
	case command.GetServerId() <= 0:
		return fmt.Errorf("ai mcp server delete server_id must be greater than 0")
	default:
		return nil
	}
}

func aiMCPRefFromListCommand(command *aiv1.ListAIMCPServersCommand) aiMCPCommandRef {
	return aiMCPCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMCPRefFromCreateCommand(command *aiv1.CreateAIMCPServerCommand) aiMCPCommandRef {
	return aiMCPCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMCPRefFromUpdateCommand(command *aiv1.UpdateAIMCPServerCommand) aiMCPCommandRef {
	return aiMCPCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func aiMCPRefFromDeleteCommand(command *aiv1.DeleteAIMCPServerCommand) aiMCPCommandRef {
	return aiMCPCommandRef{
		CommandID:   strings.TrimSpace(command.GetMetadata().GetCommandId()),
		OwnerUserID: strings.TrimSpace(command.GetOwnerUserId()),
	}
}

func listAIMCPServers(
	ctx context.Context,
	command *aiv1.ListAIMCPServersCommand,
) ([]*aiv1.AIMCPServerRecord, *aiv1.AIMCPServerPagination, int64, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, nil, 0, utils.Errorf("database not initialized")
	}

	pagination := command.GetPagination()
	if pagination == nil {
		pagination = &aiv1.AIMCPServerPagination{Page: 1, Limit: 20}
	}
	if pagination.GetPage() <= 0 {
		pagination.Page = 1
	}
	if pagination.GetLimit() <= 0 {
		pagination.Limit = 20
	}

	req := &ypb.GetAllMCPServersRequest{
		Keyword:        strings.TrimSpace(command.GetQuery()),
		ID:             command.GetServerId(),
		IsShowToolList: command.GetIncludeTools(),
		Pagination: &ypb.Paging{
			Page:    pagination.GetPage(),
			Limit:   pagination.GetLimit(),
			OrderBy: "updated_at",
			Order:   "desc",
		},
	}
	paginator, servers, err := yakit.QueryMCPServers(db, req)
	if err != nil {
		return nil, nil, 0, err
	}

	items := make([]*aiv1.AIMCPServerRecord, 0, len(servers))
	for _, server := range servers {
		if server == nil {
			continue
		}
		if transportType := strings.TrimSpace(command.GetTransportType()); transportType != "" && transportType != server.Type {
			continue
		}
		var (
			tools        []*aiv1.AIMCPServerTool
			errorMessage string
		)
		if command.GetIncludeTools() && server.Enable {
			serverTools, toolErr := getAIMCPServerTools(ctx, server)
			if toolErr != nil {
				errorMessage = toolErr.Error()
			} else {
				tools = serverTools
			}
		}
		items = append(items, mapSchemaMCPServerToLegion(server, tools, errorMessage))
	}

	return items, &aiv1.AIMCPServerPagination{
		Page:  int64(paginator.Page),
		Limit: int64(paginator.Limit),
	}, int64(paginator.TotalRecord), nil
}

func createAIMCPServer(command *aiv1.CreateAIMCPServerCommand) (*aiv1.AIMCPServerRecord, string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, "ai_mcp_server_unavailable", utils.Errorf("database not initialized")
	}
	server := &schema.MCPServer{
		Name:    strings.TrimSpace(command.GetName()),
		Type:    strings.TrimSpace(command.GetTransportType()),
		URL:     strings.TrimSpace(command.GetUrl()),
		Command: strings.TrimSpace(command.GetCommand()),
		Enable:  command.GetEnabled(),
		Envs:    aiConfigKVPairsToSchemaMap(command.GetEnvs()),
		Headers: aiConfigKVPairsToSchemaMap(command.GetHeaders()),
	}
	if err := yakit.CreateMCPServer(db, server); err != nil {
		return nil, classifyAIMCPError(err), err
	}
	stored, err := yakit.GetMCPServerByName(db, server.Name)
	if err != nil {
		return nil, classifyAIMCPError(err), err
	}
	return mapSchemaMCPServerToLegion(stored, nil, ""), "", nil
}

func updateAIMCPServer(command *aiv1.UpdateAIMCPServerCommand) (*aiv1.AIMCPServerRecord, string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, "ai_mcp_server_unavailable", utils.Errorf("database not initialized")
	}
	current, err := yakit.GetMCPServer(db, command.GetServerId())
	if err != nil {
		return nil, classifyAIMCPError(err), err
	}
	server := &schema.MCPServer{
		Name:    strings.TrimSpace(command.GetName()),
		Type:    strings.TrimSpace(command.GetTransportType()),
		URL:     strings.TrimSpace(command.GetUrl()),
		Command: strings.TrimSpace(command.GetCommand()),
		Enable:  command.GetEnabled(),
		Envs:    aiConfigKVPairsToSchemaMap(command.GetEnvs()),
		Headers: aiConfigKVPairsToSchemaMap(command.GetHeaders()),
	}
	if err := yakit.UpdateMCPServer(db, command.GetServerId(), server); err != nil {
		return nil, classifyAIMCPError(err), err
	}
	stored, err := yakit.GetMCPServer(db, int64(current.ID))
	if err != nil {
		return nil, classifyAIMCPError(err), err
	}
	return mapSchemaMCPServerToLegion(stored, nil, ""), "", nil
}

func deleteAIMCPServer(command *aiv1.DeleteAIMCPServerCommand) (*aiv1.AIMCPServerRecord, string, error) {
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return nil, "ai_mcp_server_unavailable", utils.Errorf("database not initialized")
	}
	current, err := yakit.GetMCPServer(db, command.GetServerId())
	if err != nil {
		return nil, classifyAIMCPError(err), err
	}
	record := mapSchemaMCPServerToLegion(current, nil, "")
	if err := yakit.DeleteMCPServer(db, command.GetServerId()); err != nil {
		return nil, classifyAIMCPError(err), err
	}
	return record, "", nil
}

func getAIMCPServerTools(ctx context.Context, server *schema.MCPServer) ([]*aiv1.AIMCPServerTool, error) {
	var (
		mcpClient client.MCPClient
		err       error
	)

	switch server.Type {
	case "stdio":
		commandParts := utils.PrettifyListFromStringSplited(server.Command, " ")
		if len(commandParts) == 0 {
			return nil, utils.Errorf("invalid command: %s", server.Command)
		}
		command := commandParts[0]
		args := commandParts[1:]
		envs := make([]string, 0, len(server.Envs))
		for key, value := range server.Envs {
			envs = append(envs, fmt.Sprintf("%s=%v", key, value))
		}
		mcpClient, err = client.NewStdioMCPClient(command, envs, args...)
		if err != nil {
			return nil, err
		}
	case "sse":
		sseClient, err := client.NewSSEMCPClient(server.URL, schemaMapToStringMap(server.Headers))
		if err != nil {
			return nil, utils.Errorf("create sse mcp client failed: %s", err)
		}
		if err := sseClient.Start(ctx); err != nil {
			return nil, utils.Errorf("start sse mcp client failed: %s", err)
		}
		mcpClient = sseClient
	case "streamable_http":
		streamableHTTPClient, err := client.NewStreamableHTTPMCPClient(server.URL, schemaMapToStringMap(server.Headers))
		if err != nil {
			return nil, utils.Errorf("create streamable http mcp client failed: %s", err)
		}
		mcpClient = streamableHTTPClient
	default:
		return nil, utils.Errorf("unsupported server type: %s", server.Type)
	}
	defer mcpClient.Close()

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "yaklang-mcp-client",
		Version: "1.0.0",
	}

	if _, err := mcpClient.Initialize(ctx, initRequest); err != nil {
		return nil, err
	}

	toolsResult, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}

	tools := make([]*aiv1.AIMCPServerTool, 0, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		params, err := parseAIMCPToolInputSchema(&tool.InputSchema)
		if err != nil {
			return nil, utils.Errorf("parse mcp tool input schema failed: %s", err)
		}
		tools = append(tools, &aiv1.AIMCPServerTool{
			Name:        tool.Name,
			Description: tool.Description,
			Params:      params,
		})
	}
	return tools, nil
}

func parseAIMCPToolInputSchema(inputSchema *mcp.ToolInputSchema) ([]*aiv1.AIMCPServerToolParam, error) {
	if inputSchema == nil {
		return []*aiv1.AIMCPServerToolParam{}, nil
	}
	if inputSchema.Properties == nil || inputSchema.Properties.Len() == 0 {
		return []*aiv1.AIMCPServerToolParam{}, nil
	}

	requiredMap := make(map[string]bool, len(inputSchema.Required))
	for _, item := range inputSchema.Required {
		requiredMap[item] = true
	}

	params := make([]*aiv1.AIMCPServerToolParam, 0)
	inputSchema.Properties.ForEach(func(paramName string, paramValue any) bool {
		param := &aiv1.AIMCPServerToolParam{
			Name:     paramName,
			Required: requiredMap[paramName],
		}
		if paramMap, ok := paramValue.(map[string]any); ok {
			if paramType, exists := paramMap["type"]; exists {
				param.Type = utils.InterfaceToString(paramType)
			}
			if description, exists := paramMap["description"]; exists {
				param.Description = utils.InterfaceToString(description)
			}
			if defaultVal, exists := paramMap["default"]; exists {
				param.DefaultValue = utils.InterfaceToString(defaultVal)
			}
		}
		params = append(params, param)
		return true
	})

	sort.Slice(params, func(i, j int) bool {
		return params[i].Name < params[j].Name
	})
	return params, nil
}

func mapSchemaMCPServerToLegion(
	server *schema.MCPServer,
	tools []*aiv1.AIMCPServerTool,
	errorMessage string,
) *aiv1.AIMCPServerRecord {
	if server == nil {
		return nil
	}
	return &aiv1.AIMCPServerRecord{
		ServerId:      int64(server.ID),
		Name:          strings.TrimSpace(server.Name),
		TransportType: strings.TrimSpace(server.Type),
		Url:           strings.TrimSpace(server.URL),
		Command:       strings.TrimSpace(server.Command),
		Enabled:       server.Enable,
		Tools:         tools,
		ErrorMessage:  strings.TrimSpace(errorMessage),
		Envs:          schemaMapToAIConfigKVPairs(server.Envs),
		Headers:       schemaMapToAIConfigKVPairs(server.Headers),
		CreatedAt:     server.CreatedAt.Unix(),
		UpdatedAt:     server.UpdatedAt.Unix(),
	}
}

func schemaMapToAIConfigKVPairs(items schema.MapStringAny) []*aiv1.AIConfigKVPair {
	if len(items) == 0 {
		return nil
	}
	keys := make([]string, 0, len(items))
	for key := range items {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			keys = append(keys, trimmed)
		}
	}
	sort.Strings(keys)

	result := make([]*aiv1.AIConfigKVPair, 0, len(keys))
	for _, key := range keys {
		result = append(result, &aiv1.AIConfigKVPair{
			Key:   key,
			Value: fmt.Sprintf("%v", items[key]),
		})
	}
	return result
}

func aiConfigKVPairsToSchemaMap(items []*aiv1.AIConfigKVPair) schema.MapStringAny {
	if len(items) == 0 {
		return nil
	}
	result := make(schema.MapStringAny, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		key := strings.TrimSpace(item.GetKey())
		if key == "" {
			continue
		}
		result[key] = strings.TrimSpace(item.GetValue())
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func schemaMapToStringMap(input schema.MapStringAny) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = fmt.Sprintf("%v", value)
	}
	return result
}

func classifyAIMCPError(err error) string {
	if err == nil {
		return ""
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "already exists"):
		return "ai_mcp_server_conflict"
	case strings.Contains(message, "not found"):
		return "ai_mcp_server_not_found"
	default:
		return "ai_mcp_server_failed"
	}
}
