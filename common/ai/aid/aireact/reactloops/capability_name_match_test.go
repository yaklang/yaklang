package reactloops

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestMatchCapabilitiesByTextWithConfig_DisablePlanSkipsForgeNameMatch(t *testing.T) {
	cfg := aicommon.NewConfig(context.Background(),
		aicommon.WithEnablePlanAndExec(false),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, nil
		}),
	)
	cfg.AiForgeManager = &mockForgeFactoryForCapabilityMatch{
		forges: map[string]*schema.AIForge{
			"hostscan": {
				ForgeName:        "hostscan",
				ForgeType:        schema.FORGE_TYPE_YAK,
				Description:      "host scan forge",
				ForgeVerboseName: "Host Scan",
			},
		},
	}

	result := MatchCapabilitiesByTextWithConfig(cfg, "hostscan")
	if result != nil {
		require.Empty(t, result.ForgeNames(), "plan disabled should prevent forge name auto-match exposure")
	}
}

type mockForgeFactoryForCapabilityMatch struct {
	forges map[string]*schema.AIForge
}

func (m *mockForgeFactoryForCapabilityMatch) Query(ctx context.Context, opts ...aicommon.ForgeQueryOption) ([]*schema.AIForge, error) {
	var result []*schema.AIForge
	for _, f := range m.forges {
		result = append(result, f)
	}
	return result, nil
}

func (m *mockForgeFactoryForCapabilityMatch) GetAIForge(name string) (*schema.AIForge, error) {
	if f, ok := m.forges[name]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("forge %q not found", name)
}

func (m *mockForgeFactoryForCapabilityMatch) GenerateAIForgeListForPrompt(forges []*schema.AIForge) (string, error) {
	return "", nil
}

func (m *mockForgeFactoryForCapabilityMatch) GenerateAIJSONSchemaFromSchemaAIForge(forge *schema.AIForge) (string, error) {
	return "", nil
}
