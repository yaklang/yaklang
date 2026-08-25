package scannode

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/yaklang/yaklang/common/utils/yakgit"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssagitworkdir"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
)

const testLegionCodeWorkspaceID = "aicw_0123456789abcdef0123456789abcdef"
const testLegionCodeWorkspaceFocusReleaseID = "code_security_audit@1.0.0+abcdef123456"

const testLegionProfessionalTaskFocusMode = "professional_task_fixture"

func testLegionCodeWorkspaceExecutionContract(t *testing.T) *legionFocusExecutionContract {
	t.Helper()
	contract := legionFocusExecutionContract{
		SchemaVersion: legionFocusExecutionContractSchemaV1,
		Stages: []legionFocusExecutionStage{
			{Key: "source_prepare"},
			{Key: "project_understanding"},
			{Key: "vulnerability_discovery"},
			{Key: "evidence_verification"},
			{Key: "report_generation"},
		},
		Capabilities: []string{
			"result.finding.v1", "result.report.v1", "source.list", "source.read",
			"source.search", "source.workspace.info", "task.stage",
		},
		Results: []legionFocusExecutionResultContract{
			{Key: "findings", Capability: "result.finding.v1", Kind: "ai_code_finding"},
			{Key: "report", Capability: "result.report.v1", Kind: "ai_code_audit_v1", Required: true},
		},
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseLegionFocusExecutionContract(string(raw))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func activateTestLegionCodeWorkspaceFocusTurn(t *testing.T, runtime *legionServerFocusRuntime) {
	t.Helper()
	runtime.authorizedFocusReleaseID = testLegionCodeWorkspaceFocusReleaseID
	if err := runtime.activateFocusTurn(testLegionCodeWorkspaceFocusReleaseID, testLegionCodeWorkspaceExecutionContract(t)); err != nil {
		t.Fatalf("activate source workspace Focus Turn: %v", err)
	}
	t.Cleanup(func() {
		runtime.deactivateFocusTurn(testLegionCodeWorkspaceFocusReleaseID)
	})
}

func validLegionCodeWorkspaceSpec(kind string) legionCodeWorkspaceSpec {
	locator := legionCodeWorkspaceArchiveLocator
	if kind == legionCodeWorkspaceKindGit {
		locator = "https://source.invalid/repository.git"
	}
	return legionCodeWorkspaceSpec{
		WorkspaceID:      testLegionCodeWorkspaceID,
		Kind:             kind,
		Locator:          locator,
		ReadOnly:         true,
		MaxReadBytes:     64 * 1024,
		MaxSearchResults: 20,
	}
}

func TestLegionCodeWorkspaceIDRequiresFrozenFormat(t *testing.T) {
	for _, workspaceID := range []string{
		"aicw_0123456789abcdef",
		"AICW_0123456789abcdef0123456789abcdef",
		"aicw_0123456789ABCDEF0123456789ABCDEF",
		"workspace-0123456789abcdef0123456789abcdef",
	} {
		spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
		spec.WorkspaceID = workspaceID
		if err := normalizeLegionCodeWorkspaceSpec(&spec); err == nil || !strings.Contains(err.Error(), "workspace_id is invalid") {
			t.Fatalf("expected frozen workspace id rejection for %q, got %v", workspaceID, err)
		}
	}
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		t.Fatalf("valid frozen workspace id rejected: %v", err)
	}
}

func TestLegionCodeWorkspaceLocatorRejectsPublicCredentialLeaks(t *testing.T) {
	tests := []struct {
		name    string
		locator string
	}{
		{name: "username and password", locator: "https://backend-user:backend-token@source.invalid/repository.git"},
		{name: "token userinfo", locator: "https://backend-token@source.invalid/repository.git"},
		{name: "sensitive query", locator: "https://source.invalid/repository.git?access_token=backend-token"},
		{name: "fragment", locator: "https://source.invalid/repository.git#backend-token"},
		{name: "scp-like sensitive query", locator: "git@source.invalid:org/repository.git?access_token=backend-token"},
		{name: "control character", locator: "https://source.invalid/repository.git\nsecret"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
			spec.Locator = test.locator
			err := normalizeLegionCodeWorkspaceSpec(&spec)
			if err == nil {
				t.Fatal("expected sensitive git locator rejection")
			}
			if strings.Contains(err.Error(), "backend-token") {
				t.Fatalf("locator validation error leaked credential: %v", err)
			}
		})
	}

	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.Locator = "HTTPS://source.invalid/org/repository.git"
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		t.Fatalf("clean git locator rejected: %v", err)
	}
	if spec.Locator != "https://source.invalid/org/repository.git" {
		t.Fatalf("git locator was not canonicalized: %q", spec.Locator)
	}

	spec.Locator = "git@SOURCE.INVALID:org/repository.git"
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		t.Fatalf("clean SCP-like git locator rejected: %v", err)
	}
	if spec.Locator != "git@source.invalid:org/repository.git" {
		t.Fatalf("SCP-like git locator was not canonicalized: %q", spec.Locator)
	}
}

func TestLegionCodeWorkspaceGitLocatorRejectsNodeLocalAndUnsupportedSources(t *testing.T) {
	for _, locator := range []string{
		"/tmp/repository",
		"../repository",
		"repository",
		"./repository",
		"file:///tmp/repository",
		"git://source.invalid/repository.git",
		"https:///repository.git",
		"ssh:///repository.git",
		`C:\repository`,
	} {
		spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
		spec.Locator = locator
		if err := normalizeLegionCodeWorkspaceSpec(&spec); err == nil {
			t.Fatalf("expected node-local or unsupported git locator rejection for %q", locator)
		}
	}
	for _, locator := range []string{
		"http://source.invalid/repository.git",
		"https://source.invalid/repository.git",
		"ssh://source.invalid/org/repository.git",
		"git@source.invalid:org/repository.git",
	} {
		spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
		spec.Locator = locator
		if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
			t.Fatalf("remote git locator %q rejected: %v", locator, err)
		}
	}
	localSpec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	localSpec.Locator = t.TempDir()
	if _, err := materializeLegionCodeGitWorkspace(context.Background(), localSpec); err == nil {
		t.Fatal("direct git materialization accepted a node-local path")
	}
}

