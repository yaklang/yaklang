package aitool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// combinedBuffer is a thread-safe buffer that captures stdout and stderr
// writes in real-time order, preserving the actual interleaving of output.
type combinedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *combinedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *combinedBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// ToolExecutionResult 表示工具执行的完整结果
type ToolExecutionResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr,omitempty"`
	// CombinedOutput 是 stdout + stderr 的合并输出，按时间顺序交错。
	// 截断保存时只针对 CombinedOutput 做一次，不再分别处理 stdout/stderr。
	CombinedOutput string      `json:"combined_output"`
	Result         interface{} `json:"result,omitempty"`
}

// ToJSON 将执行结果转换为JSON字符串
func (r *ToolExecutionResult) ToJSON() (string, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// GetJSONSchema 获取结果的JSON Schema
func (r *ToolExecutionResult) GetJSONSchema() map[string]interface{} {
	schema := map[string]interface{}{
		"$schema":     "http://json-schema.org/draft-07/schema#",
		"type":        "object",
		"description": "工具执行的完整结果",
		"properties": map[string]interface{}{
			"stdout": map[string]interface{}{
				"type":        "string",
				"description": "标准输出内容",
			},
			"stderr": map[string]interface{}{
				"type":        "string",
				"description": "标准错误输出内容",
			},
			"combined_output": map[string]interface{}{
				"type":        "string",
				"description": "stdout + stderr 合并输出",
			},
			"result": map[string]interface{}{
				"description": "工具执行的结果",
			},
		},
		"required": []string{"stdout", "stderr", "combined_output", "result"},
	}

	return schema
}

// GetJSONSchemaString 获取结果的JSON Schema字符串
func (r *ToolExecutionResult) GetJSONSchemaString() string {
	schema := r.GetJSONSchema()
	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}
	return string(data)
}

func (t *Tool) ExecuteToolWithCapture(ctx context.Context, params map[string]any, config *ToolInvokeConfig) (*ToolExecutionResult, error) {
	runtimeConfig := config.GetRuntimeConfig()
	stdout, stderr := config.GetStdout(), config.GetStderr()
	cancelCallback := config.GetCancelCallback()

	// combinedBuf captures stdout and stderr writes in real-time order,
	// preserving the actual interleaving of output rather than concatenating
	// stdout+stderr after the fact.
	combinedBuf := &combinedBuffer{}

	// 创建stdout和stderr的缓冲区
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	if config.ShouldCaptureOutput() {
		if stdout != nil {
			stdout = io.MultiWriter(stdout, stdoutBuf, combinedBuf)
		} else {
			stdout = io.MultiWriter(stdoutBuf, combinedBuf)
		}
		if stderr != nil {
			stderr = io.MultiWriter(stderr, stderrBuf, combinedBuf)
		} else {
			stderr = io.MultiWriter(stderrBuf, combinedBuf)
		}
	} else {
		if stdout == nil {
			stdout = io.Discard
		}
		if stderr == nil {
			stderr = io.Discard
		}
	}
	type callbackResult struct {
		value any
		err   error
	}
	// A tool callback is allowed to have external side effects. Do not start it
	// when its context was already cancelled before invocation.
	if ctxErr := ctx.Err(); ctxErr != nil {
		execResult := &ToolExecutionResult{
			Stdout:         "",
			Stderr:         "",
			CombinedOutput: "",
		}
		if cancelCallback != nil {
			return cancelCallback(execResult, ctxErr)
		}
		return execResult, nil
	}
	finished := make(chan callbackResult, 1)
	go func() {
		res, err := t.Callback(ctx, params, runtimeConfig, stdout, stderr)
		finished <- callbackResult{value: res, err: err}
	}()

	var execResult *ToolExecutionResult
	var err error
	select {
	case <-ctx.Done():
		// Do not read callback-owned buffers or result variables while a
		// non-cooperative callback may still be unwinding. Context-aware tools
		// return promptly; the buffered channel lets that goroutine exit even if
		// this cancellation path has already returned.
		execResult = &ToolExecutionResult{
			Stdout:         "",
			Stderr:         "",
			CombinedOutput: "",
		}
		if cancelCallback != nil {
			execResult, err = cancelCallback(execResult, ctx.Err())
		}
	case callback := <-finished:
		err = callback.err
		execResult = &ToolExecutionResult{
			Stdout:         stdoutBuf.String(),
			Stderr:         stderrBuf.String(),
			CombinedOutput: combinedBuf.String(),
			Result:         callback.value,
		}
	}
	return execResult, err
}

// ValidateResult 验证结果是否符合JSON Schema
func ValidateResult(resultJSON string) (bool, []string) {
	// 解析JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resultJSON), &result); err != nil {
		return false, []string{fmt.Sprintf("无法解析结果JSON: %v", err)}
	}

	errors := []string{}

	// 验证必要字段
	requiredFields := []string{"stdout", "stderr", "combined_output", "result"}
	for _, field := range requiredFields {
		if _, exists := result[field]; !exists {
			errors = append(errors, fmt.Sprintf("缺少必要字段: %s", field))
		}
	}

	// 验证stdout和stderr是字符串类型
	if stdout, exists := result["stdout"]; exists {
		if _, ok := stdout.(string); !ok {
			errors = append(errors, "stdout 必须是字符串类型")
		}
	}

	if stderr, exists := result["stderr"]; exists {
		if _, ok := stderr.(string); !ok {
			errors = append(errors, "stderr 必须是字符串类型")
		}
	}

	// 如果有错误，返回false
	if len(errors) > 0 {
		return false, errors
	}

	return true, nil
}
