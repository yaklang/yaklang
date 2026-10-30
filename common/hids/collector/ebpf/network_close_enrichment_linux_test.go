//go:build hids && linux

package ebpf

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gopscommon "github.com/shirou/gopsutil/v4/common"
	gopsnet "github.com/shirou/gopsutil/v4/net"

	"github.com/yaklang/yaklang/common/hids/model"
)

func closeProcFixture(t *testing.T, pid int, fd int32, target string) (context.Context, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, strconv.Itoa(pid), "fd", strconv.FormatInt(int64(fd), 10))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if target != "" {
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.WithValue(context.Background(), gopscommon.EnvKey, gopscommon.EnvMap{gopscommon.HostProcEnvKey: root})
	return ctx, path
}

func TestCloseEnrichmentSkipsImpossibleSocketLookups(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"missing", "ordinary-file", "readlink-error", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			ctx, path := closeProcFixture(t, 424242, 57, "")
			switch kind {
			case "ordinary-file":
				if err := os.Symlink("/some/ordinary/file", path); err != nil {
					t.Fatal(err)
				}
			case "readlink-error":
				if err := os.WriteFile(path, []byte("not a symlink"), 0600); err != nil {
					t.Fatal(err)
				}
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			calls := 0
			info := enrichNetworkCloseEventWithLookup(ctx, 424242, 57, func(context.Context, int, uint32) (gopsnet.ConnectionStat, bool) {
				calls++
				return gopsnet.ConnectionStat{}, false
			})
			if calls != 0 || info.protocol != "unknown" || info.connectionState != "closed" || info.data != nil {
				t.Fatalf("impossible socket reached scan or changed close defaults: calls=%d info=%+v", calls, info)
			}
		})
	}
}

func TestCloseEnrichmentLooksUpLiveTCPSocket(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	client, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := listener.(*net.TCPListener).SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	server, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	raw, err := client.(*net.TCPConn).SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var fd int32
	if err := raw.Control(func(value uintptr) { fd = int32(value) }); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), gopscommon.EnvKey, gopscommon.EnvMap{gopscommon.HostProcEnvKey: "/proc"})
	calls := 0
	info := enrichNetworkCloseEventWithLookup(ctx, os.Getpid(), fd, func(ctx context.Context, pid int, fd uint32) (gopsnet.ConnectionStat, bool) {
		calls++
		return lookupConnectionByFD(ctx, pid, fd)
	})
	if calls != 1 || info.protocol != "tcp" || info.connectionState != "closed" ||
		info.sourceAddress != "127.0.0.1" || info.destAddress != "127.0.0.1" ||
		info.sourcePort != client.LocalAddr().(*net.TCPAddr).Port || info.destPort != listener.Addr().(*net.TCPAddr).Port ||
		info.data["previous_state"] != "ESTABLISHED" {
		t.Fatalf("live socket lookup/tuple lost: calls=%d info=%+v", calls, info)
	}
}