func TestLegionCodeWorkspaceArchiveLocatorIsManagedOnly(t *testing.T) {
	for _, locator := range []string{
		"https://backend-user:backend-token@source.invalid/archive.zip",
		"https://source.invalid/archive.zip?access_token=backend-token",
		"managed-source?access_token=backend-token",
		"payload/managed-payload-1",
	} {
		spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindUploadedArchive)
		spec.PayloadID = "payload-test-1"
		spec.Locator = locator
		err := normalizeLegionCodeWorkspaceSpec(&spec)
		if err == nil {
			t.Fatalf("expected unmanaged archive locator rejection for %q", locator)
		}
		if strings.Contains(err.Error(), "backend-token") {
			t.Fatalf("archive locator validation error leaked credential: %v", err)
		}
	}

	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindUploadedArchive)
	spec.PayloadID = "payload-test-1"
	if err := normalizeLegionCodeWorkspaceSpec(&spec); err != nil {
		t.Fatalf("managed archive locator rejected: %v", err)
	}
}

func createLocalGitRepository(t *testing.T) (string, string) {
	t.Helper()
	directory := t.TempDir()
	repository, err := git.PlainInit(directory, false)
	if err != nil {
		t.Fatalf("init git repository: %v", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\n"), 0o640); err != nil {
		t.Fatalf("write git source: %v", err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatalf("open git worktree: %v", err)
	}
	if _, err := worktree.Add("main.go"); err != nil {
		t.Fatalf("add git source: %v", err)
	}
	hash, err := worktree.Commit("initial", &git.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.invalid", When: time.Unix(1, 0),
	}})
	if err != nil {
		t.Fatalf("commit git source: %v", err)
	}
	return directory, hash.String()
}

func TestLegionCodeWorkspaceGitLocksExactRevisionAndCleansUp(t *testing.T) {
	managedRoot := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	source, revision := createLocalGitRepository(t)
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.Branch = "master"
	spec.ExpectedRevision = revision
	originalClone := cloneLegionCodeGitWorkspace
	cloneLegionCodeGitWorkspace = func(_ string, local string, _ ...yakgit.Option) error {
		_, err := git.PlainClone(local, false, &git.CloneOptions{URL: source})
		return err
	}
	t.Cleanup(func() { cloneLegionCodeGitWorkspace = originalClone })

	workspace, err := materializeLegionCodeGitWorkspace(context.Background(), spec)
	if err != nil {
		t.Fatalf("materialize git workspace: %v", err)
	}
	if workspace.lockedRevision != revision || workspace.files != 1 || workspace.sha256 == "" {
		t.Fatalf("unexpected locked workspace: %#v", workspace.info())
	}
	cloneRoot := workspace.root
	if err := workspace.Cleanup(); err != nil {
		t.Fatalf("cleanup git workspace: %v", err)
	}
	if _, err := os.Stat(cloneRoot); !os.IsNotExist(err) {
		t.Fatalf("expected cloned workspace cleanup, stat err=%v", err)
	}

	spec.ExpectedRevision = strings.Repeat("f", 40)
	if _, err := materializeLegionCodeGitWorkspace(context.Background(), spec); err == nil || !strings.Contains(err.Error(), "revision mismatch") {
		t.Fatalf("expected exact revision rejection, got %v", err)
	}
	entries, err := os.ReadDir(managedRoot)
	if err != nil {
		t.Fatalf("read managed root: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "yakgit-") {
			t.Fatalf("failed clone left workspace behind: %s", entry.Name())
		}
	}
}

func TestLegionCodeWorkspaceGitDoesNotRejectLargeProjectFiles(t *testing.T) {
	managedRoot := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	source, _ := createLocalGitRepository(t)
	repository, err := git.PlainOpen(source)
	if err != nil {
		t.Fatal(err)
	}
	largeContent := []byte("const AI_AUDIT_LARGE_FILE = true;\n" + strings.Repeat("x", 2*1024*1024))
	if err := os.WriteFile(filepath.Join(source, "large.js"), largeContent, 0o640); err != nil {
		t.Fatal(err)
	}
	worktree, err := repository.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := worktree.Add("large.js"); err != nil {
		t.Fatal(err)
	}
	revision, err := worktree.Commit("add large source", &git.CommitOptions{Author: &object.Signature{
		Name: "Test", Email: "test@example.invalid", When: time.Unix(2, 0),
	}})
	if err != nil {
		t.Fatal(err)
	}

	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.Branch = "master"
	spec.ExpectedRevision = revision.String()
	spec.MaxFiles = 1
	spec.MaxTotalBytes = 1
	spec.MaxFileBytes = 1
	originalClone := cloneLegionCodeGitWorkspace
	cloneLegionCodeGitWorkspace = func(_ string, local string, _ ...yakgit.Option) error {
		_, err := git.PlainClone(local, false, &git.CloneOptions{URL: source})
		return err
	}
	t.Cleanup(func() { cloneLegionCodeGitWorkspace = originalClone })

	workspace, err := materializeLegionCodeGitWorkspace(context.Background(), spec)
	if err != nil {
		t.Fatalf("materialize git workspace with large source file: %v", err)
	}
	defer workspace.Cleanup()
	if workspace.files != 2 || workspace.bytes <= int64(len(largeContent)) {
		t.Fatalf("large source was not included in workspace metadata: %#v", workspace.info())
	}
	listed, err := workspace.list(map[string]any{"recursive": true})
	if err != nil || listed["count"] != 2 {
		t.Fatalf("large source was not listed: result=%#v err=%v", listed, err)
	}
	read, err := workspace.read(map[string]any{"path": "large.js", "max_bytes": 512})
	if err != nil || read["truncated"] != true || read["file_size"].(int64) != int64(len(largeContent)) {
		t.Fatalf("large source did not support bounded reads: result=%#v err=%v", read, err)
	}
	search, err := workspace.search(map[string]any{"path": "large.js", "query": "AI_AUDIT_LARGE_FILE"})
	if err != nil || search["count"] != 1 {
		t.Fatalf("large source did not support bounded search: result=%#v err=%v", search, err)
	}
	containsLine, err := legionCodeTextContainsLine(filepath.Join(workspace.root, "large.js"), 2)
	if err != nil || !containsLine {
		t.Fatalf("large source line did not support finding validation: contains=%v err=%v", containsLine, err)
	}
}

