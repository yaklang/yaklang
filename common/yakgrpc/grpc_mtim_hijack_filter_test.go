package yakgrpc

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITM_HijackFilter(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na"))
	token := uuid.NewString()

	var hijackedRequests atomic.Int32
	var hijackedResponses atomic.Int32
	var unexpectedHijacked atomic.Int32
	var responseRequestSeen atomic.Bool

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

			target := "http://" + utils.HostPort(host, port)
			_, _, err := poc.DoGET(target+"/assets/app.js", poc.WithProxy(proxy))
			require.NoError(t, err, "a non-match before the first hit should pass silently")
			_, _, err = poc.DoPOST(
				fmt.Sprintf("%s/account/login?case=response&token=%s", target, token),
				poc.WithProxy(proxy),
				poc.WithBody("username=admin"),
			)
			require.NoError(t, err, "a matching POST and its hijacked response should complete")
			_, _, err = poc.DoGET(target+"/health", poc.WithProxy(proxy))
			require.NoError(t, err, "a non-match after a hit should still pass silently")
			_, _, err = poc.DoGET(
				fmt.Sprintf("%s/account/login?case=request-only&token=%s", target, token),
				poc.WithProxy(proxy),
			)
			require.NoError(t, err, "a repeated match should still be intercepted")
			_, _, err = poc.DoPOST(target+"/api/profile", poc.WithProxy(proxy), poc.WithBody("name=test"))
			require.NoError(t, err, "a different-method non-match should pass after repeated hits")
		}, func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
			if msg.GetMessage() != nil {
				return
			}
			if msg.GetForResponse() {
				require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, msg.GetHijackTaskSource())
				hijackedResponses.Add(1)
				stream.Send(&ypb.MITMRequest{
					ResponseId: msg.GetResponseId(),
					Forward:    true,
				})
				return
			}
			if req := msg.GetRequest(); req != nil {
				require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, msg.GetHijackTaskSource())
				if lowhttp.GetHTTPRequestQueryParam(req, "token") != token {
					unexpectedHijacked.Add(1)
				} else {
					if lowhttp.GetHTTPRequestQueryParam(req, "case") == "response" && responseRequestSeen.CompareAndSwap(false, true) {
						hijackedRequests.Add(1)
						stream.Send(&ypb.MITMRequest{
							Id:             msg.GetId(),
							HijackResponse: true,
						})
						return
					}
					if lowhttp.GetHTTPRequestQueryParam(req, "case") != "response" {
						hijackedRequests.Add(1)
					}
				}
				stream.Send(&ypb.MITMRequest{
					Id:      msg.GetId(),
					Forward: true,
				})
			}
		},
	)

	require.EqualValues(t, 2, hijackedRequests.Load(), "both matching requests should be hijacked")
	require.EqualValues(t, 1, hijackedResponses.Load(), "the selected matching response should be hijacked")
	require.Zero(t, unexpectedHijacked.Load(), "non-matching requests must never be hijacked")
}

// TestGRPCMUSTPASS_MITMFilter_RuleName 测试 FilterDataItem.RuleName 在 MITM 过滤器和劫持过滤器中的持久化
func TestGRPCMUSTPASS_MITMFilter_RuleName(t *testing.T) {
	local, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx := utils.TimeoutContextSeconds(5)

	ruleName := uuid.NewString()
	config := &ypb.SetMITMFilterRequest{
		FilterData: &ypb.MITMFilterData{
			ExcludeHostnames: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_GLOB,
					Group:       []string{"*.google.com", "*gstatic.com"},
					RuleName:    ruleName,
				},
			},
		},
	}

	// 测试 MITM 过滤器 RuleName 持久化
	_, err = local.SetMITMFilter(ctx, config)
	require.NoError(t, err)
	gotFilter, err := local.GetMITMFilter(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.NotEmpty(t, gotFilter.FilterData.GetExcludeHostnames(), "ExcludeHostnames should not be empty")
	require.Equal(t, ruleName, gotFilter.FilterData.GetExcludeHostnames()[0].GetRuleName(),
		"MITM filter RuleName should persist after Set/Get")

	// 测试劫持过滤器 RuleName 持久化
	_, err = local.SetMITMHijackFilter(ctx, config)
	require.NoError(t, err)
	gotHijack, err := local.GetMITMHijackFilter(ctx, &ypb.Empty{})
	require.NoError(t, err)
	require.NotEmpty(t, gotHijack.FilterData.GetExcludeHostnames(), "ExcludeHostnames should not be empty")
	require.Equal(t, ruleName, gotHijack.FilterData.GetExcludeHostnames()[0].GetRuleName(),
		"Hijack filter RuleName should persist after Set/Get")
}

