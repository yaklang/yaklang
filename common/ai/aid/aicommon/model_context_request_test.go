package aicommon

import (
	"fmt"
	"io"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestModelContextRequestPinsQualityWithoutChangingAuxiliaryRouting(t *testing.T) {
	for _, tc := range []struct{ single, inherited bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		t.Run(fmt.Sprintf("single=%v/inherited=%v", tc.single, tc.inherited), func(t *testing.T) {
			single := tc.single
			yakit.GuardConfigurationForTest(t)
			observed := pr5128ModeFixture(t, single)
			consts.GetTieredAIConfig().IntelligentConfigs[0].ContextOwnerId = uuid.NewString()
			cfg := pr5128NewModeConfig(t)
			require.NoError(t, WithTieredAICallback(single)(cfg))
			if tc.inherited {
				cfg = pr5128NewModeConfig(t, WithInheritTieredAICallback(cfg, true))
			}
			pinned := cfg.SnapshotPrimaryModelConfig()
			require.NotNil(t, pinned)
			key := aispec.ModelContextIdentity(pinned)
			require.NotEmpty(t, key)
			request := NewAIRequest("pinned quality request", WithAIRequest_QualityModelConfig(pinned))
			// Neither a later global selection nor mutation of the source snapshot
			// can replace the configuration already attached to this request.
			pinned.ModelName = "mutated"
			consts.SetTieredAIConfig(pr5128TierConfig(single, "1"))
			for _, tc := range []struct {
				name string
				call AICallbackType
				want pr5128ModelCall
			}{
				{"quality", cfg.GetQualityPriorityAICallback(), pr5128ModelCall{"test-pr5128-quality", "Q0", "high"}},
				{"speed", cfg.GetSpeedPriorityAICallback(), pr5128ModelCall{"test-pr5128-speed", "S1", "high"}},
				{"vision", cfg.GetVisionPriorityAICallback(), pr5128ModelCall{"test-pr5128-vision", "V1", "high"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					if single && tc.name != "quality" {
						tc.want.provider, tc.want.model = "test-pr5128-quality", "Q1"
						if tc.name == "speed" {
							tc.want.thinking = "none"
						}
					}
					response, err := tc.call(cfg, request)
					require.NoError(t, err)
					_, err = io.ReadAll(response.GetOutputStreamReader("context-binding", true, cfg.GetEmitter()))
					require.NoError(t, err)
					require.NoError(t, response.GetError())
					select {
					case got := <-observed:
						require.Equal(t, tc.want, got)
					default:
						t.Fatal("local provider was not called")
					}
					if tc.name == "quality" {
						require.Equal(t, key, response.GetModelContextKey())
					}
				})
			}
		})
	}
}

func TestModelContextQualityIgnoresLegacyNameOnlyOverflow(t *testing.T) {
	yakit.GuardConfigurationForTest(t)
	cfg := pr5128NewModeConfig(t)
	model := contextTestModel()
	key := bindContext(t, cfg, model)
	require.NoError(t, yakit.SetAIModelContextSettings(consts.GetGormProfileDatabase(), "openai", model.ModelName, 300000, nil, key))
	require.EqualValues(t, 160000, cfg.RecordPromptOverflow("openai", model.ModelName, 250000,
		&aispec.PromptOverflowEvidence{MaxContextTokens: 200000}))
	require.EqualValues(t, 160000, cfg.EffectiveTimelineTokenLimit("openai", model.ModelName, 300000), "legacy auxiliary recovery remains effective")
	require.EqualValues(t, 300000, cfg.EffectiveModelContextLimit("openai", model.ModelName, 300000, key), "a name-only auxiliary error must not lower quality settings")
}
