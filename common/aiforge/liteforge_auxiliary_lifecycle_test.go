package aiforge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
)

// The label is inherited by goroutines spawned by the real Coordinator. A
// parent Config is created outside the label and cannot affect these counts.
func pr5128LabeledLoops(t *testing.T, label string) (int, int, string) {
	t.Helper()
	var profile bytes.Buffer
	require.NoError(t, pprof.Lookup("goroutine").WriteTo(&profile, 1))
	var events, hotpatches int
	var matched strings.Builder
	for _, block := range strings.Split(profile.String(), "\n\n") {
		if !strings.Contains(block, `# labels: {"pr5128-lifecycle":"`+label+`"}`) {
			continue
		}
		fields := strings.Fields(block)
		if len(fields) == 0 {
			continue
		}
		count, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		if strings.Contains(block, "StartEventLoopEx.func1") {
			events += count
			matched.WriteString(block + "\n\n")
		}
		if strings.Contains(block, "StartHotPatchLoop.func1") {
			hotpatches += count
			matched.WriteString(block + "\n\n")
		}
	}
	return events, hotpatches, matched.String()
}

func pr5128AwaitLoopBaseline(t *testing.T, label string, baseEvents, baseHotpatches int, parent context.Context) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		events, hotpatches, stacks := pr5128LabeledLoops(t, label)
		if events == baseEvents && hotpatches == baseHotpatches {
			t.Logf("child loops returned to baseline: events=%d hotpatches=%d; parent err=%v", events, hotpatches, parent.Err())
			return
		}
		select {
		case <-tick.C:
		case <-deadline:
			t.Fatalf("child loops did not return to baseline: want events=%d hotpatches=%d, got events=%d hotpatches=%d, parent err=%v\n%s", baseEvents, baseHotpatches, events, hotpatches, parent.Err(), stacks)
		}
	}
}

func pr5128RunLabeled(ctx context.Context, label string, run func(context.Context)) {
	pprof.Do(ctx, pprof.Labels("pr5128-lifecycle", label), run)
}

func pr5128LifecycleConfig(ctx context.Context, single bool, callback aicommon.AICallbackType, extra ...aicommon.ConfigOption) *aicommon.Config {
	opts := []aicommon.ConfigOption{
		aicommon.WithSingleAIModelMode(single),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithAIAutoRetry(1),
		aicommon.WithAITransactionAutoRetry(1),
		aicommon.WithAIRetryWaitFunc(func(context.Context, time.Duration) error { return nil }),
		aicommon.WithSpeedPriorityAICallback(callback),
	}
	return aicommon.NewConfig(ctx, append(opts, extra...)...)
}

func pr5128LifecycleInvoke(cfg *aicommon.Config, ctx context.Context, onResult func(*aicommon.Action), onError func(error)) {
	cfg.ScheduleAuxiliaryTask(ctx, "pr5128-lifecycle", func() string { return "extract text" }, onResult,
		aicommon.WithAuxiliaryOutputs(aitool.WithStringParam("text")),
		aicommon.WithAuxiliaryOnError(onError),
		aicommon.WithAuxiliaryOpts(aicommon.WithLiteForgeDisableTimeline()),
	)
}

func TestPR5128_AuxiliaryLifecycle_SuccessWhileParentAlive(t *testing.T) {
	previous := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(nil)
	t.Cleanup(func() { consts.SetTieredAIConfig(previous) })
	for _, single := range []bool{false, true} {
		name := "ordinary"
		if single {
			name = "single-model"
		}
		t.Run(name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			var calls, successes atomic.Int32
			cfg := pr5128LifecycleConfig(parent, single, func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				calls.Add(1)
				response := c.NewAIResponse()
				response.EmitOutputStream(strings.NewReader(`{"@action":"pr5128-lifecycle","text":"complete"}`))
				response.Close()
				return response, nil
			})
			label := "success-" + name
			baseEvents, baseHotpatches, _ := pr5128LabeledLoops(t, label)
			for round := 1; round <= 12; round++ {
				var gotError error
				pr5128RunLabeled(parent, label, func(labeled context.Context) {
					pr5128LifecycleInvoke(cfg, labeled, func(action *aicommon.Action) {
						successes.Add(1)
						require.Equal(t, "complete", action.GetString("text"))
					}, func(err error) { gotError = err })
				})
				require.NoError(t, gotError)
				require.NoError(t, parent.Err())
				require.EqualValues(t, round, calls.Load())
				require.EqualValues(t, round, successes.Load())
				pr5128AwaitLoopBaseline(t, label, baseEvents, baseHotpatches, parent)
			}
		})
	}
}