func TestGRPCMUSTPASS_Get_Set_HijackFilter(t *testing.T) {
	local, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)

	compareFilterDataItem := func(x, y *ypb.FilterDataItem) bool {
		return x.MatcherType == y.MatcherType && reflect.DeepEqual(x.Group, y.Group) && x.RuleName == y.RuleName
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
	ruleNameInclude := uuid.NewString()
	ruleNameExclude := uuid.NewString()
	want := &ypb.SetMITMFilterRequest{
		FilterData: &ypb.MITMFilterData{
			IncludeHostnames: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_GLOB,
					Group: []string{
						uuid.NewString(),
						uuid.NewString(),
					},
					RuleName: ruleNameInclude,
				},
			},
			ExcludeHostnames: []*ypb.FilterDataItem{
				{
					MatcherType: httptpl.MATCHER_TYPE_GLOB,
					Group: []string{
						uuid.NewString(),
						uuid.NewString(),
					},
					RuleName: ruleNameExclude,
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

func TestGRPCMUSTPASS_MITMV2_HijackFilter(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na"))
	token := uuid.NewString()

	var hijackedRequests atomic.Int32
	var hijackedResponses atomic.Int32
	var unexpectedHijacked atomic.Int32

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

			target := "http://" + utils.HostPort(host, port)
			_, _, err := poc.DoGET(target+"/assets/app.js", poc.WithProxy(proxy))
			require.NoError(t, err, "a non-match before the first hit should pass silently")
			_, _, err = poc.DoPOST(
				fmt.Sprintf("%s/account/login?case=response&token=%s", target, token),
				poc.WithProxy(proxy),
				poc.WithBody("username=admin"),
			)
			require.NoError(t, err, "a matching POST and its hijacked response should complete")
			_, _, err = poc.DoGET(target+"/health", poc.WithProxy(proxy))
			require.NoError(t, err, "a non-match after a hit should still pass silently")
			_, _, err = poc.DoGET(
				fmt.Sprintf("%s/account/login?case=request-only&token=%s", target, token),
				poc.WithProxy(proxy),
			)
			require.NoError(t, err, "a repeated match should still be intercepted")
			_, _, err = poc.DoPOST(target+"/api/profile", poc.WithProxy(proxy), poc.WithBody("name=test"))
			require.NoError(t, err, "a different-method non-match should pass after repeated hits")
		}, func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
			if msg.GetMessage() != nil {
				return
			}
			if msg.GetManualHijackListAction() == Hijack_List_Add {
				require.GreaterOrEqual(t, len(msg.ManualHijackList), 1)
				hijackTask := msg.ManualHijackList[0]
				require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, hijackTask.GetHijackTaskSource())
				req := hijackTask.Request
				if lowhttp.GetHTTPRequestQueryParam(req, "token") != token {
					unexpectedHijacked.Add(1)
				} else {
					hijackedRequests.Add(1)
				}
				stream.Send(&ypb.MITMV2Request{
					ManualHijackControl: true,
					ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
						TaskID:         hijackTask.TaskID,
						Forward:        true,
						HijackResponse: lowhttp.GetHTTPRequestQueryParam(req, "case") == "response",
					},
				})
			}
			if msg.GetManualHijackListAction() == Hijack_List_Update {
				require.GreaterOrEqual(t, len(msg.ManualHijackList), 1)
				hijackTask := msg.ManualHijackList[0]
				require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, hijackTask.GetHijackTaskSource())
				if hijackTask.GetStatus() == Hijack_Status_Response {
					hijackedResponses.Add(1)
					stream.Send(&ypb.MITMV2Request{
						ManualHijackControl: true,
						ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
							TaskID:  hijackTask.TaskID,
							Forward: true,
						},
					})
				}
			}
		},
	)

	require.EqualValues(t, 2, hijackedRequests.Load(), "both matching requests should be hijacked")
	require.EqualValues(t, 1, hijackedResponses.Load(), "the selected matching response should be hijacked")
	require.Zero(t, unexpectedHijacked.Load(), "non-matching requests must never be hijacked")
}

