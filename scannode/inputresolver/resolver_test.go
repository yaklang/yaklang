//go:build linux

package inputresolver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"google.golang.org/protobuf/proto"
)

func manifestFixture(contents ...string) (*aiv1.InputManifest, Identity, []*aiv1.AISessionAttachmentRef) {
	m := &aiv1.InputManifest{SchemaVersion: SchemaV1, OwnerUserId: "owner-a", ProductKey: "synthetic_documents", RunId: "run-a", SessionId: "session-a", AttemptId: "attempt-a", WorkspaceId: "workspace-a", OutputPath: "outputs", AttemptCommandId: "attempt-command-a"}
	refs := make([]*aiv1.AISessionAttachmentRef, 0, len(contents))
	for i, content := range contents {
		sum := sha256.Sum256([]byte(content))
		digest := hex.EncodeToString(sum[:])
		id := fmt.Sprintf("input-%d", i)
		m.Resources = append(m.Resources, &aiv1.InputResource{ResourceId: id, Kind: ManagedAttachment, InputField: "documents", RelativePath: RelativePath("documents", i, "same.txt"), Filename: "same.txt", MediaType: "text/plain", SizeBytes: uint64(len(content)), Sha256: digest, Required: true, ReadOnly: true})
		refs = append(refs, &aiv1.AISessionAttachmentRef{AttachmentId: id, Filename: "same.txt", ContentType: "text/plain", SizeBytes: uint64(len(content)), Sha256: digest, DownloadUrl: "https://untrusted.invalid/never"})
	}
	_ = Seal(m)
	return m, Identity{OwnerUserID: m.OwnerUserId, SessionID: m.SessionId, AttemptID: m.AttemptId, RunID: m.RunId}, refs
}

func resolverFixture(t *testing.T, mutate func(*Config)) *Resolver {
	t.Helper()
	config := Config{Root: filepath.Join(t.TempDir(), "workspaces"), OutputBytes: 1 << 20, DiskHeadroomBytes: 1, StallTimeout: time.Second, TotalTimeout: 5 * time.Second, OrphanGrace: time.Nanosecond}
	if mutate != nil {
		mutate(&config)
	}
	r, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func downloadFixture(t *testing.T, contents ...string) DownloadOptions {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-session-secret" || r.URL.Query().Get("node_session_id") != "node-session-a" {
			t.Error("missing node auth")
		}
		for i, content := range contents {
			if r.URL.Path == fmt.Sprintf("/v1/ai/attachments/input-%d/download", i) {
				fmt.Fprint(w, content)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return DownloadOptions{APIBaseURL: server.URL, NodeSessionID: "node-session-a", BearerToken: "test-session-secret", HTTPClient: server.Client()}
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	var detail *Error
	if !errors.As(err, &detail) || detail.Code != code {
		t.Fatalf("error=%v want=%s", err, code)
	}
}

func TestManifestIdentityAndUnsupportedContracts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*aiv1.InputManifest, *Identity, []*aiv1.AISessionAttachmentRef)
		code   string
	}{
		{"version", func(m *aiv1.InputManifest, _ *Identity, _ []*aiv1.AISessionAttachmentRef) { m.SchemaVersion = "future" }, "input_manifest_invalid"},
		{"kind", func(m *aiv1.InputManifest, _ *Identity, _ []*aiv1.AISessionAttachmentRef) {
			m.Resources[0].Kind = "url"
		}, "input_manifest_invalid"},
		{"optional", func(m *aiv1.InputManifest, _ *Identity, _ []*aiv1.AISessionAttachmentRef) {
			m.Resources[0].Required = false
		}, "input_manifest_invalid"},
		{"writable", func(m *aiv1.InputManifest, _ *Identity, _ []*aiv1.AISessionAttachmentRef) {
			m.Resources[0].ReadOnly = false
		}, "input_manifest_invalid"},
		{"traversal", func(m *aiv1.InputManifest, _ *Identity, _ []*aiv1.AISessionAttachmentRef) {
			m.Resources[0].RelativePath = "inputs/../../outside"
		}, "input_manifest_invalid"},
		{"windows", func(m *aiv1.InputManifest, _ *Identity, _ []*aiv1.AISessionAttachmentRef) {
			m.Resources[0].RelativePath = `inputs/C:\outside`
		}, "input_manifest_invalid"},
		{"owner", func(_ *aiv1.InputManifest, id *Identity, _ []*aiv1.AISessionAttachmentRef) {
			id.OwnerUserID = "owner-b"
		}, "input_identity_mismatch"},
		{"session", func(_ *aiv1.InputManifest, id *Identity, _ []*aiv1.AISessionAttachmentRef) {
			id.SessionID = "session-b"
		}, "input_identity_mismatch"},
		{"attempt", func(_ *aiv1.InputManifest, id *Identity, _ []*aiv1.AISessionAttachmentRef) {
			id.AttemptID = "attempt-b"
		}, "input_identity_mismatch"},
		{"run", func(_ *aiv1.InputManifest, id *Identity, _ []*aiv1.AISessionAttachmentRef) { id.RunID = "run-b" }, "input_identity_mismatch"},
		{"ref_size", func(_ *aiv1.InputManifest, _ *Identity, r []*aiv1.AISessionAttachmentRef) { r[0].SizeBytes++ }, "input_authorization_mismatch"},
		{"ref_hash", func(_ *aiv1.InputManifest, _ *Identity, r []*aiv1.AISessionAttachmentRef) {
			r[0].Sha256 = strings.Repeat("0", 64)
		}, "input_authorization_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, id, refs := manifestFixture("hello")
			tc.mutate(m, &id, refs)
			_ = Seal(m)
			requireCode(t, ValidateBinding(m, id, refs), tc.code)
		})
	}
	m, id, refs := manifestFixture("a", "b")
	m.Resources[1].RelativePath = m.Resources[0].RelativePath
	_ = Seal(m)
	requireCode(t, ValidateBinding(m, id, refs), "input_manifest_invalid")
	m, id, refs = manifestFixture("a")
	m.ManifestId = strings.Repeat("0", 64)
	requireCode(t, ValidateBinding(m, id, refs), "input_manifest_invalid")
	_, err := Digest(nil)
	if err == nil {
		t.Fatal("nil digest accepted")
	}
}

