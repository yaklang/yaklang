package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTTPASS_MITM_HijackFilter(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na"))
	token, token2 := uuid.NewString(), uuid.NewString()

	hijacked := false
	unexpectedHijacked := false

	RunMITMTestServerEx(client, ctx,
		func(stream ypb.Yak_MITMClient) {
			stream.Send(&ypb.MITMRequest{
				Host: mitmHost,
				Port: uint32(mitmPort),
			})
			stream.Send(&ypb.MITMRequest{
				SetAutoForward:   true,
				AutoForwardValue: true,
			})
			stream.Send(&ypb.MITMRequest{
				UpdateHijackFilter: true,
				HijackFilterData: &ypb.MITMFilterData{
					IncludeUri: []*ypb.FilterDataItem{
						{
							MatcherType: "word",
							Group:       []string{token},
						},
					},
				},
			})
		},
		func(stream ypb.Yak_MITMClient) {
			defer cancel()

			_, _, err := poc.DoGET(fmt.Sprintf("http://%s?a=%s", utils.HostPort(host, port), token), poc.WithProxy(proxy))
			require.NoError(t, err)
			_, _, err = poc.DoGET(fmt.Sprintf("http://%s?a=%s", utils.HostPort(host, port), token2), poc.WithProxy(proxy))
			require.NoError(t, err)
		}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
			if msg.GetMessage() != nil {
				return
			}
			if req := msg.GetRequest(); req != nil {
				a := lowhttp.GetHTTPRequestQueryParam(req, "a")
				if a == token {
					hijacked = true
				} else if a == token2 {
					unexpectedHijacked = true
				}
				// 直接Forward
				stream.Send(&ypb.MITMRequest{
					Id:      msg.GetId(),
					Forward: true,
				})

				// 设置回自动放行
				stream.Send(&ypb.MITMRequest{
					SetAutoForward:   true,
					AutoForwardValue: true,
				})
			}
		},
	)

	require.True(t, hijacked, "hijack filter not work")
	require.False(t, unexpectedHijacked, "hijack filter not work, unexpected request hijacked")
}
