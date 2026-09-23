package aispec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type legacyModelInfoHook func(string, string)

func TestPR5128_ModelInfoCallback_NilForms(t *testing.T) {
	testModelInfoNilForms(t, WithModelInfoCallback, func(c *AIConfig) ModelInfoCallback {
		return c.ModelInfoCallback
	})
}

func TestPR5128_ModelInfoConfirmCallback_NilForms(t *testing.T) {
	testModelInfoNilForms(t, WithModelInfoConfirmCallback, func(c *AIConfig) ModelInfoCallback {
		return c.ModelInfoConfirmCallback
	})
}

func testModelInfoNilForms(t *testing.T, option func(any) AIConfigOption, field func(*AIConfig) ModelInfoCallback) {
	t.Helper()
	var two func(string, string)
	var three func(string, string, string)
	var named legacyModelInfoHook
	var variadic func(string, string, ...string)
	var modelInfo ModelInfoCallback
	for _, tc := range []struct {
		name string
		cb   any
	}{
		{"untyped", nil},
		{"two arguments", two},
		{"three arguments", three},
		{"named legacy", named},
		{"variadic", variadic},
		{"ModelInfoCallback", modelInfo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &AIConfig{}
			option(tc.cb)(cfg)
			require.Nil(t, field(cfg))
		})
	}
}

func TestPR5128_ModelInfoCallback_LegacyNamedFunction(t *testing.T) {
	testLegacyModelInfoHook(t, WithModelInfoCallback, func(c *AIConfig) ModelInfoCallback {
		return c.ModelInfoCallback
	})
}

func TestPR5128_ModelInfoConfirmCallback_LegacyNamedFunction(t *testing.T) {
	testLegacyModelInfoHook(t, WithModelInfoConfirmCallback, func(c *AIConfig) ModelInfoCallback {
		return c.ModelInfoConfirmCallback
	})
}

func testLegacyModelInfoHook(t *testing.T, option func(any) AIConfigOption, field func(*AIConfig) ModelInfoCallback) {
	t.Helper()
	var gotProvider, gotModel string
	cb := legacyModelInfoHook(func(provider, model string) {
		gotProvider, gotModel = provider, model
	})
	cfg := &AIConfig{}
	option(cb)(cfg)
	require.NotNil(t, field(cfg))
	field(cfg)("local-provider", "local-model", "high")
	require.Equal(t, "local-provider", gotProvider)
	require.Equal(t, "local-model", gotModel)
}
