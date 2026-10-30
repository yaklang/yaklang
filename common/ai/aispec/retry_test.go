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
