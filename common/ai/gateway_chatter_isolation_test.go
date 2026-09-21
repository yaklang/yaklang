package ai

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type isolatedChatterGateway struct {
	aispec.AIClient
	config       atomic.Pointer[aispec.AIConfig]
	firstStarted chan struct{}
	releaseFirst chan struct{}
}

func (g *isolatedChatterGateway) LoadOption(opts ...aispec.AIConfigOption) {
	config := &aispec.AIConfig{}
	for _, opt := range opts {
		opt(config)
	}
	g.config.Store(config)
}
func (g *isolatedChatterGateway) GetConfig() *aispec.AIConfig { return g.config.Load() }
func (g *isolatedChatterGateway) CheckValid() error           { return nil }
func (g *isolatedChatterGateway) Chat(prompt string, _ ...any) (string, error) {
	if prompt == "first" {
		close(g.firstStarted)
		<-g.releaseFirst
	}
	config := g.config.Load()
	if config.RawHTTPResponseHeaderCallback != nil {
		config.RawHTTPResponseHeaderCallback([]byte(prompt))
	}
	return config.Model, nil
}

func TestLoadChaterIsolatesConcurrentInvocationOptions(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	provider := "test-isolated-chatter"
	aispec.Register(provider, func() aispec.AIClient {
		return &isolatedChatterGateway{firstStarted: started, releaseFirst: release}
	})
	chat, err := LoadChater(provider, aispec.WithModel("default-model"))
	require.NoError(t, err)
	firstHeaders, secondHeaders := make(chan string, 2), make(chan string, 2)
	result := make(chan string, 1)
	go func() {
		value, _ := chat("first", aispec.WithModel("first-model"), aispec.WithRawHTTPResponseHeaderCallback(func(data []byte) { firstHeaders <- string(data) }))
		result <- value
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("first call did not start")
	}
	value, err := chat("second", aispec.WithModel("second-model"), aispec.WithRawHTTPResponseHeaderCallback(func(data []byte) { secondHeaders <- string(data) }))
	require.NoError(t, err)
	require.Equal(t, "second-model", value)
	// Release via a separate channel send so the deferred close also handles failure paths.
	release <- struct{}{}
	select {
	case value = <-result:
		require.Equal(t, "first-model", value)
	case <-time.After(5 * time.Second):
		t.Fatal("first call did not finish")
	}
	select {
	case header := <-firstHeaders:
		require.Equal(t, "first", header)
	default:
		t.Error("first callback was overwritten")
	}
	require.Equal(t, "second", <-secondHeaders)
	require.Empty(t, secondHeaders)
	value, err = chat("third")
	require.NoError(t, err)
	require.Equal(t, "default-model", value)
}