func TestPrepareReadSearchAndOutputBoundaries(t *testing.T) {
	largeLine := strings.Repeat("x", (64<<10)-3) + "late-marker" + strings.Repeat("z", 512) + "\nlast line\n"
	m, id, refs := manifestFixture("first\n", largeLine)
	r := resolverFixture(t, nil)
	var events []Event
	var names []string
	w, err := r.Prepare(context.Background(), m, id, refs, downloadFixture(t, "first\n", largeLine), func(name string, event Event) { names = append(names, name); events = append(events, event) })
	if err != nil {
		t.Fatal(err)
	}
	defer w.Cleanup()
	listed, err := w.List(context.Background(), "")
	if err != nil || listed["count"] != 2 {
		t.Fatalf("list=%v err=%v", listed, err)
	}
	read, err := w.Read(context.Background(), m.Resources[0].RelativePath, 0, 2)
	if err != nil || read["content"] != "fi" || read["truncated"] != true {
		t.Fatalf("read=%v err=%v", read, err)
	}
	searched, err := w.Search(context.Background(), "inputs", "late-marker", true, 5)
	if err != nil || searched["count"] != 1 || searched["scanned_bytes"] != int64(len("first\n")+len(largeLine)) {
		t.Fatalf("search=%v err=%v", searched, err)
	}
	for _, invalid := range []string{"../outside", "/etc/passwd", "outputs/result.txt", "inputs/other/001-other.txt"} {
		_, err := w.Read(context.Background(), invalid, 0, 2)
		requireCode(t, err, "input_path_denied")
	}
	_, err = w.WriteOutput(context.Background(), m.Resources[0].RelativePath, "overwrite")
	requireCode(t, err, "input_path_denied")
	_, err = w.WriteOutput(context.Background(), "outputs/result.txt", "analysis")
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(w.root, m.Resources[0].RelativePath)); err != nil || info.Mode().Perm()&0222 != 0 {
		t.Fatalf("input mode=%v err=%v", info, err)
	}
	for i, event := range events {
		if event.RunID != m.RunId || event.AttemptID != m.AttemptId || event.ManifestID != m.ManifestId || event.SessionID != m.SessionId {
			t.Fatalf("event identity=%+v", event)
		}
		if names[i] == "input.workspace.ready" && len(event.Files) != 2 {
			t.Fatal("ready omitted immutable file set")
		}
	}
	root := w.root
	if err := w.Cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("workspace survived cleanup: %v", err)
	}
	_, err = w.Read(context.Background(), m.Resources[0].RelativePath, 0, 2)
	requireCode(t, err, "input_cancelled")
}

