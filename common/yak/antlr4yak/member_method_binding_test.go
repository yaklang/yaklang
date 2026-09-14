package antlr4yak

import (
	"context"
	"strings"
	"sync"
	"testing"
)

type memberMethodBindingConfig struct {
	values map[string]string
}

func (c *memberMethodBindingConfig) GetConfig(key string) (string, bool) {
	value, ok := c.values[key]
	return value, ok
}

type memberMethodBindingTarget struct {
	*memberMethodBindingConfig
}

func (*memberMethodBindingTarget) Ping() string {
	return "pong"
}

func newMemberMethodBindingEngine() *Engine {
	engine := New()
	engine.ImportLibs(map[string]interface{}{
		"memberBindingTarget": &memberMethodBindingTarget{
			memberMethodBindingConfig: &memberMethodBindingConfig{
				values: map[string]string{"answer": "42"},
			},
		},
	})
	return engine
}

func TestMemberMethodBinding(t *testing.T) {
	t.Run("zero argument pointer method stays bound", func(t *testing.T) {
		err := newMemberMethodBindingEngine().SafeEval(context.Background(), `
assert(memberBindingTarget.Ping() == "pong")
`)
		if err != nil {
			t.Fatalf("call bound zero-argument pointer method: %v", err)
		}
	})

	t.Run("promoted method stays bound with one argument", func(t *testing.T) {
		err := newMemberMethodBindingEngine().SafeEval(context.Background(), `
value, ok = memberBindingTarget.GetConfig("answer")
assert(ok)
assert(value == "42")
`)
		if err != nil {
			t.Fatalf("call bound promoted method: %v", err)
		}
	})

	t.Run("promoted method preserves explicit argument count", func(t *testing.T) {
		err := newMemberMethodBindingEngine().SafeEval(context.Background(), `
memberBindingTarget.GetConfig()
`)
		if err == nil {
			t.Fatal("expected missing promoted-method argument to fail")
		}
		if message := err.Error(); !strings.Contains(message, "GetConfig") ||
			!strings.Contains(message, "need [1] params, actually got [0] params") {
			t.Fatalf("unexpected promoted-method arity error: %v", err)
		}
	})
}

type mutableMemberLookupTarget struct {
	mu    sync.RWMutex
	value int
}

func (t *mutableMemberLookupTarget) Set(value int) {
	t.mu.Lock()
	t.value = value
	t.mu.Unlock()
}

func (t *mutableMemberLookupTarget) Current() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.value
}

func TestConcurrentMemberLookupDoesNotFormatMutableReceiver(t *testing.T) {
	target := &mutableMemberLookupTarget{}
	engine := New()
	engine.ImportLibs(map[string]interface{}{
		// Returning the object through a native call deliberately produces a Yak
		// Value without a source literal. Member lookup must describe it by type;
		// formatting the live receiver would read its fields outside the lock.
		"getMutableMemberTarget": func() *mutableMemberLookupTarget { return target },
	})

	stop := make(chan struct{})
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for value := 0; ; value++ {
			select {
			case <-stop:
				return
			default:
				target.Set(value)
			}
		}
	}()

	err := engine.SafeEvalWithoutCache(context.Background(), `
receiver = getMutableMemberTarget()
for index := 0; index < 20000; index++ {
    value = receiver.Current()
    assert value >= 0
}
`)
	close(stop)
	<-writerDone
	if err != nil {
		t.Fatalf("concurrent member lookup failed: %v", err)
	}
}