func TestPR5128_AuxiliaryLifecycle_ErrorWhileParentAlive(t *testing.T) {
	for _, failure := range []string{"provider error", "invalid action"} {
		t.Run(failure, func(t *testing.T) {
			parent, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var calls, successes atomic.Int32
			cfg := pr5128LifecycleConfig(parent, false, func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				calls.Add(1)
				if failure == "provider error" {
					return nil, errors.New("local provider failed")
				}
				response := c.NewAIResponse()
				response.EmitOutputStream(strings.NewReader(`{"@action":"wrong","text":"ignored"}`))
				response.Close()
				return response, nil
			}, aicommon.WithAITransactionAutoRetry(2))
			label := "error-" + failure
			baseEvents, baseHotpatches, _ := pr5128LabeledLoops(t, label)
			var gotError error
			pr5128RunLabeled(parent, label, func(labeled context.Context) {
				pr5128LifecycleInvoke(cfg, labeled, func(*aicommon.Action) { successes.Add(1) }, func(err error) { gotError = err })
			})
			require.Error(t, gotError)
			require.Zero(t, successes.Load())
			require.EqualValues(t, 2, calls.Load())
			require.NoError(t, parent.Err())
			pr5128AwaitLoopBaseline(t, label, baseEvents, baseHotpatches, parent)
		})
	}
}

func TestPR5128_AuxiliaryLifecycle_RequestCancellation(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelParent()
	request, cancelRequest := context.WithCancel(parent)
	defer cancelRequest()
	started := make(chan struct{})
	finished := make(chan struct{})
	var calls, successes atomic.Int32
	var gotError error
	cfg := pr5128LifecycleConfig(parent, false, func(_ aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls.Add(1)
		close(started)
		<-req.GetContext().Done()
		return nil, req.GetContext().Err()
	})
	label := "request-cancel"
	baseEvents, baseHotpatches, _ := pr5128LabeledLoops(t, label)
	pr5128RunLabeled(parent, label, func(context.Context) {
		go func() {
			defer close(finished)
			pr5128LifecycleInvoke(cfg, request, func(*aicommon.Action) { successes.Add(1) }, func(err error) { gotError = err })
		}()
	})
	select {
	case <-started:
	case <-parent.Done():
		t.Fatal("request did not start")
	}
	cancelRequest()
	select {
	case <-finished:
	case <-parent.Done():
		t.Fatal("cancelled request did not finish")
	}
	require.ErrorIs(t, gotError, context.Canceled)
	require.EqualValues(t, 1, calls.Load())
	require.Zero(t, successes.Load())
	require.NoError(t, parent.Err())
	pr5128AwaitLoopBaseline(t, label, baseEvents, baseHotpatches, parent)
}