func TestPrepareLegionCodeWorkspaceSanitizesBackendAuthAndProxy(t *testing.T) {
	managedRoot := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	source, revision := createLocalGitRepository(t)
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.Locator = "https://source.invalid/repository.git"
	spec.ExpectedRevision = revision
	spec.Auth = &legionCodeWorkspaceAuth{
		Kind: "token", UserName: "backend-user", Password: "backend-token",
	}
	spec.Proxy = &legionCodeWorkspaceProxy{
		URL: "http://proxy.invalid:8080", UserName: "proxy-user", Password: "proxy-password",
	}
	raw, err := json.Marshal(yakRuntimeOptions{SourceWorkspace: &spec})
	if err != nil {
		t.Fatal(err)
	}
	originalClone := cloneLegionCodeGitWorkspace
	cloneLegionCodeGitWorkspace = func(_ string, local string, _ ...yakgit.Option) error {
		_, err := git.PlainClone(local, false, &git.CloneOptions{URL: source})
		return err
	}
	t.Cleanup(func() { cloneLegionCodeGitWorkspace = originalClone })

	workspace, publicRaw, err := prepareLegionCodeWorkspace(context.Background(), raw, legionCodeWorkspaceMaterializeOptions{})
	if err != nil {
		t.Fatalf("prepare private source workspace: %v", err)
	}
	defer workspace.Cleanup()
	publicOptions, err := decodeYakRuntimeOptions(publicRaw, true)
	if err != nil {
		t.Fatalf("decode public runtime options: %v", err)
	}
	if publicOptions.SourceWorkspace == nil || publicOptions.SourceWorkspace.Auth != nil || publicOptions.SourceWorkspace.Proxy != nil {
		t.Fatalf("public source workspace retained credentials: %s", publicRaw)
	}
	if publicOptions.SourceWorkspace.WorkspaceID != spec.WorkspaceID || publicOptions.SourceWorkspace.ExpectedRevision != revision {
		t.Fatalf("public source workspace lost its pin: %#v", publicOptions.SourceWorkspace)
	}
}

