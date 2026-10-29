//go:build hids && linux

package ebpf

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	gopscommon "github.com/shirou/gopsutil/v4/common"
	gopsprocess "github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/yaklang/yaklang/common/hids/model"
)

func writeMetadataFixture(t *testing.T, root string, pid, parent, uid int, name, command, image string, state byte) {
	t.Helper()
	directory := filepath.Join(root, strconv.Itoa(pid))
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	for file, content := range map[string]string{
		"comm":    name + "\n",
		"status":  fmt.Sprintf("Name:\t%s\nState:\t%c (fixture)\nPPid:\t%d\nUid:\t%d\t%d\t%d\t%d\n", name, state, parent, uid, uid, uid, uid),
		"cmdline": command,
	} {
		if err := os.WriteFile(filepath.Join(directory, file), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.Remove(filepath.Join(directory, "exe"))
	if err := os.Symlink(image, filepath.Join(directory, "exe")); err != nil {
		t.Fatal(err)
	}
}

func metadataContext(root string) context.Context {
	return context.WithValue(context.Background(), gopscommon.EnvKey, gopscommon.EnvMap{gopscommon.HostProcEnvKey: root})
}

func TestProcessEventMetadataNeedsNoBootOrCreationTimeAndRemainsFresh(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := metadataContext(root)
	uid := os.Getuid()
	account, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		t.Fatal(err)
	}
	writeMetadataFixture(t, root, 424242, 424241, uid, "long-executable", "/opt/long-executable-name\x00arg\x00\x00tail\x00", "/opt/old-image", 'S')
	writeMetadataFixture(t, root, 424241, 0, uid, "parent-longname", "/opt/parent-longname-extended\x00", "/opt/parent", 'R')
	// No host stat, PID stat or creation-time field exists in this selected proc
	// view. All event identity fields must still be available.
	first, data := enrichProcessEventWithContext(ctx, 424242, processEventOptions{preferProcState: true})
	want := model.Process{PID: 424242, ParentPID: 424241, ParentName: "parent-longname-extended", Name: "long-executable-name", Username: account.Username, Image: "/opt/old-image", Command: "/opt/long-executable-name arg tail"}
	if !reflect.DeepEqual(first, want) || data["username"] != account.Username || data["process_name"] != want.Name || data["process_status"] != gopsprocess.Sleep {
		t.Fatalf("metadata required unrelated stat/boot fields or lost values: got=%+v data=%v want=%+v", first, data, want)
	}

	// Reuse the PID with different identity fields. Each call must observe the
	// new values instead of retaining a PID/name/parent/UID snapshot.
	writeMetadataFixture(t, root, 424242, 0, 2147483647, "replacement", "/opt/replacement\x00--changed\x00", "/opt/new-image", 'T')
	second, secondData := enrichProcessEventWithContext(ctx, 424242, processEventOptions{preferProcState: true})
	if second.Name != "replacement" || second.ParentPID != 0 || second.ParentName != "" || second.Image != "/opt/new-image" || second.Command != "/opt/replacement --changed" || second.Username != "" || secondData["process_status"] != gopsprocess.Stop {
		t.Fatalf("PID reuse/exec kept old metadata: got=%+v data=%v", second, secondData)
	}
	if _, exists := secondData["username"]; exists {
		t.Fatal("old username retained after UID changed to an unknown account")
	}
}

func TestProcessEventMetadataStatusMatchesPublicGetter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	ctx := metadataContext(root)
	for _, state := range []byte("ADUEILORSTtWYZ?Xx") {
		writeMetadataFixture(t, root, 424242, 0, os.Getuid(), "fixture", "/opt/fixture\x00", "/opt/fixture", state)
		statuses, err := (&gopsprocess.Process{Pid: 424242}).StatusWithContext(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, data := enrichProcessEventWithContext(ctx, 424242, processEventOptions{})
		if data["process_status"] != strings.Join(statuses, ",") {
			t.Fatalf("state %c changed public status: got=%v want=%v", state, data["process_status"], statuses)
		}
	}
}

func TestProcessEventMetadataKeepsStatParentWhenStatusIsUnavailable(t *testing.T) {
	t.Parallel()
	for name, stat := range map[string]string{
		"plain":       "424242 (worker) S 424241",
		"parentheses": "424242 (worker (name) with ) spaces) S 424241",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeMetadataFixture(t, root, 424242, 0, os.Getuid(), "worker", "/opt/worker\x00arg\x00", "/opt/worker", 'S')
			writeMetadataFixture(t, root, 424241, 0, os.Getuid(), "parent", "/opt/parent\x00", "/opt/parent", 'S')
			processRoot := filepath.Join(root, "424242")
			// A directory ensures the status read itself fails even under root.
			if err := os.Remove(filepath.Join(processRoot, "status")); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(processRoot, "status"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(processRoot, "stat"), []byte(stat), 0600); err != nil {
				t.Fatal(err)
			}
			got, data := enrichProcessEventWithContext(metadataContext(root), 424242, processEventOptions{preferProcState: true})
			if got.ParentPID != 424241 || got.ParentName != "parent" || got.Name != "worker" || got.Image != "/opt/worker" || got.Command != "/opt/worker arg" || data["process_status"] != "" {
				t.Fatalf("partial status read lost readable parent/identity fields: %+v %v", got, data)
			}
		})
	}
}

