package netx

import (
	"os/exec"
	"runtime"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/stretchr/testify/require"
)

func TestFixProxy(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid host:port without scheme",
			input:    "127.0.0.1:1080",
			expected: "http://127.0.0.1:1080",
		},
		{
			name:     "valid host:port with socks5 scheme",
			input:    "socks5://127.0.0.1:1080",
			expected: "socks5://127.0.0.1:1080",
		},
		{
			name:     "host without port (invalid)",
			input:    "127.0.0.1",
			expected: "", // 修复后：返回空字符串
		},
		{
			name:     "socks5 scheme without port (invalid)",
			input:    "socks5://127.0.0.1",
			expected: "", // 修复后：返回空字符串
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FixProxy(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSet(t *testing.T) {
	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command(`osascript`, "-e", `do shell script "networksetup -setwebproxy Wi-Fi 127.0.0.1 8083; networksetup -setsecurewebproxy Wi-Fi 127.0.0.1 8083; networksetup -setsocksfirewallproxy Wi-Fi \"\" \"\"" with administrator privileges`)
		spew.Dump(cmd.Args)
		cmd.Args[0] = "AAAA"
		spew.Dump(cmd.Args)
		err := cmd.Run()
		if err != nil {
			panic(err)
		}
	}
}

//func TestSet2(t *testing.T) {
//	Set(SystemProxySetting{
//		Enabled:       false,
//		DefaultServer: "http://127.0.0.1:7890",
//	})
//}
