package test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/authhack"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const jwtAnalyzeToolName = "jwt_analyze"

func getJWTAnalyzeTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/codec/jwt_analyze.yak")
	if err != nil {
		t.Fatalf("failed to read jwt_analyze.yak from embed FS: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools(jwtAnalyzeToolName, string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse jwt_analyze.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execJWTTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestJWTAnalyze_BasicDecode(t *testing.T) {
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub":  "1234567890",
		"name": "TestUser",
	}, "JWT", []byte("test-secret-key-long-enough"))
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
	})

	assert.Assert(t, strings.Contains(stdout, "HS256"), "should show algorithm")
	assert.Assert(t, strings.Contains(stdout, "1234567890"), "should show sub claim")
	assert.Assert(t, strings.Contains(stdout, "TestUser"), "should show name claim")
	assert.Assert(t, strings.Contains(stdout, "Analysis Complete"), "should complete analysis")
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_VerifyWithCorrectKey(t *testing.T) {
	secret := "my-secret-key-for-jwt-test"
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub": "user123",
	}, "JWT", []byte(secret))
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
		"keys":  secret,
	})

	assert.Assert(t,
		strings.Contains(stdout, "VERIFIED") || strings.Contains(stdout, "KEY_MATCHED"),
		"should verify signature with correct key, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_VerifyWithWrongKey(t *testing.T) {
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub": "user",
	}, "JWT", []byte("correct-secret-key-12345"))
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
		"keys":  "wrong-key-1,wrong-key-2",
	})

	assert.Assert(t, strings.Contains(stdout, "FAILED"), "should report failed verification, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_WeakKeyDetection(t *testing.T) {
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub": "admin",
	}, "JWT", []byte("secret"))
	if err != nil {
		t.Fatalf("failed to generate test JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token":           token,
		"check-weak-keys": "yes",
	})

	assert.Assert(t, strings.Contains(stdout, "CRITICAL"), "should report critical for weak key, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, "secret"), "should report the matched weak key")
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_AlgNone(t *testing.T) {
	token, err := authhack.JwtGenerate("None", map[string]interface{}{
		"sub":  "admin",
		"role": "superuser",
	}, "JWT", nil)
	if err != nil {
		t.Fatalf("failed to generate alg:none JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
	})

	assert.Assert(t, strings.Contains(stdout, "CRITICAL"), "should report critical for alg:none, got:\n%s", stdout)
	assert.Assert(t,
		strings.Contains(stdout, "none") || strings.Contains(stdout, "None"),
		"should mention alg none")
	assert.Assert(t, strings.Contains(stdout, "admin"), "should decode claims even with alg:none")
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_MalformedToken(t *testing.T) {
	tool := getJWTAnalyzeTool(t)
	stdout, stderr := execJWTTool(t, tool, aitool.InvokeParams{
		"token": "not-a-valid-jwt",
	})

	combined := stdout + stderr
	assert.Assert(t,
		strings.Contains(strings.ToLower(combined), "invalid") || strings.Contains(strings.ToLower(combined), "error"),
		"should report error for malformed token, got:\n%s", combined)
	t.Logf("stdout:\n%s\nstderr:\n%s", stdout, stderr)
}

func TestJWTAnalyze_ExpiredToken(t *testing.T) {
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub": "user",
		"exp": float64(1000000),
	}, "JWT", []byte("test-key-long-enough-123"))
	if err != nil {
		t.Fatalf("failed to generate expired JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
	})

	assert.Assert(t, strings.Contains(stdout, "EXPIRED"), "should report token as expired, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_NoExpiration(t *testing.T) {
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub": "admin",
	}, "JWT", []byte("test-key-long-enough-123"))
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
	})

	assert.Assert(t, strings.Contains(stdout, "never expires"),
		"should warn about missing exp claim, got:\n%s", stdout)
	t.Logf("stdout:\n%s", stdout)
}

func TestJWTAnalyze_MultipleKeys(t *testing.T) {
	secret := "the-correct-key-here-12345"
	token, err := authhack.JwtGenerate("HS256", map[string]interface{}{
		"sub": "user",
	}, "JWT", []byte(secret))
	if err != nil {
		t.Fatalf("failed to generate JWT: %v", err)
	}

	tool := getJWTAnalyzeTool(t)
	stdout, _ := execJWTTool(t, tool, aitool.InvokeParams{
		"token": token,
		"keys":  "wrong1,wrong2," + secret + ",wrong3",
	})

	assert.Assert(t,
		strings.Contains(stdout, "VERIFIED") || strings.Contains(stdout, "KEY_MATCHED"),
		"should find the correct key among multiple, got:\n%s", stdout)
	assert.Assert(t, strings.Contains(stdout, secret), "should report which key matched")
	t.Logf("stdout:\n%s", stdout)
}
