package sysproc

import (
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func openLoopbackSocketForProcessLookup(tb testing.TB) (netip.Addr, int, netip.Addr, int, net.Conn) {
	tb.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("listen on loopback: %v", err)
	}
	client, err := net.Dial("tcp4", listener.Addr().String())
	if err != nil {
		listener.Close()
		tb.Fatalf("dial loopback: %v", err)
	}
	server, err := listener.Accept()
	if err != nil {
		client.Close()
		listener.Close()
		tb.Fatalf("accept loopback: %v", err)
	}
	tb.Cleanup(func() {
		server.Close()
		client.Close()
		listener.Close()
	})
	clientAddress := client.LocalAddr().(*net.TCPAddr)
	srcIP, ok := netip.AddrFromSlice(clientAddress.IP)
	if !ok {
		tb.Fatalf("parse loopback address: %v", clientAddress.IP)
	}
	serverAddress := client.RemoteAddr().(*net.TCPAddr)
	dstIP, ok := netip.AddrFromSlice(serverAddress.IP)
	if !ok {
		tb.Fatalf("parse loopback server address: %v", serverAddress.IP)
	}
	return srcIP.Unmap(), clientAddress.Port, dstIP.Unmap(), serverAddress.Port, server
}

func TestResolveSocketByNetlinkLoopback(t *testing.T) {
	ip, port, _, _, _ := openLoopbackSocketForProcessLookup(t)
	uid, inode, err := resolveSocketByNetlink(TCP, ip, port)
	if err != nil {
		t.Fatalf("first socket lookup: %v", err)
	}
	if inode == 0 {
		t.Fatal("first socket lookup returned an empty inode")
	}
	nextUID, nextInode, err := resolveSocketByNetlink(TCP, ip, port)
	if err != nil {
		t.Fatalf("second socket lookup: %v", err)
	}
	if nextUID != uid || nextInode != inode {
		t.Fatalf("socket identity changed: (%d, %d) -> (%d, %d)", uid, inode, nextUID, nextInode)
	}
}

func TestResolveSocketByNetlinkEndpointsMatchesDump(t *testing.T) {
	srcIP, srcPort, dstIP, dstPort, _ := openLoopbackSocketForProcessLookup(t)
	wantUID, wantInode, err := resolveSocketByNetlink(TCP, srcIP, srcPort)
	if err != nil {
		t.Fatalf("dump socket lookup: %v", err)
	}
	uid, inode, err := resolveSocketByNetlinkEndpoints(TCP, srcIP, srcPort, dstIP, dstPort)
	if err != nil {
		t.Fatalf("endpoint socket lookup: %v", err)
	}
	if uid != wantUID || inode != wantInode {
		t.Fatalf("endpoint identity = (%d, %d), want (%d, %d)", uid, inode, wantUID, wantInode)
	}
}

func TestFindProcessNameByConnWithEndpoints(t *testing.T) {
	_, _, _, _, server := openLoopbackSocketForProcessLookup(t)
	_, processName, err := FindProcessNameByConn(server)
	if err != nil {
		t.Fatalf("find process by accepted connection: %v", err)
	}
	want, err := os.Executable()
	if err != nil {
		t.Fatalf("get test executable: %v", err)
	}
	want, err = filepath.EvalSymlinks(want)
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	if processName != want {
		t.Fatalf("process name = %q, want %q", processName, want)
	}
}

