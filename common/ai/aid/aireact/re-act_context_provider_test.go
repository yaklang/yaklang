package aireact

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestReAct_AllContextProviders(t *testing.T) {
	// 1. Setup test file
	testFileContent := "UniqueFileContent_" + ksuid.New().String()
	tempFile, err := os.CreateTemp("", "test_all_ctx_*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	if _, err := tempFile.WriteString(testFileContent); err != nil {
		t.Fatalf("Failed to write to temp file: %v", err)
	}
	tempFile.Close()

	// 2. Setup knowledge base
	kbName := "test_kb_all_" + ksuid.New().String()
	mockEmbedding := vectorstore.NewDefaultMockEmbedding()
	ragSystem, err := rag.Get(kbName, rag.WithEmbeddingClient(mockEmbedding))
	if err != nil {
		t.Fatalf("Failed to create rag system: %v", err)
	}
	ragSystem.Add("test", mockEmbedding.GenerateRandomText(10))
	defer func() {
		rag.DeleteRAG(consts.GetGormProfileDatabase(), kbName)
	}()

	// 3. Setup AIForge
	testForgeName := "test_forge_all_" + ksuid.New().String()
	forge := &schema.AIForge{
		ForgeName:   testForgeName,
		Description: "Test forge for all context providers",
		ForgeType:   "json",
	}
	err = yakit.CreateAIForge(consts.GetGormProfileDatabase(), forge)
	if err != nil {
		t.Fatalf("Failed to create AIForge: %v", err)
	}
	defer func() {
		yakit.DeleteAIForgeByName(consts.GetGormProfileDatabase(), testForgeName)
	}()

	// 4. Setup test tool
	testToolName := "test_tool_all_" + ksuid.New().String()
	testTool, err := aitool.New(
		testToolName,
		aitool.WithDescription("Test tool for all context providers"),
		aitool.WithStringParam("input"),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			return "test", nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create test tool: %v", err)
	}

	in := make(chan *ypb.AIInputEvent, 10)
	out := make(chan *ypb.AIOutputEvent, 100)

	var promptReceived string
	var promptMutex sync.Mutex
	fileFound := false
	kbFound := false
	forgeFound := false
	toolFound := false

	reactIns, err := NewTestReAct(
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
			out <- e.ToGRPC()
		}),
		aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			promptMutex.Lock()
			promptReceived = req.GetPrompt()
			if utils.MatchAllOfSubString(promptReceived, testFileContent) {
				fileFound = true
			}
			if utils.MatchAllOfSubString(promptReceived, "Knowledge Base Info", kbName) {
				kbFound = true
			}
			if utils.MatchAllOfSubString(promptReceived, "AIForge Info", testForgeName) {
				forgeFound = true
			}
			if utils.MatchAllOfSubString(promptReceived, "AITool Info", testToolName) {
				toolFound = true
			}
			promptMutex.Unlock()

			rsp := i.NewAIResponse()
			rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
			rsp.Close()
			return rsp, nil
		}),
	)
	if err != nil {
		t.Fatalf("Failed to create ReAct instance: %v", err)
	}

	// Add test tool to manager
	err = reactIns.config.GetAiToolManager().AppendTools(testTool)
	if err != nil {
		t.Fatalf("Failed to append test tool: %v", err)
	}

	// Send input event with all AttachedResourceInfo types
	// Key 是内部常量（如 file_path, name），Value 是实际值
	go func() {
		in <- &ypb.AIInputEvent{
			IsFreeInput: true,
			FreeInput:   "Please use all attached resources",
			AttachedResourceInfo: []*ypb.AttachedResourceInfo{
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
					Value: tempFile.Name(),
				},
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_NAME,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_KNOWLEDGE_BASE,
					Value: kbName,
				},
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_NAME,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_AIFORGE,
					Value: testForgeName,
				},
				{
					Key:   aicommon.CONTEXT_PROVIDER_KEY_NAME,
					Type:  aicommon.CONTEXT_PROVIDER_TYPE_AITOOL,
					Value: testToolName,
				},
			},
		}
		close(in)
	}()

	// Wait for AI callback to be triggered
	after := time.After(10 * time.Second)
	allFound := false