func TestAISessionRuntimeBindPublishesSourceLockAndCleansWorkspaceOnCancel(t *testing.T) {
	managedRoot := t.TempDir()
	t.Setenv(ssagitworkdir.WorkDirEnv, managedRoot)
	t.Setenv(ssagitworkdir.MinFreeBytesEnv, "0")
	source, revision := createLocalGitRepository(t)
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.Branch = "master"
	spec.ExpectedRevision = revision
	originalClone := cloneLegionCodeGitWorkspace
	cloneLegionCodeGitWorkspace = func(_ string, local string, _ ...yakgit.Option) error {
		_, err := git.PlainClone(local, false, &git.CloneOptions{URL: source})
		return err
	}
	t.Cleanup(func() { cloneLegionCodeGitWorkspace = originalClone })
	runtimeRaw, err := json.Marshal(yakRuntimeOptions{SourceWorkspace: &spec})
	if err != nil {
		t.Fatal(err)
	}
	command := validAISessionBindCommand()
	command.Session.RunId = "code-run-1"
	command.RuntimeOptionSnapshotJson = runtimeRaw
	command.ResultContext = validCodeAuditResultContext()
	publisher := &recordingAIFocusRiskPublisher{}
	resultSink, err := newLegionAIFocusResultSink(publisher, command.Metadata.CommandId, command.ResultContext)
	if err != nil {
		t.Fatal(err)
	}
	driver := &recordingAISessionRuntimeDriver{}
	manager := newAISessionRuntimeManager(driver)
	ref, err := manager.Bind(context.Background(), command, nil, aiSessionRuntimeBindOptions{ResultSink: resultSink})
	if err != nil {
		t.Fatalf("bind source workspace runtime: %v", err)
	}
	if len(publisher.assets) != 1 || publisher.assets[0].assetKind != "source_locked" {
		t.Fatalf("source_locked asset was not published: %#v", publisher.assets)
	}
	driver.mu.Lock()
	if len(driver.bindings) != 1 {
		driver.mu.Unlock()
		t.Fatalf("unexpected runtime bind count: %d", len(driver.bindings))
	}
	publicOptions, err := decodeYakRuntimeOptions(driver.bindings[0].RuntimeOptionSnapshotJSON, true)
	resultRuntime := driver.bindings[0].LegionResultRuntime
	driver.mu.Unlock()
	if err != nil || publicOptions.SourceWorkspace == nil || publicOptions.SourceWorkspace.Auth != nil || publicOptions.SourceWorkspace.Proxy != nil {
		t.Fatalf("runtime did not receive public source workspace snapshot: options=%#v err=%v", publicOptions, err)
	}
	manager.mu.Lock()
	workspacePath := manager.sessions[ref.SessionID].codeWorkspace.root
	workspaceSHA256 := manager.sessions[ref.SessionID].codeWorkspace.sha256
	manager.mu.Unlock()
	findingParams := map[string]any{
		"workspace_id":        testLegionCodeWorkspaceID,
		"file":                "main.go",
		"start_line":          1,
		"end_line":            1,
		"cwe":                 "CWE-89",
		"vulnerability_type":  "SQL injection",
		"category":            "injection",
		"severity":            "high",
		"confidence":          0.95,
		"verification_status": "confirmed",
		"title":               "SQL injection",
		"description":         "Untrusted input reaches a query.",
		"evidence":            "The query is assembled without parameters.",
		"data_flow":           "input -> query",
		"exploit_scenario":    "An attacker controls the query input.",
		"recommendation":      "Use parameterized queries.",
		"dedupe_key":          "model-selected-key",
	}
	if _, err := resultRuntime.Execute(serverFocusCapabilitySubmitCodeFinding, findingParams); err == nil {
		t.Fatal("source result capability was available before the authorized Focus Turn")
	}
	focusRuntime, ok := resultRuntime.(*legionServerFocusRuntime)
	if !ok {
		t.Fatalf("unexpected source result runtime type: %T", resultRuntime)
	}
	activateTestLegionCodeWorkspaceFocusTurn(t, focusRuntime)
	if _, err := resultRuntime.Execute(serverFocusCapabilitySubmitCodeFinding, findingParams); err != nil {
		t.Fatalf("submit bound source code finding: %v", err)
	}
	if len(publisher.risks) != 1 {
		t.Fatalf("bound source code finding was not published: %#v", publisher.risks)
	}
	var boundFinding aiFocusCodeFinding
	if err := json.Unmarshal(publisher.risks[0].raw, &boundFinding); err != nil {
		t.Fatalf("decode bound source code finding: %v", err)
	}
	if boundFinding.LockedRevision != revision || boundFinding.SourceSHA256 != workspaceSHA256 || boundFinding.DedupeKey == "model-selected-key" {
		t.Fatalf("finding did not inherit immutable bound source evidence: %#v", boundFinding)
	}
	cancelCommand := validAISessionCancelCommand()
	cancelCommand.Session.RunId = "code-run-1"
	cancelled, err := manager.Cancel(cancelCommand)
	if err != nil {
		t.Fatalf("cancel source workspace runtime: %v", err)
	}
	if cancelled.applyHandle {
		cancelled.handle.Cancel(cancelled.reason)
	}
	if err := manager.CompleteTerminal(cancelled.ref, "cancel"); err != nil {
		t.Fatalf("complete source workspace cancel: %v", err)
	}
	if _, err := os.Stat(workspacePath); !os.IsNotExist(err) {
		t.Fatalf("source workspace survived cancel: stat err=%v", err)
	}
}

func zipPayload(t *testing.T, entries map[string][]byte, modes map[string]os.FileMode) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		if mode, ok := modes[name]; ok {
			header.SetMode(mode)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buffer.Bytes()
}

func TestLegionCodeWorkspaceArchiveRejectsUnsafeZipHashAndDownloadLimit(t *testing.T) {
	limits := legionCodeArchiveLimits{MaxFiles: 100, MaxTotalBytes: 4 * 1024 * 1024}
	for name, archive := range map[string][]byte{
		"traversal": zipPayload(t, map[string][]byte{"../escape.go": []byte("bad")}, nil),
		"backslash": zipPayload(t, map[string][]byte{"dir\\escape.go": []byte("bad")}, nil),
		"symlink": zipPayload(t, map[string][]byte{"link": []byte("../outside")}, map[string]os.FileMode{
			"link": os.ModeSymlink | 0o777,
		}),
	} {
		t.Run(name, func(t *testing.T) {
			archivePath := filepath.Join(t.TempDir(), "source.zip")
			if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := extractLegionCodeWorkspaceZip(archivePath, t.TempDir(), limits); err == nil {
				t.Fatal("expected unsafe zip rejection")
			}
		})
	}

	largeSource := bytes.Repeat([]byte("x"), 2*1024*1024)
	largeArchive := zipPayload(t, map[string][]byte{"large.js": largeSource}, nil)
	largeArchivePath := filepath.Join(t.TempDir(), "large-source.zip")
	if err := os.WriteFile(largeArchivePath, largeArchive, 0o600); err != nil {
		t.Fatal(err)
	}
	largeDestination := t.TempDir()
	if err := extractLegionCodeWorkspaceZip(largeArchivePath, largeDestination, limits); err != nil {
		t.Fatalf("archive rejected a common large source file: %v", err)
	}
	files, total, _, err := inspectLegionCodeWorkspace(largeDestination)
	if err != nil || files != 1 || total != int64(len(largeSource)) {
		t.Fatalf("unexpected extracted large source metadata: files=%d total=%d err=%v", files, total, err)
	}
	if err := extractLegionCodeWorkspaceZip(largeArchivePath, t.TempDir(), legionCodeArchiveLimits{
		MaxFiles: 10, MaxTotalBytes: int64(len(largeSource)) - 1,
	}); err == nil || !strings.Contains(err.Error(), "safety limit") {
		t.Fatalf("expected archive aggregate safety rejection, got %v", err)
	}

	body := zipPayload(t, map[string][]byte{"main.go": []byte("package main\n")}, nil)
	hash := sha256.Sum256(body)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer node-token" || request.URL.Query().Get("node_session_id") != "node-session" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = response.Write(body)
	}))
	defer server.Close()
	if _, _, err := downloadManagedSourcePayloadBounded(
		context.Background(), server.Client(), server.URL, "node-session", "node-token", "payload-1", int64(len(body))-1, "",
	); err == nil || !strings.Contains(err.Error(), "max bytes") {
		t.Fatalf("expected bounded download rejection, got %v", err)
	}
	if _, _, err := downloadManagedSourcePayloadBounded(
		context.Background(), server.Client(), server.URL, "node-session", "node-token", "payload-1", int64(len(body)), strings.Repeat("0", 64),
	); err == nil || !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Fatalf("expected hash rejection, got %v", err)
	}
	path, actual, err := downloadManagedSourcePayloadBounded(
		context.Background(), server.Client(), server.URL, "node-session", "node-token", "payload-1", int64(len(body)), hex.EncodeToString(hash[:]),
	)
	if err != nil {
		t.Fatalf("download trusted archive: %v", err)
	}
	defer os.Remove(path)
	if actual != hex.EncodeToString(hash[:]) {
		t.Fatalf("unexpected archive hash %s", actual)
	}
}