func TestSymlinkAndCrossWorkspaceDenial(t *testing.T) {
	m, id, refs := manifestFixture("confidential")
	r := resolverFixture(t, nil)
	options := downloadFixture(t, "confidential")
	one, err := r.Prepare(context.Background(), m, id, refs, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Cleanup()
	m2 := proto.Clone(m).(*aiv1.InputManifest)
	m2.AttemptId = "attempt-b"
	m2.WorkspaceId = "workspace-b"
	_ = Seal(m2)
	id.AttemptID = m2.AttemptId
	two, err := r.Prepare(context.Background(), m2, id, refs, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer two.Cleanup()
	if one.root == two.root {
		t.Fatal("attempt roots collided")
	}
	_, err = one.Read(context.Background(), filepath.Join(two.root, m2.Resources[0].RelativePath), 0, 10)
	requireCode(t, err, "input_path_denied")
	name := m.Resources[0].RelativePath
	parent := filepath.Dir(filepath.Join(one.root, name))
	_ = os.Chmod(parent, 0700)
	_ = os.Remove(filepath.Join(one.root, name))
	if err := os.Symlink(filepath.Join(two.root, name), filepath.Join(one.root, name)); err != nil {
		t.Fatal(err)
	}
	_, err = one.Read(context.Background(), name, 0, 10)
	requireCode(t, err, "input_path_denied")
	outside := t.TempDir()
	if err := os.Remove(filepath.Join(one.root, "outputs")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(one.root, "outputs")); err != nil {
		t.Fatal(err)
	}
	_, err = one.WriteOutput(context.Background(), "outputs/escape.txt", "bad")
	requireCode(t, err, "input_output_write_failed")
	if _, err := os.Stat(filepath.Join(outside, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("output followed symlink")
	}
	if err := one.Cleanup(); err != nil {
		t.Fatal(err)
	}
	read, err := two.Read(context.Background(), name, 0, 100)
	if err != nil || read["content"] != "confidential" {
		t.Fatalf("old cleanup affected current attempt: %v %v", read, err)
	}
}

func TestDownloadFailureClassificationAndCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		declared   int
		code       string
		stall      bool
	}{
		{"missing", "", 404, -1, "input_missing", false}, {"unauthorized", "", 403, -1, "input_unauthorized", false},
		{"size", "bad", 200, -1, "input_size_mismatch", false}, {"hash", "other", 200, -1, "input_hash_mismatch", false},
		{"truncated", "he", 200, 5, "input_download_truncated", false}, {"stalled", "", 200, 5, "input_download_stalled", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, id, refs := manifestFixture("hello")
			r := resolverFixture(t, func(c *Config) {
				c.StallTimeout = 2 * time.Second
				if tc.stall {
					c.StallTimeout = 100 * time.Millisecond
				}
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				if tc.declared >= 0 {
					w.Header().Set("Content-Length", fmt.Sprint(tc.declared))
				}
				w.WriteHeader(tc.status)
				if tc.stall {
					w.(http.Flusher).Flush()
					<-q.Context().Done()
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			_, err := r.Prepare(context.Background(), m, id, refs, DownloadOptions{APIBaseURL: server.URL, NodeSessionID: "node", BearerToken: "secret"}, nil)
			requireCode(t, err, tc.code)
			entries, _ := os.ReadDir(r.config.Root)
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "workspace-") {
					t.Fatal("failed preparation leaked workspace")
				}
			}
		})
	}
}

func TestCancelDuringDownloadAndDiskReservation(t *testing.T) {
	m, id, refs := manifestFixture("hello")
	r := resolverFixture(t, nil)
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		w.Header().Set("Content-Length", "5")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-q.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := r.Prepare(ctx, m, id, refs, DownloadOptions{APIBaseURL: server.URL, NodeSessionID: "node", BearerToken: "secret"}, nil)
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		requireCode(t, err, "input_cancelled")
	case <-time.After(time.Second):
		t.Fatal("cancel did not interrupt download")
	}
	lowDisk := resolverFixture(t, func(c *Config) { c.AvailableBytes = func(string) (uint64, error) { return 1, nil } })
	_, err := lowDisk.Prepare(context.Background(), m, id, refs, downloadFixture(t, "hello"), nil)
	requireCode(t, err, "input_disk_insufficient")
	quota := resolverFixture(t, func(c *Config) { c.MaxReservedBytes = (1 << 20) + 9 })
	w, err := quota.Prepare(context.Background(), m, id, refs, downloadFixture(t, "hello"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Cleanup()
	_, err = quota.Prepare(context.Background(), m, id, refs, downloadFixture(t, "hello"), nil)
	requireCode(t, err, "input_quota_exceeded")
}

func TestOrphanLeaseReclaimedOnlyAfterLockRelease(t *testing.T) {
	m, id, refs := manifestFixture("hello")
	r := resolverFixture(t, nil)
	w, err := r.Prepare(context.Background(), m, id, refs, downloadFixture(t, "hello"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := r.Reclaim(context.Background()); err != nil || count != 0 {
		t.Fatalf("reclaimed active lease count=%d err=%v", count, err)
	}
	// Simulate process exit: the kernel releases this lock without Cleanup.
	if err := w.leaseFile.Close(); err != nil {
		t.Fatal(err)
	}
	w.leaseFile = nil
	w.cancel()
	restarted, err := New(r.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.root); !os.IsNotExist(err) {
		t.Fatalf("startup failed to reclaim orphan: %v", err)
	}
	unknown := filepath.Join(restarted.config.Root, "unowned")
	if err := os.Mkdir(unknown, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Reclaim(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unknown); err != nil {
		t.Fatal("reclaimed unowned data")
	}
}

func TestYoungCrashLeaseMaturesAndIsReclaimedBeforeAllocation(t *testing.T) {
	m, id, refs := manifestFixture("hello")
	r := resolverFixture(t, func(c *Config) { c.OrphanGrace = time.Hour })
	options := downloadFixture(t, "hello")
	w, err := r.Prepare(context.Background(), m, id, refs, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.leaseFile.Close(); err != nil {
		t.Fatal(err)
	}
	w.leaseFile = nil
	w.cancel()
	restarted, err := New(r.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.root); err != nil {
		t.Fatal("young lease removed during restart")
	}
	metadata, err := readLease(r.config.Root, filepath.Base(w.root))
	if err != nil {
		t.Fatal(err)
	}
	metadata.CreatedAt = time.Now().Add(-2 * time.Hour)
	raw, _ := json.Marshal(metadata)
	if err := os.WriteFile(filepath.Join(w.root, ".lease.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	next, err := restarted.Prepare(context.Background(), m, id, refs, options, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Cleanup()
	if _, err := os.Stat(w.root); !os.IsNotExist(err) {
		t.Fatalf("mature orphan survived a later allocation: %v", err)
	}
}

func TestIncompleteStagingCannotPoisonActiveReservations(t *testing.T) {
	m, id, refs := manifestFixture("hello")
	r := resolverFixture(t, nil)
	incomplete := filepath.Join(r.config.Root, ".staging", "workspace-incomplete")
	if err := os.Mkdir(incomplete, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(incomplete, ".lease.json"), []byte(`{"version":`), 0600); err != nil {
		t.Fatal(err)
	}
	completeName := "workspace-complete"
	complete := filepath.Join(r.config.Root, ".staging", completeName)
	if err := os.Mkdir(complete, 0700); err != nil {
		t.Fatal(err)
	}
	metadata := lease{Version: leaseVersion, Directory: completeName, CreatedAt: time.Now().Add(-time.Hour), ReservedBytes: 100, Manifest: m}
	raw, _ := json.Marshal(metadata)
	for name, content := range map[string][]byte{".lease.json": raw, ".lease.lock": nil} {
		if err := os.WriteFile(filepath.Join(complete, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	restarted, err := New(r.config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(complete); !os.IsNotExist(err) {
		t.Fatal("complete abandoned staging lease was not reclaimed")
	}
	w, err := restarted.Prepare(context.Background(), m, id, refs, downloadFixture(t, "hello"), nil)
	if err != nil {
		t.Fatalf("partial metadata poisoned allocation: %v", err)
	}
	defer w.Cleanup()
	if strings.Contains(w.root, ".staging") {
		t.Fatal("published workspace remained in staging")
	}
	if _, err := os.Stat(incomplete); err != nil {
		t.Fatal("unknown partial staging metadata was destructively guessed")
	}
}

func TestDownloadConcurrencyAndRedirectFence(t *testing.T) {
	m, id, refs := manifestFixture("hello")
	r := resolverFixture(t, func(c *Config) { c.DownloadConcurrency = 1 })
	var active, max atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		n := active.Add(1)
		if n > max.Load() {
			max.Store(n)
		}
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, "hello")
		active.Add(-1)
	}))
	defer server.Close()
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w, err := r.Prepare(context.Background(), m, id, refs, DownloadOptions{APIBaseURL: server.URL, NodeSessionID: "node", BearerToken: "secret"}, nil)
			if err != nil {
				t.Error(err)
				return
			}
			_ = w.Cleanup()
		}()
	}
	wg.Wait()
	if max.Load() != 1 {
		t.Fatalf("download concurrency=%d", max.Load())
	}
	var redirected atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) { redirected.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) { http.Redirect(w, q, target.URL, 302) }))
	defer redirect.Close()
	_, err := r.Prepare(context.Background(), m, id, refs, DownloadOptions{APIBaseURL: redirect.URL, NodeSessionID: "node", BearerToken: "secret"}, nil)
	requireCode(t, err, "input_download_failed")
	if redirected.Load() {
		t.Fatal("credentialed request followed redirect")
	}
}
