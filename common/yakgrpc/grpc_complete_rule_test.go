package yakgrpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

type TestCompleteSyntaxFlowRuleStream struct {
	grpc.ServerStream
	ctx      context.Context
	messages []*ypb.CompleteSyntaxFlowRuleResponse
}

func (s *TestCompleteSyntaxFlowRuleStream) Context() context.Context {
	return s.ctx
}

func (s *TestCompleteSyntaxFlowRuleStream) Send(resp *ypb.CompleteSyntaxFlowRuleResponse) error {
	s.messages = append(s.messages, resp)
	return nil
}

func TestCompleteSyntaxFlowRule(t *testing.T) {
	t.Run("invalid rule content", func(t *testing.T) {
		server := &Server{}
		stream := &TestCompleteSyntaxFlowRuleStream{ctx: context.Background()}

		req := &ypb.CompleteSyntaxFlowRuleRequest{
			RuleContent: "",
			FileName:    "test.sf",
		}

		err := server.CompleteSyntaxFlowRule(req, stream)
		require.Error(t, err)
		require.Contains(t, err.Error(), "rule content is empty")
	})

	t.Run("nil request", func(t *testing.T) {
		server := &Server{}
		stream := &TestCompleteSyntaxFlowRuleStream{ctx: context.Background()}

		err := server.CompleteSyntaxFlowRule(nil, stream)
		require.Error(t, err)
		require.Contains(t, err.Error(), "request is nil")
	})

	t.Run("buildAIOptions validation", func(t *testing.T) {
		req := &ypb.CompleteSyntaxFlowRuleRequest{
			RuleContent: "test",
			FileName:    "test.sf",
			AIType:      "openai",
			APIKey:      "sk-test",
			Model:       "gpt-4",
			Proxy:       "http://proxy:8080",
			Domain:      "api.openai.com",
			BaseURL:     "https://api.openai.com/v1",
		}

		opts := buildAIOptions(req)
		require.NotNil(t, opts)
		require.Equal(t, 6, len(opts)) // AIType, APIKey, Model, Proxy, Domain, BaseURL
	})

	// 注意：本地测试时需要填写AI配置
	t.Run("real AI completion", func(t *testing.T) {
		if utils.InGithubActions() {
			t.Skip("Skipping AI-dependent test in CI")
			return
		}

		server := &Server{}
		stream := &TestCompleteSyntaxFlowRuleStream{ctx: context.Background()}

		req := &ypb.CompleteSyntaxFlowRuleRequest{
			RuleContent: `
desc(
	title: "Golang OS Exec"
	type: audit
	level: high
	risk: ""
	desc: <<<DESC

DESC
	rule_id: "667eac9d-cad7-4b2d-af2b-7cf3c3ed6106"
)


exec?{<fullTypeName>?{have: 'os/exec'}} as $entry
$entry.Command(* #-> as $sink)

http.ResponseWriter as $input
$sink & $input as $high;


desc(
	lang: golang
	alert_high: 1
	"file://unsafe.go": <<<UNSAFE
package main

import (
    "fmt"
    "os/exec"
    "net/http"
)

func executeCommand(userInput string) {
    cmd := exec.Command("echo", userInput)
    output, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    fmt.Println(string(output))
}

func handler(w http.ResponseWriter, r *http.Request) {
	cmd := r.URL.Query().Get("cmd")
	executeCommand(cmd)
}

func main() {
    http.HandleFunc("/", handler)
}

UNSAFE
        "file://safe.go": <<<SAFE
package main

func main() {

}
SAFE
)

exec?{<fullTypeName>?{have: 'os/exec'}} as $entry
$entry.Command(* #-> as $mid)


desc(
        lang: golang
        alert_high: 1
        "file://unsafe.go": <<<UNSAFE
package main

import (
    "fmt"
    "os/exec"
        "net/http"
)

func executeCommand(userInput string) {
    cmd := exec.Command("echo", userInput)
    output, err := cmd.CombinedOutput()
    if err != nil {
        fmt.Println("Error:", err)
        return
    }
    fmt.Println(string(output))
}

func handler(w http.ResponseWriter, r *http.Request) {
        cmd := r.URL.Query().Get("cmd")
        executeCommand(cmd)
}

func main() {
        http.HandleFunc("/", handler)
}
UNSAFE
        "file://safe.go": <<<SAFE
package main

func main() {

}
SAFE
)
`,
			FileName: "test.sf",
			BaseURL:  "",
			APIKey:   "",
			Model:    "",
		}

		err := server.CompleteSyntaxFlowRule(req, stream)
		require.NoError(t, err)

		require.True(t, len(stream.messages) > 0)

		for i, msg := range stream.messages {
			t.Logf("Message %d: Status=%s, Progress=%.2f, Message=%s",
				i+1, msg.Status, msg.Progress, msg.Message)
		}

		lastMsg := stream.messages[len(stream.messages)-1]

		if lastMsg.Status == "completed" {
			require.NotEmpty(t, lastMsg.CompletedRule)
			require.Contains(t, lastMsg.CompletedRule, "exec?{<fullTypeName>?{have: 'os/exec'}}")
			require.Equal(t, 1.0, lastMsg.Progress)

			t.Logf("Completed Rule:\n%s", lastMsg.CompletedRule)
		} else {
			t.Logf("AI completion failed (expected in some cases): %s", lastMsg.Message)
		}
	})

}
