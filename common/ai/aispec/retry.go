package aispec

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/consts"
)

// RetryDelay bounds exponential backoff, never the server's Retry-After.
func RetryDelay(attempt int, retryAfter string, now time.Time) time.Duration {
	base := 2 * time.Second
	for i := 1; i < attempt && base < 5*time.Minute; i++ {
		base = min(base*2, 5*time.Minute)
	}
	delay := base
	if seconds, err := strconv.ParseUint(strings.TrimSpace(retryAfter), 10, 32); err == nil {
		delay = max(delay, time.Duration(seconds)*time.Second)
	} else if until, err := http.ParseTime(retryAfter); err == nil {
		delay = max(delay, until.Sub(now))
	}
	return delay + time.Duration(rand.Int63n(int64(base/10)+1))
}

func WaitModelRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return ctx.Err()
	}
}

// ChatWithGlobalRetry executes every provider through the same request policy.
func ChatWithGlobalRetry(client AIClient, prompt string) (string, error) {
	return chatWithRecovery(client, prompt, ModelRetryOptions{})
}

func chatWithRecovery(client AIClient, prompt string, policy ModelRetryOptions) (string, error) {
	cfg := client.GetConfig()
	original := *cfg
	defer func() { *cfg = original }()
	ctx := original.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if !original.Deadline.IsZero() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(ctx, original.Deadline)
		defer cancel()
	}
	policy.Notify = original.ModelRequestStatus
	return RunModelCall(ctx, policy, func(attemptCtx context.Context) (string, error) {
		bufferOutput := consts.IsAIRetryForeverEnabled()
		*cfg = original
		// Script source owns its HTTP timeout; keep only the caller deadline.
		if original.Type != "yak-script" && original.Timeout > 0 {
			var cancel context.CancelFunc
			attemptCtx, cancel = context.WithTimeout(attemptCtx, time.Duration(original.Timeout*float64(time.Second)))
			defer cancel()
		}
		cfg.Context = attemptCtx

		var mu sync.Mutex
		var deliveries []func()
		var headers []byte
		var callbackErr error
		add := func(deliver func()) {
			if !bufferOutput {
				deliver()
				return
			}
			mu.Lock()
			deliveries = append(deliveries, deliver)
			mu.Unlock()
		}
		captureStream := func(cb func(io.Reader)) func(io.Reader) {
			if cb == nil || !bufferOutput {
				return cb
			}
			return func(r io.Reader) {
				data, err := io.ReadAll(r)
				mu.Lock()
				if err != nil {
					callbackErr = err
				}
				mu.Unlock()
				add(func() { cb(bytes.NewReader(data)) })
			}
		}
		cfg.StreamHandler = captureStream(original.StreamHandler)
		cfg.ReasonStreamHandler = captureStream(original.ReasonStreamHandler)
		cfg.ToolCallArgumentsStreamHandler = captureStream(original.ToolCallArgumentsStreamHandler)
		cfg.HTTPErrorHandler = func(err error) {
			mu.Lock()
			callbackErr = err
			mu.Unlock()
			if !bufferOutput && original.HTTPErrorHandler != nil {
				original.HTTPErrorHandler(err)
			}
		}
		cfg.RawHTTPResponseHeaderCallback = func(b []byte) {
			b = bytes.Clone(b)
			mu.Lock()
			headers = b
			mu.Unlock()
			if original.RawHTTPResponseHeaderCallback != nil {
				add(func() { original.RawHTTPResponseHeaderCallback(b) })
			}
		}
		cfg.RawHTTPResponseCallback = func(h, b []byte) {
			h, b = bytes.Clone(h), bytes.Clone(b)
			mu.Lock()
			headers = h
			mu.Unlock()
			if original.RawHTTPResponseCallback != nil {
				add(func() { original.RawHTTPResponseCallback(h, b) })
			}
		}
		cfg.RawHTTPRequestResponseCallback = func(q, h, b []byte, usage *ChatUsage) {
			q, h, b = bytes.Clone(q), bytes.Clone(h), bytes.Clone(b)
			mu.Lock()
			headers = h
			mu.Unlock()
			if original.RawHTTPRequestResponseCallback != nil {
				add(func() { original.RawHTTPRequestResponseCallback(q, h, b, usage) })
			}
		}
		if original.ToolCallCallback != nil {
			cfg.ToolCallCallback = func(calls []*ToolCall) { add(func() { original.ToolCallCallback(calls) }) }
		}
		if original.FinishReasonCallback != nil {
			cfg.FinishReasonCallback = func(reason string, raw []byte) {
				raw = bytes.Clone(raw)
				add(func() { original.FinishReasonCallback(reason, raw) })
			}
		}
		if original.UsageCallback != nil {
			cfg.UsageCallback = func(usage *ChatUsage) { add(func() { original.UsageCallback(usage) }) }
		}
		text, err := client.Chat(prompt)
		if err == nil {
			err = attemptCtx.Err()
		}
		mu.Lock()
		if err == nil {
			err = callbackErr
		}
		header := bytes.Clone(headers)
		pending := append([]func(){}, deliveries...)
		mu.Unlock()
		retryAfter := ""
		status := 0
		if response, parseErr := http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), nil); parseErr == nil {
			status = response.StatusCode
			retryAfter = response.Header.Get("Retry-After")
			if err == nil && response.StatusCode >= 400 {
				err = fmt.Errorf("AI HTTP %d", response.StatusCode)
			}
			response.Body.Close()
		}
		if err != nil {
			return "", ClassifyModelFailure(err, status, retryAfter)
		}
		for _, deliver := range pending {
			deliver()
		}
		return text, nil
	})
}