func exerciseConditionalHijackConcurrentTraffic(
	t *testing.T,
	target string,
	proxy string,
	token string,
	matchedSeen <-chan struct{},
	releaseMatched chan struct{},
) {
	t.Helper()

	matchingDone := make(chan error, 1)
	go func() {
		_, _, err := poc.DoGET(
			fmt.Sprintf("%s/account/login?token=%s", target, token),
			poc.WithProxy(proxy),
			poc.WithTimeout(15),
		)
		matchingDone <- err
	}()

	select {
	case <-matchedSeen:
	case <-time.After(5 * time.Second):
		close(releaseMatched)
		require.FailNow(t, "the matching request was not conditionally hijacked")
	}

	nonMatchingPaths := []string{
		"/assets/app.js",
		"/assets/app.css?cache=1",
		"/health",
		"/api/profile?page=2",
		"/favicon.ico",
	}
	nonMatchingErrors := make(chan error, len(nonMatchingPaths))
	var nonMatchingWait sync.WaitGroup
	for _, path := range nonMatchingPaths {
		path := path
		nonMatchingWait.Add(1)
		go func() {
			defer nonMatchingWait.Done()
			_, _, err := poc.DoGET(target+path, poc.WithProxy(proxy), poc.WithTimeout(3))
			nonMatchingErrors <- err
		}()
	}

	nonMatchingDone := make(chan struct{})
	go func() {
		nonMatchingWait.Wait()
		close(nonMatchingDone)
	}()

	nonMatchingTimedOut := false
	select {
	case <-nonMatchingDone:
	case <-time.After(5 * time.Second):
		nonMatchingTimedOut = true
	}
	close(releaseMatched)

	require.False(t, nonMatchingTimedOut, "non-matches must finish while a matching request remains intercepted")
	if !nonMatchingTimedOut {
		close(nonMatchingErrors)
		for err := range nonMatchingErrors {
			require.NoError(t, err, "concurrent non-matching traffic should pass silently")
		}
	}

	select {
	case err := <-matchingDone:
		require.NoError(t, err, "the matching request should complete after it is released")
	case <-time.After(5 * time.Second):
		require.FailNow(t, "the matching request did not complete after release")
	}
}

func TestGRPCMUSTPASS_MITM_HijackFilter_ConcurrentIsolation(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na"))
	target := "http://" + utils.HostPort(host, port)
	token := uuid.NewString()
	matchedSeen := make(chan struct{})
	releaseMatched := make(chan struct{})
	var matchedOnce sync.Once
	var unexpectedHijacked atomic.Int32

	RunMITMTestServerEx(client, ctx,
		func(stream ypb.Yak_MITMClient) {
			stream.Send(&ypb.MITMRequest{Host: mitmHost, Port: uint32(mitmPort)})
			stream.Send(&ypb.MITMRequest{SetAutoForward: true, AutoForwardValue: true})
			stream.Send(&ypb.MITMRequest{
				UpdateHijackFilter: true,
				HijackFilterData: &ypb.MITMFilterData{
					IncludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{token}}},
				},
			})
		},
		func(stream ypb.Yak_MITMClient) {
			defer cancel()
			exerciseConditionalHijackConcurrentTraffic(t, target, proxy, token, matchedSeen, releaseMatched)
		},
		func(stream ypb.Yak_MITMClient, msg *ypb.MITMResponse) {
			if msg.GetMessage() != nil || msg.GetForResponse() || msg.GetRequest() == nil {
				return
			}
			if lowhttp.GetHTTPRequestQueryParam(msg.GetRequest(), "token") == token {
				require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, msg.GetHijackTaskSource())
				matchedOnce.Do(func() { close(matchedSeen) })
				<-releaseMatched
			} else {
				unexpectedHijacked.Add(1)
			}
			stream.Send(&ypb.MITMRequest{Id: msg.GetId(), Forward: true})
		},
	)

	require.Zero(t, unexpectedHijacked.Load(), "a live conditional task must not capture concurrent non-matches")
}

