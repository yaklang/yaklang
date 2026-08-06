package sysproc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
	"unsafe"

	"github.com/mdlayher/netlink"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/sys/unix"
)

const (
	SOCK_DIAG_BY_FAMILY  = 20
	inetDiagRequestSize  = int(unsafe.Sizeof(inetDiagRequest{}))
	inetDiagResponseSize = int(unsafe.Sizeof(inetDiagResponse{}))
	inetDiagConnPoolSize = 16
)

const maxProcessSearchHintsPerUID = 8

type processSearchHintKey struct {
	procRoot string
	uid      uint32
}

type processSearchHint struct {
	pid string
}

var (
	processSearchHints = utils.NewTTLCacheWithKey[processSearchHintKey, []processSearchHint](time.Minute)
	// A small fixed stripe set bounds synchronization state while preventing a
	// burst of new connections from scanning all of /proc concurrently.
	processSearchFallbackLocks [64]sync.Mutex
	// Dialing NETLINK_INET_DIAG creates a socket and a namespace worker. MITM
	// traffic can otherwise pay that setup cost once per client TCP connection.
	// Keep the pool bounded so high-concurrency scans cannot retain unbounded
	// descriptors after a burst.
	inetDiagConnPool = make(chan *netlink.Conn, inetDiagConnPoolSize)
)

type inetDiagRequest struct {
	Family   byte
	Protocol byte
	Ext      byte
	Pad      byte
	States   uint32

	SrcPort [2]byte
	DstPort [2]byte
	Src     [16]byte
	Dst     [16]byte
	If      uint32
	Cookie  [2]uint32
}

type inetDiagResponse struct {
	Family  byte
	State   byte
	Timer   byte
	ReTrans byte

	SrcPort [2]byte
	DstPort [2]byte
	Src     [16]byte
	Dst     [16]byte
	If      uint32
	Cookie  [2]uint32

	Expires uint32
	RQueue  uint32
	WQueue  uint32
	UID     uint32
	INode   uint32
}

func findProcessName(network string, ip netip.Addr, srcPort int) (uint32, string, error) {
	uid, inode, err := resolveSocketByNetlink(network, ip, srcPort)
	if err != nil {
		return 0, "", err
	}
	pp, err := resolveProcessNameByProcSearch(inode, uid)
	return uid, pp, err
}

func findProcessNameByEndpoints(
	network string,
	srcIP netip.Addr,
	srcPort int,
	dstIP netip.Addr,
	dstPort int,
) (uint32, string, error) {
	uid, inode, err := resolveSocketByNetlinkEndpoints(network, srcIP, srcPort, dstIP, dstPort)
	if err != nil {
		// Keep compatibility with unusual net.Conn implementations and kernels
		// which do not support exact INET_DIAG tuple queries.
		return findProcessName(network, srcIP, srcPort)
	}
	processName, err := resolveProcessNameByProcSearch(inode, uid)
	return uid, processName, err
}

func acquireInetDiagConn() (*netlink.Conn, error) {
	select {
	case conn := <-inetDiagConnPool:
		return conn, nil
	default:
		return netlink.Dial(unix.NETLINK_INET_DIAG, nil)
	}
}

func releaseInetDiagConn(conn *netlink.Conn, reusable bool) {
	if conn == nil {
		return
	}
	if !reusable {
		_ = conn.Close()
		return
	}
	select {
	case inetDiagConnPool <- conn:
	default:
		_ = conn.Close()
	}
}

