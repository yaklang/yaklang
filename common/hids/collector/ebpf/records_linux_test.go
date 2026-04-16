//go:build hids && linux

package ebpf

import (
	"encoding/binary"
	"testing"

	gopsnet "github.com/shirou/gopsutil/v4/net"
)

func TestParseProcessRecord(t *testing.T) {
	t.Parallel()

	raw := make([]byte, processRecordSize)
	binary.LittleEndian.PutUint32(raw[processFieldKindOffset:processFieldKindOffset+4], recordKindProcessExec)
	binary.LittleEndian.PutUint32(raw[processFieldPIDOffset:processFieldPIDOffset+4], 1001)
	binary.LittleEndian.PutUint32(raw[processFieldTGIDOffset:processFieldTGIDOffset+4], 1000)
	copy(raw[processFieldCommOffset:processFieldCommOffset+processCommSize], []byte("bash\x00"))
	copy(raw[processFieldImageOffset:processFieldImageOffset+processImageSize], []byte("/bin/bash\x00"))

	record, err := parseProcessRecord(raw)
	if err != nil {
		t.Fatalf("parse process record: %v", err)
	}
	if record.pid != 1001 {
		t.Fatalf("unexpected pid: %d", record.pid)
	}
	if record.tgid != 1000 {
		t.Fatalf("unexpected tgid: %d", record.tgid)
	}
	if record.comm != "bash" {
		t.Fatalf("unexpected comm: %q", record.comm)
	}
	if record.image != "/bin/bash" {
		t.Fatalf("unexpected image: %q", record.image)
	}
}

func TestParseNetworkRecord(t *testing.T) {
	t.Parallel()

	raw := make([]byte, networkRecordSize)
	binary.LittleEndian.PutUint32(raw[networkFieldKindOffset:networkFieldKindOffset+4], recordKindNetworkConnect)
	binary.LittleEndian.PutUint32(raw[networkFieldPIDOffset:networkFieldPIDOffset+4], 1001)
	binary.LittleEndian.PutUint32(raw[networkFieldTGIDOffset:networkFieldTGIDOffset+4], 1000)
	binary.LittleEndian.PutUint32(raw[networkFieldFDOffset:networkFieldFDOffset+4], 7)
	binary.LittleEndian.PutUint16(raw[networkFieldFamilyOffset:networkFieldFamilyOffset+2], 2)
	copy(raw[networkFieldPortOffset:networkFieldPortOffset+2], []byte{0x11, 0x5c})
	binary.LittleEndian.PutUint32(raw[networkFieldAddrLenOffset:networkFieldAddrLenOffset+4], 4)
	copy(raw[networkFieldCommOffset:networkFieldCommOffset+networkCommSize], []byte("curl\x00"))
	copy(raw[networkFieldAddrOffset:networkFieldAddrOffset+4], []byte{8, 8, 8, 8})

	record, err := parseNetworkRecord(raw)
	if err != nil {
		t.Fatalf("parse network record: %v", err)
	}
	if record.fd != 7 {
		t.Fatalf("unexpected fd: %d", record.fd)
	}
	if record.family != 2 {
		t.Fatalf("unexpected family: %d", record.family)
	}
	if got := formatNetworkAddress(record.family, record.addrLen, record.addr); got != "8.8.8.8" {
		t.Fatalf("unexpected address: %s", got)
	}
	if got := binary.BigEndian.Uint16(record.portRaw[:]); got != 4444 {
		t.Fatalf("unexpected port: %d", got)
	}
}

