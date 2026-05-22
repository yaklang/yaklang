package buildinaitools

import (
	"context"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// MCPToolInitWaitTimeout is the default max wait for a stub MCP tool to be
	// replaced by a live tool after ReAct startup / background MCP loading.
	MCPToolInitWaitTimeout = 10 * time.Second
	// MCPToolInitPollInterval is how often to re-check the tool manager for a live tool.
	MCPToolInitPollInterval = 2 * time.Second
	// MCPToolInitializingErrPrefix is returned by MCP stub callbacks while connecting.
	MCPToolInitializingErrPrefix = "TOOL_INITIALIZING:"
)

// IsMCPToolName reports whether name follows the runtime MCP tool naming convention.
func IsMCPToolName(name string) bool {
	return strings.HasPrefix(name, "mcp_")
}

// IsMCPPendingStub reports whether the tool is still a DB-cache stub awaiting live connection.
func IsMCPPendingStub(tool *aitool.Tool) bool {
	return tool != nil && tool.MCPPendingStub
}

// IsMCPInitializingError reports whether err/result text indicates MCP is still connecting.
func IsMCPInitializingError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), MCPToolInitializingErrPrefix)
}

// IsMCPInitializingMessage reports the same for a plain error string (e.g. ToolResult.Error).
func IsMCPInitializingMessage(msg string) bool {
	return strings.Contains(msg, MCPToolInitializingErrPrefix)
}

// WaitForMCPLiveTool blocks until the named MCP tool is no longer a pending stub,
// the context is cancelled, or timeout elapses. onWaiting is optional and called on
// each poll tick (not on the first successful immediate return).
func WaitForMCPLiveTool(
	ctx context.Context,
	mgr *AiToolManager,
	toolName string,
	timeout time.Duration,
	pollInterval time.Duration,
	onWaiting func(elapsed time.Duration),
) (*aitool.Tool, error) {
	if mgr == nil {
		return nil, utils.Errorf("ai tool manager is nil")
	}
	if !IsMCPToolName(toolName) {
		return mgr.GetToolByName(toolName)
	}
	if timeout <= 0 {
		timeout = MCPToolInitWaitTimeout
	}
	if pollInterval <= 0 {
		pollInterval = MCPToolInitPollInterval
	}

	deadline := time.Now().Add(timeout)
	start := time.Now()
	attempt := 0

	for {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		tool, err := mgr.GetToolByName(toolName)
		if err != nil {
			return nil, err
		}
		if !IsMCPPendingStub(tool) {
			if attempt > 0 {
				log.Infof("MCP tool %q became live after %v", toolName, time.Since(start))
			}
			return tool, nil
		}

		elapsed := time.Since(start)
		if time.Now().After(deadline) {
			return tool, utils.Errorf(
				"MCP tool %q is still initializing after %v: remote MCP server has not finished loading; "+
					"last status: %s",
				toolName, timeout, MCPToolInitializingErrPrefix,
			)
		}

		if attempt > 0 && onWaiting != nil {
			onWaiting(elapsed)
		}
		attempt++

		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
