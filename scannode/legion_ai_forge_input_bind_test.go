package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiengine"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"github.com/yaklang/yaklang/scannode/inputresolver"
	"google.golang.org/protobuf/proto"
)

func forgeManagedBindFixture(t *testing.T, content string) *aiv1.BindAISessionCommand {
	t.Helper()
	command := managedInputBindFixture(t, "log_analysis", content)
	command.ResultContext = nil
	command.RuntimeOptionSnapshotJson = mustJSON(map[string]any{"ai_task_run_id": command.InputManifest.RunId, "ai_task_key": "forge:fixture", "ai_task_version": "1.0.0", "ai_task_definition_checksum": strings.Repeat("a", 64), "ai_task_session_role": "execution", "input_manifest_id": command.InputManifest.ManifestId, "ai_application_attempt_id": command.InputManifest.AttemptId})
	return command
}

func TestManagedForgeActualStatelessBindAndScopedTools(t *testing.T) {
	if !inputresolver.Supported() {
		t.Skip("managed input unsupported")
	}
	for _, kind := range []string{"log", "pcap"} {
		t.Run(kind, func(t *testing.T) {
			content := "authorized log marker\n"
			if kind == "pcap" {
				content = "\xd4\xc3\xb2\xa1\x02\x00\x04\x00" + strings.Repeat("\x00", 8) + "\xff\xff\x00\x00\x01\x00\x00\x00"
			}
			command := forgeManagedBindFixture(t, content)
			if kind == "pcap" {
				resource := command.InputManifest.Resources[0]
				resource.Filename = "capture.pcap"
				resource.RelativePath = "inputs/documents/001-capture.pcap"
				resource.MediaType = "application/vnd.tcpdump.pcap"
				command.Attachments[0].Filename = resource.Filename
				command.Attachments[0].ContentType = resource.MediaType
				if err := inputresolver.Seal(command.InputManifest); err != nil {
					t.Fatal(err)
				}
				var opts map[string]any
				if err := json.Unmarshal(command.RuntimeOptionSnapshotJson, &opts); err != nil {
					t.Fatal(err)
				}
				opts["input_manifest_id"] = command.InputManifest.ManifestId
				command.RuntimeOptionSnapshotJson = mustJSON(opts)
			}
			var downloads atomic.Int32
			options := inputBindOptionsFixture(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				downloads.Add(1)
				if r.URL.Path != "/v1/ai/attachments/input-a/download" || r.Header.Get("Authorization") != "Bearer node-secret" {
					t.Errorf("unexpected download: %s", r.URL.Path)
				}
				fmt.Fprint(w, content)
			}))
			manager := newAISessionRuntimeManager(newStatelessAIEngineRuntimeDriver())
			ref, err := manager.Bind(context.Background(), command, nil, options)
			if err != nil {
				t.Fatal(err)
			}
			manager.mu.Lock()
			entry := manager.sessions[ref.SessionID]
			manager.mu.Unlock()
			wrapper, ok := entry.handle.(*inputWorkspaceRuntimeHandle)
			if !ok {
				t.Fatalf("missing workspace handle: %T", entry.handle)
			}
			t.Cleanup(func() { wrapper.Close("test cleanup") })
			handle, ok := wrapper.handle.(*statelessAIEngineRuntimeHandle)
			if !ok {
				t.Fatalf("not actual stateless driver: %T", wrapper.handle)
			}
			binding := handle.binding
			if binding.InputWorkspace == nil || binding.LegionResultRuntime != nil || downloads.Load() != 1 {
				t.Fatal("Forge unexpectedly required Focus or bypassed download")
			}
			built, err := buildYakAIEngineOptions(context.Background(), binding, noopEmitter{})
			if err != nil {
				t.Fatal(err)
			}
			cfg := aiengine.NewAIEngineConfig(built...)
			if !cfg.DisableToolUse {
				t.Fatal("bind enabled ambient tools before immutable release")
			}
			common := aicommon.NewConfig(context.Background(), cfg.ExtOptions...)
			if _, err := common.GetAiToolManager().GetToolByName("read_file"); err == nil {
				t.Fatal("bind exposed file tools before release")
			}
			release := testLegionContextForgeRelease(t)
			release.CapabilityProfile = legionForgeEvidenceProfile
			release.DeclaredToolNames = append([]string(nil), legionForgeEvidenceTools...)
			release.Parameters = []*aiv1.ContextForgeParameter{{Key: "documents", ValueKind: "resource", Value: command.InputManifest.Resources[0].RelativePath}}
			rehashLegionContextForgeRelease(t, release)
			scoped, _, err := legionForgeCapabilityOptions(context.Background(), release, binding)
			if err != nil {
				t.Fatal(err)
			}
			execution := aicommon.NewConfig(context.Background(), scoped...)
			if execution.DisableToolUse {
				t.Fatal("Forge inherited bind tool-disable instead of scoped execution options")
			}
			tool, err := execution.GetAiToolManager().GetToolByName("query_file_meta")
			if err != nil {
				t.Fatal(err)
			}
			result, err := tool.InvokeWithParams(map[string]any{"path": release.Parameters[0].Value, "runtime_id": "fixture"}, aitool.WithContext(context.Background()))
			if err != nil || !result.Success {
				t.Fatalf("scoped tool: %+v %v", result, err)
			}
			typedName := "read_file_lines"
			if kind == "pcap" {
				typedName = "parse_packet_capture"
			}
			typed, err := execution.GetAiToolManager().GetToolByName(typedName)
			if err != nil {
				t.Fatal(err)
			}
			observed, err := typed.InvokeWithParams(map[string]any{"path": release.Parameters[0].Value, "runtime_id": "typed-fixture"}, aitool.WithContext(context.Background()))
			if err != nil || !observed.Success {
				t.Fatalf("typed %s: %+v %v", typedName, observed, err)
			}
			if kind == "log" && !strings.Contains(fmt.Sprint(observed.Data), "authorized log marker") {
				t.Fatal("scoped log read did not return uploaded content")
			}
			if kind == "log" {
				reader, err := execution.GetAiToolManager().GetToolByName("read_file")
				if err != nil {
					t.Fatal(err)
				}
				alias, err := reader.InvokeWithParams(map[string]any{"file": release.Parameters[0].Value, "runtime_id": "file-alias-fixture"}, aitool.WithContext(context.Background()))
				if err != nil || !alias.Success || !strings.Contains(fmt.Sprint(alias.Data), "authorized log marker") {
					t.Fatalf("managed read_file file alias: %+v %v", alias, err)
				}
				conflict, err := reader.InvokeWithParams(map[string]any{"path": release.Parameters[0].Value, "file": "inputs/other", "runtime_id": "conflict-fixture"}, aitool.WithContext(context.Background()))
				if err == nil && conflict.Success {
					t.Fatal("managed read_file accepted conflicting path and file")
				}
			}
			if _, err := execution.GetAiToolManager().GetToolByName("bash"); err == nil {
				t.Fatal("Forge exposed unrestricted tool")
			}
			for _, field := range []string{"ai_task_run_id", "ai_application_attempt_id", "input_manifest_id"} {
				invalid := binding
				var values map[string]any
				_ = json.Unmarshal(binding.RuntimeOptionSnapshotJSON, &values)
				values[field] = "other"
				invalid.RuntimeOptionSnapshotJSON = mustJSON(values)
				if _, err := buildYakAIEngineOptions(context.Background(), invalid, noopEmitter{}); err == nil {
					t.Fatalf("accepted mismatched %s", field)
				}
			}
			crossOwner := proto.Clone(command).(*aiv1.BindAISessionCommand)
			crossOwner.OwnerUserId = "another-owner"
			if _, err := newAISessionRuntimeManager(newStatelessAIEngineRuntimeDriver()).Bind(context.Background(), crossOwner, nil, options); err == nil {
				t.Fatal("cross-owner manifest bind accepted")
			}
			if downloads.Load() != 1 {
				t.Fatal("unauthorized bind attempted download")
			}
		})
	}
}
