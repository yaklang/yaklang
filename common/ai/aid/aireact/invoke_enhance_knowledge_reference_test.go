package aireact

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestReAct_EnhanceKnowledge_WithArtifact tests that knowledge enhancement
// produces summary result and saves knowledge reference to artifacts
func TestReAct_EnhanceKnowledge_WithArtifact(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	manager, token := aicommon.NewMockEKManagerAndToken()

	callback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()

		// First call: AI chooses knowledge enhancement
		if utils.MatchAllOfSubString(prompt, string(ActionDirectlyAnswer), string(ActionRequireTool), string(ActionKnowledgeEnhanceAnswer)) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "knowledge_enhance_answer", "rewrite_user_query_for_knowledge_enhance": "What are the main features of Go?" },
"human_readable_thought": "Using knowledge enhancement.", "cumulative_summary": "Summary."}
`))
			rsp.Close()
			return rsp, nil
		}

		// DirectlyAnswer call
		if utils.MatchAllOfSubString(prompt, `USE THIS FIELD ONLY IF @action is 'directly_answer'`) {
			if !utils.MatchAllOfSubString(prompt, token) {
				return nil, utils.Errorf("knowledge token should appear in prompt")
			}
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_answer", "answer_payload": "Go is a statically typed language with concurrency support." },
"human_readable_thought": "Answering.", "cumulative_summary": "Provided answer."}
`))
			rsp.Close()
			return rsp, nil
		}

		// Verification
		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "Satisfied."}`))
			rsp.Close()
			return rsp, nil
		}

		return nil, utils.Errorf("unexpected prompt: %s", prompt)
	}

	_, err := NewTestReAct(
		aicommon.WithAICallback(callback),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithEnhanceKnowledgeManager(manager),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "Tell me about Go",
		}
	}()

	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var (
		gotKnowledgeEvent bool
		gotResultEvent    bool
		gotArtifactFile   bool
		gotTaskCompleted  bool
		artifactFiles     []string
	)

LOOP:
	for {
		select {
		case e := <-out:
			// Knowledge event
			if e.Type == string(schema.EVENT_TYPE_KNOWLEDGE) {
				if utils.MatchAllOfSubString(string(e.Content), token) {
					gotKnowledgeEvent = true
					log.Infof("✓ Got knowledge event with token")
				}
			}

			// Result event
			if e.Type == string(schema.EVENT_TYPE_RESULT) {
				gotResultEvent = true
				log.Infof("✓ Got result event")
			}

			// Artifact file (knowledge_reference.md saved to artifacts)
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.Content)
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.Contains(filePath, "knowledge_reference") {
					gotArtifactFile = true
					artifactFiles = append(artifactFiles, filePath)
					log.Infof("✓ Got knowledge reference artifact: %s", filePath)
				}
			}

			// Task completion
			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) {
				if utils.MatchAllOfSubString(string(e.Content), string(aicommon.AITaskState_Completed)) {
					gotTaskCompleted = true
				}
			}

			if gotKnowledgeEvent && gotResultEvent && gotArtifactFile && gotTaskCompleted {
				time.Sleep(200 * time.Millisecond)
				break LOOP
			}

		case <-after:
			break LOOP
		}
	}
	close(in)

	// Verify results
	if !gotKnowledgeEvent {
		t.Error("Expected knowledge event but not received")
	}
	if !gotResultEvent {
		t.Error("Expected result event (summary) but not received")
	}
	if !gotArtifactFile {
		t.Error("Expected knowledge reference artifact file but not received")
	}
	if !gotTaskCompleted {
		t.Error("Expected task completion but not received")
	}

	// Verify artifact file content
	for _, filePath := range artifactFiles {
		if utils.FileExists(filePath) {
			content, err := os.ReadFile(filePath)
			if err != nil {
				t.Errorf("Failed to read artifact file: %v", err)
				continue
			}
			contentStr := string(content)

			// Check required fields in markdown format
			expectedFields := []string{"知识增强查询结果", "查询内容", "结果数量", "知识条目", "标题", "类型", "来源", "相关度评分"}
			for _, field := range expectedFields {
				if !strings.Contains(contentStr, field) {
					t.Errorf("Artifact should contain '%s'", field)
				}
			}
			log.Infof("✓ Artifact content verified")
		}
	}

	log.Infof("✓ All checks passed: knowledge=%v, result=%v, artifact=%v, completed=%v",
		gotKnowledgeEvent, gotResultEvent, gotArtifactFile, gotTaskCompleted)

	// Cleanup
	defer func() {
		for _, f := range artifactFiles {
			os.Remove(f)
		}
	}()
}

// TestReAct_EnhanceKnowledge_ArtifactContent verifies artifact file content structure
func TestReAct_EnhanceKnowledge_ArtifactContent(t *testing.T) {
	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	manager, token := aicommon.NewMockEKManagerAndToken()

	callback := func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		prompt := req.GetPrompt()

		if utils.MatchAllOfSubString(prompt, string(ActionDirectlyAnswer), string(ActionRequireTool), string(ActionKnowledgeEnhanceAnswer)) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(fmt.Sprintf(`
{"@action": "object", "next_action": { "type": "knowledge_enhance_answer", "rewrite_user_query_for_knowledge_enhance": "test query %s" },
"human_readable_thought": "test", "cumulative_summary": "test"}
`, token)))
			rsp.Close()
			return rsp, nil
		}

		if utils.MatchAllOfSubString(prompt, `USE THIS FIELD ONLY IF @action is 'directly_answer'`) {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`
{"@action": "object", "next_action": { "type": "directly_answer", "answer_payload": "test answer" },
"human_readable_thought": "test", "cumulative_summary": "test"}
`))
			rsp.Close()
			return rsp, nil
		}

		if utils.MatchAllOfSubString(prompt, "verify-satisfaction", "user_satisfied") {
			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "verify-satisfaction", "user_satisfied": true, "reasoning": "ok"}`))
			rsp.Close()
			return rsp, nil
		}

		return nil, utils.Errorf("unexpected: %s", prompt)
	}

	_, err := NewTestReAct(
		aicommon.WithAICallback(callback),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithEnhanceKnowledgeManager(manager),
		aicommon.WithAgreeYOLO(true),
	)
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		in <- &ypb.AIInputEvent{IsFreeInput: true, FreeInput: "test"}
	}()

	du := time.Duration(8)
	if utils.InGithubActions() {
		du = time.Duration(5)
	}
	after := time.After(du * time.Second)

	var artifactFile string
	var done bool