func TestPR5128_AuxiliaryLifecycle_DrainBeforeCleanup(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	started := make(chan struct{})
	allowTail := make(chan struct{})
	finished := make(chan struct{})
	var gotText string
	var gotError error
	cfg := pr5128LifecycleConfig(parent, false, func(c aicommon.AICallerConfigIf, _ *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		response := c.NewAIResponse()
		response.EmitOutputStream(reader)
		response.Close()
		return response, nil
	})
	label := "drain"
	baseEvents, baseHotpatches, _ := pr5128LabeledLoops(t, label)
	go func() {
		_, _ = io.WriteString(writer, `{"@action":"pr5128-lifecycle","text":"first`)
		select {
		case <-allowTail:
			_, _ = io.WriteString(writer, ` second"}`)
		case <-parent.Done():
		}
		_ = writer.Close()
	}()
	pr5128RunLabeled(parent, label, func(labeled context.Context) {
		go func() {
			defer close(finished)
			cfg.ScheduleAuxiliaryTask(labeled, "pr5128-lifecycle", func() string { return "extract text" },
				func(action *aicommon.Action) { gotText = action.GetString("text") },
				aicommon.WithAuxiliaryOutputs(aitool.WithStringParam("text")),
				aicommon.WithAuxiliaryOnError(func(err error) { gotError = err }),
				aicommon.WithAuxiliaryOpts(aicommon.WithLiteForgeDisableTimeline(),
					aicommon.WithGeneralConfigStreamableFieldEmitterCallback([]string{"text"},
						func(_ string, field io.Reader, _ *aicommon.Emitter) {
							close(started)
							_, _ = io.Copy(io.Discard, field)
						})),
			)
		}()
	})
	select {
	case <-started:
	case <-parent.Done():
		t.Fatal("field stream did not start")
	}
	events, hotpatches, stacks := pr5128LabeledLoops(t, label)
	require.Greater(t, events, baseEvents, "child event loop was not observed while the field stream was active: %s", stacks)
	require.Greater(t, hotpatches, baseHotpatches, "child hotpatch loop was not observed while the field stream was active: %s", stacks)
	select {
	case <-finished:
		t.Fatal("auxiliary result was delivered before the final field segment")
	default:
	}
	close(allowTail)
	select {
	case <-finished:
	case <-parent.Done():
		t.Fatal("field stream did not finish")
	}
	require.NoError(t, gotError)
	require.Equal(t, "first second", gotText)
	require.NoError(t, parent.Err())
	pr5128AwaitLoopBaseline(t, label, baseEvents, baseHotpatches, parent)
}

func TestPR5128_SpeedLoop_LifecycleAcrossRounds(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var calls, handled atomic.Int32
	cfg := pr5128LifecycleConfig(parent, false, func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		calls.Add(1)
		require.Equal(t, "react-loop:pr5128-two-rounds", req.GetCallerLabel())
		response := c.NewAIResponse()
		response.EmitOutputStream(strings.NewReader(`{"action":"accept","human_readable_thought":"ready"}`))
		response.Close()
		return response, nil
	})
	invoker := mock.NewMockInvoker(parent)
	invoker.SetConfig(cfg)
	label := "speed-rounds"
	baseEvents, baseHotpatches, _ := pr5128LabeledLoops(t, label)
	loop, err := reactloops.NewReActLoop("pr5128-two-rounds", invoker,
		reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
		reactloops.WithUseSpeedPriorityAICallback(true),
		reactloops.WithDisablePeriodicVerification(true),
		reactloops.WithMaxIterations(2),
		reactloops.WithRegisterLoopAction("require_tool", "unused tool route", nil, nil, nil),
		reactloops.WithRegisterLoopAction("accept", "accept result", nil, nil,
			func(_ *reactloops.ReActLoop, _ *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
				pr5128AwaitLoopBaseline(t, label, baseEvents, baseHotpatches, parent)
				if handled.Add(1) == 1 {
					operator.Continue()
				} else {
					operator.Exit()
				}
			}),
	)
	require.NoError(t, err)
	var executeErr error
	pr5128RunLabeled(parent, label, func(labeled context.Context) {
		task := aicommon.NewStatefulTaskBase("pr5128-task", "two rounds", labeled, cfg.GetEmitter())
		executeErr = loop.ExecuteWithExistedTask(task)
	})
	require.NoError(t, executeErr)
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 2, handled.Load())
	require.NoError(t, parent.Err())
	pr5128AwaitLoopBaseline(t, label, baseEvents, baseHotpatches, parent)
}