func TestLegionCodeWorkspaceReadOnlyToolsEnforceTraversalSymlinkAndBudgets(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("package main\nfunc vulnerable() {}\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary.bin"), []byte{'a', 0, 'b'}, 0o640); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.go")
	if err := os.WriteFile(outside, []byte("secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.go")); err != nil {
		t.Fatal(err)
	}
	spec := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	spec.MaxReadBytes = 24
	workspace := &legionCodeWorkspaceRuntime{spec: spec, root: root, files: 3}
	runtimeValue, err := newLegionServerFocusRuntime(
		context.Background(), "https://workspace.invalid/"+testLegionCodeWorkspaceID+"/", &recordingServerFocusSink{}, workspace,
	)
	if err != nil {
		t.Fatalf("new source runtime: %v", err)
	}
	runtime := runtimeValue.(*legionServerFocusRuntime)
	if _, err := runtime.Execute(serverFocusCapabilitySourceWorkspaceInfo, nil); err == nil || !strings.Contains(err.Error(), "authorized Focus Turn") {
		t.Fatalf("expected dormant workspace rejection before the Focus Turn, got %v", err)
	}
	runtime.authorizedFocusReleaseID = testLegionCodeWorkspaceFocusReleaseID
	if err := runtime.activateFocusTurn("code_security_audit@1.0.0+000000000000"); err == nil {
		t.Fatal("expected mismatched Focus release activation rejection")
	}
	activateTestLegionCodeWorkspaceFocusTurn(t, runtime)
	var phaseEventType string
	var phaseEventPayload []byte
	runtime.emitEvent = func(eventType string, payload []byte) {
		phaseEventType = eventType
		phaseEventPayload = append([]byte(nil), payload...)
	}
	listed, err := runtime.Execute(serverFocusCapabilitySourceList, map[string]any{"recursive": true})
	if err != nil {
		t.Fatalf("source.list: %v", err)
	}
	files := listed["files"].([]map[string]any)
	if len(files) != 1 || files[0]["path"] != "src/main.go" {
		t.Fatalf("binary or escaping symlink leaked into list: %#v", listed)
	}
	read, err := runtime.Execute(serverFocusCapabilitySourceRead, map[string]any{"path": "src/main.go", "max_bytes": 13})
	if err != nil || read["content"] != "package main\n" || read["truncated"] != true {
		t.Fatalf("unexpected bounded read: result=%#v err=%v", read, err)
	}
	search, err := runtime.Execute(serverFocusCapabilitySourceSearch, map[string]any{"query": "vulnerable", "path": "src"})
	if err != nil {
		t.Fatalf("source.search: %v", err)
	}
	results := search["results"].([]map[string]any)
	if search["truncated"] != false || len(results) != 1 || results[0]["path"] != "src/main.go" || search["scanned_bytes"].(int64) <= spec.MaxReadBytes {
		t.Fatalf("workspace search was incorrectly limited by the per-read byte budget: %#v", search)
	}
	for _, bad := range []string{"../secret.go", "/etc/passwd", "src/../binary.bin", "escape.go"} {
		if _, err := runtime.Execute(serverFocusCapabilitySourceRead, map[string]any{"path": bad}); err == nil {
			t.Fatalf("expected source.read rejection for %q", bad)
		}
	}
	if _, err := runtime.Execute(serverFocusCapabilityHTTPRequest, nil); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected HTTP capability disabled, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilityExtractReferences, nil); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected web reference capability disabled, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitRisk, map[string]any{"verified": true}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected generic risk capability disabled, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilitySubmitAsset, map[string]any{}); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected generic asset capability disabled, got %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilityProgressPhase, map[string]any{
		"phase": "evidence_verification", "status": "started", "progress": 0.5,
	}); err != nil {
		t.Fatalf("publish code audit phase: %v", err)
	}
	if _, err := runtime.Execute(serverFocusCapabilityProgressPhase, map[string]any{
		"phase": "model_selected_phase", "status": "started", "progress": 0.5,
	}); err == nil {
		t.Fatal("progress.phase accepted a phase outside the immutable lifecycle")
	}
	if phaseEventType != "task.stage" || !bytes.Contains(phaseEventPayload, []byte(`"workspace_id":"`+testLegionCodeWorkspaceID+`"`)) {
		t.Fatalf("unexpected code audit phase event: type=%q payload=%s", phaseEventType, phaseEventPayload)
	}
	info, err := runtime.Execute(serverFocusCapabilitySourceWorkspaceInfo, nil)
	if err != nil {
		t.Fatalf("workspace info: %v", err)
	}
	raw, _ := json.Marshal(info)
	if bytes.Contains(raw, []byte("auth")) || bytes.Contains(raw, []byte("proxy")) {
		t.Fatalf("workspace info leaked private fields: %s", raw)
	}
}

