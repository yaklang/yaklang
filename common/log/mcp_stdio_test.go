package log

import "testing"

func TestIsMCPStdioCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "yak default", args: []string{"yak", "mcp"}, want: true},
		{name: "yak explicit stdio", args: []string{"yak", "mcp", "--transport", "stdio"}, want: true},
		{name: "yak explicit stdio equals", args: []string{"yak", "mcp", "--transport=STDIO"}, want: true},
		{name: "yak streamable http", args: []string{"yak", "mcp", "--transport", "streamable_http"}, want: false},
		{name: "yak sse", args: []string{"yak", "mcp", "--transport=sse"}, want: false},
		{name: "unrelated yak command", args: []string{"yak", "grpc"}, want: false},
		{name: "standalone unix", args: []string{"/usr/local/bin/mcp", "--transport", "stdio"}, want: true},
		{name: "standalone windows", args: []string{`C:\\tools\\mcp.exe`, "--transport=stdio"}, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isMCPStdioCommand(test.args); got != test.want {
				t.Fatalf("isMCPStdioCommand(%q) = %v, want %v", test.args, got, test.want)
			}
		})
	}
}
