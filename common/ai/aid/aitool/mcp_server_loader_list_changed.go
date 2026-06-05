package aitool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/client"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const mcpToolsListChangedMethod = "notifications/tools/list_changed"

// MCPToolsListChangedHandler is invoked when a remote MCP server sends
// notifications/tools/list_changed. tools holds refreshed live AITools;
// removedAIToolNames lists full names (mcp_{server}_{tool}) no longer offered.
type MCPToolsListChangedHandler func(serverName string, tools []*Tool, removedAIToolNames []string)

type mcpToolsListChangedState struct {
	mu        sync.Mutex
	lastKnown map[string]struct{}
}

func newMCPToolsListChangedState(initialTools []*Tool) *mcpToolsListChangedState {
	lastKnown := make(map[string]struct{}, len(initialTools))
	for _, t := range initialTools {
		if t != nil {
			lastKnown[t.Name] = struct{}{}
		}
	}
	return &mcpToolsListChangedState{lastKnown: lastKnown}
}

func (s *mcpToolsListChangedState) applyRefresh(
	db *gorm.DB,
	mcpServer *schema.MCPServer,
	mcpClient client.MCPClient,
	onChange MCPToolsListChangedHandler,
) {
	if s == nil || onChange == nil || mcpClient == nil || mcpServer == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	newTools, err := refreshAIToolsFromMCPServer(ctx, db, mcpServer, mcpClient)
	if err != nil {
		log.Errorf("mcp list_changed refresh for server %s failed: %v", mcpServer.Name, err)
		return
	}

	newSet := make(map[string]struct{}, len(newTools))
	for _, t := range newTools {
		if t != nil {
			newSet[t.Name] = struct{}{}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var removed []string
	for name := range s.lastKnown {
		if _, ok := newSet[name]; !ok {
			removed = append(removed, name)
		}
	}
	s.lastKnown = newSet

	onChange(mcpServer.Name, newTools, removed)
}

func registerMCPToolsListChangedHandler(
	mcpClient client.MCPClient,
	db *gorm.DB,
	mcpServer *schema.MCPServer,
	initialTools []*Tool,
	onChange MCPToolsListChangedHandler,
) *mcpToolsListChangedState {
	if mcpClient == nil || onChange == nil {
		return nil
	}

	state := newMCPToolsListChangedState(initialTools)
	var refreshMu sync.Mutex

	// Register before further RPC so notifications are not missed during startup.
	mcpClient.OnNotification(func(notification mcp.JSONRPCNotification) {
		if notification.Method != mcpToolsListChangedMethod {
			return
		}
		refreshMu.Lock()
		defer refreshMu.Unlock()
		state.applyRefresh(db, mcpServer, mcpClient, onChange)
	})
	return state
}

// SyncMCPToolsListChangedState seeds the last-known tool set after the initial ListTools.
func SyncMCPToolsListChangedState(state *mcpToolsListChangedState, tools []*Tool) {
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.lastKnown = make(map[string]struct{}, len(tools))
	for _, t := range tools {
		if t != nil {
			state.lastKnown[t.Name] = struct{}{}
		}
	}
}

// refreshAIToolsFromMCPServer re-lists tools from the connected MCP server and rebuilds AITools.
func refreshAIToolsFromMCPServer(
	ctx context.Context,
	db *gorm.DB,
	mcpServer *schema.MCPServer,
	mcpClient client.MCPClient,
) ([]*Tool, error) {
	if db == nil {
		db = consts.GetGormProfileDatabase()
		if db == nil {
			return nil, utils.Error("profile database is not initialized")
		}
	}

	toolsRequest := mcp.ListToolsRequest{}
	toolsResult, err := mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		return nil, utils.Errorf("list tools failed: %v", err)
	}

	liveEntries := make([]yakit.MCPToolEntry, 0, len(toolsResult.Tools))
	for _, t := range toolsResult.Tools {
		liveEntries = append(liveEntries, yakit.MCPToolEntry{
			ToolName:    t.Name,
			FullName:    fmt.Sprintf("mcp_%s_%s", mcpServer.Name, t.Name),
			Description: t.Description,
			ParamsJSON:  serializeMCPToolParams(&t.InputSchema),
		})
	}
	if syncErr := yakit.SyncAndCacheMCPServerTools(db, mcpServer.Name, liveEntries); syncErr != nil {
		log.Warnf("sync mcp tool cache for server %s failed: %v", mcpServer.Name, syncErr)
	}

	toolConfigs, err := yakit.BatchGetMCPServerToolConfigs(db, mcpServer.Name)
	if err != nil {
		log.Warnf("batch load tool configs for server %s failed: %v, falling back to defaults", mcpServer.Name, err)
		toolConfigs = map[string]*schema.MCPServerToolConfig{}
	}

	var aiTools []*Tool
	for _, mcpTool := range toolsResult.Tools {
		cfg, ok := toolConfigs[mcpTool.Name]
		if ok && !cfg.Enable {
			continue
		}
		aiTool, err := convertMCPToolToAITool(mcpTool, mcpServer, mcpClient)
		if err != nil {
			log.Errorf("convert mcp tool to aitool failed: %v", err)
			continue
		}
		aiTools = append(aiTools, aiTool)
	}
	return aiTools, nil
}
