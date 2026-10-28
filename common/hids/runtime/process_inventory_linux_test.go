//go:build hids && linux

package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	gopscommon "github.com/shirou/gopsutil/v4/common"
	gopsprocess "github.com/shirou/gopsutil/v4/process"

	"github.com/yaklang/yaklang/common/hids/model"
)

func fixtureProcessInventory(tb testing.TB, count int) (context.Context, []*gopsprocess.Process) {
	tb.Helper()
	root := tb.TempDir()
	write := func(path, content string) {
		tb.Helper()
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			tb.Fatal(err)
		}
	}
	write("stat", "btime 1700000000\n")
	write("uptime", "100000 0\n")
	write("meminfo", "MemTotal: 1048576 kB\nMemFree: 524288 kB\nMemAvailable: 524288 kB\n")
	processes := make([]*gopsprocess.Process, count)
	for index := 0; index < count; index++ {
		pid := 200000 + index
		processes[index] = &gopsprocess.Process{Pid: int32(pid)}
		ppid := 0
		if index > 0 {
			ppid = 200000 + (index-1)/2
		}
		dir := strconv.Itoa(pid)
		if err := os.MkdirAll(filepath.Join(root, dir, "fd"), 0700); err != nil {
			tb.Fatal(err)
		}
		name := fmt.Sprintf("process-%d", index)
		fields := make([]string, 50)
		for index := range fields {
			fields[index] = "0"
		}
		fields[0] = "S" // Linux proc stat field 3; subsequent fields start here.
		fields[1] = strconv.Itoa(ppid)
		fields[19] = strconv.Itoa(1000 + index) // starttime, field 22
		write(filepath.Join(dir, "stat"), fmt.Sprintf("%d (%s) %s\n", pid, name, strings.Join(fields, " ")))
		write(filepath.Join(dir, "comm"), name+"\n")
		write(filepath.Join(dir, "status"), fmt.Sprintf("Name:\t%s\nState:\tS (sleeping)\nPPid:\t%d\nUid:\t0\t0\t0\t0\nGid:\t0\t0\t0\t0\nThreads:\t1\nVmRSS:\t4 kB\nVmSize:\t16 kB\n", name, ppid))
		write(filepath.Join(dir, "cmdline"), "/usr/bin/"+name+"\x00--inventory\x00")
		if err := os.Symlink("/usr/bin/"+name, filepath.Join(root, dir, "exe")); err != nil {
			tb.Fatal(err)
		}
	}
	return context.WithValue(context.Background(), gopscommon.EnvKey, gopscommon.EnvMap{gopscommon.HostProcEnvKey: root}), processes
}

func TestProcessInventoryEventsRetainsTreeAndIdentity(t *testing.T) {
	t.Parallel()
	ctx, processes := fixtureProcessInventory(t, 7)
	events, err := collectProcessInventoryEvents(ctx, processes, "fixture-boot")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 7 {
		t.Fatalf("expected every fixture process, got %d", len(events))
	}
	byPID := make(map[int]model.Event, len(events))
	for _, event := range events {
		if event.Type != model.EventTypeProcessState || event.Process == nil || event.Process.StartTimeUnixMillis <= 0 || event.Process.BootID == "" {
			t.Fatalf("missing canonical identity: %#v", event)
		}
		byPID[event.Process.PID] = event
	}
	if got := byPID[200000].Process.ChildrenPIDs; !reflect.DeepEqual(got, []int{200001, 200002}) {
		t.Fatalf("root child relationships changed: %v", got)
	}
	if got := byPID[200001].Process.ChildrenPIDs; !reflect.DeepEqual(got, []int{200003, 200004}) {
		t.Fatalf("nested child relationships changed: %v", got)
	}
	parent, child := byPID[200001].Process, byPID[200004].Process
	if child.ParentName != parent.Name || child.ParentImage != parent.Image || child.ParentCommand != parent.Command || child.ParentStartTimeUnixMillis != parent.StartTimeUnixMillis {
		t.Fatalf("parent fields do not match collected parent: child=%#v parent=%#v", child, parent)
	}
}

func TestInventoryTreeKeepsChildrenWithoutReadableImage(t *testing.T) {
	t.Parallel()
	ctx, processes := fixtureProcessInventory(t, 3)
	root := ctx.Value(gopscommon.EnvKey).(gopscommon.EnvMap)[gopscommon.HostProcEnvKey]
	for _, name := range []string{"comm", "status", "exe", "cmdline"} {
		if err := os.Remove(filepath.Join(root, "200001", name)); err != nil {
			t.Fatal(err)
		}
	}
	events, err := collectProcessInventoryEvents(ctx, processes, "fixture-boot")
	if err != nil || len(events) != 2 {
		t.Fatalf("expected only readable process events, count=%d err=%v", len(events), err)
	}
	if got := events[0].Process.ChildrenPIDs; !reflect.DeepEqual(got, []int{200001, 200002}) {
		t.Fatalf("lost child without readable image: %v", got)
	}
}

func BenchmarkProcessInventoryEvents(b *testing.B) {
	for _, count := range []int{310, 714} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			ctx, fixture := fixtureProcessInventory(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				// Match a fresh PID enumeration, without reusing per-Process caches.
				processes := make([]*gopsprocess.Process, len(fixture))
				for index, process := range fixture {
					processes[index] = &gopsprocess.Process{Pid: process.Pid}
				}
				events, err := collectProcessInventoryEvents(ctx, processes, "fixture-boot")
				if err != nil || len(events) != count {
					b.Fatalf("incomplete inventory count=%d err=%v", len(events), err)
				}
			}
		})
	}
}

// Opt in explicitly when measuring the actual host, including on a customer
// comparison VM. It starts no collectors and emits no process contents.
func BenchmarkSystemInventoryProcessEvents(b *testing.B) {
	if os.Getenv("YAK_HIDS_BENCH_LIVE_PROC") != "1" {
		b.Skip("set YAK_HIDS_BENCH_LIVE_PROC=1 to read the actual host inventory")
	}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		events, err := (systemInventoryProvider{}).ListProcessEvents(ctx)
		cancel()
		if err != nil {
			b.Fatal(err)
		}
		missingIdentity := 0
		for _, event := range events {
			if event.Process == nil || event.Process.PID <= 0 || event.Process.BootID == "" || event.Process.StartTimeUnixMillis <= 0 {
				missingIdentity++
			}
		}
		b.ReportMetric(float64(len(events)), "processes/op")
		b.ReportMetric(float64(missingIdentity), "missing_identity/op")
	}
}
