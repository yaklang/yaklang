package aicommon

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCallAITransaction_APICallErrorAndPostHandlerError 测试当 API 调用和 postHandler 在不同重试中都失败时，
// 错误信息应该包含 API 调用的错误（而不是只包含 postHandler 的错误）
func TestCallAITransaction_APICallErrorAndPostHandlerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 5

	var callCount int64

	// 模拟场景：
	// - 重试 1-2: callAi 成功，但 postHandler 返回解析失败
	// - 重试 3-5: callAi 返回 500 错误
	//
	// 预期：最终错误应该包含 500 错误信息，而不是只有 postHandler 错误
	callAi := func(req *AIRequest) (*AIResponse, error) {
		n := atomic.AddInt64(&callCount, 1)

		// 后几次调用返回 500 错误
		if n >= 3 {
			rsp := NewUnboundAIResponse()
			rsp.SetRawHTTPResponseData(
				[]byte("HTTP/1.1 500 Internal Server Error\r\nContent-Type: application/json\r\n\r\n"),
				[]byte(`{"error":"internal server error","message":"database connection failed"}`),
			)
			return rsp, fmt.Errorf("HTTP 500: internal server error - database connection failed")
		}

		// 前几次调用成功，但响应内容为空（会导致 postHandler 失败）
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData(
			[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(`{"choices":[]}`),
		)
		return rsp, nil
	}

	postHandler := func(rsp *AIResponse) error {
		// 模拟 action 解析失败
		return fmt.Errorf("action type is empty (available_actions=[directly_answer finish])")
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)

	// 验证：错误应该包含 API 调用错误（500），而不是只有 postHandler 错误
	require.Error(t, err, "transaction should fail")
	errMsg := err.Error()

	t.Logf("Error message: %s", errMsg)

	// 关键验证：错误应该包含 500 错误信息
	assert.True(t, strings.Contains(errMsg, "500") || strings.Contains(errMsg, "internal server error") || strings.Contains(errMsg, "database connection failed"),
		"error should contain API call error (500), but got: %s", errMsg)

	// 验证：错误不应该只是 postHandler 的错误
	assert.False(t, strings.Contains(errMsg, "action type is empty") && !strings.Contains(errMsg, "500"),
		"error should not be just postHandler error without API error context")
}

// TestCallAITransaction_APICall401AndPostHandlerError 测试当 API 返回 401 且之前 postHandler 也失败过时，
// 错误信息应该包含 401 错误
func TestCallAITransaction_APICall401AndPostHandlerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 5

	var callCount int64

	// 模拟场景：
	// - 重试 1: callAi 成功，但 postHandler 返回解析失败
	// - 重试 2-3: callAi 返回 401 错误
	callAi := func(req *AIRequest) (*AIResponse, error) {
		n := atomic.AddInt64(&callCount, 1)

		// 第一次调用成功，但响应内容为空
		if n == 1 {
			rsp := NewUnboundAIResponse()
			rsp.SetRawHTTPResponseData(
				[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
				[]byte(`{"choices":[]}`),
			)
			return rsp, nil
		}

		// 后续调用返回 401 认证失败
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData(
			[]byte("HTTP/1.1 401 Unauthorized\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(`{"error":"invalid api key"}`),
		)
		return rsp, fmt.Errorf("HTTP 401: unauthorized - invalid api key")
	}

	postHandler := func(rsp *AIResponse) error {
		// 模拟 action 解析失败
		return fmt.Errorf("failed to parse action: unexpected end of JSON input")
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)

	// 验证：错误应该包含 API 调用错误（401）
	require.Error(t, err, "transaction should fail")
	errMsg := err.Error()

	t.Logf("Error message: %s", errMsg)

	// 关键验证：错误应该包含 401 认证错误
	assert.True(t, strings.Contains(errMsg, "401") || strings.Contains(errMsg, "unauthorized") || strings.Contains(errMsg, "invalid api key"),
		"error should contain API call error (401), but got: %s", errMsg)
}

// TestCallAITransaction_OnlyPostHandlerError 测试当只有 postHandler 失败（API 调用成功）时，
// 错误应该包含 postHandler 的错误
func TestCallAITransaction_OnlyPostHandlerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 3

	// API 调用成功
	callAi := func(req *AIRequest) (*AIResponse, error) {
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData(
			[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(`{"choices":[]}`),
		)
		return rsp, nil
	}

	postHandler := func(rsp *AIResponse) error {
		return fmt.Errorf("action type is empty (available_actions=[directly_answer finish])")
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)

	// 验证：错误应该包含 postHandler 的错误
	require.Error(t, err, "transaction should fail")
	errMsg := err.Error()

	t.Logf("Error message: %s", errMsg)

	// 验证：错误应该包含 postHandler 的错误信息
	assert.True(t, strings.Contains(errMsg, "action type is empty"),
		"error should contain postHandler error, but got: %s", errMsg)
}

// TestCallAITransaction_SuccessfulCall 测试正常调用成功的情况
func TestCallAITransaction_SuccessfulCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := newTransactionTestConfig(ctx)
	cfg.retryMax = 3

	// 调用成功，postHandler 也成功
	callAi := func(req *AIRequest) (*AIResponse, error) {
		rsp := NewUnboundAIResponse()
		rsp.SetRawHTTPResponseData(
			[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\n\r\n"),
			[]byte(`{"choices":[{"message":{"content":"test"}}]}`),
		)
		return rsp, nil
	}

	postHandler := func(rsp *AIResponse) error {
		return nil // 成功
	}

	err := CallAITransaction(cfg, "test prompt", callAi, postHandler)
	require.NoError(t, err, "transaction should succeed")
}