func resolveSocketByNetlink(network string, ip netip.Addr, srcPort int) (uint32, uint32, error) {
	request := &inetDiagRequest{
		States: 0xffffffff,
		Cookie: [2]uint32{0xffffffff, 0xffffffff},
	}

	if ip.Is4() {
		request.Family = unix.AF_INET
	} else {
		request.Family = unix.AF_INET6
	}

	if strings.HasPrefix(network, "tcp") {
		request.Protocol = unix.IPPROTO_TCP
	} else if strings.HasPrefix(network, "udp") {
		request.Protocol = unix.IPPROTO_UDP
	} else {
		return 0, 0, ErrInvalidNetwork
	}

	copy(request.Src[:], ip.AsSlice())

	binary.BigEndian.PutUint16(request.SrcPort[:], uint16(srcPort))

	conn, err := acquireInetDiagConn()
	if err != nil {
		return 0, 0, err
	}
	reusable := false
	defer func() {
		releaseInetDiagConn(conn, reusable)
	}()

	message := netlink.Message{
		Header: netlink.Header{
			Type:  SOCK_DIAG_BY_FAMILY,
			Flags: netlink.Request | netlink.Dump,
		},
		Data: (*(*[inetDiagRequestSize]byte)(unsafe.Pointer(request)))[:],
	}

	messages, err := conn.Execute(message)
	if err != nil {
		return 0, 0, err
	}
	reusable = true

	for _, msg := range messages {
		if len(msg.Data) < inetDiagResponseSize {
			continue
		}

		response := (*inetDiagResponse)(unsafe.Pointer(&msg.Data[0]))

		return response.UID, response.INode, nil
	}

	return 0, 0, ErrNotFound
}

func resolveSocketByNetlinkEndpoints(
	network string,
	srcIP netip.Addr,
	srcPort int,
	dstIP netip.Addr,
	dstPort int,
) (uint32, uint32, error) {
	if !srcIP.IsValid() || !dstIP.IsValid() || srcIP.Is4() != dstIP.Is4() {
		return 0, 0, ErrNotFound
	}
	request := &inetDiagRequest{
		States: 0xffffffff,
		Cookie: [2]uint32{0xffffffff, 0xffffffff},
	}
	if srcIP.Is4() {
		request.Family = unix.AF_INET
	} else {
		request.Family = unix.AF_INET6
	}
	if strings.HasPrefix(network, "tcp") {
		request.Protocol = unix.IPPROTO_TCP
	} else if strings.HasPrefix(network, "udp") {
		request.Protocol = unix.IPPROTO_UDP
	} else {
		return 0, 0, ErrInvalidNetwork
	}
	copy(request.Src[:], srcIP.AsSlice())
	copy(request.Dst[:], dstIP.AsSlice())
	binary.BigEndian.PutUint16(request.SrcPort[:], uint16(srcPort))
	binary.BigEndian.PutUint16(request.DstPort[:], uint16(dstPort))

	conn, err := acquireInetDiagConn()
	if err != nil {
		return 0, 0, err
	}
	reusable := false
	defer func() {
		releaseInetDiagConn(conn, reusable)
	}()

	messages, err := conn.Execute(netlink.Message{
		Header: netlink.Header{Type: SOCK_DIAG_BY_FAMILY, Flags: netlink.Request},
		Data:   (*(*[inetDiagRequestSize]byte)(unsafe.Pointer(request)))[:],
	})
	if err != nil {
		return 0, 0, err
	}
	reusable = true
	for _, msg := range messages {
		if len(msg.Data) < inetDiagResponseSize {
			continue
		}
		response := (*inetDiagResponse)(unsafe.Pointer(&msg.Data[0]))
		return response.UID, response.INode, nil
	}
	return 0, 0, ErrNotFound
}

func resolveProcessNameByProcSearch(inode, uid uint32) (string, error) {
	return resolveProcessNameByProcSearchRoot("/proc", inode, uid)
}

func readDirUnsorted(dirPath string) ([]os.DirEntry, error) {
	dir, err := os.Open(dirPath)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.ReadDir(-1)
}

