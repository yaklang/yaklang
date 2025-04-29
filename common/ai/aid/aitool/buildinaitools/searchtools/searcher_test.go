package searchtools

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
)

func TestToolKeywordSummary(t *testing.T) {
	// Create mock tools with keywords
	tools := []*aitool.Tool{
		{
			Tool:     mcp.NewTool("base64-encode"),
			Keywords: []string{"base64", "编码", "encode"},
		},
		{
			Tool:     mcp.NewTool("base64-decode"),
			Keywords: []string{"base64", "解码", "decode"},
		},
		{
			Tool:     mcp.NewTool("url-encode"),
			Keywords: []string{"url", "编码", "encode"},
		},
		{
			Tool:     mcp.NewTool("url-decode"),
			Keywords: []string{"url", "解码", "decode"},
		},
		{
			Tool:     mcp.NewTool("hex-encode"),
			Keywords: []string{"hex", "编码", "encode"},
		},
		{
			Tool:     mcp.NewTool("hex-decode"),
			Keywords: []string{"hex", "解码", "decode"},
		},
	}

	// Test case 1: Query related to encoding
	t.Run("QueryEncoding", func(t *testing.T) {
		query := "我需要对数据进行编码"

		// Mock AI callback
		mockAICallback := func(prompt string) (io.Reader, error) {
			// Verify prompt contains expected data
			if !strings.Contains(prompt, "限制数量") {
				t.Errorf("Prompt missing limit: %s", prompt)
			}
			if !strings.Contains(prompt, query) {
				t.Errorf("Prompt missing query: %s", prompt)
			}

			// Simulate AI response with summary keywords relevant to encoding
			response := struct {
				Result []string `json:"result"`
			}{
				Result: []string{"编码", "数据转换", "加密"},
			}
			jsonResponse, _ := json.Marshal(response)
			return bytes.NewReader(jsonResponse), nil
		}

		// Test with limit = 2
		result, err := ToolKeywordSummary(query, tools, 2, mockAICallback)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should return top 2 keywords
		if len(result) != 2 {
			t.Errorf("Expected 2 keywords, got: %d", len(result))
		}

		if result[0] != "编码" {
			t.Errorf("Expected first keyword to be 编码, got: %s", result[0])
		}
	})

	// Test case 2: Query related to decoding
	t.Run("QueryDecoding", func(t *testing.T) {
		query := "我想解码一段密文"

		// Mock AI callback
		mockAICallback := func(prompt string) (io.Reader, error) {
			// Verify prompt contains expected data
			if !strings.Contains(prompt, "限制数量") {
				t.Errorf("Prompt missing limit: %s", prompt)
			}
			if !strings.Contains(prompt, query) {
				t.Errorf("Prompt missing query: %s", prompt)
			}

			// Simulate AI response with summary keywords relevant to decoding
			response := struct {
				Result []string `json:"result"`
			}{
				Result: []string{"解码", "密文处理", "转换"},
			}
			jsonResponse, _ := json.Marshal(response)
			return bytes.NewReader(jsonResponse), nil
		}

		// Test with limit = 2
		result, err := ToolKeywordSummary(query, tools, 2, mockAICallback)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should return top 2 keywords
		if len(result) != 2 {
			t.Errorf("Expected 2 keywords, got: %d", len(result))
		}

		if result[0] != "解码" {
			t.Errorf("Expected first keyword to be 解码, got: %s", result[0])
		}
	})

	// Test case 3: Empty query
	t.Run("EmptyQuery", func(t *testing.T) {
		query := ""

		// Mock AI callback
		mockAICallback := func(prompt string) (io.Reader, error) {
			// Simulate AI response with general summary keywords
			response := struct {
				Result []string `json:"result"`
			}{
				Result: []string{"编解码", "加密解密", "转换"},
			}
			jsonResponse, _ := json.Marshal(response)
			return bytes.NewReader(jsonResponse), nil
		}

		// Test with limit = 1
		result, err := ToolKeywordSummary(query, tools, 1, mockAICallback)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should return only the first keyword
		if len(result) != 1 {
			t.Errorf("Expected 1 keyword, got: %d", len(result))
		}

		if result[0] != "编解码" {
			t.Errorf("Expected first keyword to be 编解码, got: %s", result[0])
		}
	})

	// Test case 4: Very specific query
	t.Run("VerySpecificQuery", func(t *testing.T) {
		query := "base64编码"

		// Mock AI callback
		mockAICallback := func(prompt string) (io.Reader, error) {
			// Verify prompt contains expected data
			if !strings.Contains(prompt, "限制数量") {
				t.Errorf("Prompt missing limit: %s", prompt)
			}
			if !strings.Contains(prompt, query) {
				t.Errorf("Prompt missing query: %s", prompt)
			}

			// First check that the prompt contains strict requirements for matching
			if !strings.Contains(prompt, "直接相关") {
				t.Errorf("Prompt missing strict requirement phrases: %s", prompt)
			}

			// Simulate AI response with highly specific summary keywords
			response := struct {
				Result []string `json:"result"`
			}{
				Result: []string{"base64编码", "编码"},
			}
			jsonResponse, _ := json.Marshal(response)
			return bytes.NewReader(jsonResponse), nil
		}

		// Test with limit = 2
		result, err := ToolKeywordSummary(query, tools, 2, mockAICallback)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		// Should return exactly matching keyword first
		if len(result) != 2 {
			t.Errorf("Expected 2 keywords, got: %d", len(result))
		}

		if result[0] != "base64编码" {
			t.Errorf("Expected first keyword to exact match 'base64编码', got: %s", result[0])
		}
	})
}
