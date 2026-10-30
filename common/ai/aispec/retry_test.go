package aispec

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

type recoveryClient struct {
	AIClient
	cfg  *AIConfig
	call func(*AIConfig) (string, error)
}

func (c *recoveryClient) GetConfig() *AIConfig                { return c.cfg }
func (c *recoveryClient) Chat(string, ...any) (string, error) { return c.call(c.cfg) }

func globalRecovery(t *testing.T, enabled bool) {
	old := consts.GetTieredAIConfig()
	consts.SetTieredAIConfig(&consts.TieredAIConfig{RetryForever: enabled})
	t.Cleanup(func() { consts.SetTieredAIConfig(old) })
}

func TestGlobalRetryDiscardsFailedAttemptsAndDeliversReasoning(t *testing.T) {
	globalRecovery(t, true)
	var output, reasoning, arguments strings.Builder
	var headers []string
	var waits []time.Duration
	tools, finishes, calls := 0, 0, 0
	client := &recoveryClient{cfg: NewDefaultAIConfig(
		WithStreamHandler(func(r io.Reader) { io.Copy(&output, r) }),
		WithReasonStreamHandler(func(r io.Reader) { io.Copy(&reasoning, r) }),
		WithToolCallArgumentsStreamHandler(func(r io.Reader) { io.Copy(&arguments, r) }),
		WithToolCallCallback(func([]*ToolCall) { tools++ }),
		WithFinishReasonCallback(func(string, []byte) { finishes++ }),
		WithRawHTTPResponseHeaderCallback(func(h []byte) { headers = append(headers, string(h)) }),
	)}
	client.call = func(c *AIConfig) (string, error) {
		calls++
		text := "partial"
		if calls == 9 {
			text = "complete"
		}
		c.StreamHandler(strings.NewReader(text))
		c.ReasonStreamHandler(strings.NewReader(text))
		c.ToolCallArgumentsStreamHandler(strings.NewReader(text))
		c.ToolCallCallback(nil)
		c.FinishReasonCallback("stop", nil)
		if calls < 9 {
			c.RawHTTPResponseHeaderCallback([]byte("HTTP/1.1 429 Too Many Requests\r\nRetry-After: 900\r\n\r\n"))
			return text, errors.New("unknown customer error")
		}
		c.RawHTTPResponseHeaderCallback([]byte("HTTP/1.1 200 OK\r\n\r\n"))
		return text, nil
	}
	result, err := chatWithGlobalRetry(client, "fixture", func(ctx context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		require.Empty(t, output.String())
		require.Zero(t, tools)
		return ctx.Err()
	})
	require.NoError(t, err)
	require.Equal(t, 9, calls)
	require.Equal(t, "complete", result)
	require.Equal(t, result, output.String())
	require.Equal(t, result, reasoning.String())
	require.Equal(t, result, arguments.String())
	require.Equal(t, 1, tools)
	require.Equal(t, 1, finishes)
	require.Equal(t, []string{"HTTP/1.1 200 OK\r\n\r\n"}, headers)
	for _, wait := range waits {
		require.GreaterOrEqual(t, wait, 15*time.Minute)
	}
}

func TestGlobalRetryCancellationAndSwitchOff(t *testing.T) {
	for _, mode := range []string{"disabled", "stop", "switch off", "attempt timeout"} {
		t.Run(mode, func(t *testing.T) {
			globalRecovery(t, mode != "disabled")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			client := &recoveryClient{cfg: NewDefaultAIConfig(WithContext(ctx), WithTimeout(.002))}
			client.call = func(c *AIConfig) (string, error) {
				calls++
				if mode == "attempt timeout" {
					if calls == 2 {
						return "recovered", nil
					}
					<-c.Context.Done()
					return "", c.Context.Err()
				}
				return "", errors.New("failure")
			}
			_, err := chatWithGlobalRetry(client, "fixture", func(context.Context, time.Duration) error {
				if mode == "stop" {
					cancel()
					return ctx.Err()
				}
				if mode == "switch off" {
					consts.SetTieredAIConfig(&consts.TieredAIConfig{})
				}
				return nil
			})
			if mode == "attempt timeout" {
				require.NoError(t, err)
				require.Equal(t, 2, calls)
			} else {
				require.Error(t, err)
				require.Equal(t, 1, calls)
			}
			if mode == "stop" {
				require.ErrorIs(t, err, context.Canceled)
			}
		})
	}
}

func TestGlobalRetryDelayHonorsServerDateAndCancellation(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	require.GreaterOrEqual(t, RetryDelay(100, now.Add(time.Hour).Format(http.TimeFormat), now), time.Hour)
	require.LessOrEqual(t, RetryDelay(100, "", now), 330*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, WaitModelRetry(ctx, time.Hour), context.Canceled)
}

