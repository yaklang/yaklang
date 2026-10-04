package scannode

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aiforge"
	"strings"
	"testing"
)

// Adapt a deterministic response source to the real adapter wrapper.
func runLegionResultGenerator(prompt string, call func(string) (string, error)) (string, error) {
	execution, err := aiforge.NewForgeBlueprint("result-unit").CreateCoordinator(context.Background(), "", aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true), aicommon.WithNoOpMemoryTriage())
	if err != nil {
		return "", err
	}
	defer execution.Close()
	return legionForgeResultGenerator(func(_ *aiforge.ForgeExecution, value string) (string, error) { return call(value) }, true, "original input")(execution, prompt)
}
func TestRetryEmptyForgeResult(t *testing.T) {
	calls := 0
	result, err := runLegionResultGenerator("original report instructions", func(prompt string) (string, error) {
		calls++
		if calls == 1 {
			if !strings.HasPrefix(prompt, "original report instructions") {
				t.Fatalf("first prompt changed: %q", prompt)
			}
			return " \n", nil
		}
		if !strings.Contains(prompt, "最终输出通道") {
			t.Fatalf("retry did not request a final output: %q", prompt)
		}
		return "# Report", nil
	})
	if err != nil || result != "# Report" || calls != 2 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

func TestRetryEmptyForgeResultNeverPublishesReasoningOrLoops(t *testing.T) {
	calls := 0
	result, err := runLegionResultGenerator("report", func(string) (string, error) {
		calls++
		return "", nil
	})
	if err == nil || result != "" || calls != 2 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
	calls = 0
	want := errors.New("provider failed")
	result, err = runLegionResultGenerator("report", func(string) (string, error) {
		calls++
		return "", want
	})
	if !errors.Is(err, want) || result != "" || calls != 1 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

func TestLegionForgeResultRetryPreservesPartialFailure(t *testing.T) {
	calls := 0
	failure := errors.New("interrupted stream")
	result, err := runLegionResultGenerator("report", func(string) (string, error) { calls++; return "partial", failure })
	if result != "partial" || !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("result=%q err=%v calls=%d", result, err, calls)
	}
}

func TestValidatedInvocationResultRetainsInputAndEvidence(t *testing.T) {
	promptTemplate := "Return a Markdown analysis."
	memory := coordinator.GetDefaultContextProvider()
	memory.StoreQuery("internal rendered task instructions")
	memory.Timeline.PushText(1, "Observed credential request; no network request was performed")
	prompt, err := renderLegionForgeResultPrompt(promptTemplate, "email-content: billing@example.test; expires in one hour", memory.Snapshot())
	require.NotContains(t, prompt, "internal rendered task instructions")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Return a Markdown analysis", "billing@example.test", "expires in one hour", "Observed credential request", "no network request was performed", "Do not invent"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("final result prompt lost %q", expected)
		}
	}

}

func TestValidatedInvocationResultRejectsMissingContext(t *testing.T) {
	promptTemplate := "Return a Markdown analysis."
	if _, err := renderLegionForgeResultPrompt(promptTemplate, "input", nil); err == nil {
		t.Fatal("missing context must not generate an ungrounded report")
	}
}

func TestLegionForgePlainTemplateRetainsInvocation(t *testing.T) {
	release := testLegionContextForgeRelease(t)
	release.InitPrompt = "Analyze supplied data"
	rehashLegionContextForgeRelease(t, release)
	_, blueprint, params, err := buildContextForgeBlueprint(release, "original user input")
	require.NoError(t, err)
	prompt, _, err := blueprint.GenerateFirstPromptWithMemoryOptionWithQueryAndParams("specific user query", params)
	require.NoError(t, err)
	for _, expected := range []string{release.InitPrompt, "specific user query", "topic", "bounded input"} {
		require.Contains(t, prompt, expected)
	}
	require.Equal(t, "Analyze supplied data", release.InitPrompt, "composition must not mutate the immutable release")
}