func TestProcessEventMetadataRejectsUnavailableOrInvalidStatParent(t *testing.T) {
	t.Parallel()
	for name, stat := range map[string]string{
		"missing": "", "no-comm": "424242 S 424241", "no-state": "424242 (worker)",
		"no-parent": "424242 (worker) S", "invalid-parent": "424242 (worker) S bad",
		"negative-parent": "424242 (worker) S -1", "overflow-parent": "424242 (worker) S 2147483648",
		"zero-parent": "424242 (worker) S 0",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeMetadataFixture(t, root, 424242, 0, os.Getuid(), "worker", "/opt/worker\x00", "/opt/worker", 'S')
			processRoot := filepath.Join(root, "424242")
			if err := os.Remove(filepath.Join(processRoot, "status")); err != nil {
				t.Fatal(err)
			}
			if stat != "" {
				if err := os.WriteFile(filepath.Join(processRoot, "stat"), []byte(stat), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, data := enrichProcessEventWithContext(metadataContext(root), 424242, processEventOptions{preferProcState: true})
			if got.ParentPID != 0 || got.ParentName != "" || got.Name != "worker" || data["process_name"] != "worker" {
				t.Fatalf("invalid stat parent leaked into event or discarded readable identity: %+v %v", got, data)
			}
		})
	}
}

func TestProcessEventMetadataMatchesPublicGettersForStoppedProcess(t *testing.T) {
	t.Parallel()
	child := exec.Command("/bin/sleep", "30")
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
	if err := unix.Kill(child.Process.Pid, unix.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	ctx := metadataContext("/proc")
	process, err := gopsprocess.NewProcessWithContext(ctx, int32(child.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var statuses []string
	for time.Now().Before(deadline) {
		statuses, err = process.StatusWithContext(ctx)
		if err == nil && reflect.DeepEqual(statuses, []string{gopsprocess.Stop}) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !reflect.DeepEqual(statuses, []string{gopsprocess.Stop}) {
		t.Fatalf("controlled child did not stop: %v %v", statuses, err)
	}
	parentPID, err := process.PpidWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := gopsprocess.NewProcessWithContext(ctx, parentPID)
	if err != nil {
		t.Fatal(err)
	}
	parentName, _ := parent.NameWithContext(ctx)
	name, _ := process.NameWithContext(ctx)
	username, _ := process.UsernameWithContext(ctx)
	image, _ := process.ExeWithContext(ctx)
	command, _ := process.CmdlineWithContext(ctx)
	want := model.Process{PID: child.Process.Pid, ParentPID: int(parentPID), ParentName: strings.TrimSpace(parentName), Name: strings.TrimSpace(name), Username: strings.TrimSpace(username), Image: strings.TrimSpace(image), Command: strings.TrimSpace(command)}
	got, data := enrichProcessEventWithContext(ctx, child.Process.Pid, processEventOptions{preferProcState: true})
	if !reflect.DeepEqual(got, want) || data["process_status"] != strings.Join(statuses, ",") || data["process_name"] != want.Name || data["username"] != want.Username {
		t.Fatalf("public getter field parity failed: got=%+v data=%v want=%+v statuses=%v", got, data, want, statuses)
	}
	options := processEventOptions{fallbackName: "comm", fallbackImage: "/captured/image", fallbackCommand: "captured command"}
	got, _ = enrichProcessEventWithContext(ctx, child.Process.Pid, options)
	if got.Image != options.fallbackImage || got.Command != options.fallbackCommand || got.Name != want.Name {
		t.Fatalf("captured command/image precedence changed: %+v", got)
	}
}

func TestProcessEventMetadataUsesSelectedProcViewAndKeepsFallback(t *testing.T) {
	root := t.TempDir()
	writeMetadataFixture(t, root, 424242, 0, os.Getuid(), "selected", "/selected\x00", "/selected", 'S')
	t.Setenv("HOST_PROC", root)
	fromEnv, _ := enrichProcessEventWithContext(context.Background(), 424242, processEventOptions{preferProcState: true})
	if fromEnv.Name != "selected" {
		t.Fatal("HOST_PROC was ignored")
	}
	ctx := metadataContext(t.TempDir())
	options := processEventOptions{fallbackName: "captured", fallbackImage: "/captured/image", fallbackCommand: "captured command", preferProcState: true}
	missing, data := enrichProcessEventWithContext(ctx, 424242, options)
	if missing.PID != 424242 || missing.Name != options.fallbackName || missing.Image != options.fallbackImage || missing.Command != options.fallbackCommand || data != nil {
		t.Fatalf("missing selected PID discarded raw fallback or used another proc view: %+v %v", missing, data)
	}
	canceled, cancel := context.WithCancel(metadataContext(root))
	cancel()
	got, data := enrichProcessEventWithContext(canceled, 424242, options)
	if !reflect.DeepEqual(got, missing) || data != nil {
		t.Fatalf("canceled enrichment changed captured identity: %+v %v", got, data)
	}
}
