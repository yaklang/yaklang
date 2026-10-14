package yak

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklang"
)

type observedSleepContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *observedSleepContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func TestAIToolSleepCancellation(t *testing.T) {
	for _, name := range []string{"sleep", "time.Sleep", "time.sleep"} {
		t.Run(name, func(t *testing.T) {
			watchdog, stop := context.WithTimeout(context.Background(), time.Minute)
			defer stop()
			ctx, cancel := context.WithCancel(watchdog)
			defer cancel()
			observed := &observedSleepContext{Context: ctx, entered: make(chan struct{})}
			engine := yaklang.New()
			bindAIToolSleep(engine, observed)
			fn, _ := engine.Var("sleep").(func(float64))
			if name != "sleep" {
				fn, _ = engine.Var("time").(map[string]any)[name[5:]].(func(float64))
			}
			require.NotNil(t, fn)
			done := make(chan struct{})
			go func() { defer close(done); fn(3600) }()
			select {
			case <-observed.entered:
			case <-watchdog.Done():
				t.Fatal("sleep never observed its invocation context")
			}
			cancel()
			select {
			case <-done:
			case <-watchdog.Done():
				t.Fatal("cancel did not interrupt native sleep")
			}
			// The binding must preserve other time exports and isolated engines.
			require.NotNil(t, engine.Var("time").(map[string]any)["Now"])
			fresh := yaklang.New()
			require.NoError(t, fresh.SafeEval(watchdog, `sleep(0); time.Sleep(0); time.sleep(0)`))
		})
	}
}
