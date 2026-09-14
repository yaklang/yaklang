package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/utils/yakgit"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

func TestHandleAISessionBindPublishesActionableSourceFailure(t *testing.T) {
	// Match the real Git materialization path without relying on external DNS.
	oldPrepare, oldClone := prepareLegionCodeGitWorkspace, cloneLegionCodeGitWorkspace
	t.Cleanup(func() {
		prepareLegionCodeGitWorkspace, cloneLegionCodeGitWorkspace = oldPrepare, oldClone
	})
	cleaned := false
	prepareLegionCodeGitWorkspace = func(context.Context, int) (string, func() error, error) {
		return t.TempDir(), func() error { cleaned = true; return nil }, nil
	}
	cloneLegionCodeGitWorkspace = func(string, string, ...yakgit.Option) error {
		return fmt.Errorf("Get https://user:secret@private.invalid/repo?token=secret: %w", context.DeadlineExceeded)
	}
	bridge, fakeJS, driver := newTestAISessionBridge(t)
	command := validAISessionBindCommand()
	command.ResultContext = validCodeAuditResultContext()
	command.Session.RunId = command.ResultContext.FocusRunId
	command.RuntimeOptionSnapshotJson = mustJSON(map[string]any{
		"source_workspace": validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit),
	})
	if err := bridge.handleAISessionBind(context.Background(), mustMarshalProto(t, command)); err != nil {
		t.Fatal(err)
	}
	if !cleaned {
		t.Fatal("failed clone workspace was not cleaned")
	}
	driver.mu.Lock()
	bindings := len(driver.bindings)
	driver.mu.Unlock()
	if bindings != 0 {
		t.Fatal("source failure started AI runtime")
	}
	first := waitForPublishedMessage(t, fakeJS, 0)
	var workspace aiv1.AISessionEvent
	if err := proto.Unmarshal(first.Data, &workspace); err != nil {
		t.Fatal(err)
	}
	var payload struct{ Code, Message string }
	if err := json.Unmarshal(workspace.GetPayloadJson(), &payload); err != nil {
		t.Fatal(err)
	}
	if workspace.GetEventType() != "source.workspace.failed" || payload.Code != "source_workspace_materialize_failed" {
		t.Fatalf("changed event contract: %v, %#v", workspace.GetEventType(), payload)
	}
	if !strings.Contains(payload.Message, "访问源服务超时") || !strings.Contains(payload.Message, "代理") {
		t.Fatalf("missing cause/action: %s", payload.Message)
	}
	second := waitForPublishedMessage(t, fakeJS, 1)
	var failed aiv1.AISessionFailed
	if err := proto.Unmarshal(second.Data, &failed); err != nil {
		t.Fatal(err)
	}
	if second.Subject != "legion.event.ai.session.failed" || failed.GetErrorCode() != "ai_session_bind_failed" || failed.GetErrorMessage() != payload.Message {
		t.Fatalf("bind failure lost the actionable message: %v", &failed)
	}
	for _, data := range [][]byte{first.Data, second.Data} {
		for _, secret := range []string{"secret", "private.invalid", "user:"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("failure event leaked %q", secret)
			}
		}
	}
}
