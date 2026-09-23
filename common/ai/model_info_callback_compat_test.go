package ai

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type pr5128LegacyModelInfoHook func(string, string)

const pr5128ModelInfoProvider = "test-pr5128-model-info-compat"

type pr5128FailingGateway struct{ TestGateway }

func (*pr5128FailingGateway) Chat(string, ...any) (string, error) {
	return "", errors.New("local provider failure")
}

func registerPR5128ModelInfoProvider() {
	aispec.Register(pr5128ModelInfoProvider, func() aispec.AIClient { return &TestGateway{} })
}

func TestPR5128_ModelInfoCallback_GatewayNilSafety(t *testing.T) {
	registerPR5128ModelInfoProvider()
	var two func(string, string)
	var three func(string, string, string)
	var named pr5128LegacyModelInfoHook
	var variadic func(string, string, ...string)
	var modelInfo aispec.ModelInfoCallback
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
			_, err := Chat("hello",
				aispec.WithType(pr5128ModelInfoProvider),
				aispec.WithModel("local-model"),
				aispec.WithModelInfoCallback(tc.cb),
				aispec.WithModelInfoConfirmCallback(tc.cb),
			)
			require.NoError(t, err)
		})
	}
}

func TestPR5128_ModelInfoCallback_GatewayValidForms(t *testing.T) {
	registerPR5128ModelInfoProvider()
	for _, kind := range []string{"two arguments", "three arguments", "variadic", "named variadic", "named legacy"} {
		t.Run(kind, func(t *testing.T) {
			var calls, confirms [][3]string
			makeHook := func(dst *[][3]string) any {
				switch kind {
				case "two arguments":
					return func(provider, model string) {
						*dst = append(*dst, [3]string{provider, model, ""})
					}
				case "three arguments":
					return func(provider, model, thinking string) {
						*dst = append(*dst, [3]string{provider, model, thinking})
					}
				case "variadic":
					return func(provider, model string, levels ...string) {
						*dst = append(*dst, [3]string{provider, model, levels[0]})
					}
				case "named variadic":
					return aispec.ModelInfoCallback(func(provider, model string, levels ...string) {
						*dst = append(*dst, [3]string{provider, model, levels[0]})
					})
				default:
					return pr5128LegacyModelInfoHook(func(provider, model string) {
						*dst = append(*dst, [3]string{provider, model, ""})
					})
				}
			}
			_, err := Chat("hello",
				aispec.WithType(pr5128ModelInfoProvider),
				aispec.WithModel("local-model"),
				aispec.WithThinkingLevel("low"),
				aispec.WithModelInfoCallback(makeHook(&calls)),
				aispec.WithModelInfoConfirmCallback(makeHook(&confirms)),
			)
			require.NoError(t, err)
			thinking := ""
			if kind == "three arguments" || kind == "variadic" || kind == "named variadic" {
				thinking = "low"
			}
			want := [][3]string{{pr5128ModelInfoProvider, "local-model", thinking}}
			require.Equal(t, want, calls)
			require.Equal(t, want, confirms)
		})
	}
	t.Run("failed provider does not confirm", func(t *testing.T) {
		const provider = "test-pr5128-model-info-failure"
		aispec.Register(provider, func() aispec.AIClient { return &pr5128FailingGateway{} })
		var selected, confirmed int
		_, err := Chat("hello",
			aispec.WithType(provider),
			aispec.WithModel("local-model"),
			aispec.WithDisableProviderFallback(true),
			aispec.WithModelInfoCallback(func(string, string) { selected++ }),
			aispec.WithModelInfoConfirmCallback(func(string, string) { confirmed++ }),
		)
		require.Error(t, err)
		require.Equal(t, 1, selected)
		require.Zero(t, confirmed)
	})
}
