package aiforge

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestValidatedInvocationResultRetainsInputAndEvidence(t *testing.T) {
	blueprint := &ForgeBlueprint{ResultPrompt: "Return a Markdown analysis."}
	memory := aid.GetDefaultContextProvider()
	memory.StoreQuery("email-content: billing@example.test; expires in one hour")
	memory.PushText(1, "Observed credential request; no network request was performed")
	prompt, err := blueprint.renderValidatedResultPrompt(memory)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Return a Markdown analysis", "billing@example.test", "expires in one hour", "Observed credential request", "no network request was performed", "Do not invent"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("final result prompt lost %q", expected)
		}
	}
	legacy, err := blueprint.renderResultPrompt(memory)
	if err != nil || legacy != blueprint.ResultPrompt {
		t.Fatalf("legacy rendering changed: %q, %v", legacy, err)
	}
}

func TestValidatedInvocationResultRejectsMissingContext(t *testing.T) {
	blueprint := &ForgeBlueprint{ResultPrompt: "Return a Markdown analysis."}
	if _, err := blueprint.renderValidatedResultPrompt(nil); err == nil {
		t.Fatal("missing context must not generate an ungrounded report")
	}
}

func TestValidatedInvocationRetainsInputWithoutTemplatePlaceholders(t *testing.T) {
	blueprint := &ForgeBlueprint{
		InitializePrompt:         "Analyze the supplied email without sending it.",
		ParameterRuleYaklangCode: `panic("must not execute")`,
	}
	prompt, _, err := blueprint.GenerateFirstPromptWithMemoryOptionWithQueryAndParams(
		"Return the final analysis", []*ypb.ExecParamItem{{Key: "email-content", Value: "SUBJECT: urgent password verification"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Analyze the supplied email", "Return the final analysis", "email-content", "SUBJECT: urgent password verification", "<user_params_"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("missing invocation data %q: %s", expected, prompt)
		}
	}
}

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
