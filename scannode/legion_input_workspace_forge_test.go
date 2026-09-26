//go:build linux

package scannode

import (
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func TestManagedForgeInputBindUsesTypedManifestWithoutPrivateReleaseCopy(t *testing.T) {
	fixture := func() *aiv1.BindAISessionCommand {
		command := managedInputBindFixture(t, "report", "pcap input")
		command.ResultContext = nil
		manifest := command.InputManifest
		command.RuntimeOptionSnapshotJson = mustJSON(map[string]any{
			"ai_task_run_id": manifest.RunId, "ai_task_session_role": "execution", "ai_task_key": "forge:packet-analysis",
			"ai_task_version": "1.0.0", "ai_task_definition_checksum": strings.Repeat("a", 64),
			"ai_application_attempt_id": manifest.AttemptId, "input_manifest_id": manifest.ManifestId,
		})
		return command
	}
	if err := validateInputWorkspaceBind(fixture()); err != nil {
		t.Fatalf("typed Forge input bind rejected: %v", err)
	}
	for name, mutate := range map[string]func(*aiv1.BindAISessionCommand){
		"owner":            func(c *aiv1.BindAISessionCommand) { c.OwnerUserId = "other" },
		"session":          func(c *aiv1.BindAISessionCommand) { c.Session.SessionId = "other" },
		"missing manifest": func(c *aiv1.BindAISessionCommand) { c.InputManifest = nil },
		"attempt": func(c *aiv1.BindAISessionCommand) {
			c.RuntimeOptionSnapshotJson = []byte(strings.ReplaceAll(string(c.RuntimeOptionSnapshotJson), c.InputManifest.AttemptId, "other"))
		},
		"run": func(c *aiv1.BindAISessionCommand) {
			c.RuntimeOptionSnapshotJson = []byte(strings.ReplaceAll(string(c.RuntimeOptionSnapshotJson), c.InputManifest.RunId, "other"))
		},
		"wrong task kind": func(c *aiv1.BindAISessionCommand) {
			c.RuntimeOptionSnapshotJson = []byte(strings.ReplaceAll(string(c.RuntimeOptionSnapshotJson), "forge:packet-analysis", "log_analysis"))
		},
		"attachment sha": func(c *aiv1.BindAISessionCommand) { c.Attachments[0].Sha256 = strings.Repeat("e", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			command := fixture()
			mutate(command)
			if err := validateInputWorkspaceBind(command); err == nil {
				t.Fatal("invalid Forge identity accepted")
			}
		})
	}
}