func TestModelFailurePolicy(t *testing.T) {
	for _, tc := range []struct {
		f     *ModelCallFailure
		retry bool
	}{
		{&ModelCallFailure{Kind: "http", StatusCode: 429, RetryAfter: "86400"}, true},
		{&ModelCallFailure{Kind: "http", StatusCode: 429}, true},
		{&ModelCallFailure{Kind: "http", StatusCode: 401, Decision: Retry("force", "")}, false},
		{&ModelCallFailure{Kind: "script", Decision: Retry("force", "")}, false},
		{&ModelCallFailure{Kind: "business"}, false},
		{&ModelCallFailure{Kind: "business", Decision: Retry("busy", "")}, true},
		{&ModelCallFailure{Kind: "protocol"}, true},
		{&ModelCallFailure{Kind: "network", Cause: context.Canceled}, false},
	} {
		require.Equal(t, tc.retry, tc.f.CanRetry(), "%+v", tc.f)
	}
}

func TestUnknownCallbackFailureDoesNotCoolDownOtherRequests(t *testing.T) {
	globalRecovery(t, false)
	queue := &modelRequestQueue{changed: make(chan struct{})}
	policy := ModelRetryOptions{MaxAttempts: 1, acquire: queue.acquire}
	_, err := RunModelCall(context.Background(), policy, func(context.Context) (string, error) {
		return "", errors.New("local provider failed")
	})
	require.Error(t, err)
	require.Equal(t, "unknown", ClassifyModelFailure(err, 0, "").Kind)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	result, err := RunModelCall(ctx, policy, func(context.Context) (string, error) { return "healthy", nil })
	require.NoError(t, err, "an unrelated callback error must not block a healthy model")
	require.Equal(t, "healthy", result)
	require.Equal(t, "network", ClassifyModelFailure(io.ErrUnexpectedEOF, 0, "").Kind)
	require.Equal(t, "http", ClassifyModelFailure(errors.New("rate limit"), 429, "60").Kind)
}

func TestModelCallFiniteBudgetAndNestedAdmission(t *testing.T) {
	globalRecovery(t, false)
	calls, admitted, reports := 0, 0, 0
	policy := ModelRetryOptions{MaxAttempts: 3, Wait: func(context.Context, time.Duration) error { return nil },
		acquire: func(context.Context, func(string)) (ModelAttemptDone, error) {
			admitted++
			return func(bool, bool, time.Duration) time.Duration { reports++; return 0 }, nil
		}}
	_, err := RunModelCall(context.Background(), policy, func(ctx context.Context) (string, error) {
		return RunModelCall(ctx, policy, func(context.Context) (string, error) { calls++; return "", io.ErrUnexpectedEOF })
	})
	require.Error(t, err)
	var failure *ModelCallFailure
	require.ErrorAs(t, err, &failure)
	require.Equal(t, 3, calls)
	require.Equal(t, calls, admitted)
	require.Equal(t, calls, reports)
}

// The injected waiter is used by deterministic policy tests; they do not share
// the process admission queue or perform real cooldown sleeps.
func chatWithGlobalRetry(client AIClient, prompt string, wait func(context.Context, time.Duration) error) (string, error) {
	return chatWithRecovery(client, prompt, ModelRetryOptions{Wait: wait, acquire: func(context.Context, func(string)) (ModelAttemptDone, error) { return nil, nil }})
}

// A routed fallback is another request, while wrappers around that request are not.
func TestModelCallFallbackAdmissionAndTrace(t *testing.T) {
	globalRecovery(t, false)
	admissions, reports := 0, 0
	var scopes []httptrace.Scope
	policy := ModelRetryOptions{MaxAttempts: 2, Wait: func(context.Context, time.Duration) error { return nil }, acquire: func(context.Context, func(string)) (ModelAttemptDone, error) {
		require.Equal(t, admissions, reports, "previous probe must finish before a fallback is admitted")
		admissions++
		return func(bool, bool, time.Duration) time.Duration { reports++; return 0 }, nil
	}}
	_, err := RunModelCall(context.Background(), policy, func(ctx context.Context) (string, error) {
		for i := 0; i < 2; i++ {
			_, _ = RunModelCall(ctx, policy, func(inner context.Context) (string, error) {
				return RunModelCall(inner, policy, func(leaf context.Context) (string, error) {
					scopes = append(scopes, httptrace.GetScope(leaf))
					return "", errors.New("temporary")
				})
			})
		}
		return "", errors.New("temporary")
	})
	require.Error(t, err)
	require.Equal(t, 4, admissions)
	require.Equal(t, 4, reports)
	require.NotEmpty(t, scopes[0].CallID)
	for i, s := range scopes {
		require.Equal(t, scopes[0].CallID, s.CallID)
		require.Equal(t, 1+i/2, s.Attempt)
	}
}