LOOP:
	for {
		select {
		case e := <-out:
			if e.Type == string(schema.EVENT_TYPE_FILESYSTEM_PIN_FILENAME) {
				content := string(e.Content)
				filePath := utils.InterfaceToString(jsonpath.FindFirst(content, "$.path"))
				if filePath != "" && strings.Contains(filePath, "knowledge_reference") {
					artifactFile = filePath
				}
			}
			if e.Type == string(schema.EVENT_TYPE_STRUCTURED) {
				if utils.MatchAllOfSubString(string(e.Content), string(aicommon.AITaskState_Completed)) {
					done = true
				}
			}
			if done && artifactFile != "" {
				time.Sleep(200 * time.Millisecond)
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}
	close(in)

	if artifactFile == "" {
		t.Fatal("Expected knowledge reference artifact file")
	}

	// Verify file structure
	if !utils.FileExists(artifactFile) {
		t.Fatalf("Artifact file should exist: %s", artifactFile)
	}

	content, err := os.ReadFile(artifactFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	contentStr := string(content)

	// Verify markdown structure
	if !strings.HasPrefix(contentStr, "# 知识增强查询结果") {
		t.Error("Artifact should start with markdown header")
	}

	// Verify sections
	requiredSections := []string{
		"**查询内容**",
		"**结果数量**",
		"## 知识条目 #1",
		"### 详细内容",
	}
	for _, section := range requiredSections {
		if !strings.Contains(contentStr, section) {
			t.Errorf("Artifact should contain section: %s", section)
		}
	}

	log.Infof("✓ Artifact structure verified: %s", artifactFile)

	// Cleanup
	defer os.Remove(artifactFile)
}
