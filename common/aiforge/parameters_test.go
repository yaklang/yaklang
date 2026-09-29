package aiforge

import (
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestParameterFormattingPreservesClientContract(t *testing.T) {
	values := []Parameter{
		{Key: "topic", Value: "first\nsecond"},
		{Key: "query", Value: "question one"},
		{},
		{Key: "topic", Value: "duplicate"},
		{Key: "query", Value: "question two"},
		{Key: "empty", Value: ""},
	}
	const want = "question one\nquestion two\n<|PARAM_topic_|>\nfirst\nsecond\n<|PARAM_END_topic_|>\n<|PARAM_topic_|>\nduplicate\n<|PARAM_END_topic_|>\n<|PARAM_empty_|>\n\n<|PARAM_END_empty_|>\n"
	legacy := make([]*ypb.ExecParamItem, 0, len(values))
	for _, value := range values {
		legacy = append(legacy, &ypb.ExecParamItem{Key: value.Key, Value: value.Value})
	}
	for name, got := range map[string]string{"native": parametersToPromptString(values), "client": ExecParams2PromptString(legacy)} {
		if got != want {
			t.Errorf("%s formatting changed: got %q, want %q", name, got, want)
		}
	}
	if parametersToPromptString(nil) != "" || ExecParams2PromptString(nil) != "" {
		t.Fatal("empty input changed")
	}
}
