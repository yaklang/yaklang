package yakgrpc

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestManualHijackManagerConditionalBehavior(t *testing.T) {
	for _, toManual := range []bool{false, true} {
		t.Run(fmt.Sprint(toManual), func(t *testing.T) {
			m := newManualHijackManager()
			m.setHijackToManual(toManual)
			require.Nil(t, m.register(&ypb.SingleManualHijackInfoMessage{}, false))
			matched := m.register(&ypb.SingleManualHijackInfoMessage{}, true)
			require.NotNil(t, matched)
			wantSource := ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL
			if toManual {
				wantSource = ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL_MANUAL
			}
			require.Equal(t, wantSource, matched.infoMessage.HijackTaskSource)

			// Register concurrent non-matches while the matching task is still pending.
			var wg sync.WaitGroup
			tasks := make(chan *manualHijackTask, 16)
			for i := 0; i < cap(tasks); i++ {
				wg.Add(1)
				go func() { defer wg.Done(); tasks <- m.register(&ypb.SingleManualHijackInfoMessage{}, false) }()
			}
			wg.Wait()
			close(tasks)
			for task := range tasks {
				if toManual {
					require.NotNil(t, task)
					require.Equal(t, wantSource, task.infoMessage.HijackTaskSource)
				} else {
					require.Nil(t, task)
				}
			}
			m.setCanRegister(false)
			require.Empty(t, m.getHijackingTaskInfo())
			_, open := <-matched.messageChan
			require.False(t, open, "returning to auto-forward must release pending tasks")
			require.Nil(t, m.register(&ypb.SingleManualHijackInfoMessage{}, false))
			require.Equal(t, wantSource, m.register(&ypb.SingleManualHijackInfoMessage{}, true).infoMessage.HijackTaskSource)

			m.setCanRegister(false)
			m.setHijackToManual(false)
			isolated := m.register(&ypb.SingleManualHijackInfoMessage{}, true)
			require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, isolated.infoMessage.HijackTaskSource)
			require.Nil(t, m.register(&ypb.SingleManualHijackInfoMessage{}, false))
			m.setCanRegister(false)
		})
	}
}

func TestGRPCMUSTPASS_MITM_ConditionalBehaviorPersistence(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, enabled := range []bool{true, false} {
		filter := &ypb.MITMFilterData{HijackToManual: enabled}
		_, err := client.SetMITMHijackFilter(ctx, &ypb.SetMITMFilterRequest{FilterData: filter})
		require.NoError(t, err)
		got, err := client.GetMITMHijackFilter(ctx, &ypb.Empty{})
		require.NoError(t, err)
		require.Equal(t, enabled, got.GetFilterData().GetHijackToManual())
		require.True(t, NewMITMFilter(got.FilterData).IsEmpty(), "the behavior alone must not enable interception")
	}
	_, err = client.SetMITMHijackFilter(ctx, &ypb.SetMITMFilterRequest{FilterData: &ypb.MITMFilterData{HijackToManual: true}})
	require.NoError(t, err)
	reset, err := client.ResetMITMHijackFilter(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.False(t, reset.GetFilterData().GetHijackToManual())
}

func TestGRPCMUSTPASS_MITM_ConditionalBehavior(t *testing.T) {
	for _, version := range []string{"v1", "v2"} {
		for _, persisted := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/persisted=%v", version, persisted), func(t *testing.T) {
				client, err := NewLocalClientWithTempDatabase(t)
				require.NoError(t, err)
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok"))
				target := "http://" + utils.HostPort(host, port)
				mitmPort := utils.GetRandomAvailableTCPPort()
				proxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
				filter := func(toManual bool) *ypb.MITMFilterData {
					return &ypb.MITMFilterData{HijackToManual: toManual, IncludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{"/login"}}}}
				}
				if persisted {
					_, err := client.SetMITMHijackFilter(ctx, &ypb.SetMITMFilterRequest{FilterData: filter(true)})
					require.NoError(t, err)
				}
				var seen []string // Written only by the receive loop, read after it exits.
				onLoad := func() {
					defer cancel()
					for _, path := range []string{"/before", "/login", "/after", "/reset", "/after-reset", "/login-again", "/disable", "/login-isolated", "/after-disabled"} {
						_, _, err := poc.DoGET(target+path, poc.WithProxy(proxy), poc.WithTimeout(5))
						require.NoError(t, err, path)
					}
				}
				onTask := func(req []byte, source ypb.MITMHijackTaskSource) string {
					path := lowhttp.GetHTTPRequestPath(req)
					seen = append(seen, path)
					want := ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL_MANUAL
					if path == "/login-isolated" {
						want = ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL
					}
					require.Equal(t, want, source, path)
					return path
				}
				if version == "v1" {
					RunMITMTestServerEx(client, ctx, func(stream ypb.Yak_MITMClient) {
						require.NoError(t, stream.Send(&ypb.MITMRequest{Host: "127.0.0.1", Port: uint32(mitmPort)}))
						if !persisted {
							require.NoError(t, stream.Send(&ypb.MITMRequest{UpdateHijackFilter: true, HijackFilterData: filter(true)}))
						}
					}, func(ypb.Yak_MITMClient) { onLoad() }, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
						if len(msg.GetRequest()) == 0 || msg.GetForResponse() {
							return
						}
						path := onTask(msg.Request, msg.HijackTaskSource)
						if path == "/disable" {
							require.NoError(t, stream.Send(&ypb.MITMRequest{UpdateHijackFilter: true, HijackFilterData: filter(false)}))
						}
						require.NoError(t, stream.Send(&ypb.MITMRequest{Id: msg.Id, Forward: true, SetAutoForward: path == "/reset" || path == "/disable", AutoForwardValue: true}))
					})
				} else {
					RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
						require.NoError(t, stream.Send(&ypb.MITMV2Request{Host: "127.0.0.1", Port: uint32(mitmPort)}))
						if !persisted {
							require.NoError(t, stream.Send(&ypb.MITMV2Request{UpdateHijackFilter: true, HijackFilterData: filter(true)}))
						}
					}, func(ypb.Yak_MITMV2Client) { onLoad() }, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
						if msg.GetManualHijackListAction() != Hijack_List_Add {
							return
						}
						for _, task := range msg.ManualHijackList {
							path := onTask(task.Request, task.HijackTaskSource)
							if path == "/disable" {
								require.NoError(t, stream.Send(&ypb.MITMV2Request{UpdateHijackFilter: true, HijackFilterData: filter(false)}))
							}
							if path == "/reset" || path == "/disable" {
								require.NoError(t, stream.Send(&ypb.MITMV2Request{SetAutoForward: true, AutoForwardValue: true}))
							} else {
								require.NoError(t, stream.Send(&ypb.MITMV2Request{ManualHijackControl: true, ManualHijackMessage: &ypb.SingleManualHijackControlMessage{TaskID: task.TaskID, Forward: true}}))
							}
						}
					})
				}
				require.Equal(t, []string{"/login", "/after", "/reset", "/login-again", "/disable", "/login-isolated"}, seen)
			})
		}
	}
}
