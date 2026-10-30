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
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
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

// ChatWithGlobalRetry is shared by explicit providers and all model tiers,
// including LiteForge's chatter. Configuration validation happens before here.
// The opt-in recovery path buffers one attempt's callbacks: failed HTTP bodies,
// reasoning, partial tool arguments and finish events must not enter an Agent.
// Yak scripts own their admission queue and retain their stable RequestID.
func ChatWithGlobalRetry(client AIClient, prompt string) (string, error) {
	return chatWithGlobalRetry(client, prompt, WaitModelRetry)
}

func chatWithGlobalRetry(client AIClient, prompt string, wait func(context.Context, time.Duration) error) (string, error) {
	if managed, ok := client.(interface{ ManagesModelRetries() bool }); ok && managed.ManagesModelRetries() {
		return client.Chat(prompt)
	}
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
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		reportResult, admissionErr := AcquireModelRequest(ctx, original.ModelRequestStatus)
		if admissionErr != nil {
			return "", admissionErr
		}
		bufferOutput := consts.IsAIRetryForeverEnabled()
		*cfg = original
		scope := httptrace.GetScope(ctx)
		scope.Attempt = attempt
		attemptCtx := httptrace.WithScope(ctx, scope)
		var cancel context.CancelFunc
		if original.Timeout > 0 {
			attemptCtx, cancel = context.WithTimeout(attemptCtx, time.Duration(original.Timeout*float64(time.Second)))
		} else {
			attemptCtx, cancel = context.WithCancel(attemptCtx)
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
		cancel()
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
		delay := RetryDelay(attempt, retryAfter, time.Now())
		if reportResult != nil {
			transient := err != nil && (bufferOutput || status == 408 || status == 429 || status >= 500)
			delay = reportResult(err == nil, transient, delay)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if err == nil {
			for _, deliver := range pending {
				deliver()
			}
			return text, nil
		}
		if !consts.IsAIRetryForeverEnabled() {
			return "", err
		}
		if original.ModelRequestStatus != nil {
			original.ModelRequestStatus(fmt.Sprintf("模型调用失败，全局无限重试已开启；%.1f 秒后进行第 %d 次尝试，可随时停止任务", delay.Seconds(), attempt+1))
		}
		if reportResult == nil {
			if err := wait(ctx, delay); err != nil {
				return "", err
			}
		}
		if !consts.IsAIRetryForeverEnabled() {
			return "", err
		}
	}
}
