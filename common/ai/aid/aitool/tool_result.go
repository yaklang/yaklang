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

// ToolExecutionResult is the completed tool-callback envelope.
//
// Result is the semantic execution result. Stdout, Stderr and CombinedOutput
// are observations produced while the callback ran; their wording is not a
// reliable success/failure signal. Keep every field present in JSON so callers
// can distinguish an explicit null semantic result from a missing envelope.
type ToolExecutionResult struct {
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	// CombinedOutput 是 stdout + stderr 的合并输出，按时间顺序交错。
	// 截断保存时只针对 CombinedOutput 做一次，不再分别处理 stdout/stderr。
	CombinedOutput string      `json:"combined_output"`
	Result         interface{} `json:"result"`
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
		"description": "工具调用完成后的结果信封；result 是执行语义，stdout/stderr/combined_output 仅为观察日志",
		"properties": map[string]interface{}{
			"stdout": map[string]interface{}{
				"type":        "string",
				"description": "标准输出观察日志；内容本身不代表执行成功",
			},
			"stderr": map[string]interface{}{
				"type":        "string",
				"description": "标准错误观察日志；非空不等于执行失败",
			},
			"combined_output": map[string]interface{}{
				"type":        "string",
				"description": "stdout + stderr 按时间顺序合并的观察日志",
			},
			"result": map[string]interface{}{
				"description": "工具提供的结构化执行语义；调用方应据此判断命令退出码、HTTP 状态或任务效果",
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
			execResult, callbackErr := cancelCallback(execResult, ctxErr)
			if callbackErr != nil {
				return execResult, callbackErr
			}
		}
		return execResult, ctxErr
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
		err = ctx.Err()
		if cancelCallback != nil {
			var callbackErr error
			execResult, callbackErr = cancelCallback(execResult, err)
			if callbackErr != nil {
				err = callbackErr
			}
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