func processOwnsSocket(procRoot, pid string, socket, buffer []byte) bool {
	fdPath := filepath.Join(procRoot, pid, "fd")
	fdDirectory, err := os.Open(fdPath)
	if err != nil {
		return false
	}
	defer fdDirectory.Close()
	directoryFD := int(fdDirectory.Fd())
	for {
		// Process descriptors are usually a small set. Read bounded batches so
		// a match does not require materializing every os.DirEntry and use
		// readlinkat to avoid building one absolute path per descriptor.
		names, readErr := fdDirectory.Readdirnames(32)
		for _, name := range names {
			n, linkErr := unix.Readlinkat(directoryFD, name, buffer)
			if linkErr == nil && bytes.Equal(buffer[:n], socket) {
				return true
			}
		}
		if readErr != nil {
			return false
		}
	}
}

func readProcessName(processPath string) (string, error) {
	if runtime.GOOS == "android" {
		cmdline, err := os.ReadFile(path.Join(processPath, "cmdline"))
		if err != nil {
			return "", err
		}
		return splitCmdline(cmdline), nil
	}
	return os.Readlink(filepath.Join(processPath, "exe"))
}

func findProcessNameFromHints(key processSearchHintKey, socket, buffer []byte) (string, bool) {
	hints, ok := processSearchHints.Get(key)
	if !ok {
		return "", false
	}
	for _, hint := range hints {
		if processOwnsSocket(key.procRoot, hint.pid, socket, buffer) {
			name, err := readProcessName(filepath.Join(key.procRoot, hint.pid))
			if err == nil {
				return name, true
			}
		}
	}
	return "", false
}

func rememberProcessSearchHint(key processSearchHintKey, hint processSearchHint) {
	previous, _ := processSearchHints.Get(key)
	next := make([]processSearchHint, 0, min(len(previous)+1, maxProcessSearchHintsPerUID))
	next = append(next, hint)
	for _, item := range previous {
		if item.pid == hint.pid {
			continue
		}
		next = append(next, item)
		if len(next) == maxProcessSearchHintsPerUID {
			break
		}
	}
	processSearchHints.Set(key, next)
}

func resolveProcessNameByProcSearchRoot(procRoot string, inode, uid uint32) (string, error) {
	buffer := make([]byte, unix.PathMax)
	socket := fmt.Appendf(nil, "socket:[%d]", inode)
	key := processSearchHintKey{procRoot: procRoot, uid: uid}
	if name, ok := findProcessNameFromHints(key, socket, buffer); ok {
		return name, nil
	}

	lock := &processSearchFallbackLocks[uid%uint32(len(processSearchFallbackLocks))]
	lock.Lock()
	defer lock.Unlock()
	// A concurrent lookup for the same client process may have populated a hint
	// while this connection was waiting for the fallback scan.
	if name, ok := findProcessNameFromHints(key, socket, buffer); ok {
		return name, nil
	}

	files, err := readDirUnsorted(procRoot)
	if err != nil {
		return "", err
	}

	for _, f := range files {
		if !f.IsDir() || !isPid(f.Name()) {
			continue
		}

		info, err := f.Info()
		if err != nil {
			// /proc is inherently racy: a process may exit between ReadDir and
			// Lstat. It cannot own the socket anymore, so keep scanning.
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		if info.Sys().(*syscall.Stat_t).Uid != uid {
			continue
		}

		processPath := filepath.Join(procRoot, f.Name())
		if !processOwnsSocket(procRoot, f.Name(), socket, buffer) {
			continue
		}

		name, err := readProcessName(processPath)
		if err != nil {
			return "", err
		}
		rememberProcessSearchHint(key, processSearchHint{pid: f.Name()})
		return name, nil
	}

	return "", fmt.Errorf("process of uid(%d),inode(%d) not found", uid, inode)
}

func splitCmdline(cmdline []byte) string {
	cmdline = bytes.Trim(cmdline, " ")

	idx := bytes.IndexFunc(cmdline, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	})

	if idx == -1 {
		return filepath.Base(string(cmdline))
	}
	return filepath.Base(string(cmdline[:idx]))
}

func isPid(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool {
		return !unicode.IsDigit(r)
	}) == -1
}
