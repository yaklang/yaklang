package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeIdI18n_RequiredNodeIds(t *testing.T) {
	requiredNodeIds := []struct {
		nodeId     string
		expectZh   string
		expectEn   string
	}{
		{"ai-error", "AI 调用错误", "AI Invocation Error"},
		{"rate-limit", "请求限频", "Rate Limited"},
		{"notify", "系统通知", "System Notification"},
	}

	for _, tc := range requiredNodeIds {
		t.Run(tc.nodeId, func(t *testing.T) {
			i18n := NodeIdToI18n(tc.nodeId, false)
			require.NotNil(t, i18n, "nodeId %q should have i18n mapping", tc.nodeId)
			assert.Equal(t, tc.expectZh, i18n.Zh, "Chinese translation mismatch for %q", tc.nodeId)
			assert.Equal(t, tc.expectEn, i18n.En, "English translation mismatch for %q", tc.nodeId)
		})
	}
}

func TestNodeIdI18n_StreamLookup(t *testing.T) {
	for _, nodeId := range []string{"ai-error", "rate-limit", "notify"} {
		t.Run(nodeId+"_stream", func(t *testing.T) {
			i18n := NodeIdToI18n(nodeId, true)
			require.NotNil(t, i18n, "nodeId %q should be found in stream mode too", nodeId)
			assert.NotEmpty(t, i18n.Zh)
			assert.NotEmpty(t, i18n.En)
		})
	}
}