func TestResolveSocketByNetlinkLoopbackConcurrent(t *testing.T) {
	const workers = 32
	ip, port, _, _, _ := openLoopbackSocketForProcessLookup(t)
	wantUID, wantInode, err := resolveSocketByNetlink(TCP, ip, port)
	if err != nil {
		t.Fatalf("baseline socket lookup: %v", err)
	}

	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			uid, inode, err := resolveSocketByNetlink(TCP, ip, port)
			if err != nil {
				errors <- err
				return
			}
			if uid != wantUID || inode != wantInode {
				errors <- fmt.Errorf("socket identity = (%d, %d), want (%d, %d)", uid, inode, wantUID, wantInode)
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
}

func BenchmarkResolveSocketByNetlinkLoopback(b *testing.B) {
	ip, port, _, _, _ := openLoopbackSocketForProcessLookup(b)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, _, err := resolveSocketByNetlink(TCP, ip, port); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkResolveSocketByNetlinkEndpointsLoopback(b *testing.B) {
	srcIP, srcPort, dstIP, dstPort, _ := openLoopbackSocketForProcessLookup(b)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if _, _, err := resolveSocketByNetlinkEndpoints(TCP, srcIP, srcPort, dstIP, dstPort); err != nil {
			b.Fatal(err)
		}
	}
}

func prepareProcessSocketOwnershipBenchmark(tb testing.TB, fileDescriptors int) (string, string, []byte, []byte) {
	tb.Helper()
	procRoot := tb.TempDir()
	pid := "1234"
	fdPath := filepath.Join(procRoot, pid, "fd")
	if err := os.MkdirAll(fdPath, 0o755); err != nil {
		tb.Fatalf("create fd directory: %v", err)
	}
	for index := 0; index < fileDescriptors; index++ {
		if err := os.Symlink(
			fmt.Sprintf("socket:[%d]", index+1),
			filepath.Join(fdPath, fmt.Sprintf("%d", index)),
		); err != nil {
			tb.Fatalf("create fd link: %v", err)
		}
	}
	target := []byte(fmt.Sprintf("socket:[%d]", fileDescriptors))
	missing := []byte("socket:[999999]")
	return procRoot, pid, target, missing
}

func TestProcessOwnsSocket(t *testing.T) {
	procRoot, pid, target, missing := prepareProcessSocketOwnershipBenchmark(t, 64)
	buffer := make([]byte, 4096)
	if !processOwnsSocket(procRoot, pid, target, buffer) {
		t.Fatal("expected process to own target socket")
	}
	if processOwnsSocket(procRoot, pid, missing, buffer) {
		t.Fatal("process unexpectedly owns missing socket")
	}
}

func BenchmarkProcessOwnsSocketFound(b *testing.B) {
	procRoot, pid, target, _ := prepareProcessSocketOwnershipBenchmark(b, 64)
	buffer := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if !processOwnsSocket(procRoot, pid, target, buffer) {
			b.Fatal("target socket not found")
		}
	}
}

func BenchmarkProcessOwnsSocketMissing(b *testing.B) {
	procRoot, pid, _, missing := prepareProcessSocketOwnershipBenchmark(b, 64)
	buffer := make([]byte, 4096)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if processOwnsSocket(procRoot, pid, missing, buffer) {
			b.Fatal("missing socket found")
		}
	}
}

func addFakeProcessSocket(t *testing.T, procRoot, pid, executable string, inode uint32) {
	t.Helper()
	processPath := filepath.Join(procRoot, pid)
	fdPath := filepath.Join(processPath, "fd")
	if err := os.MkdirAll(fdPath, 0o755); err != nil {
		t.Fatalf("create fake process: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(processPath, "exe")); os.IsNotExist(err) {
		if err := os.Symlink(executable, filepath.Join(processPath, "exe")); err != nil {
			t.Fatalf("create fake executable link: %v", err)
		}
	}
	if err := os.Symlink(fmt.Sprintf("socket:[%d]", inode), filepath.Join(fdPath, fmt.Sprintf("%d", inode))); err != nil {
		t.Fatalf("create fake socket link: %v", err)
	}
}

func TestResolveProcessNameByProcSearchUsesValidatedHints(t *testing.T) {
	processSearchHints.Purge()
	t.Cleanup(processSearchHints.Purge)
	procRoot := t.TempDir()
	uid := uint32(os.Getuid())

	addFakeProcessSocket(t, procRoot, "101", "/usr/bin/browser", 1001)
	name, err := resolveProcessNameByProcSearchRoot(procRoot, 1001, uid)
	if err != nil || name != "/usr/bin/browser" {
		t.Fatalf("first lookup = %q, %v", name, err)
	}

	addFakeProcessSocket(t, procRoot, "101", "/ignored", 1002)
	name, err = resolveProcessNameByProcSearchRoot(procRoot, 1002, uid)
	if err != nil || name != "/usr/bin/browser" {
		t.Fatalf("hinted lookup = %q, %v", name, err)
	}

	// A socket owned by another PID must not inherit the browser hint.
	addFakeProcessSocket(t, procRoot, "202", "/usr/bin/nuclei", 2001)
	name, err = resolveProcessNameByProcSearchRoot(procRoot, 2001, uid)
	if err != nil || name != "/usr/bin/nuclei" {
		t.Fatalf("second process lookup = %q, %v", name, err)
	}
}

func TestResolveProcessNameByProcSearchHintIsRaceSafe(t *testing.T) {
	processSearchHints.Purge()
	t.Cleanup(processSearchHints.Purge)
	procRoot := t.TempDir()
	uid := uint32(os.Getuid())
	for inode := uint32(3000); inode < 3032; inode++ {
		addFakeProcessSocket(t, procRoot, "303", "/usr/bin/browser", inode)
	}

	var wait sync.WaitGroup
	for inode := uint32(3000); inode < 3032; inode++ {
		inode := inode
		wait.Add(1)
		go func() {
			defer wait.Done()
			name, err := resolveProcessNameByProcSearchRoot(procRoot, inode, uid)
			if err != nil || name != "/usr/bin/browser" {
				t.Errorf("lookup %d = %q, %v", inode, name, err)
			}
		}()
	}
	wait.Wait()
}
