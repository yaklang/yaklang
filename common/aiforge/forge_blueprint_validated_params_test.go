package aiforge

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGenerateFirstPromptWithValidatedParamsDoesNotParseImportedCLI(t *testing.T) {
	blueprint := &ForgeBlueprint{
		InitializePrompt:         `query={{.Forge.UserQuery}} params={{.Forge.UserParams}}`,
		ParameterRuleYaklangCode: `panic("must not execute or parse")`,
	}
	prompt, _, err := blueprint.GenerateFirstPromptWithMemoryOptionWithQueryAndParams(
		"server query",
		[]*ypb.ExecParamItem{{Key: "topic", Value: "bounded value"}},
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