func TestStatelessCodeWorkspacePinRejectsMismatchAndPrivateFields(t *testing.T) {
	bound := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	bound.ExpectedRevision = strings.Repeat("a", 40)
	bound.ExpectedSHA256 = strings.Repeat("b", 64)
	bindRaw, _ := json.Marshal(yakRuntimeOptions{SourceWorkspace: &bound})
	public := bound
	contextRaw, _ := json.Marshal(yakRuntimeOptions{SourceWorkspace: &public})
	if err := validateLegionCodeWorkspaceContextPin(bindRaw, contextRaw); err != nil {
		t.Fatalf("matching pin rejected: %v", err)
	}
	public.ExpectedRevision = strings.Repeat("c", 40)
	contextRaw, _ = json.Marshal(yakRuntimeOptions{SourceWorkspace: &public})
	if err := validateLegionCodeWorkspaceContextPin(bindRaw, contextRaw); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected pin mismatch, got %v", err)
	}
	public = bound
	public.Auth = &legionCodeWorkspaceAuth{Kind: "token", Password: "secret"}
	contextRaw, _ = json.Marshal(yakRuntimeOptions{SourceWorkspace: &public})
	if err := validateLegionCodeWorkspaceContextPin(bindRaw, contextRaw); err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("expected private context rejection, got %v", err)
	}
	public = bound
	public.Locator = "https://source.invalid/repository.git?access_token=backend-token"
	contextRaw, _ = json.Marshal(yakRuntimeOptions{SourceWorkspace: &public})
	if err := validateLegionCodeWorkspaceContextPin(bindRaw, contextRaw); err == nil || !strings.Contains(err.Error(), "locator is invalid") {
		t.Fatalf("expected sensitive context locator rejection, got %v", err)
	} else if strings.Contains(err.Error(), "backend-token") {
		t.Fatalf("context locator rejection leaked credential: %v", err)
	}
}

func validCodeAuditResultContext() *aiv1.AIFocusResultContext {
	return &aiv1.AIFocusResultContext{
		FocusRunId:     "code-run-1",
		FocusMode:      testLegionProfessionalTaskFocusMode,
		FocusReleaseId: "code_security_audit@1.0.0+abcdef123456",
		SchemaVersion:  legionAIFocusResultSchemaV1,
		ExecutionMode:  "single_run",
		TargetUrl:      "https://workspace.invalid/" + testLegionCodeWorkspaceID + "/",
		Job: &jobv1.JobRef{
			JobId: "job-code-1", SubtaskId: "subtask-code-1", AttemptId: "attempt-code-1",
		},
	}
}

func validCodeFinding() aiFocusCodeFinding {
	return aiFocusCodeFinding{
		WorkspaceID:        testLegionCodeWorkspaceID,
		File:               "src/main.go",
		StartLine:          10,
		EndLine:            14,
		CWE:                "CWE-89",
		VulnerabilityType:  "SQL injection",
		Category:           "injection",
		Module:             "database",
		Severity:           "high",
		Confidence:         0.95,
		VerificationStatus: "confirmed",
		Title:              "Untrusted input reaches SQL query",
		Description:        "Request input is concatenated into a SQL statement.",
		Evidence:           "The source value reaches the query call without parameterization.",
		DataFlow:           "request.query -> buildQuery -> db.Exec",
		ExploitScenario:    "An attacker supplies a crafted query parameter.",
		Recommendation:     "Use a parameterized query.",
		DedupeKey:          "src/main.go:10:CWE-89",
	}
}

