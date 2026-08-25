package scannode

import (
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const testContextFocusExecutionContractJSON = `{"schema_version":"legion.focus-execution/v1","stages":[{"key":"source_prepare"},{"key":"project_understanding"},{"key":"vulnerability_discovery"},{"key":"evidence_verification"},{"key":"report_generation"}],"capabilities":["result.finding.v1","result.report.v1","source.list","source.read","source.search","source.workspace.info","task.stage"],"results":[{"key":"findings","capability":"result.finding.v1","kind":"ai_code_finding"},{"key":"report","capability":"result.report.v1","kind":"ai_code_audit_v1","required":true}]}`

func testContextFocusRelease(entryCode string) *aiv1.ContextFocusRelease {
	entryFile := "test.ai-focus.yak"
	checksum := contextFocusReleaseChecksum(
		"test", "1.0.0", entryFile, entryCode, nil, testContextFocusExecutionContractJSON,
	)
	return &aiv1.ContextFocusRelease{
		ReleaseId:             "test@1.0.0+" + checksum[:12],
		FocusName:             "test",
		RuntimeName:           "legion_release_test_v1_" + checksum[:12],
		Version:               "1.0.0",
		EntryFile:             entryFile,
		EntryCode:             entryCode,
		Sha256:                checksum,
		ExecutionContractJson: testContextFocusExecutionContractJSON,
	}
}

func TestRegisterContextFocusReleaseRegistersImmutableYakBundle(t *testing.T) {
	release := testContextFocusRelease(`__VERBOSE_NAME__ = "Server Test"`)
	runtimeName, err := registerContextFocusRelease(release)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if runtimeName != release.RuntimeName {
		t.Fatalf("runtime name = %q, want %q", runtimeName, release.RuntimeName)
	}
	if _, ok := reactloops.GetLoopFactory(runtimeName); !ok {
		t.Fatalf("runtime %q was not registered", runtimeName)
	}
	second, err := registerContextFocusRelease(release)
	if err != nil || second != runtimeName {
		t.Fatalf("idempotent register = %q, %v", second, err)
	}
}

func TestRegisterContextFocusReleaseRejectsTamperedContent(t *testing.T) {
	release := testContextFocusRelease(`__VERBOSE_NAME__ = "Original"`)
	release.EntryCode = `__VERBOSE_NAME__ = "Tampered"`
	_, err := registerContextFocusRelease(release)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestValidateContextFocusReleaseChecksumsExecutionContract(t *testing.T) {
	release := testContextFocusRelease(`__VERBOSE_NAME__ = "Contract Test"`)
	release.ExecutionContractJson = validFocusExecutionContractJSON(t)
	release.Sha256 = contextFocusReleaseChecksum(
		release.FocusName,
		release.Version,
		release.EntryFile,
		release.EntryCode,
		nil,
		release.ExecutionContractJson,
	)
	release.ReleaseId = release.FocusName + "@" + release.Version + "+" + release.Sha256[:12]
	release.RuntimeName = "legion_release_test_v1_" + release.Sha256[:12]
	validated, err := validateContextFocusRelease(release)
	if err != nil || validated.executionContract == nil {
		t.Fatalf("validate checksummed execution contract: validated=%#v err=%v", validated, err)
	}

	release.ExecutionContractJson = strings.Replace(
		release.ExecutionContractJson,
		"source_prepare",
		"source_validate",
		1,
	)
	if _, err := validateContextFocusRelease(release); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("tampered execution contract was accepted: %v", err)
	}
}

func TestRegisterContextFocusReleaseRejectsNameOverride(t *testing.T) {
	release := testContextFocusRelease(`__NAME__ = "http_fuzztest"`)
	_, err := registerContextFocusRelease(release)
	if err == nil || !strings.Contains(err.Error(), "fixed name") {
		t.Fatalf("expected fixed-name rejection, got %v", err)
	}
}

func TestValidateContextFocusReleaseRejectsPathTraversal(t *testing.T) {
	release := testContextFocusRelease(`__VERBOSE_NAME__ = "Safe"`)
	release.Sidekicks = []*aiv1.ContextFocusSidekick{{Path: "../escape.yak", Content: ""}}
	release.Sha256 = contextFocusReleaseChecksum(release.FocusName, release.Version, release.EntryFile, release.EntryCode, []reactloops.FocusModeSidekick{{Path: "../escape.yak"}})
	release.ReleaseId = release.FocusName + "@" + release.Version + "+" + release.Sha256[:12]
	release.RuntimeName = "legion_release_test_v1_" + release.Sha256[:12]
	_, err := validateContextFocusRelease(release)
	if err == nil || !strings.Contains(err.Error(), "sidekick path") {
		t.Fatalf("expected sidekick path rejection, got %v", err)
	}
}
