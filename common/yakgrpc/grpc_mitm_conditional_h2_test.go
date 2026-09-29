package yakgrpc

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestMITMDefaultFilterExcludesGoogleBeforeHijack(t *testing.T) {
	filter := NewMITMFilter(defaultMITMFilterData)
	for _, host := range []string{"google.com", "www.google.com", "accounts.google.com"} {
		require.False(t, filter.IsPassed("GET", host, "https://"+host+"/search?q=test", ""), host)
	}
	require.True(t, filter.IsPassed("GET", "www.baidu.com", "https://www.baidu.com/", ""))
}

// Reuse both browser-facing H2 tunnels across a live behavior change, including
// an ordinary filter exclusion. Changing hijack mode must not bypass that filter.
func TestGRPCMUSTPASS_MITM_ConditionalBehavior_H2Reuse(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		t.Run(version, func(t *testing.T) {
			origin := func() *httptest.Server {
				srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Cache-Control", "no-store")
					_, _ = io.WriteString(w, r.URL.Path)
				}))
				srv.EnableHTTP2 = true
				srv.StartTLS()
				t.Cleanup(srv.Close)
				return srv
			}
			matchedOrigin, otherOrigin := origin(), origin()
			matchedHost := strings.TrimPrefix(matchedOrigin.URL, "https://")
			otherHost := strings.TrimPrefix(otherOrigin.URL, "https://")
			client, err := NewLocalClientWithTempDatabase(t)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			port := utils.GetRandomAvailableTCPPort()
			proxyURL, err := url.Parse("http://" + utils.HostPort("127.0.0.1", port))
			require.NoError(t, err)
			transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
			defer transport.CloseIdleConnections()
			browser := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			connections := map[string]net.Conn{}
			intercepted := make(chan ypb.MITMHijackTaskSource, 16)
			filterAck := make(chan string, 16)
			var update func(bool, *ypb.MITMFilterData)
			observeFilter := func(data *ypb.MITMFilterData) {
				if rules := data.GetExcludeUri(); len(rules) == 1 && len(rules[0].Group) == 1 {
					filterAck <- rules[0].Group[0]
				}
			}
			request := func(target, path string, wantSource ypb.MITMHijackTaskSource) {
				req, err := http.NewRequestWithContext(ctx, "GET", target+path, nil)
				require.NoError(t, err)
				var info httptrace.GotConnInfo
				req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{GotConn: func(got httptrace.GotConnInfo) { info = got }}))
				rsp, err := browser.Do(req)
				require.NoError(t, err, path)
				body, err := io.ReadAll(rsp.Body)
				rsp.Body.Close()
				require.NoError(t, err)
				require.Equal(t, 2, rsp.ProtoMajor, "must exercise H2, not an H1 fallback")
				require.Equal(t, path, string(body))
				if previous, ok := connections[target]; ok {
					require.True(t, info.Reused, path)
					require.Same(t, previous, info.Conn, "requests must reuse the original browser tunnel")
				}
				connections[target] = info.Conn
				if wantSource == ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_UNSPECIFIED {
					select {
					case source := <-intercepted:
						t.Fatalf("%s unexpectedly intercepted: %v", path, source)
					default:
					}
				} else {
					select {
					case source := <-intercepted:
						require.Equal(t, wantSource, source, path)
					case <-ctx.Done():
						t.Fatal("missing hijack event: " + path)
					}
				}
			}
			onLoad := func() {
				defer cancel()
				configure := func(stage string, toManual, excludeOther bool) {
					// The filter response acknowledges all preceding stream commands; no sleeps.
					filter := &ypb.MITMFilterData{ExcludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{stage}}}}
					if excludeOther {
						filter.ExcludeHostnames = []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{otherHost}}}
					}
					update(toManual, filter)
					for {
						select {
						case ack := <-filterAck:
							if ack == stage {
								return
							}
						case <-ctx.Done():
							t.Fatal("filter update was not acknowledged")
						}
					}
				}
				configure("/never-match-stage-1", false, false)
				request(otherOrigin.URL, "/search-before", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_UNSPECIFIED)
				request(matchedOrigin.URL, "/matched-only", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL)
				request(otherOrigin.URL, "/search-isolated", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_UNSPECIFIED)
				configure("/never-match-stage-2", true, false)
				request(matchedOrigin.URL, "/trigger-manual", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL_MANUAL)
				request(otherOrigin.URL, "/search-manual", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL_MANUAL)
				configure("/never-match-stage-3", true, true)
				request(otherOrigin.URL, "/search-excluded", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_UNSPECIFIED)
				configure("/never-match-stage-4", true, false)
				request(otherOrigin.URL, "/search-unexcluded", ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL_MANUAL)
			}
			hijackFilter := func(toManual bool) *ypb.MITMFilterData {
				return &ypb.MITMFilterData{HijackToManual: toManual, IncludeHostnames: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{matchedHost}}}}
			}
			if version == "v1" {
				RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
					require.NoError(t, stream.Send(&ypb.MITMRequest{Host: "127.0.0.1", Port: uint32(port), EnableHttp2: true}))
					update = func(toManual bool, filter *ypb.MITMFilterData) {
						require.NoError(t, stream.Send(&ypb.MITMRequest{UpdateHijackFilter: true, HijackFilterData: hijackFilter(toManual)}))
						require.NoError(t, stream.Send(&ypb.MITMRequest{UpdateFilter: true, FilterData: filter}))
					}
				}, func(ypb.Yak_MITMClient) { onLoad() }, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
					if msg.GetJustFilter() {
						observeFilter(msg.FilterData)
					}
					if len(msg.GetRequest()) == 0 || msg.GetForResponse() {
						return
					}
					intercepted <- msg.HijackTaskSource
					require.NoError(t, stream.Send(&ypb.MITMRequest{Id: msg.Id, Forward: true}))
				})
			} else {
				RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
					require.NoError(t, stream.Send(&ypb.MITMV2Request{Host: "127.0.0.1", Port: uint32(port), EnableHttp2: true, DisableSystemProxy: true}))
					update = func(toManual bool, filter *ypb.MITMFilterData) {
						require.NoError(t, stream.Send(&ypb.MITMV2Request{UpdateHijackFilter: true, HijackFilterData: hijackFilter(toManual)}))
						require.NoError(t, stream.Send(&ypb.MITMV2Request{UpdateFilter: true, FilterData: filter}))
					}
				}, func(ypb.Yak_MITMV2Client) { onLoad() }, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
					if msg.GetJustFilter() {
						observeFilter(msg.FilterData)
					}
					if msg.GetManualHijackListAction() != Hijack_List_Add {
						return
					}
					for _, task := range msg.ManualHijackList {
						intercepted <- task.HijackTaskSource
						require.NoError(t, stream.Send(&ypb.MITMV2Request{ManualHijackControl: true, ManualHijackMessage: &ypb.SingleManualHijackControlMessage{TaskID: task.TaskID, Forward: true}}))
					}
				})
			}
		})
	}
}
