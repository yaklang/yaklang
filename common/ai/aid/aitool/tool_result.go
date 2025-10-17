package aitool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// ToolExecutionResult 表示工具执行的完整结果
type ToolExecutionResult struct {
	Stdout string      `json:"stdout"`
	Stderr string      `json:"stderr,omitempty"`
	Result interface{} `json:"result,omitempty"`
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
			"result": map[string]interface{}{
				"description": "工具执行的结果",
			},
		},
		"required": []string{"stdout", "stderr", "result"},
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

	// 创建stdout和stderr的缓冲区
	stdoutBuf := new(bytes.Buffer)
	stderrBuf := new(bytes.Buffer)
	if stdout != nil {
		stdout = io.MultiWriter(stdout, stdoutBuf)
	} else {
		stdout = stdoutBuf
	}
	if stderr != nil {
		stderr = io.MultiWriter(stderr, stderrBuf)
	} else {
		stderr = stderrBuf
	}
	var res any
	var err error
	var finsh = make(chan struct{})
	go func() {
		res, err = t.Callback(ctx, params, runtimeConfig, stdout, stderr)
		close(finsh)
	}()

	var execResult *ToolExecutionResult
	select {
	case <-ctx.Done():
		execResult = &ToolExecutionResult{
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
			Result: res,
		}
		if cancelCallback != nil {
			execResult, err = cancelCallback(execResult, err)
		}
	case <-finsh:
		execResult = &ToolExecutionResult{
			Stdout: stdoutBuf.String(),
			Stderr: stderrBuf.String(),
			Result: res,
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
	requiredFields := []string{"stdout", "stderr", "result"}
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