func TestLegionCodeFindingAndAuditReportContracts(t *testing.T) {
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "bind-code-1", validCodeAuditResultContext())
	if err != nil {
		t.Fatalf("new code audit sink: %v", err)
	}
	lockedRevision := strings.Repeat("a", 40)
	sourceSHA256 := strings.Repeat("b", 64)
	if err := rawSink.(aiFocusCodeWorkspaceEvidenceBinder).bindCodeWorkspaceEvidence(lockedRevision, sourceSHA256); err != nil {
		t.Fatalf("bind code workspace evidence: %v", err)
	}
	sink := rawSink.(aiFocusCodeResultSink)
	if err := rawSink.(aiFocusExecutionContractBinder).bindFocusExecutionContract(testLegionCodeWorkspaceExecutionContract(t)); err != nil {
		t.Fatalf("bind Focus execution contract: %v", err)
	}
	finding := validCodeFinding()
	finding.LockedRevision = "model-selected-revision"
	finding.SourceSHA256 = strings.Repeat("f", 64)
	receipt, err := sink.SubmitCodeFinding(context.Background(), "ai_code_finding", finding)
	if err != nil {
		t.Fatalf("submit code finding: %v", err)
	}
	if receipt.DedupeKey == finding.DedupeKey || len(receipt.DedupeKey) != sha256.Size*2 || !isLowerHex(receipt.DedupeKey) || len(publisher.risks) != 1 {
		t.Fatalf("unexpected code finding receipt: %#v risks=%#v", receipt, publisher.risks)
	}
	published := publisher.risks[0]
	if published.riskKind != "ai_code_finding" ||
		published.target != "https://workspace.invalid/"+testLegionCodeWorkspaceID+"/src/main.go" {
		t.Fatalf("unexpected code finding publication: %#v", published)
	}
	var publishedFinding aiFocusCodeFinding
	if err := json.Unmarshal(published.raw, &publishedFinding); err != nil {
		t.Fatalf("decode code finding payload: %v", err)
	}
	if publishedFinding.DedupeKey != receipt.DedupeKey ||
		publishedFinding.LockedRevision != lockedRevision ||
		publishedFinding.SourceSHA256 != sourceSHA256 {
		t.Fatalf("code finding payload did not use bound identity and source evidence: %#v", publishedFinding)
	}

	multiLine := validCodeFinding()
	multiLine.File = "src/SpringBootController.java"
	multiLine.StartLine = 60
	multiLine.StartColumn = 13
	multiLine.EndLine = 85
	multiLine.EndColumn = 9
	multiLine.Title = "Unsafe deserialization across a multi-line data flow"
	if _, err := sink.SubmitCodeFinding(context.Background(), "ai_code_finding", multiLine); err != nil {
		t.Fatalf("multi-line finding may end at an earlier column on its final line: %v", err)
	}

	invalidSameLine := validCodeFinding()
	invalidSameLine.StartLine = 60
	invalidSameLine.StartColumn = 13
	invalidSameLine.EndLine = 60
	invalidSameLine.EndColumn = 9
	if _, err := sink.SubmitCodeFinding(context.Background(), "ai_code_finding", invalidSameLine); err == nil || !strings.Contains(err.Error(), "column range is invalid") {
		t.Fatalf("same-line reversed columns must remain invalid, got %v", err)
	}

	safe := validCodeFinding()
	safe.VerificationStatus = "safe"
	if _, err := sink.SubmitCodeFinding(context.Background(), "ai_code_finding", safe); err == nil || !strings.Contains(err.Error(), "confirmed or uncertain") {
		t.Fatalf("expected safe finding to be rejected from JobRisk, got %v", err)
	}
	if err := rawSink.(*legionAIFocusResultSink).Succeed(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "ai_code_audit_v1") {
		t.Fatalf("expected dedicated report completion gate, got %v", err)
	}

	report, err := sink.SubmitCodeAuditReport(context.Background(), "ai_code_audit_v1", aiFocusCodeAuditReport{
		WorkspaceID: testLegionCodeWorkspaceID,
		Markdown:    "# Code audit\n\nOne confirmed finding; three safe candidates.",
		StructuredSummary: json.RawMessage(`{
			"confirmed_count":1,
			"uncertain_count":0,
			"safe_count":3
		}`),
	})
	if err != nil {
		t.Fatalf("submit code audit report: %v", err)
	}
	if report.ResultID != focusResultReportEventID("job-code-1", "ai_code_audit_v1") ||
		len(publisher.reportKinds) != 1 || publisher.reportKinds[0] != "ai_code_audit_v1" {
		t.Fatalf("unexpected dedicated report: receipt=%#v publisher=%#v", report, publisher)
	}
	if err := rawSink.(*legionAIFocusResultSink).Succeed(context.Background(), []byte(`{"completed":true}`)); err != nil {
		t.Fatalf("complete code audit: %v", err)
	}
	if len(publisher.reportKinds) != 2 || publisher.reportKinds[1] != "ai_focus_summary" || publisher.succeeded != 1 {
		t.Fatalf("expected dedicated and compatibility reports before success: %#v", publisher)
	}
}

func TestLegionRequiredFindingResultSatisfiesCompletionContract(t *testing.T) {
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "bind-required-finding", validCodeAuditResultContext())
	if err != nil {
		t.Fatal(err)
	}
	if err := rawSink.(aiFocusCodeWorkspaceEvidenceBinder).bindCodeWorkspaceEvidence(strings.Repeat("a", 40), strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	contract := testLegionCodeWorkspaceExecutionContract(t)
	for index := range contract.Results {
		contract.Results[index].Required = contract.Results[index].Capability == serverFocusCapabilitySubmitFindingV1
	}
	if err := rawSink.(aiFocusExecutionContractBinder).bindFocusExecutionContract(contract); err != nil {
		t.Fatal(err)
	}
	if _, err := rawSink.(aiFocusCodeResultSink).SubmitCodeFinding(context.Background(), "ai_code_finding", validCodeFinding()); err != nil {
		t.Fatal(err)
	}
	if err := rawSink.(*legionAIFocusResultSink).Succeed(context.Background(), nil); err != nil {
		t.Fatalf("published required finding did not satisfy completion contract: %v", err)
	}
}

func TestLegionCodeFindingDedupeStableAcrossWorkspaceAndRun(t *testing.T) {
	newSink := func(workspaceID, runID, jobID string) aiFocusCodeResultSink {
		t.Helper()
		resultContext := validCodeAuditResultContext()
		resultContext.FocusRunId = runID
		resultContext.TargetUrl = "https://workspace.invalid/" + workspaceID + "/"
		resultContext.Job.JobId = jobID
		rawSink, err := newLegionAIFocusResultSink(
			&recordingAIFocusRiskPublisher{},
			"bind-"+runID,
			resultContext,
		)
		if err != nil {
			t.Fatalf("new code audit sink: %v", err)
		}
		if err := rawSink.(aiFocusCodeWorkspaceEvidenceBinder).bindCodeWorkspaceEvidence(
			strings.Repeat("a", 40),
			strings.Repeat("b", 64),
		); err != nil {
			t.Fatalf("bind code workspace evidence: %v", err)
		}
		return rawSink.(aiFocusCodeResultSink)
	}

	first := validCodeFinding()
	first.DedupeKey = "model-key-first"
	firstSink := newSink(testLegionCodeWorkspaceID, "run-first", "job-first")
	firstReceipt, err := firstSink.SubmitCodeFinding(context.Background(), "ai_code_finding", first)
	if err != nil {
		t.Fatalf("submit first code finding: %v", err)
	}

	secondWorkspaceID := "aicw_fedcba9876543210fedcba9876543210"
	second := first
	second.WorkspaceID = secondWorkspaceID
	second.DedupeKey = "model-key-second"
	secondSink := newSink(secondWorkspaceID, "run-second", "job-second")
	secondReceipt, err := secondSink.SubmitCodeFinding(context.Background(), "ai_code_finding", second)
	if err != nil {
		t.Fatalf("submit cross-run code finding: %v", err)
	}
	if secondReceipt.DedupeKey != firstReceipt.DedupeKey {
		t.Fatalf("workspace/run changed stable dedupe key: first=%s second=%s", firstReceipt.DedupeKey, secondReceipt.DedupeKey)
	}

	moved := second
	moved.StartLine++
	moved.EndLine++
	movedReceipt, err := secondSink.SubmitCodeFinding(context.Background(), "ai_code_finding", moved)
	if err != nil {
		t.Fatalf("submit moved code finding: %v", err)
	}
	if movedReceipt.DedupeKey == firstReceipt.DedupeKey {
		t.Fatal("line range change did not change code finding dedupe key")
	}

	reclassified := second
	reclassified.VulnerabilityType = "Command injection"
	reclassifiedReceipt, err := secondSink.SubmitCodeFinding(context.Background(), "ai_code_finding", reclassified)
	if err != nil {
		t.Fatalf("submit reclassified code finding: %v", err)
	}
	if reclassifiedReceipt.DedupeKey == firstReceipt.DedupeKey {
		t.Fatal("vulnerability type change did not change code finding dedupe key")
	}
}

