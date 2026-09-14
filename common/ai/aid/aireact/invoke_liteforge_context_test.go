package aireact

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

func TestResilienceLiteForgeChildContextReachesInheritedProvider(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			parent, stopParent := context.WithCancel(context.Background())
			defer stopParent()
			type contextKey struct{}
			started := make(chan struct{})
			observed := make(chan error, 1)
			var once sync.Once
			r, err := NewTestReAct(
				aicommon.WithContext(parent),
				aicommon.WithAIAutoRetry(1),
				aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithSpeedPriorityAICallback(func(caller aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					ctx := request.GetContext()
					if ctx == nil {
						ctx = caller.GetContext()
					}
					once.Do(func() { close(started) })
					if ctx.Value(contextKey{}) != "child" {
						err := errors.New("child context was discarded before provider invocation")
						observed <- err
						return nil, err
					}
					select {
					case <-ctx.Done():
						observed <- ctx.Err()
						return nil, ctx.Err()
					case <-time.After(time.Second):
						err := errors.New("provider outlived child context")
						observed <- err
						return nil, err
					}
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			valueCtx := context.WithValue(parent, contextKey{}, "child")
			child, cancel := context.WithCancel(valueCtx)
			if mode == "deadline" {
				cancel()
				child, cancel = context.WithTimeout(valueCtx, 100*time.Millisecond)
			}
			defer cancel()
			if mode == "cancel" {
				go func() {
					select {
					case <-started:
						cancel()
					case <-parent.Done():
					}
				}()
			}
			_, err = r.InvokeSpeedPriorityLiteForge(child, "bounded-init", "Generate a short title", []aitool.ToolOption{aitool.WithStringParam("title")})
			if err == nil {
				t.Fatal("cancelled child invocation succeeded")
			}
			select {
			case err := <-observed:
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("provider cancellation was not observed")
			}
			if parent.Err() != nil {
				t.Fatal("child invocation cancelled the parent Agent")
			}
		})
	}
}
