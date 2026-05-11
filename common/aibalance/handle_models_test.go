package aibalance

import (
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
)

func modelsListBodyFromHTTP(t *testing.T, raw []byte) []byte {
	t.Helper()
	idx := strings.Index(string(raw), "\r\n\r\n")
	require.Greater(t, idx, 0)
	return raw[idx+4:]
}

func parseModelsListIDs(t *testing.T, httpRaw []byte) []string {
	t.Helper()
	body := modelsListBodyFromHTTP(t, httpRaw)
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &resp))
	out := make([]string, 0, len(resp.Data))
	for _, d := range resp.Data {
		out = append(out, d.ID)
	}
	return out
}

func runServeModels(t *testing.T, cfg *ServerConfig, key *Key) []byte {
	t.Helper()
	client, srv := net.Pipe()
	defer client.Close()
	go func() {
		cfg.serveModels(key, srv)
		srv.Close()
	}()
	raw, err := io.ReadAll(client)
	require.NoError(t, err)
	return raw
}

func providerChat(mode string) *Provider {
	return &Provider{
		ModelName:    "m",
		TypeName:     "openai",
		ProviderMode: mode,
		DbProvider:   &schema.AiProvider{ProviderMode: mode},
	}
}

func TestServeModels_NilKey_OnlyFreeNonEmbedding(t *testing.T) {
	cfg := NewServerConfig()
	cfg.Entrypoints.providers = map[string][]*Provider{
		"paid-1":               {providerChat("chat")},
		"paid-2":               {providerChat("chat")},
		"free-a-free":          {providerChat("chat")},
		"free-b-free":          {providerChat("chat")},
		"embedding-free":       {providerChat("chat")},
		"text-embedding-free":  {providerChat("")},
		"text-embedding-3":     {providerChat("chat")},
	}

	raw := runServeModels(t, cfg, nil)
	ids := parseModelsListIDs(t, raw)
	assert.Equal(t, []string{"free-a-free", "free-b-free"}, ids)
}

func TestServeModels_ValidKey_GlobAllowed(t *testing.T) {
	cfg := NewServerConfig()
	apiKey := "k-glob-test"
	cfg.Keys.keys[apiKey] = &Key{
		Key:           apiKey,
		AllowedModels: map[string]bool{"memfit-*": true},
	}
	cfg.KeyAllowedModels.allowedModels[apiKey] = map[string]bool{"memfit-*": true}

	cfg.Entrypoints.providers = map[string][]*Provider{
		"memfit-pro":  {providerChat("chat")},
		"memfit-lite": {providerChat("chat")},
		"gpt-4":       {providerChat("chat")},
		"gpt-4-free":  {providerChat("chat")},
	}

	key, _ := cfg.Keys.Get(apiKey)
	raw := runServeModels(t, cfg, key)
	ids := parseModelsListIDs(t, raw)
	assert.Equal(t, []string{"memfit-lite", "memfit-pro", "gpt-4-free"}, ids)
}

func TestServeModels_StableSortAcrossCalls(t *testing.T) {
	cfg := NewServerConfig()
	apiKey := "k-sort-stable"
	all := map[string]bool{
		"z-paid": true, "a-paid": true, "m-paid": true,
		"z-free": true, "a-free": true, "m-free": true,
	}
	cfg.Keys.keys[apiKey] = &Key{Key: apiKey, AllowedModels: all}
	cfg.KeyAllowedModels.allowedModels[apiKey] = all

	cfg.Entrypoints.providers = map[string][]*Provider{
		"z-paid": {providerChat("chat")},
		"a-paid": {providerChat("chat")},
		"m-paid": {providerChat("chat")},
		"z-free": {providerChat("chat")},
		"a-free": {providerChat("chat")},
		"m-free": {providerChat("chat")},
	}
	key, _ := cfg.Keys.Get(apiKey)

	want := []string{"a-paid", "m-paid", "z-paid", "a-free", "m-free", "z-free"}
	for i := 0; i < 3; i++ {
		raw := runServeModels(t, cfg, key)
		ids := parseModelsListIDs(t, raw)
		assert.Equal(t, want, ids, "iteration %d", i)
	}
}

func TestServeModels_EmbeddingExclusionByMode(t *testing.T) {
	cfg := NewServerConfig()
	cfg.Entrypoints.providers = map[string][]*Provider{
		"chat-style-name":    {providerChat("embedding")},
		"normal-chat-free":   {providerChat("chat")},
	}

	raw := runServeModels(t, cfg, nil)
	ids := parseModelsListIDs(t, raw)
	assert.Equal(t, []string{"normal-chat-free"}, ids)
	for _, id := range ids {
		assert.NotEqual(t, "chat-style-name", id)
	}
}

func TestServeModels_EmbeddingExclusionByName(t *testing.T) {
	cfg := NewServerConfig()
	cfg.Entrypoints.providers = map[string][]*Provider{
		"text-embedding-free": {providerChat("")},
		"some-chat-free":      {providerChat("chat")},
	}

	raw := runServeModels(t, cfg, nil)
	ids := parseModelsListIDs(t, raw)
	assert.Equal(t, []string{"some-chat-free"}, ids)
}

func TestIsEmbeddingWrapper_NameAndMode(t *testing.T) {
	assert.True(t, isEmbeddingWrapper("text-embedding-3", []*Provider{providerChat("chat")}))
	assert.True(t, isEmbeddingWrapper("my-model", []*Provider{providerChat("embedding")}))
	assert.False(t, isEmbeddingWrapper("gpt-4", []*Provider{providerChat("chat")}))
}