func TestCloseEnrichmentKeepsSocketFDZeroInSelectedProcView(t *testing.T) {
	t.Parallel()
	const pid, inode = 424242, 12345
	ctx, path := closeProcFixture(t, pid, 0, fmt.Sprintf("socket:[%d]", inode))
	processRoot := filepath.Dir(filepath.Dir(path))
	if err := os.Mkdir(filepath.Join(processRoot, "net"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"tcp":  fmt.Sprintf("header\n0: 0100007F:3039 0100007F:01BB 01 00000000:00000000 00:00000000 00000000 0 0 %d\n", inode),
		"tcp6": "header\n", "udp": "header\n", "udp6": "header\n", "unix": "header\n",
	} {
		if err := os.WriteFile(filepath.Join(processRoot, "net", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	info := enrichNetworkCloseEventWithLookup(ctx, pid, 0, func(ctx context.Context, pid int, fd uint32) (gopsnet.ConnectionStat, bool) {
		calls++
		return lookupConnectionByFD(ctx, pid, fd)
	})
	if calls != 1 || info.protocol != "tcp" || info.sourcePort != 12345 || info.destPort != 443 {
		t.Fatalf("socket on FD 0 was rejected: calls=%d info=%+v", calls, info)
	}
}

func TestCloseEnrichmentHonorsContextThenEnvironmentProcRoot(t *testing.T) {
	ctx, _ := closeProcFixture(t, 424242, 57, "socket:[12345]")
	root := ctx.Value(gopscommon.EnvKey).(gopscommon.EnvMap)[gopscommon.HostProcEnvKey]
	t.Setenv("HOST_PROC", root)
	calls := 0
	lookup := func(context.Context, int, uint32) (gopsnet.ConnectionStat, bool) {
		calls++
		return gopsnet.ConnectionStat{}, false
	}
	_ = enrichNetworkCloseEventWithLookup(context.Background(), 424242, 57, lookup)
	if calls != 1 {
		t.Fatal("environment-selected proc root was ignored")
	}
	missingCtx, _ := closeProcFixture(t, 424242, 57, "")
	_ = enrichNetworkCloseEventWithLookup(missingCtx, 424242, 57, lookup)
	if calls != 1 {
		t.Fatal("environment overrode context-selected proc root")
	}
	// An empty context override has the same environment fallback as gopsutil.
	empty := context.WithValue(context.Background(), gopscommon.EnvKey, gopscommon.EnvMap{gopscommon.HostProcEnvKey: ""})
	_ = enrichNetworkCloseEventWithLookup(empty, 424242, 57, lookup)
	if calls != 2 {
		t.Fatal("empty context proc root did not fall back to environment")
	}
	t.Setenv("HOST_PROC", t.TempDir())
	_ = enrichNetworkCloseEventWithLookup(ctx, 424242, 57, lookup)
	if calls != 3 {
		t.Fatal("context-selected socket view was discarded by environment")
	}
}

func TestCloseEnrichmentDoesNotCacheFDClassification(t *testing.T) {
	t.Parallel()
	ctx, path := closeProcFixture(t, 424242, 57, "socket:[12345]")
	calls := 0
	lookup := func(context.Context, int, uint32) (gopsnet.ConnectionStat, bool) {
		calls++
		return gopsnet.ConnectionStat{Type: 1, Laddr: gopsnet.Addr{IP: "127.0.0.1", Port: uint32(12000 + calls)}}, true
	}
	first := enrichNetworkCloseEventWithLookup(ctx, 424242, 57, lookup)
	if calls != 1 || first.sourcePort != 12001 {
		t.Fatalf("initial socket lookup failed: %+v", first)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/some/reused/ordinary-file", path); err != nil {
		t.Fatal(err)
	}
	second := enrichNetworkCloseEventWithLookup(ctx, 424242, 57, lookup)
	if calls != 1 || second.protocol != "unknown" || second.sourcePort != 0 {
		t.Fatalf("reused ordinary FD used stale socket state: calls=%d info=%+v", calls, second)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("socket:[98765]", path); err != nil {
		t.Fatal(err)
	}
	third := enrichNetworkCloseEventWithLookup(ctx, 424242, 57, lookup)
	if calls != 2 || third.sourcePort != 12002 {
		t.Fatalf("reused socket FD did not get a fresh lookup: calls=%d info=%+v", calls, third)
	}
}

func TestNetworkCloseRecordRemainsWhenSocketCannotBeEnriched(t *testing.T) {
	t.Parallel()
	file, err := os.CreateTemp(t.TempDir(), "ordinary-file-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	for _, fd := range []int32{int32(file.Fd()), 1<<30 - 1} {
		event := networkCloseRecordToEvent("ebpf.network", networkRecord{
			pid: uint32(os.Getpid()), tgid: uint32(os.Getpid()), fd: fd, comm: "close-fixture",
		})
		if event.Type != model.EventTypeNetworkClose || event.Process == nil || event.Process.PID != os.Getpid() ||
			event.Network == nil || event.Network.ConnectionState != "closed" ||
			event.Data["fd"] != int(fd) || event.Data["comm"] != "close-fixture" || event.Data["tgid"] != os.Getpid() {
			t.Fatalf("unresolved raw close identity/lifecycle was dropped: %+v", event)
		}
	}
}
