package aicommon

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

type recordingPromptOverflowJudge struct {
	*transactionTestConfig
	calls int
	yes   bool
	err   error
	seen  aispec.PromptOverflowAmbiguity
}

func (j *recordingPromptOverflowJudge) JudgeAmbiguousPromptOverflow(_ context.Context, sample aispec.PromptOverflowAmbiguity) (bool, error) {
	j.calls++
	j.seen = sample
	return j.yes, j.err
}

func TestTransactionAmbiguousOverflowUsesOneJudgeCall(t *testing.T) {
	cfg := &recordingPromptOverflowJudge{transactionTestConfig: newTransactionTestConfig(context.Background()), yes: true}
	cfg.retryMax = 5
	calls := 0
	err := CallAITransaction(cfg, "original prompt must stay out of the judge", func(*AIRequest) (*AIResponse, error) {
		calls++
		rsp := NewUnboundAIResponse()
		rsp.SetModelInfo("local-provider", "local-model")
		rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"), []byte(`{"error":{"code":"upstream_error","message":"the upstream worker rejected the request payload"}}`))
		rsp.Close()
		return rsp, fmt.Errorf("provider rejected request")
	}, func(*AIResponse) error {
		t.Fatal("ambiguous overflow confirmed by the judge must not be parsed as an action")
		return nil
	})
	require.ErrorIs(t, err, ErrPromptTooLong)
	evidence, ok := PromptOverflowEvidenceFrom(err)
	require.True(t, ok)
	require.Equal(t, aispec.PromptOverflowSourceAuxiliary, evidence.Source)
	require.Equal(t, "auxiliary_judge", evidence.MatchedPattern)
	require.Equal(t, "local-provider", evidence.Provider)
	require.Equal(t, "local-model", evidence.Model)
	require.Equal(t, 1, calls)
	require.Equal(t, 1, cfg.calls)
	require.NotContains(t, cfg.seen.Message, "original prompt")
}

func TestTransactionJudgeRejectionPreservesExistingFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		yes  bool
		err  error
	}{
		{"negative", false, nil},
		{"judge_failed", false, fmt.Errorf("judge timeout")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &recordingPromptOverflowJudge{transactionTestConfig: newTransactionTestConfig(context.Background()), yes: tc.yes, err: tc.err}
			cfg.retryMax = 4
			calls := 0
			err := CallAITransaction(cfg, "prompt", func(*AIRequest) (*AIResponse, error) {
				calls++
				rsp := NewUnboundAIResponse()
				rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"), []byte(`{"error":{"message":"the upstream worker rejected the request payload"}}`))
				rsp.Close()
				return rsp, fmt.Errorf("provider rejected request")
			}, func(*AIResponse) error { return nil })
			require.NotErrorIs(t, err, ErrPromptTooLong)
			require.ErrorContains(t, err, "provider rejected request")
			require.Equal(t, 1, calls)
			require.Equal(t, 1, cfg.calls)
		})
	}
}

func TestTransactionConfirmedOverflowDoesNotCallJudge(t *testing.T) {
	cfg := &recordingPromptOverflowJudge{transactionTestConfig: newTransactionTestConfig(context.Background()), yes: false}
	cfg.retryMax = 3
	err := CallAITransaction(cfg, "prompt", func(*AIRequest) (*AIResponse, error) {
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData([]byte("HTTP/1.1 400 Bad Request\r\n\r\n"), []byte(`{"error":{"code":"context_length_exceeded","message":"prompt is too long"}}`))
		rsp.Close()
		return rsp, fmt.Errorf("provider rejected request")
	}, func(*AIResponse) error { return nil })
	require.ErrorIs(t, err, ErrPromptTooLong)
	evidence, ok := PromptOverflowEvidenceFrom(err)
	require.True(t, ok)
	require.NotEqual(t, aispec.PromptOverflowSourceAuxiliary, evidence.Source)
	require.Zero(t, cfg.calls)
}

func TestPromptOverflowJudgeCachesAndRejectsReentry(t *testing.T) {
	cfg := newConfig(context.Background())
	sample := aispec.PromptOverflowAmbiguity{HTTPStatus: 400, Code: "upstream_error", Message: "worker rejected payload"}
	calls := 0
	cfg.LiteForgeExecutor = func(prompt string, _ ...any) (*ForgeResult, error) {
		calls++
		require.Contains(t, prompt, "worker rejected payload")
		require.NotContains(t, prompt, "original user prompt")
		decision, err := cfg.JudgeAmbiguousPromptOverflow(context.Background(), sample)
		require.NoError(t, err)
		require.False(t, decision)
		return &ForgeResult{Action: NewSimpleAction(CallerLabelPromptOverflowClassify, aitool.InvokeParams{"is_prompt_overflow": true})}, nil
	}
	first, err := cfg.JudgeAmbiguousPromptOverflow(context.Background(), sample)
	require.NoError(t, err)
	require.True(t, first)
	second, err := cfg.JudgeAmbiguousPromptOverflow(context.Background(), sample)
	require.NoError(t, err)
	require.True(t, second)
	require.Equal(t, 1, calls)

	calls = 0
	cfg.promptOverflowJudge = promptOverflowJudgeState{}
	cfg.LiteForgeExecutor = func(string, ...any) (*ForgeResult, error) {
		calls++
		return nil, fmt.Errorf("temporary judge failure")
	}
	_, err = cfg.JudgeAmbiguousPromptOverflow(context.Background(), sample)
	require.Error(t, err)
	cfg.LiteForgeExecutor = func(string, ...any) (*ForgeResult, error) {
		calls++
		return &ForgeResult{Action: NewSimpleAction(CallerLabelPromptOverflowClassify, aitool.InvokeParams{"is_prompt_overflow": false})}, nil
	}
	second, err = cfg.JudgeAmbiguousPromptOverflow(context.Background(), sample)
	require.NoError(t, err)
	require.False(t, second)
	require.Equal(t, 2, calls)
}
