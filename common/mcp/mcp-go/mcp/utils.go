package mcp

import "fmt"

// ClientRequest types
var _ ClientRequest = &PingRequest{}
var _ ClientRequest = &InitializeRequest{}
var _ ClientRequest = &CompleteRequest{}
var _ ClientRequest = &SetLevelRequest{}
var _ ClientRequest = &GetPromptRequest{}
var _ ClientRequest = &ListPromptsRequest{}
var _ ClientRequest = &ListResourcesRequest{}
var _ ClientRequest = &ReadResourceRequest{}
var _ ClientRequest = &SubscribeRequest{}
var _ ClientRequest = &UnsubscribeRequest{}
var _ ClientRequest = &CallToolRequest{}
var _ ClientRequest = &ListToolsRequest{}

// ClientNotification types
var _ ClientNotification = &CancelledNotification{}
var _ ClientNotification = &ProgressNotification{}
var _ ClientNotification = &InitializedNotification{}
var _ ClientNotification = &RootsListChangedNotification{}

// ClientResult types
var _ ClientResult = &EmptyResult{}
var _ ClientResult = &CreateMessageResult{}
var _ ClientResult = &ListRootsResult{}

// ServerRequest types
var _ ServerRequest = &PingRequest{}
var _ ServerRequest = &CreateMessageRequest{}
var _ ServerRequest = &ListRootsRequest{}

// ServerNotification types
var _ ServerNotification = &CancelledNotification{}
var _ ServerNotification = &ProgressNotification{}
var _ ServerNotification = &LoggingMessageNotification{}
var _ ServerNotification = &ResourceUpdatedNotification{}
var _ ServerNotification = &ResourceListChangedNotification{}
var _ ServerNotification = &ToolListChangedNotification{}
var _ ServerNotification = &PromptListChangedNotification{}

// ServerResult types
var _ ServerResult = &EmptyResult{}
var _ ServerResult = &InitializeResult{}
var _ ServerResult = &CompleteResult{}
var _ ServerResult = &GetPromptResult{}
var _ ServerResult = &ListPromptsResult{}
var _ ServerResult = &ListResourcesResult{}
var _ ServerResult = &ReadResourceResult{}
var _ ServerResult = &CallToolResult{}
var _ ServerResult = &ListToolsResult{}

// Helper functions for type assertions

// AsTextContent attempts to cast the given interface to TextContent
func AsTextContent(content interface{}) (*TextContent, bool) {
	tc, ok := content.(TextContent)
	if !ok {
		return nil, false
	}
	return &tc, true
}

// AsImageContent attempts to cast the given interface to ImageContent
func AsImageContent(content interface{}) (*ImageContent, bool) {
	ic, ok := content.(ImageContent)
	if !ok {
		return nil, false
	}
	return &ic, true
}

// AsEmbeddedResource attempts to cast the given interface to EmbeddedResource
func AsEmbeddedResource(content interface{}) (*EmbeddedResource, bool) {
	er, ok := content.(EmbeddedResource)
	if !ok {
		return nil, false
	}
	return &er, true
}

// AsTextResourceContents attempts to cast the given interface to TextResourceContents
func AsTextResourceContents(content interface{}) (*TextResourceContents, bool) {
	trc, ok := content.(TextResourceContents)
	if !ok {
		return nil, false
	}
	return &trc, true
}

// AsBlobResourceContents attempts to cast the given interface to BlobResourceContents
func AsBlobResourceContents(content interface{}) (*BlobResourceContents, bool) {
	brc, ok := content.(BlobResourceContents)
	if !ok {
		return nil, false
	}
	return &brc, true
}

// Helper function for JSON-RPC

// NewJSONRPCResponse creates a new JSONRPCResponse with the given id and result
func NewJSONRPCResponse(id RequestId, result Result) JSONRPCResponse {
	return JSONRPCResponse{
		JSONRPC: JSONRPC_VERSION,
		ID:      id,
		Result:  result,
	}
}

