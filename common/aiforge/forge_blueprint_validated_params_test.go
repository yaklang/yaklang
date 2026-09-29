package aiforge

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid"
)

func TestParameterRenderingPreservesPlainTemplate(t *testing.T) {
	blueprint := &ForgeBlueprint{InitializePrompt: "Analyze supplied data", ParameterRuleYaklangCode: `panic("must not execute")`}
	prompt, _, err := blueprint.GenerateFirstPromptWithMemoryOptionWithQueryAndParams("caller query", []Parameter{{Key: "topic", Value: "input"}})
	require.NoError(t, err)
	require.Equal(t, blueprint.InitializePrompt, prompt, "shared renderer must not append platform input policy")
	memory := aid.GetDefaultContextProvider()
	memory.StoreQuery("caller input")
	blueprint.ResultPrompt = "Return only JSON."
	result, err := blueprint.renderResultPrompt(memory)
	require.NoError(t, err)
	require.Equal(t, blueprint.ResultPrompt, result, "shared renderer must not append platform report policy")
}

func TestGenerateFirstPromptWithValidatedParamsDoesNotParseImportedCLI(t *testing.T) {
	blueprint := &ForgeBlueprint{
		InitializePrompt:         `query={{.Forge.UserQuery}} params={{.Forge.UserParams}}`,
		ParameterRuleYaklangCode: `panic("must not execute or parse")`,
	}
	prompt, _, err := blueprint.GenerateFirstPromptWithMemoryOptionWithQueryAndParams(
		"server query",
		[]Parameter{{Key: "topic", Value: "bounded value"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"query=server query", "topic", "bounded value", "<user_params_"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("validated prompt %q does not contain %q", prompt, expected)
		}
	}
}

func TestGenerateFirstPromptWithQueryRetainsLegacyCLIValidation(t *testing.T) {
	blueprint := &ForgeBlueprint{
		InitializePrompt:         `{{.Forge.UserParams}}`,
		ParameterRuleYaklangCode: `cli.String("query", cli.setRequired(true))`,
	}
	prompt, _, err := blueprint.GenerateFirstPromptWithMemoryOptionWithQuery("legacy query")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "legacy query") {
		t.Fatalf("legacy query was not rendered through CLI parameters: %q", prompt)
	}
}

func TestForgeEntrypointsPreserveOptionOrder(t *testing.T) {
	for _, validated := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "validated"}[validated], func(t *testing.T) {
			blueprint := &ForgeBlueprint{
				InitializePrompt: "Analyze supplied data",
				PersistentPrompt: "base context",
				AIOptions: []aicommon.ConfigOption{func(config *aicommon.Config) error {
					require.Equal(t, []string{"base context"}, config.PersistentMemory)
					config.PersistentMemory = append(config.PersistentMemory, "caller context")
					return nil
				}},
			}
			var opts []aicommon.ConfigOption
			var err error
			if validated {
				_, opts, err = blueprint.GenerateFirstPromptWithMemoryOptionWithQueryAndParams("query", nil)
			} else {
				_, opts, err = blueprint.GenerateFirstPromptWithMemoryOption(nil)
			}
			require.NoError(t, err)
			config := &aicommon.Config{}
			for _, opt := range opts {
				require.NoError(t, opt(config))
			}
			require.Equal(t, []string{"base context", "caller context"}, config.PersistentMemory)
		})
	}
}

func TestParameterRenderingRetainsPersistentValues(t *testing.T) {
	blueprint := &ForgeBlueprint{PersistentPrompt: `query={{.Forge.UserQuery}}; params={{.Forge.UserParams}}`, ParameterRuleYaklangCode: `panic("must not parse")`}
	prompt, err := blueprint.renderPersistentPromptWithParams("question", []Parameter{{Key: "file", Value: "evidence.txt"}})
	require.NoError(t, err)
	for _, value := range []string{"question", "file", "evidence.txt"} {
		require.Contains(t, prompt, value)
	}
}
