package scannode

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func TestAITrafficAtomicWriteRejectsPathsOutsideSpool(t *testing.T) {
	outside := t.TempDir()
	root := filepath.Join(outside, "allowed", "spool")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", ".", "..", "../escape", "../../escape", filepath.Join(outside, "absolute"), "sub/file", `sub\file`, "sub/../file"} {
		t.Run(name, func(t *testing.T) {
			if err := trafficAtomicWrite(root, name, []byte("evidence")); err == nil {
				t.Fatalf("unsafe spool name accepted: %q", name)
			}
		})
	}
	for _, directory := range []string{"", ".", "relative"} {
		if err := trafficAtomicWrite(directory, "flow.terminal.pb", nil); err == nil {
			t.Fatalf("non-absolute spool accepted: %q", directory)
		}
	}
	for _, path := range []string{filepath.Join(outside, "escape"), filepath.Join(outside, "absolute"), filepath.Join(outside, "allowed", "escape")} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("rejected write escaped spool: %q %v", path, err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("rejected write left spool files: %v %v", entries, err)
	}
}

func TestAITrafficAtomicWritePreservesPrivateEvidenceAndCleansFailedWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "flow.terminal.pb")
	for _, raw := range [][]byte{[]byte("old evidence"), []byte("replacement evidence")} {
		if err := trafficAtomicWrite(root, filepath.Base(path), raw); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, raw) {
			t.Fatalf("atomic evidence write failed: %q %v", got, err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("evidence permissions: %v", info.Mode())
	}
	if err := os.Mkdir(filepath.Join(root, "blocked"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := trafficAtomicWrite(root, "blocked", []byte("cannot replace a directory")); err == nil {
		t.Fatal("write over directory succeeded")
	}
	temps, err := filepath.Glob(filepath.Join(root, ".traffic-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("failed write left temporary evidence: %v %v", temps, err)
	}
}

func TestAITrafficCollectorHashesUntrustedIdentityWithinLocalRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("YAKIT_HOME", root)
	for _, identity := range []string{"../../outside", "/absolute", `..\..\outside`, "normal-session"} {
		t.Run(identity, func(t *testing.T) {
			binding := aiSessionBinding{
				Ref:                 aiSessionCommandRef{SessionID: identity, BindEpoch: 7},
				NodeSessionID:       identity,
				PlatformAPIBaseURL:  "https://platform.invalid",
				PlatformBearerToken: "test-token",
				TrafficCapture:      &aiv1.AITrafficCapturePolicy{ProtocolVersion: 1, UploadBaseUrl: "https://platform.invalid/traffic"},
			}
			collector, err := newAITrafficCollector(binding, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer collector.stopped.Do(func() { close(collector.stop) })
			defer aiTrafficCollectors.Delete(collector.key())
			relative, err := filepath.Rel(root, collector.dir)
			parts := strings.Split(relative, string(filepath.Separator))
			if err != nil || len(parts) != 2 || parts[0] != "legion-ai-traffic" {
				t.Fatalf("remote identity escaped local root: %q %v", relative, err)
			}
			hash, err := hex.DecodeString(parts[1])
			if err != nil || len(hash) != 32 {
				t.Fatalf("remote identity selected an unhashed directory: %q", parts[1])
			}
		})
	}
}