LOOP:
	for {
		select {
		case <-out:
			promptMutex.Lock()
			allFound = fileFound && kbFound && forgeFound && toolFound
			promptMutex.Unlock()
			if allFound {
				break LOOP
			}
		case <-after:
			break LOOP
		}
	}

	promptMutex.Lock()
	defer promptMutex.Unlock()

	// Check each context provider result
	if !fileFound {
		t.Errorf("File content not found in prompt. Looking for: %s", testFileContent)
	}
	if !kbFound {
		t.Errorf("Knowledge base info not found in prompt. Looking for: 'Knowledge Base Info' and '%s'", kbName)
	}
	if !forgeFound {
		t.Errorf("AIForge info not found in prompt. Looking for: 'AIForge Info' and '%s'", testForgeName)
	}
	if !toolFound {
		t.Errorf("AITool info not found in prompt. Looking for: 'AITool Info' and '%s'", testToolName)
	}

	if !allFound {
		t.Fatalf("Not all context providers were found in prompt.\nFile: %v, KB: %v, Forge: %v, Tool: %v\nPrompt (first 3000 chars): %s",
			fileFound, kbFound, forgeFound, toolFound, utils.ShrinkString(promptReceived, 3000))
	}

	t.Logf("All context providers via input channel test passed")
}

func TestReAct_ContextProvider_ErrorHandling(t *testing.T) {
	t.Run("FileNotExist_ViaInputChannel", func(t *testing.T) {
		nonExistentFile := "/tmp/non_existent_file_" + ksuid.New().String() + ".txt"

		in := make(chan *ypb.AIInputEvent, 10)
		out := make(chan *ypb.AIOutputEvent, 100)

		var promptReceived string
		var promptMutex sync.Mutex
		errorInPrompt := false

		_, err := NewTestReAct(
			aicommon.WithEventInputChan(in),
			aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				out <- e.ToGRPC()
			}),
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				promptMutex.Lock()
				promptReceived = req.GetPrompt()
				if utils.MatchAllOfSubString(promptReceived, "Error getting context") {
					errorInPrompt = true
				}
				promptMutex.Unlock()

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
		)
		if err != nil {
			t.Fatalf("Failed to create ReAct instance: %v", err)
		}

		go func() {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "Please read this file",
				AttachedResourceInfo: []*ypb.AttachedResourceInfo{
					{
						Key:   aicommon.CONTEXT_PROVIDER_KEY_FILE_PATH,
						Type:  aicommon.CONTEXT_PROVIDER_TYPE_FILE,
						Value: nonExistentFile,
					},
				},
			}
			close(in)
		}()

		after := time.After(10 * time.Second)

	LOOP:
		for {
			select {
			case <-out:
				if errorInPrompt {
					break LOOP
				}
			case <-after:
				break LOOP
			}
		}

		promptMutex.Lock()
		defer promptMutex.Unlock()

		if !errorInPrompt {
			t.Fatalf("Error message should be in prompt for non-existent file.\nPrompt (first 2000 chars): %s",
				utils.ShrinkString(promptReceived, 2000))
		}

		t.Logf("FileNotExist error handling via input channel test passed")
	})

	t.Run("UnknownType_ViaInputChannel", func(t *testing.T) {
		in := make(chan *ypb.AIInputEvent, 10)
		out := make(chan *ypb.AIOutputEvent, 100)

		var promptReceived string
		var promptMutex sync.Mutex
		errorInPrompt := false

		_, err := NewTestReAct(
			aicommon.WithEventInputChan(in),
			aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
				out <- e.ToGRPC()
			}),
			aicommon.WithAICallback(func(i aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
				promptMutex.Lock()
				promptReceived = req.GetPrompt()
				if utils.MatchAllOfSubString(promptReceived, "Error getting context", "unknown context provider type") {
					errorInPrompt = true
				}
				promptMutex.Unlock()

				rsp := i.NewAIResponse()
				rsp.EmitOutputStream(bytes.NewBufferString(`{"@action": "object", "next_action": {"type": "directly_answer", "answer_payload": "test"}, "cumulative_summary": "test summary", "human_readable_thought": "test thought"}`))
				rsp.Close()
				return rsp, nil
			}),
		)
		if err != nil {
			t.Fatalf("Failed to create ReAct instance: %v", err)
		}

		go func() {
			in <- &ypb.AIInputEvent{
				IsFreeInput: true,
				FreeInput:   "Please use this unknown resource",
				AttachedResourceInfo: []*ypb.AttachedResourceInfo{
					{
						Key:  "some_key",
						Type: "unknown_type",
					},
				},
			}
			close(in)
		}()

		after := time.After(10 * time.Second)

	LOOP:
		for {
			select {
			case <-out:
				if errorInPrompt {
					break LOOP
				}
			case <-after:
				break LOOP
			}
		}

		promptMutex.Lock()
		defer promptMutex.Unlock()

		if !errorInPrompt {
			t.Fatalf("Error message should be in prompt for unknown type.\nPrompt (first 2000 chars): %s",
				utils.ShrinkString(promptReceived, 2000))
		}

		t.Logf("UnknownType error handling via input channel test passed")
	})
}