func TestLegionCodeAuditReportAcceptsCanonicalObjectStringOnly(t *testing.T) {
	publisher := &recordingAIFocusRiskPublisher{}
	rawSink, err := newLegionAIFocusResultSink(publisher, "bind-code-summary", validCodeAuditResultContext())
	if err != nil {
		t.Fatal(err)
	}
	sink := rawSink.(aiFocusCodeResultSink)
	invalid := map[string]json.RawMessage{
		"invalid json": []byte(`not-json`),
		"array":        []byte(`[1,2,3]`),
		"json string":  []byte(`"summary"`),
		"oversize":     []byte(strings.Repeat("x", maxInlineCodeAuditSummaryBytes+1)),
	}
	for name, summary := range invalid {
		t.Run(name, func(t *testing.T) {
			if _, err := sink.SubmitCodeAuditReport(context.Background(), "ai_code_audit_v1", aiFocusCodeAuditReport{
				WorkspaceID:       testLegionCodeWorkspaceID,
				Markdown:          "# Audit",
				StructuredSummary: summary,
			}); err == nil {
				t.Fatal("expected non-object structured summary rejection")
			}
		})
	}

	workspace := &legionCodeWorkspaceRuntime{
		spec: validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit),
		root: t.TempDir(),
	}
	runtimeValue, err := newLegionServerFocusRuntime(
		context.Background(), validCodeAuditResultContext().TargetUrl, rawSink, workspace,
	)
	if err != nil {
		t.Fatalf("new code audit runtime: %v", err)
	}
	activateTestLegionCodeWorkspaceFocusTurn(t, runtimeValue.(*legionServerFocusRuntime))
	if _, err := runtimeValue.Execute(serverFocusCapabilitySubmitCodeAudit, map[string]any{
		"markdown":                "# Audit",
		"structured_summary_json": `{ "safe_count": 3, "confirmed_count": 1 }`,
	}); err != nil {
		t.Fatalf("submit object-string summary: %v", err)
	}
	if len(publisher.reports) != 1 {
		t.Fatalf("expected one canonical report, got %#v", publisher.reports)
	}
	var report aiFocusCodeAuditReport
	if err := json.Unmarshal(publisher.reports[0], &report); err != nil {
		t.Fatalf("decode canonical report: %v", err)
	}
	if string(report.StructuredSummary) != `{"confirmed_count":1,"safe_count":3}` {
		t.Fatalf("structured summary was not canonicalized: %s", report.StructuredSummary)
	}
}

func TestValidateAISessionBindPairsWorkspaceWithSentinelTarget(t *testing.T) {
	if _, err := validateLegionAIFocusResultContext("bind-code", validCodeAuditResultContext()); err != nil {
		t.Fatalf("professional task result context rejected: %v", err)
	}

	ordinary := validAIFocusResultContext()
	ordinary.TargetUrl = "https://workspace.invalid/" + testLegionCodeWorkspaceID + "/"
	ordinaryCommand := validAISessionBindCommand()
	ordinaryCommand.ResultContext = ordinary
	ordinaryCommand.Session.RunId = ordinary.FocusRunId
	if err := validateAISessionBindCommand("node-ai", ordinaryCommand); err == nil || !strings.Contains(err.Error(), "requires source_workspace") {
		t.Fatalf("expected sentinel without workspace rejection, got %v", err)
	}

	code := validCodeAuditResultContext()
	code.TargetUrl = "https://example.com/source"
	workspace := validLegionCodeWorkspaceSpec(legionCodeWorkspaceKindGit)
	runtimeOptions, err := json.Marshal(yakRuntimeOptions{SourceWorkspace: &workspace})
	if err != nil {
		t.Fatal(err)
	}
	codeCommand := validAISessionBindCommand()
	codeCommand.ResultContext = code
	codeCommand.Session.RunId = code.FocusRunId
	codeCommand.RuntimeOptionSnapshotJson = runtimeOptions
	if err := validateAISessionBindCommand("node-ai", codeCommand); err == nil || !strings.Contains(err.Error(), "target_url must equal") {
		t.Fatalf("expected workspace with non-sentinel rejection, got %v", err)
	}

	codeCommand.ResultContext = validCodeAuditResultContext()
	codeCommand.Session.RunId = codeCommand.ResultContext.FocusRunId
	if err := validateAISessionBindCommand("node-ai", codeCommand); err != nil {
		t.Fatalf("matching workspace sentinel rejected: %v", err)
	}
}
