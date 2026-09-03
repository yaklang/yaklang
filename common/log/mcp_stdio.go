package log

import (
	"os"
	"strings"
	"sync/atomic"
)

const mcpStdioModeEnv = "YAK_MCP_STDIO"

var mcpStdioMode atomic.Bool

func init() {
	ConfigureMCPStdioLogging(os.Args)
}

// ConfigureMCPStdioLogging detects whether args start Yak's MCP command with
// the stdio transport. It is intentionally called from package init so logs
// emitted by dependent package initializers cannot corrupt the JSON-RPC
// stream before the CLI action starts.
func ConfigureMCPStdioLogging(args []string) bool {
	if !isMCPStdioCommand(args) && !envEnablesMCPStdioMode() {
		return false
	}
	EnableMCPStdioLogging()
	return true
}

// EnableMCPStdioLogging reserves stdout for MCP JSON-RPC and moves the shared
// Yak logger to stderr. The operation is idempotent and also affects loggers
// created later through GetLogger.
func EnableMCPStdioLogging() {
	mcpStdioMode.Store(true)
	SetOutput(os.Stderr)
}

// IsMCPStdioLogging reports whether stdout is reserved for MCP JSON-RPC.
func IsMCPStdioLogging() bool {
	return mcpStdioMode.Load()
}

func envEnablesMCPStdioMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(mcpStdioModeEnv))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func isMCPStdioCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}

	commandIndex := -1
	executable := args[0]
	if index := strings.LastIndexAny(executable, `/\\`); index >= 0 {
		executable = executable[index+1:]
	}
	executable = strings.TrimSuffix(strings.ToLower(executable), ".exe")
	if executable == "mcp" {
		commandIndex = 0
	}

	for index := 1; index < len(args); index++ {
		if strings.EqualFold(strings.TrimSpace(args[index]), "mcp") {
			commandIndex = index
			break
		}
	}
	if commandIndex < 0 {
		return false
	}

	transport := "stdio"
	for index := commandIndex + 1; index < len(args); index++ {
		argument := strings.TrimSpace(args[index])
		switch {
		case argument == "--transport":
			if index+1 < len(args) {
				transport = args[index+1]
			}
		case strings.HasPrefix(argument, "--transport="):
			transport = strings.TrimPrefix(argument, "--transport=")
		}
	}

	return strings.EqualFold(strings.TrimSpace(transport), "stdio")
}
