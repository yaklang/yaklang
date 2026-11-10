package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/mcp"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

// MCP's set_mitm_filter uses this unary RPC, independently of the MITM stream.
func TestGRPCMUSTPASS_MITM_Filter_UnaryLiveUpdate(t *testing.T) {
	testMITMFilterLiveUpdate(t, false)
}

func TestGRPCMUSTPASS_MITM_Filter_MCPLiveUpdate(t *testing.T) {
	testMITMFilterLiveUpdate(t, true)
}

func testMITMFilterLiveUpdate(t *testing.T, viaMCP bool) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			client := isolateMITMTestSideEffects(t)
			var mcpServer *mcp.MCPServer
			if viaMCP {
				var err error
				mcpServer, err = mcp.NewMCPServer(mcp.WithEnableMITMToolSet())
				require.NoError(t, err)
				// Use MCP's production local-client path, not the client holding
				// the MITM stream. These are separate gRPC servers in one process,
				// sharing the project connection bound by the isolation helper.
				require.NoError(t, mcpServer.BindLocalGRPCClient())
				t.Cleanup(func() { mcpServer.Close() })
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				if strings.HasSuffix(r.URL.Path, ".js") {
					w.Header().Set("Content-Type", "application/javascript")
				}
				fmt.Fprint(w, "ok")
			}))
			defer target.Close()
			var feedback atomic.Pointer[ypb.SetMITMFilterRequest]
			var expected *ypb.MITMFilterData
			setFilter := func(data *ypb.MITMFilterData) {
				t.Helper()
				if viaMCP {
					encoded, err := json.Marshal(data)
					require.NoError(t, err)
					var filterData map[string]any
					require.NoError(t, json.Unmarshal(encoded, &filterData))
					result, err := mcp.CallBuiltinTool(mcpServer, ctx, "set_mitm_filter", map[string]any{"filterData": filterData})
					require.NoError(t, err)
					require.NotNil(t, result)
					require.False(t, result.IsError)
				} else {
					_, err := client.SetMITMFilter(ctx, &ypb.SetMITMFilterRequest{FilterData: data})
					require.NoError(t, err)
				}
				expected = data
			}
			setFilter(&ypb.MITMFilterData{})
			port := utils.GetRandomAvailableTCPPort()
			proxy := "http://" + utils.HostPort("127.0.0.1", port)
			checkTraffic := func(path string, want int) {
				t.Helper()
				token := uuid.NewString()
				_, err := lowhttp.HTTPWithoutRedirect(
					lowhttp.WithPacketBytes([]byte(fmt.Sprintf("GET %s?token=%s HTTP/1.1\r\nHost: %s\r\n\r\n", path, token, strings.TrimPrefix(target.URL, "http://")))),
					lowhttp.WithProxy(proxy),
				)
				require.NoError(t, err)
				waitMITMFlowWrites(t)
				require.Equal(t, want, yakit.QuickSearchMITMHTTPFlowCount(token), "capture count for %s", path)
				require.Eventually(t, func() bool {
					got := feedback.Load()
					return got != nil && proto.Equal(expected, got.FilterData)
				}, 2*time.Second, time.Millisecond, "the stream must report the active filter to the UI")
			}
			loaded := false
			onLoad := func() {
				defer cancel()
				loaded = true
				checkTraffic("/blocked", 1)
				setFilter(&ypb.MITMFilterData{ExcludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"/blocked"}}}})
				checkTraffic("/blocked", 0)
				checkTraffic("/allowed", 1)
				setFilter(&ypb.MITMFilterData{ExcludeMIME: []*ypb.FilterDataItem{{MatcherType: "mime", Group: []string{"text/plain"}}}})
				checkTraffic("/allowed", 0)
				setFilter(nil)
				checkTraffic("/blocked", 1)
				checkTraffic("/image.png", 1)
				if version == "v2" {
					setFilter(&ypb.MITMFilterData{FilterBundledStaticJS: true})
					checkTraffic("/static/js/main.js", 0)
					setFilter(&ypb.MITMFilterData{FilterBundledStaticJS: false})
					checkTraffic("/static/js/main.js", 1)
				}
				reset, err := client.ResetMITMFilter(ctx, &ypb.Empty{})
				require.NoError(t, err)
				expected = reset.FilterData
				checkTraffic("/image.png", 0)
				checkTraffic("/allowed", 1)
			}
			if version == "v1" {
				RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
					require.NoError(t, stream.Send(&ypb.MITMRequest{Host: "127.0.0.1", Port: uint32(port)}))
				}, func(ypb.Yak_MITMClient) { onLoad() }, func(_ ypb.Yak_MITMClient, response *ypb.MITMResponse) {
					if response.JustFilter {
						feedback.Store(&ypb.SetMITMFilterRequest{FilterData: response.FilterData})
					}
				})
			} else {
				RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
					require.NoError(t, stream.Send(&ypb.MITMV2Request{Host: "127.0.0.1", Port: uint32(port), DisableSystemProxy: true}))
				}, func(ypb.Yak_MITMV2Client) { onLoad() }, func(_ ypb.Yak_MITMV2Client, response *ypb.MITMV2Response) {
					if response.JustFilter {
						feedback.Store(&ypb.SetMITMFilterRequest{FilterData: response.FilterData})
					}
				})
			}
			require.True(t, loaded, "MITM server must start")
		})
	}
}