func TestGRPCMUSTPASS_MITMV2_HijackFilter_ConcurrentIsolation(t *testing.T) {
	client, err := NewLocalClientWithTempDatabase(t)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(40))
	mitmHost, mitmPort := "127.0.0.1", utils.GetRandomAvailableTCPPort()
	proxy := "http://" + utils.HostPort(mitmHost, mitmPort)
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\nContent-Length: 1\r\n\r\na"))
	target := "http://" + utils.HostPort(host, port)
	token := uuid.NewString()
	matchedSeen := make(chan struct{})
	releaseMatched := make(chan struct{})
	var matchedOnce sync.Once
	var unexpectedHijacked atomic.Int32

	RunMITMV2TestServerEx(client, ctx,
		func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{Host: mitmHost, Port: uint32(mitmPort)})
			stream.Send(&ypb.MITMV2Request{SetAutoForward: true, AutoForwardValue: true})
			stream.Send(&ypb.MITMV2Request{
				UpdateHijackFilter: true,
				HijackFilterData: &ypb.MITMFilterData{
					IncludeUri: []*ypb.FilterDataItem{{MatcherType: "word", Group: []string{token}}},
				},
			})
		},
		func(stream ypb.Yak_MITMV2Client) {
			defer cancel()
			exerciseConditionalHijackConcurrentTraffic(t, target, proxy, token, matchedSeen, releaseMatched)
		},
		func(stream ypb.Yak_MITMV2Client, msg *ypb.MITMV2Response) {
			if msg.GetMessage() != nil || msg.GetManualHijackListAction() != Hijack_List_Add {
				return
			}
			require.GreaterOrEqual(t, len(msg.GetManualHijackList()), 1)
			hijackTask := msg.GetManualHijackList()[0]
			if lowhttp.GetHTTPRequestQueryParam(hijackTask.GetRequest(), "token") == token {
				require.Equal(t, ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL, hijackTask.GetHijackTaskSource())
				matchedOnce.Do(func() { close(matchedSeen) })
				<-releaseMatched
			} else {
				unexpectedHijacked.Add(1)
			}
			stream.Send(&ypb.MITMV2Request{
				ManualHijackControl: true,
				ManualHijackMessage: &ypb.SingleManualHijackControlMessage{
					TaskID:  hijackTask.GetTaskID(),
					Forward: true,
				},
			})
		},
	)

	require.Zero(t, unexpectedHijacked.Load(), "a live conditional task must not capture concurrent non-matches")
}

func TestMITMHijackTaskSource(t *testing.T) {
	require.Equal(t,
		ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_MANUAL,
		resolveMITMHijackTaskSource(false),
	)
	require.Equal(t,
		ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL,
		resolveMITMHijackTaskSource(true),
	)
}

func TestManualHijackManagerTaskSource(t *testing.T) {
	manager := newManualHijackManager()
	manager.setCanRegister(true)

	manualTask := manager.register(&ypb.SingleManualHijackInfoMessage{}, true)
	require.NotNil(t, manualTask)
	require.Equal(t,
		ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_MANUAL,
		manualTask.infoMessage.GetHijackTaskSource(),
	)
	manager.unRegister(manualTask.taskID)

	manager.setCanRegister(false)
	conditionalTask := manager.register(&ypb.SingleManualHijackInfoMessage{}, true)
	require.NotNil(t, conditionalTask)
	require.Equal(t,
		ypb.MITMHijackTaskSource_MITM_HIJACK_TASK_SOURCE_CONDITIONAL,
		conditionalTask.infoMessage.GetHijackTaskSource(),
	)
	manager.unRegister(conditionalTask.taskID)
}
