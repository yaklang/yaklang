package mcp

import (
	"context"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	AddGlobalToolSet("reverse_shell",
		WithTool(mcp.NewTool(string("generate_reverse_shell_command"),
			mcp.WithDescription("Generate a reverse shell command"),
			mcp.WithString("program",
				mcp.Description("Type of reverse shell"),
				mcp.Required(),
				mcp.Enum([]string{"Bash -i", "Bash 196", "Bash read line", "Bash 5", "Bash udp", "nc mkfifo", "nc -e", "nc.exe -e", "BusyBox nc -e", "nc -c", "ncat -e", "ncat.exe -e", "ncat udp", "curl", "rustcat", "C", "C Windows", "C# TCP Client", "C# Bash -i", "Haskell #1", "Perl", "Perl no sh", "Perl PentestMonkey", "PHP PentestMonkey", "PHP Ivan Sincek", "PHP cmd", "PHP cmd 2", "PHP cmd small", "PHP exec", "PHP shell_exec", "PHP system", "PHP passthru", "PHP `", "PHP popen", "PHP proc_open", "Windows ConPty", "PowerShell #1", "PowerShell #2", "PowerShell #3", "PowerShell #4 (TLS)", "PowerShell #3 (Base64)", "Python #1", "Python #2", "Python3 #1", "Python3 #2", "Python3 Windows", "Python3 shortest", "Ruby #1", "Ruby no sh", "socat #1", "socat #2 (TTY)", "sqlite3 nc mkfifo", "node.js", "node.js #2", "Java #1", "Java #2", "Java #3", "Java Web", "Java Two Way", "Javascript", "Groovy", "telnet", "zsh", "Lua #1", "Lua #2", "Golang", "Vlang", "Awk", "Dart", "Crystal (system)", "Crystal (code)", "Windows Meterpreter Staged Reverse TCP (x64)", "Windows Meterpreter Stageless Reverse TCP (x64)", "Windows Staged Reverse TCP (x64)", "Windows Stageless Reverse TCP (x64)", "Windows Staged JSP Reverse TCP", "Linux Meterpreter Staged Reverse TCP (x64)", "Linux Stageless Reverse TCP (x64)", "Windows Bind TCP ShellCode - BOF", "macOS Meterpreter Staged Reverse TCP (x64)", "macOS Meterpreter Stageless Reverse TCP (x64)", "macOS Stageless Reverse TCP (x64)", "PHP Meterpreter Stageless Reverse TCP", "PHP Reverse PHP", "JSP Stageless Reverse TCP", "WAR Stageless Reverse TCP", "Android Meterpreter Reverse TCP", "Android Meterpreter Embed Reverse TCP", "Apple iOS Meterpreter Reverse TCP Inline", "Python Stageless Reverse TCP", "Bash Stageless Reverse TCP"}),
			),
			mcp.WithString("shellType",
				mcp.Description("replace {shell} placeholder"),
				mcp.Enum([]string{"sh", "/bin/sh", "bash", "/bin/bash", "cmd", "powershell", "pwsh", "ash", "bsh", "csh", "ksh", "zsh", "pdksh", "tcsh", "mksh", "dash"}),
				mcp.Required(),
			),
			mcp.WithString("ip",
				mcp.Description("IP address for the reverse shell, replace {ip} placeholder"),
				mcp.Required(),
			),
			mcp.WithNumber("port",
				mcp.Description("Port number for the reverse shell, replace {port} placeholder"),
				mcp.Required(),
				mcp.Min(1),
				mcp.Max(65535),
			),
		), handleGenerateReverseShellCommand),
	)
}

func handleGenerateReverseShellCommand(s *MCPServer) server.ToolHandlerFunc {
	return func(
		ctx context.Context,
		request mcp.CallToolRequest,
	) (*mcp.CallToolResult, error) {
		var req ypb.GenerateReverseShellCommandRequest
		err := mapstructure.Decode(request.Params.Arguments, &req)
		if err != nil {
			return nil, utils.Wrap(err, "invalid argument")
		}
		req.CmdType = "All"
		req.System = "All"
		rsp, err := s.grpcClient.GenerateReverseShellCommand(ctx, &req)
		if err != nil {
			return nil, utils.Wrap(err, "failed to generate reverse shell command")
		}
		return NewCommonCallToolResult(rsp.Result)
	}
}
