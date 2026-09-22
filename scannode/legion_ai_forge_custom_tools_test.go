package scannode

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"github.com/yaklang/yaklang/scannode/inputresolver"
	"google.golang.org/protobuf/proto"
	"net/http"
	"sort"
	"strings"
	"testing"
)

func testCustomToolRelease(t *testing.T) *aiv1.ContextForgeRelease {
	t.Helper()
	r := testLegionContextForgeRelease(t)
	r.CapabilityProfile = legionForgeCustomToolsProfile
	r.DeclaredToolNames = []string{"custom_echo"}
	tool := &aiv1.ContextForgeTool{ToolId: "tool-1", Name: "custom_echo", OwnerUserId: "owner-1", Code: `forgeHandle = func(params) { return "echo:" + params["text"] }`, ParamsJson: []byte(`{"type":"object","properties":{"text":{"type":"string"}}}`)}
	tool.Sha256, _ = contextForgeToolSHA256(tool)
	r.ToolSnapshots = []*aiv1.ContextForgeTool{tool}
	rehashLegionContextForgeRelease(t, r)
	return r
}

func TestCustomForgeToolSnapshotsValidation(t *testing.T) {
	original := testCustomToolRelease(t)
	if err := validateContextForgeRelease(original); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*aiv1.ContextForgeRelease){
		func(r *aiv1.ContextForgeRelease) { r.ToolSnapshots[0].Code += " changed" },
		func(r *aiv1.ContextForgeRelease) { r.ToolSnapshots = append(r.ToolSnapshots, r.ToolSnapshots[0]) },
		func(r *aiv1.ContextForgeRelease) { r.DeclaredToolNames = []string{"other"} },
		func(r *aiv1.ContextForgeRelease) { r.ToolSnapshots[0].ParamsJson = []byte(`[]`) },
		func(r *aiv1.ContextForgeRelease) { r.ToolSnapshots[0].Code = strings.Repeat("x", (256<<10)+1) },
		func(r *aiv1.ContextForgeRelease) { r.CapabilityProfile = legionForgeAdvisoryProfile },
	} {
		r := proto.Clone(original).(*aiv1.ContextForgeRelease)
		mutate(r)
		rehashLegionContextForgeRelease(t, r)
		if validateContextForgeRelease(r) == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
}

func TestCustomForgeCapabilityOnlyAdvertisedForStatelessRuntime(t *testing.T) {
	for _, mode := range []string{aiSessionRuntimeModeStateful, aiSessionRuntimeModeStateless} {
		found := false
		for _, key := range normalizeScanNodeCapabilityKeysForRuntime(nil, mode) {
			if key == capabilityKeyAIForgeCustomToolsV1 {
				found = true
			}
		}
		if found != (mode == aiSessionRuntimeModeStateless) {
			t.Fatalf("mode %s custom capability = %v", mode, found)
		}
	}
}

func TestCustomForgeToolsAreReleaseScoped(t *testing.T) {
	r := testCustomToolRelease(t)
	r.DeclaredToolNames = append(r.DeclaredToolNames, "read_file")
	sort.Strings(r.DeclaredToolNames)
	opts, err := legionForgeCustomToolOptions(context.Background(), r, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := aicommon.NewConfig(context.Background(), opts...)
	manager := cfg.GetAiToolManager()
	if _, err := manager.GetToolByName("read_file"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.GetToolByName("query_file_meta"); err == nil {
		t.Fatal("undeclared managed tool leaked")
	}
	if _, err := manager.GetToolByName("exec_shell"); err == nil {
		t.Fatal("host tool leaked")
	}
	tool, err := manager.GetToolByName("custom_echo")
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"text": "hello"}, aitool.NewToolInvokeConfig())
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || result.Result != "echo:hello" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}

func TestCustomForgeMaterializedFileInput(t *testing.T) {
	if !inputresolver.Supported() {
		t.Skip("managed input unsupported")
	}
	content := "custom input marker"
	command := forgeManagedBindFixture(t, content)
	options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, content) }))
	manager := newAISessionRuntimeManager(newStatelessAIEngineRuntimeDriver())
	ref, err := manager.Bind(context.Background(), command, nil, options)
	if err != nil {
		t.Fatal(err)
	}
	manager.mu.Lock()
	entry := manager.sessions[ref.SessionID]
	manager.mu.Unlock()
	wrapper := entry.handle.(*inputWorkspaceRuntimeHandle)
	t.Cleanup(func() { wrapper.Close("test cleanup") })
	workspace := wrapper.handle.(*statelessAIEngineRuntimeHandle).binding.InputWorkspace
	release := testCustomToolRelease(t)
	path := command.InputManifest.Resources[0].RelativePath
	release.Parameters = []*aiv1.ContextForgeParameter{{Key: "upload", Value: path, ValueKind: "resource"}}
	release.ToolSnapshots[0].Code = `forgeHandle = func(params) { return string(file.ReadFile(params["file-path"])~) }`
	release.ToolSnapshots[0].ParamsJson = []byte(`{"type":"object","properties":{"file-path":{"type":"string"}}}`)
	release.ToolSnapshots[0].Sha256, _ = contextForgeToolSHA256(release.ToolSnapshots[0])
	opts, err := legionForgeCustomToolOptions(context.Background(), release, workspace)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := aicommon.NewConfig(context.Background(), opts...).GetAiToolManager().GetToolByName("custom_echo")
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.ExecuteToolWithCapture(context.Background(), map[string]any{"file-path": path}, aitool.NewToolInvokeConfig())
	if err != nil || result.Result != content {
		t.Fatalf("materialized file result=%#v err=%v", result, err)
	}
	if release.Parameters[0].Value != path {
		t.Fatal("immutable relative path changed")
	}
}
