package netx

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestIsTLSService(t *testing.T) {
	mockGMHost, mockGMPort := utils.DebugMockOnlyGMHTTP(context.Background(), nil)

	addr := utils.HostPort(mockGMHost, mockGMPort)
	type args struct {
		addr    string
		proxies []string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "mysql",
			args: args{
				addr: "192.168.3.113:3306",
			},
			want: false,
		},
		{
			name: "http",
			args: args{
				addr: "47.120.44.219:8087",
			},
			want: false,
		},
		{
			name: "only gmtls handshake failure",
			args: args{
				addr: addr,
			},
			want: true,
		},
		{
			name: "tls",
			args: args{
				addr: "114.251.196.39:443",
			},
			want: true,
		},
		{
			name: "only gmtls tls: protocol version not supported",
			args: args{
				addr: "ebssec.boc.cn:443",
			},
			want: true,
		},
		{
			name: "sm2 unsupported elliptic curve",
			args: args{
				addr: "113.96.111.24:4441",
			},
			want: true,
		},
		{
			name: "test2 ",
			args: args{
				addr: "113.96.111.24:443",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTLSService(tt.args.addr, tt.args.proxies...); got != tt.want {
				t.Errorf("IsTLSService() = %v, want %v", got, tt.want)
			}
		})
	}
}
