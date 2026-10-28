//go:build hids && linux

package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	gopscommon "github.com/shirou/gopsutil/v4/common"
	gopsnet "github.com/shirou/gopsutil/v4/net"
)

func fixtureNetworkInventory(tb testing.TB, processCount, sockets, networkPIDs int) context.Context {
	tb.Helper()
	ctx, _ := fixtureProcessInventory(tb, processCount)
	root := ctx.Value(gopscommon.EnvKey).(gopscommon.EnvMap)[gopscommon.HostProcEnvKey]
	if err := os.Mkdir(filepath.Join(root, "net"), 0700); err != nil {
		tb.Fatal(err)
	}
	var tcp strings.Builder
	tcp.WriteString("sl local_address rem_address st tx_queue tr retrnsmt uid timeout inode\n")
	for index := 0; index < sockets; index++ {
		inode := 1000000 + index
		fmt.Fprintf(&tcp, "%d: 0100007F:%04X 00000000:0000 0A 00000000:00000000 00:00000000 00000000 0 0 %d\n", index, 10000+index, inode)
		pid := 200000 + index%networkPIDs
		if err := os.Symlink(fmt.Sprintf("socket:[%d]", inode), filepath.Join(root, strconv.Itoa(pid), "fd", strconv.Itoa(3+index))); err != nil {
			tb.Fatal(err)
		}
	}
	for name, content := range map[string]string{"tcp": tcp.String(), "tcp6": "header\n", "udp": "header\n", "udp6": "header\n", "unix": "header\n"} {
		if err := os.WriteFile(filepath.Join(root, "net", name), []byte(content), 0600); err != nil {
			tb.Fatal(err)
		}
	}
	return ctx
}

func TestNetworkInventoryPreservesSocketProcessAndParentIdentity(t *testing.T) {
	t.Parallel()
	ctx := fixtureNetworkInventory(t, 7, 14, 7)
	events, err := (systemInventoryProvider{}).ListNetworkEvents(ctx)
	if err != nil || len(events) != 14 {
		t.Fatalf("expected every socket, count=%d err=%v", len(events), err)
	}
	for _, event := range events {
		p, n := event.Process, event.Network
		if p == nil || p.PID < 200000 || p.BootID == "" || p.StartTimeUnixMillis <= 0 || p.UID != "0" || p.GID != "0" || p.Username == "" {
			t.Fatalf("process identity was lost: %#v", p)
		}
		if n == nil || n.Protocol != "tcp" || n.SourceAddress != "127.0.0.1" || n.SourcePort < 10000 || n.FD < 3 || n.ConnectionState != "LISTEN" || n.Direction != "local" {
			t.Fatalf("socket fields changed: %#v", n)
		}
		if p.PID > 200000 {
			parent := (p.PID - 200001) / 2
			if p.ParentPID != 200000+parent || p.ParentName != fmt.Sprintf("process-%d", parent) || p.ParentImage != fmt.Sprintf("/usr/bin/process-%d", parent) || p.ParentCommand != fmt.Sprintf("/usr/bin/process-%d --inventory", parent) || p.ParentStartTimeUnixMillis <= 0 {
				t.Fatalf("parent identity was lost: %#v", p)
			}
		}
	}
}

func TestNetworkInventoryCachesIdentityWithoutLosingParentsOwnParent(t *testing.T) {
	t.Parallel()
	ctx, _ := fixtureProcessInventory(t, 7)
	cache := make(map[int32]*processInventorySnapshot)
	connection := gopsnet.ConnectionStat{Pid: 200003, Type: 1, Family: 2, Laddr: gopsnet.Addr{IP: "127.0.0.1", Port: 8080}}
	child, ok := buildNetworkInventoryEvent(ctx, time.Now(), "fixture-boot", connection, cache)
	if !ok || child.Process.ParentName != "process-1" {
		t.Fatalf("missing child parent: %#v", child.Process)
	}
	// PID 200001 was already cached as the first connection's parent. A later
	// connection owned by it must still receive its own parent display fields.
	connection.Pid = 200001
	parent, ok := buildNetworkInventoryEvent(ctx, time.Now(), "fixture-boot", connection, cache)
	if !ok || parent.Process.ParentName != "process-0" || parent.Process.ParentStartTimeUnixMillis <= 0 {
		t.Fatalf("cached parent lost its own parent: %#v", parent.Process)
	}
	root := ctx.Value(gopscommon.EnvKey).(gopscommon.EnvMap)[gopscommon.HostProcEnvKey]
	for _, pid := range []string{"200000", "200001", "200003"} {
		if err := os.RemoveAll(filepath.Join(root, pid)); err != nil {
			t.Fatal(err)
		}
	}
	connection.Pid = 200003
	again, ok := buildNetworkInventoryEvent(ctx, time.Now(), "fixture-boot", connection, cache)
	if !ok || again.Process.StartTimeUnixMillis != child.Process.StartTimeUnixMillis || again.Process.ParentName != child.Process.ParentName {
		t.Fatalf("repeated socket re-read a disappearing PID: %#v", again.Process)
	}
}

func TestNetworkInventoryRejectsCanceledCollection(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if events, err := (systemInventoryProvider{}).ListNetworkEvents(ctx); !errors.Is(err, context.Canceled) || len(events) != 0 {
		t.Fatalf("canceled inventory was reported successful: count=%d err=%v", len(events), err)
	}
}

func BenchmarkNetworkInventoryProcessIdentity(b *testing.B) {
	for _, count := range []int{310, 714} {
		b.Run(fmt.Sprintf("P%d_U50", count), func(b *testing.B) {
			ctx, processes := fixtureProcessInventory(b, count)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				cache := make(map[int32]*processInventorySnapshot)
				for _, process := range processes[:50] {
					if snapshot := lookupProcessInventorySnapshot(ctx, process.Pid, "fixture-boot", cache); snapshot == nil || snapshot.startTimeUnixMillis <= 0 {
						b.Fatal("missing network identity")
					}
				}
			}
		})
	}
}

func BenchmarkNetworkInventorySocketUIDs(b *testing.B) {
	ctx := fixtureNetworkInventory(b, 1, 1000, 1)
	for _, withUIDs := range []bool{true, false} {
		b.Run(fmt.Sprintf("socket_uids_%t", withUIDs), func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				var connections []gopsnet.ConnectionStat
				var err error
				if withUIDs {
					connections, err = gopsnet.ConnectionsWithContext(ctx, "all")
				} else {
					connections, err = gopsnet.ConnectionsWithoutUidsWithContext(ctx, "all")
				}
				if err != nil || len(connections) != 1000 {
					b.Fatalf("incomplete connection inventory count=%d err=%v", len(connections), err)
				}
			}
		})
	}
}