func TestParseNetworkStateRecord(t *testing.T) {
	t.Parallel()

	raw := make([]byte, networkStateRecordSize)
	binary.LittleEndian.PutUint32(raw[networkStateFieldKindOffset:networkStateFieldKindOffset+4], recordKindNetworkState)
	binary.LittleEndian.PutUint16(raw[networkStateFieldFamilyOffset:networkStateFieldFamilyOffset+2], 2)
	raw[networkStateFieldProtocolOffset] = 6
	binary.LittleEndian.PutUint32(raw[networkStateFieldOldStateOffset:networkStateFieldOldStateOffset+4], 2)
	binary.LittleEndian.PutUint32(raw[networkStateFieldNewStateOffset:networkStateFieldNewStateOffset+4], 1)
	copy(raw[networkStateFieldSourcePortOffset:networkStateFieldSourcePortOffset+2], []byte{0x11, 0x5c})
	copy(raw[networkStateFieldDestPortOffset:networkStateFieldDestPortOffset+2], []byte{0x01, 0xbb})
	binary.LittleEndian.PutUint32(raw[networkStateFieldSourceAddrLen:networkStateFieldSourceAddrLen+4], 4)
	binary.LittleEndian.PutUint32(raw[networkStateFieldDestAddrLen:networkStateFieldDestAddrLen+4], 4)
	copy(raw[networkStateFieldSourceAddrOffset:networkStateFieldSourceAddrOffset+4], []byte{10, 0, 0, 5})
	copy(raw[networkStateFieldDestAddrOffset:networkStateFieldDestAddrOffset+4], []byte{1, 1, 1, 1})

	record, err := parseNetworkStateRecord(raw)
	if err != nil {
		t.Fatalf("parse network state record: %v", err)
	}
	if record.protocol != 6 {
		t.Fatalf("unexpected protocol: %d", record.protocol)
	}
	if record.oldState != 2 || record.newState != 1 {
		t.Fatalf("unexpected state transition: %d -> %d", record.oldState, record.newState)
	}
	if got := formatNetworkAddress(record.family, record.sourceAddrLen, record.sourceAddr); got != "10.0.0.5" {
		t.Fatalf("unexpected source address: %s", got)
	}
	if got := formatNetworkAddress(record.family, record.destAddrLen, record.destAddr); got != "1.1.1.1" {
		t.Fatalf("unexpected destination address: %s", got)
	}
	if got := binary.BigEndian.Uint16(record.sourcePortRaw[:]); got != 4444 {
		t.Fatalf("unexpected source port: %d", got)
	}
	if got := binary.BigEndian.Uint16(record.destPortRaw[:]); got != 443 {
		t.Fatalf("unexpected dest port: %d", got)
	}
}

func TestSelectConnectionPrefersFDMatch(t *testing.T) {
	t.Parallel()

	connections := []gopsnet.ConnectionStat{
		{
			Fd:     5,
			Family: 2,
			Type:   1,
			Laddr:  gopsnet.Addr{IP: "10.0.0.5", Port: 40000},
			Raddr:  gopsnet.Addr{IP: "1.1.1.1", Port: 443},
			Status: "SYN_SENT",
			Pid:    42,
		},
		{
			Fd:     7,
			Family: 2,
			Type:   1,
			Laddr:  gopsnet.Addr{IP: "10.0.0.6", Port: 41000},
			Raddr:  gopsnet.Addr{IP: "1.1.1.1", Port: 443},
			Status: "ESTABLISHED",
			Pid:    42,
		},
	}

	connection, ok := selectConnection(connections, 7, 2, "1.1.1.1", 443)
	if !ok {
		t.Fatal("expected matching connection")
	}
	if connection.Fd != 7 {
		t.Fatalf("expected fd 7 match, got %d", connection.Fd)
	}
}

func TestSelectConnectionFallsBackToRemoteEndpoint(t *testing.T) {
	t.Parallel()

	connections := []gopsnet.ConnectionStat{
		{
			Fd:     9,
			Family: 2,
			Type:   2,
			Laddr:  gopsnet.Addr{IP: "10.0.0.7", Port: 53000},
			Raddr:  gopsnet.Addr{IP: "8.8.8.8", Port: 53},
			Status: "",
			Pid:    99,
		},
	}

	connection, ok := selectConnection(connections, 0, 2, "8.8.8.8", 53)
	if !ok {
		t.Fatal("expected endpoint fallback match")
	}
	if connection.Fd != 9 {
		t.Fatalf("unexpected fallback connection fd: %d", connection.Fd)
	}
}

func TestSelectConnectionByFD(t *testing.T) {
	t.Parallel()

	connections := []gopsnet.ConnectionStat{
		{
			Fd:     4,
			Family: 2,
			Type:   1,
			Laddr:  gopsnet.Addr{IP: "10.0.0.5", Port: 40000},
			Raddr:  gopsnet.Addr{IP: "1.1.1.1", Port: 443},
			Status: "ESTABLISHED",
			Pid:    42,
		},
		{
			Fd:     8,
			Family: 2,
			Type:   1,
			Laddr:  gopsnet.Addr{IP: "10.0.0.5", Port: 40100},
			Raddr:  gopsnet.Addr{IP: "8.8.8.8", Port: 53},
			Status: "ESTABLISHED",
			Pid:    42,
		},
	}

	connection, ok := selectConnectionByFD(connections, 8)
	if !ok {
		t.Fatal("expected fd match")
	}
	if connection.Raddr.IP != "8.8.8.8" || connection.Raddr.Port != 53 {
		t.Fatalf("unexpected fd-selected remote endpoint: %s:%d", connection.Raddr.IP, connection.Raddr.Port)
	}
}

func TestProtocolFromSocketType(t *testing.T) {
	t.Parallel()

	if got := protocolFromSocketType(1); got != "tcp" {
		t.Fatalf("expected tcp, got %q", got)
	}
	if got := protocolFromSocketType(2); got != "udp" {
		t.Fatalf("expected udp, got %q", got)
	}
	if got := protocolFromSocketType(3); got != "raw" {
		t.Fatalf("expected raw, got %q", got)
	}
}
