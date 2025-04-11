package yakgrpc

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTTPASS_MITM_HijackFilter(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
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

func TestGRPCMUSTPASS_Get_Set_HijackFilter(t *testing.T) {
	local, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	compareFilterDataItem := func(x, y *ypb.FilterDataItem) bool {
		return x.MatcherType == y.MatcherType && reflect.DeepEqual(x.Group, y.Group)
	}
	compareFilterDataItems := func(x, y []*ypb.FilterDataItem) bool {
		if len(x) != len(y) {
			return false
		}
		for i := 0; i < len(x); i++ {
			if !compareFilterDataItem(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	compareFilterData := func(x, y *ypb.MITMFilterData) bool {
		if !compareFilterDataItems(x.ExcludeHostnames, y.ExcludeHostnames) {
			return false
		}
		if !compareFilterDataItems(x.IncludeHostnames, y.IncludeHostnames) {
			return false
		}
		if !compareFilterDataItems(x.ExcludeSuffix, y.ExcludeSuffix) {
			return false
		}
		if !compareFilterDataItems(x.IncludeHostnames, y.IncludeHostnames) {
			return false
		}
		if !compareFilterDataItems(x.ExcludeUri, y.ExcludeUri) {
			return false
		}
		if !compareFilterDataItems(x.IncludeUri, y.IncludeUri) {
			return false
		}

		if !compareFilterDataItems(x.ExcludeMIME, y.ExcludeMIME) {
			return false
		}
		if !compareFilterDataItems(x.ExcludeMethods, y.ExcludeMethods) {
			return false
		}
		return true
	}

	ctx := utils.TimeoutContextSeconds(5)
	want := &ypb.SetMITMFilterRequest{
		FilterData: &ypb.MITMFilterData{
			IncludeHostnames: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_GLOB,
					Group: []string{
						uuid.NewString(),
						uuid.NewString(),
					},
				},
			},
			ExcludeHostnames: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_GLOB,
					Group: []string{
						uuid.NewString(),
						uuid.NewString(),
					},
				},
			},
			IncludeSuffix: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
					Group: []string{
						uuid.NewString(),
						uuid.NewString(),
					},
				},
			},
			ExcludeSuffix: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_SUFFIX,
					Group: []string{
						uuid.NewString(),
						uuid.NewString(),
					},
				},
			},
		},
	}

	_, err = local.SetMITMHijackFilter(ctx, want)
	require.NoError(t, err)
	got, err := local.GetMITMHijackFilter(ctx, &ypb.Empty{})

	require.Truef(t, compareFilterData(want.FilterData, got.FilterData), "got:\n%s\n\nwant:\n%s", spew.Sdump(got), spew.Sdump(want))
}

func TestGRPCMUSTTPASS_MITMV2_HijackFilter(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na"))
	token, token2 := uuid.NewString(), uuid.NewString()

	hijacked := false
	unexpectedHijacked := false

	RunMITMV2TestServerEx(client, ctx,
		func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{
				Host: mitmHost,
				Port: uint32(mitmPort),
			})
			stream.Send(&ypb.MITMV2Request{
				SetAutoForward:   true,
				AutoForwardValue: true,
			})
			stream.Send(&ypb.MITMV2Request{
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
		func(stream ypb.Yak_MITMV2Client) {
			defer cancel()

			_, _, err := poc.DoGET(fmt.Sprintf("http://%s?a=%s", utils.HostPort(host, port), token), poc.WithProxy(proxy))
			require.NoError(t, err)
			_, _, err = poc.DoGET(fmt.Sprintf("http://%s?a=%s", utils.HostPort(host, port), token2), poc.WithProxy(proxy))
			require.NoError(t, err)
		}, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
			if msg.GetMessage() != nil {
				return
			}
			if msg.GetManualHijackListAction() == "add" {
				require.GreaterOrEqual(t, len(msg.ManualHijackList), 1)
				hijackTask := msg.ManualHijackList[0]

				req := hijackTask.Request

				a := lowhttp.GetHTTPRequestQueryParam(req, "a")
				if a == token {
					hijacked = true
				} else if a == token2 {
					unexpectedHijacked = true
				}
				// 直接Forward
				stream.Send(&ypb.MITMV2Request{
					ManualHijackControl: true,
					ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
						TaskID:  hijackTask.TaskID,
						Forward: true,
					},
				})

				// 设置回自动放行
				stream.Send(&ypb.MITMV2Request{
					SetAutoForward:   true,
					AutoForwardValue: true,
				})
			}
		},
	)

	require.True(t, hijacked, "hijack filter not work")
	require.False(t, unexpectedHijacked, "hijack filter not work, unexpected request hijacked")
}
