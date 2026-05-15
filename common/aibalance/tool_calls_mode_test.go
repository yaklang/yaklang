package aibalance

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/schema"
)

// tool_calls_mode_test.go 验证 ResolveToolCallsMode 在 env / DB / default 各档位下的优先级.
//
// 关键词: aibalance tool_calls mode resolver, env > DB > default

// envScope 在测试期间临时设置 env, 测试结束后恢复并清空 flatten 内部缓存.
type envScope struct {
	keys map[string]string
}

func newEnvScope(t *testing.T, kv map[string]string) *envScope {
	t.Helper()
	saved := map[string]string{}
	for k, v := range kv {
		saved[k] = os.Getenv(k)
		if v == "" {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, v)
		}
	}
	resetFlattenEnvCacheForTest()
	return &envScope{keys: saved}
}

func (e *envScope) restore() {
	for k, v := range e.keys {
		if v == "" {
			_ = os.Unsetenv(k)
		} else {
			_ = os.Setenv(k, v)
		}
	}
	resetFlattenEnvCacheForTest()
}

// 关键词: resolver default unknown -> native + auto_fallback
func TestResolveToolCallsMode_DefaultUnknown(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "",
		envFlattenToolCallsAll:       "",
	})
	defer es.restore()

	p := &Provider{
		ModelName: "x", TypeName: "openai", WrapperName: "w",
		DbProvider: &schema.AiProvider{}, // empty mode fields
	}
	m := ResolveToolCallsMode(p, "x")
	assert.Equal(t, "native", m.Round1)
	assert.Equal(t, "native", m.Round2)
	assert.True(t, m.AutoFallback)
	assert.Equal(t, "default", m.Source)
}

// 关键词: resolver db precedence, mode 来自 db
func TestResolveToolCallsMode_DbValues(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "",
		envFlattenToolCallsAll:       "",
	})
	defer es.restore()

	p := &Provider{
		ModelName: "x", TypeName: "openai", WrapperName: "w",
		DbProvider: &schema.AiProvider{
			ToolCallsRound1Mode: "native",
			ToolCallsRound2Mode: "react",
		},
	}
	m := ResolveToolCallsMode(p, "x")
	assert.Equal(t, "native", m.Round1)
	assert.Equal(t, "react", m.Round2)
	assert.False(t, m.AutoFallback)
	assert.Equal(t, "db", m.Source)
}

// 关键词: resolver db 部分字段空, 另一字段保持 native 默认
func TestResolveToolCallsMode_DbPartial(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "",
		envFlattenToolCallsAll:       "",
	})
	defer es.restore()

	p := &Provider{
		ModelName: "x", TypeName: "openai", WrapperName: "w",
		DbProvider: &schema.AiProvider{
			ToolCallsRound2Mode: "react", // 只填了 round2
		},
	}
	m := ResolveToolCallsMode(p, "x")
	assert.Equal(t, "native", m.Round1, "round1 空 -> 默认 native")
	assert.Equal(t, "react", m.Round2)
	assert.False(t, m.AutoFallback)
	assert.Equal(t, "db", m.Source)
}

// 关键词: resolver env model whitelist 命中
func TestResolveToolCallsMode_EnvModelWhitelist(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "z-deepseek-v4-pro , another-model",
		envFlattenToolCallsAll:       "",
	})
	defer es.restore()

	p := &Provider{
		ModelName: "deepseek-chat", TypeName: "deepseek", WrapperName: "z-deepseek-v4-pro",
		DbProvider: &schema.AiProvider{
			ToolCallsRound1Mode: "native", // DB 说 native, 但 env 优先级更高
			ToolCallsRound2Mode: "native",
		},
	}
	m := ResolveToolCallsMode(p, "z-deepseek-v4-pro")
	assert.Equal(t, "react", m.Round1)
	assert.Equal(t, "react", m.Round2)
	assert.Equal(t, "env:model", m.Source)
}

// 关键词: resolver env all 全局 kill switch
func TestResolveToolCallsMode_EnvAllKillSwitch(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "",
		envFlattenToolCallsAll:       "true",
	})
	defer es.restore()

	p := &Provider{
		ModelName: "x", TypeName: "openai", WrapperName: "w",
		DbProvider: &schema.AiProvider{
			ToolCallsRound1Mode: "native",
			ToolCallsRound2Mode: "native",
		},
	}
	m := ResolveToolCallsMode(p, "x")
	assert.Equal(t, "react", m.Round1)
	assert.Equal(t, "react", m.Round2)
	assert.Equal(t, "env:all", m.Source)
}

// 关键词: resolver dirty db 值归一化
func TestResolveToolCallsMode_DbDirtyNormalization(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "",
		envFlattenToolCallsAll:       "",
	})
	defer es.restore()

	p := &Provider{
		ModelName: "x", TypeName: "openai", WrapperName: "w",
		DbProvider: &schema.AiProvider{
			ToolCallsRound1Mode: "  NATIVE ",   // 大小写 + 空格
			ToolCallsRound2Mode: "flatten",     // 历史同义词
		},
	}
	m := ResolveToolCallsMode(p, "x")
	assert.Equal(t, "native", m.Round1)
	assert.Equal(t, "react", m.Round2)
}

// 关键词: resolver provider nil 防御
func TestResolveToolCallsMode_NilSafety(t *testing.T) {
	es := newEnvScope(t, map[string]string{
		envFlattenToolCallsForModels: "",
		envFlattenToolCallsAll:       "",
	})
	defer es.restore()

	m := ResolveToolCallsMode(nil, "")
	// nil 视为完全 unknown
	assert.Equal(t, "native", m.Round1)
	assert.Equal(t, "native", m.Round2)
	assert.True(t, m.AutoFallback)
	assert.Equal(t, "default", m.Source)
}
