package ai

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
)

func TestWithExtraHeader(t *testing.T) {
	for _, tt := range []struct {
		name    string
		headers any
	}{
		{"string map", map[string]string{" X-Test ": "test-value"}},
		{"any map", map[string]any{" X-Test ": "test-value"}},
		{"ordered map", orderedmap.New(map[string]any{" X-Test ": "test-value"})},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := &aispec.AIConfig{}
			option := WithExtraHeader(tt.headers)
			option(config)
			option(config)
			require.Equal(t, map[string]string{"X-Test": "test-value"}, aispec.ExtraHeadersToMap(config.Headers))
			require.Len(t, config.Headers, 1)
		})
	}

	for _, headers := range []any{nil, (*orderedmap.OrderedMap)(nil), map[string]string{}, orderedmap.New()} {
		config := &aispec.AIConfig{}
		WithExtraHeader(headers)(config)
		require.Empty(t, config.Headers)
	}
}

func TestWithExtraHeaderRejectsInvalidInput(t *testing.T) {
	require.PanicsWithValue(t, "ai.extraHeader expects a map of header names to string values", func() {
		WithExtraHeader("not-a-map")
	})
	for _, value := range []any{123, nil, []string{"secret-value"}, map[string]string{"key": "secret-value"}} {
		require.PanicsWithValue(t, "ai.extraHeader expects string header values", func() {
			WithExtraHeader(map[string]any{"Authorization": value})
		})
	}
}
