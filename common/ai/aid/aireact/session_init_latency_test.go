package aireact

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReActFirstAnswerDoesNotWaitForSessionNaming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	namingStarted := make(chan struct{})
	answer := make(chan struct{})
	var namingOnce, answerOnce sync.Once
	var calls atomic.Int32
	r, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableSessionTitleGeneration(false),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if calls.Add(1) > 1 {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return mockedFreeInputOutput(c, "fast-first-answer")
		}),
		aicommon.WithSpeedPriorityAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			namingOnce.Do(func() { close(namingStarted) })
			<-ctx.Done()
			return nil, ctx.Err()
		}),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			if e.NodeId == "re-act-loop-answer-payload" && len(e.StreamDelta) > 0 {
				answerOnce.Do(func() { close(answer) })
			}
		}),
	)
	require.NoError(t, err)
	defer func() {
		cancel()
		r.Wait()
		if r.config.IsWorkDirReady() {
			_ = os.RemoveAll(r.config.GetOrCreateWorkDir())
		}
	}()
	started := time.Now()
	require.NoError(t, r.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "你好"}))
	select {
	case <-namingStarted:
	case <-time.After(3 * time.Second):
		t.Fatal("session naming did not start")
	}
	select {
	case <-answer:
		// The naming provider is still blocked until test cleanup cancels it.
		t.Logf("first answer arrived after %s while session naming was blocked", time.Since(started))
	case <-time.After(3 * time.Second):
		t.Fatal("first answer waited for the session naming provider")
	}
}

func TestReActSessionNamingStillSupportsSyncOptIn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var calls atomic.Int32
	r, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithAllowSyncInitContext(true),
		aicommon.WithDisableSessionTitleGeneration(false),
		aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			calls.Add(1)
			return mockedLoopDirectlyAnswerOutput(c, `{"@action":"session-init-generator","folder_name":"greeting","session_title":"Greeting"}`)
		}),
	)
	require.NoError(t, err)
	defer func() {
		cancel()
		r.Wait()
		if r.config.IsWorkDirReady() {
			_ = os.RemoveAll(r.config.GetOrCreateWorkDir())
		}
	}()
	r.ensureWorkDirectory("hello")
	require.EqualValues(t, 1, calls.Load())
	dir := r.config.GetOrCreateWorkDir()
	require.Contains(t, filepath.Base(dir), "_greeting_")
	require.DirExists(t, dir)
	require.Equal(t, "Greeting", r.config.GetConfigString("session_title"))
	r.ensureWorkDirectory("another message")
	require.Equal(t, dir, r.config.GetOrCreateWorkDir())
	require.EqualValues(t, 1, calls.Load(), "later turns must reuse the directory")
}