// NewJSONRPCError creates a new JSONRPCResponse with the given id, code, and message
func NewJSONRPCError(
	id RequestId,
	code int,
	message string,
	data interface{},
) JSONRPCError {
	return JSONRPCError{
		JSONRPC: JSONRPC_VERSION,
		ID:      id,
		Error: struct {
			Code    int         `json:"code"`
			Message string      `json:"message"`
			Data    interface{} `json:"data,omitempty"`
		}{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}
}

// Helper function for creating a progress notification
func NewProgressNotification(
	token ProgressToken,
	progress float64,
	total *float64,
) ProgressNotification {
	notification := ProgressNotification{
		Notification: Notification{
			Method: "notifications/progress",
		},
		Params: struct {
			ProgressToken ProgressToken `json:"progressToken"`
			Progress      float64       `json:"progress"`
			Total         float64       `json:"total,omitempty"`
		}{
			ProgressToken: token,
			Progress:      progress,
		},
	}
	if total != nil {
		notification.Params.Total = *total
	}
	return notification
}

// Helper function for creating a logging message notification
func NewLoggingMessageNotification(
	level LoggingLevel,
	logger string,
	data interface{},
) LoggingMessageNotification {
	return LoggingMessageNotification{
		Notification: Notification{
			Method: "notifications/message",
		},
		Params: struct {
			Level  LoggingLevel `json:"level"`
			Logger string       `json:"logger,omitempty"`
			Data   interface{}  `json:"data"`
		}{
			Level:  level,
			Logger: logger,
			Data:   data,
		},
	}
}

// Helper function to create a new PromptMessage
func NewPromptMessage(role Role, content interface{}) PromptMessage {
	return PromptMessage{
		Role:    role,
		Content: content,
	}
}

// Helper function to create a new TextContent
func NewTextContent(text string) TextContent {
	return TextContent{
		Type: "text",
		Text: text,
	}
}

// Helper function to create a new ImageContent
func NewImageContent(data, mimeType string) ImageContent {
	return ImageContent{
		Type:     "image",
		Data:     data,
		MIMEType: mimeType,
	}
}

// Helper function to create a new EmbeddedResource
func NewEmbeddedResource(resource ResourceContents) EmbeddedResource {
	return EmbeddedResource{
		Type:     "resource",
		Resource: resource,
	}
}

// NewToolResultText creates a new CallToolResult with a text content
func NewToolResultText(text string) *CallToolResult {
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: text,
			},
		},
	}
}

// NewToolResultError creates a new CallToolResult that indicates an error
func NewToolResultError(errText string) *CallToolResult {
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: errText,
			},
		},
		IsError: true,
	}
}

// NewToolResultImage creates a new CallToolResult with both text and image content
func NewToolResultImage(text, imageData, mimeType string) *CallToolResult {
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: text,
			},
			ImageContent{
				Type:     "image",
				Data:     imageData,
				MIMEType: mimeType,
			},
		},
	}
}

// NewToolResultResource creates a new CallToolResult with an embedded resource
func NewToolResultResource(
	text string,
	resource ResourceContents,
) *CallToolResult {
	return &CallToolResult{
		Content: []interface{}{
			TextContent{
				Type: "text",
				Text: text,
			},
			EmbeddedResource{
				Type:     "resource",
				Resource: resource,
			},
		},
	}
}

// NewListResourcesResult creates a new ListResourcesResult
func NewListResourcesResult(
	resources []Resource,
	nextCursor Cursor,
) *ListResourcesResult {
	return &ListResourcesResult{
		PaginatedResult: PaginatedResult{
			NextCursor: nextCursor,
		},
		Resources: resources,
	}
}

// NewListResourceTemplatesResult creates a new ListResourceTemplatesResult
func NewListResourceTemplatesResult(
	templates []ResourceTemplate,
	nextCursor Cursor,
) *ListResourceTemplatesResult {
	return &ListResourceTemplatesResult{
		PaginatedResult: PaginatedResult{
			NextCursor: nextCursor,
		},
		ResourceTemplates: templates,
	}
}

// NewReadResourceResult creates a new ReadResourceResult with text content
func NewReadResourceResult(text string) *ReadResourceResult {
	return &ReadResourceResult{
		Contents: []interface{}{
			TextResourceContents{
				ResourceContents: ResourceContents{},
				Text:             text,
			},
		},
	}
}

// NewListPromptsResult creates a new ListPromptsResult
func NewListPromptsResult(
	prompts []Prompt,
	nextCursor Cursor,
) *ListPromptsResult {
	return &ListPromptsResult{
		PaginatedResult: PaginatedResult{
			NextCursor: nextCursor,
		},
		Prompts: prompts,
	}
}

// NewGetPromptResult creates a new GetPromptResult
func NewGetPromptResult(
	description string,
	messages []PromptMessage,
) *GetPromptResult {
	return &GetPromptResult{
		Description: description,
		Messages:    messages,
	}
}

// NewListToolsResult creates a new ListToolsResult
func NewListToolsResult(tools []Tool, nextCursor Cursor) *ListToolsResult {
	return &ListToolsResult{
		PaginatedResult: PaginatedResult{
			NextCursor: nextCursor,
		},
		Tools: tools,
	}
}

// NewInitializeResult creates a new InitializeResult
func NewInitializeResult(
	protocolVersion string,
	capabilities ServerCapabilities,
	serverInfo Implementation,
	instructions string,
) *InitializeResult {
	return &InitializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    capabilities,
		ServerInfo:      serverInfo,
		Instructions:    instructions,
	}
}

// Helper for formatting numbers in tool results
func FormatNumberResult(value float64) *CallToolResult {
	return NewToolResultText(fmt.Sprintf("%.2f", value))
}
