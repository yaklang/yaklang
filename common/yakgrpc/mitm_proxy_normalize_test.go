package yakgrpc

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

func TestNormalizeProxyURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string // 期望的 scheme://encodedUser:encodedPass@host:port 形式
		wantErr bool
	}{
		{
			name:  "密码含@",
			input: "socks5://user:pass@word@127.0.0.1:1080",
			want:  "socks5://user:pass%40word@127.0.0.1:1080",
		},
		{
			name:  "密码含#需fallback",
			input: "socks5://user:pass#frag@127.0.0.1:1080",
			want:  "socks5://user:pass%23frag@127.0.0.1:1080",
		},
		{
			name:  "密码含:",
			input: "http://u:p:w@127.0.0.1:8080",
			want:  "http://u:p%3Aw@127.0.0.1:8080",
		},
		{
			name:  "无认证",
			input: "http://127.0.0.1:8080",
			want:  "http://127.0.0.1:8080",
		},
		{
			name:  "仅用户名",
			input: "socks5://user@127.0.0.1:1080",
			want:  "socks5://user@127.0.0.1:1080",
		},
		{
			name:    "空串",
			input:   "",
			wantErr: true,
		},
		{
			name:    "无scheme",
			input:   "127.0.0.1:1080",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeProxyURL(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			// 解析结果并校验 host 与编码后的凭据
			u, err := url.Parse(got)
			require.NoError(t, err)
			_, port, err := utils.ParseStringToHostPort(u.Host)
			require.NoError(t, err)
			require.True(t, port > 0, "port should be valid")
			// 与期望字符串等价（可能顺序不同，直接比较）
			uWant, _ := url.Parse(tt.want)
			require.Equal(t, uWant.Host, u.Host, "host mismatch")
			if uWant.User != nil {
				require.NotNil(t, u.User)
				wu, _ := uWant.User.Password()
				gu, _ := u.User.Password()
				require.Equal(t, uWant.User.Username(), u.User.Username())
				require.Equal(t, wu, gu)
			}
		})
	}
}
