//go:build hids && linux

package ebpf

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	gopsprocess "github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/unix"

	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	tcpStateEstablished = 1
	tcpStateSynSent     = 2
	tcpStateSynRecv     = 3
	tcpStateFinWait1    = 4
	tcpStateFinWait2    = 5
	tcpStateTimeWait    = 6
	tcpStateClose       = 7
	tcpStateCloseWait   = 8
	tcpStateLastAck     = 9
	tcpStateListen      = 10
	tcpStateClosing     = 11
	tcpStateNewSynRecv  = 12
)

type processRecord struct {
	pid   uint32
	tgid  uint32
	comm  string
	image string
}

type networkRecord struct {
	pid     uint32
	tgid    uint32
	fd      int32
	family  uint16
	portRaw [2]byte
	addrLen uint32
	comm    string
	addr    [networkAddrSize]byte
}

type networkStateRecord struct {
	family        uint16
	protocol      uint8
	oldState      uint32
	newState      uint32
	sourcePortRaw [2]byte
	destPortRaw   [2]byte
	sourceAddrLen uint32
	destAddrLen   uint32
	sourceAddr    [networkAddrSize]byte
	destAddr      [networkAddrSize]byte
}

func decodeRecordKind(raw []byte) uint32 {
	if len(raw) < 4 {
		return 0
	}
	return binary.LittleEndian.Uint32(raw[:4])
}

func parseProcessRecord(raw []byte) (processRecord, error) {
	if len(raw) < processRecordSize {
		return processRecord{}, fmt.Errorf("short process record: got=%d want>=%d", len(raw), processRecordSize)
	}
	return processRecord{
		pid:   binary.LittleEndian.Uint32(raw[processFieldPIDOffset : processFieldPIDOffset+4]),
		tgid:  binary.LittleEndian.Uint32(raw[processFieldTGIDOffset : processFieldTGIDOffset+4]),
		comm:  trimCString(raw[processFieldCommOffset : processFieldCommOffset+processCommSize]),
		image: trimCString(raw[processFieldImageOffset : processFieldImageOffset+processImageSize]),
	}, nil
}

func parseNetworkRecord(raw []byte) (networkRecord, error) {
	if len(raw) < networkRecordSize {
		return networkRecord{}, fmt.Errorf("short network record: got=%d want>=%d", len(raw), networkRecordSize)
	}

	var portRaw [2]byte
	copy(portRaw[:], raw[networkFieldPortOffset:networkFieldPortOffset+2])

	var addr [networkAddrSize]byte
	copy(addr[:], raw[networkFieldAddrOffset:networkFieldAddrOffset+networkAddrSize])

	return networkRecord{
		pid:     binary.LittleEndian.Uint32(raw[networkFieldPIDOffset : networkFieldPIDOffset+4]),
		tgid:    binary.LittleEndian.Uint32(raw[networkFieldTGIDOffset : networkFieldTGIDOffset+4]),
		fd:      int32(binary.LittleEndian.Uint32(raw[networkFieldFDOffset : networkFieldFDOffset+4])),
		family:  binary.LittleEndian.Uint16(raw[networkFieldFamilyOffset : networkFieldFamilyOffset+2]),
		portRaw: portRaw,
		addrLen: binary.LittleEndian.Uint32(raw[networkFieldAddrLenOffset : networkFieldAddrLenOffset+4]),
		comm:    trimCString(raw[networkFieldCommOffset : networkFieldCommOffset+networkCommSize]),
		addr:    addr,
	}, nil
}

func parseNetworkStateRecord(raw []byte) (networkStateRecord, error) {
	if len(raw) < networkStateRecordSize {
		return networkStateRecord{}, fmt.Errorf("short network state record: got=%d want>=%d", len(raw), networkStateRecordSize)
	}

	var sourcePortRaw [2]byte
	copy(sourcePortRaw[:], raw[networkStateFieldSourcePortOffset:networkStateFieldSourcePortOffset+2])

	var destPortRaw [2]byte
	copy(destPortRaw[:], raw[networkStateFieldDestPortOffset:networkStateFieldDestPortOffset+2])

	var sourceAddr [networkAddrSize]byte
	copy(sourceAddr[:], raw[networkStateFieldSourceAddrOffset:networkStateFieldSourceAddrOffset+networkAddrSize])

	var destAddr [networkAddrSize]byte
	copy(destAddr[:], raw[networkStateFieldDestAddrOffset:networkStateFieldDestAddrOffset+networkAddrSize])

	return networkStateRecord{
		family:        binary.LittleEndian.Uint16(raw[networkStateFieldFamilyOffset : networkStateFieldFamilyOffset+2]),
		protocol:      raw[networkStateFieldProtocolOffset],
		oldState:      binary.LittleEndian.Uint32(raw[networkStateFieldOldStateOffset : networkStateFieldOldStateOffset+4]),
		newState:      binary.LittleEndian.Uint32(raw[networkStateFieldNewStateOffset : networkStateFieldNewStateOffset+4]),
		sourcePortRaw: sourcePortRaw,
		destPortRaw:   destPortRaw,
		sourceAddrLen: binary.LittleEndian.Uint32(raw[networkStateFieldSourceAddrLen : networkStateFieldSourceAddrLen+4]),
		destAddrLen:   binary.LittleEndian.Uint32(raw[networkStateFieldDestAddrLen : networkStateFieldDestAddrLen+4]),
		sourceAddr:    sourceAddr,
		destAddr:      destAddr,
	}, nil
}

func processRecordToEvent(source string, record processRecord) model.Event {
	now := time.Now().UTC()
	pid := pickProcessID(record.pid, record.tgid)
	processInfo, data := enrichProcessEvent(pid, processEventOptions{
		fallbackCommand: record.image,
		fallbackImage:   record.image,
		fallbackName:    filepath.Base(strings.TrimSpace(record.image)),
		preferProcState: false,
	})

	return model.Event{
		Type:      model.EventTypeProcessExec,
		Source:    source,
		Timestamp: now,
		Tags:      []string{"process", "ebpf"},
		Process:   &processInfo,
		Data: mergeData(data, map[string]any{
			"thread_id": int(record.pid),
			"tgid":      int(record.tgid),
			"comm":      record.comm,
		}),
	}
}

func processExitRecordToEvent(source string, record processRecord) model.Event {
	now := time.Now().UTC()
	pid := pickProcessID(record.pid, record.tgid)
	processInfo := model.Process{
		PID:  pid,
		Name: strings.TrimSpace(record.comm),
	}

	return model.Event{
		Type:      model.EventTypeProcessExit,
		Source:    source,
		Timestamp: now,
		Tags:      []string{"process", "ebpf", "exit"},
		Process:   &processInfo,
		Data: map[string]any{
			"thread_id": int(record.pid),
			"tgid":      int(record.tgid),
			"comm":      record.comm,
		},
	}
}

func networkRecordToEvent(source string, record networkRecord) model.Event {
	now := time.Now().UTC()
	pid := pickProcessID(record.pid, record.tgid)
	destAddress := formatNetworkAddress(record.family, record.addrLen, record.addr)
	destPort := int(binary.BigEndian.Uint16(record.portRaw[:]))
	processInfo, data := enrichProcessEvent(pid, processEventOptions{
		fallbackCommand: record.comm,
		fallbackImage:   "",
		fallbackName:    strings.TrimSpace(record.comm),
		preferProcState: true,
	})
	networkInfo := enrichNetworkEvent(pid, record.fd, record.family, destAddress, destPort)
	data = mergeData(data, networkInfo.data)

	return model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    source,
		Timestamp: now,
		Tags:      []string{"network", "ebpf", "outbound"},
		Process:   &processInfo,
		Network: &model.Network{
			Protocol:        networkInfo.protocol,
			SourceAddress:   networkInfo.sourceAddress,
			DestAddress:     destAddress,
			SourcePort:      networkInfo.sourcePort,
			DestPort:        destPort,
			ConnectionState: networkInfo.connectionState,
		},
		Data: mergeData(data, map[string]any{
			"thread_id":    int(record.pid),
			"tgid":         int(record.tgid),
			"fd":           int(record.fd),
			"family":       int(record.family),
			"comm":         record.comm,
			"dest_address": destAddress,
			"dest_port":    destPort,
		}),
	}
}

func networkCloseRecordToEvent(source string, record networkRecord) model.Event {
	now := time.Now().UTC()
	pid := pickProcessID(record.pid, record.tgid)
	processInfo, data := enrichProcessEvent(pid, processEventOptions{
		fallbackCommand: record.comm,
		fallbackImage:   "",
		fallbackName:    strings.TrimSpace(record.comm),
		preferProcState: true,
	})
	networkInfo := enrichNetworkCloseEvent(pid, record.fd)
	data = mergeData(data, networkInfo.data)

	return model.Event{
		Type:      model.EventTypeNetworkClose,
		Source:    source,
		Timestamp: now,
		Tags:      []string{"network", "ebpf", "close"},
		Process:   &processInfo,
		Network: &model.Network{
			Protocol:        networkInfo.protocol,
			SourceAddress:   networkInfo.sourceAddress,
			DestAddress:     networkInfo.destAddress,
			SourcePort:      networkInfo.sourcePort,
			DestPort:        networkInfo.destPort,
			ConnectionState: networkInfo.connectionState,
		},
		Data: mergeData(data, map[string]any{
			"thread_id": int(record.pid),
			"tgid":      int(record.tgid),
			"fd":        int(record.fd),
			"comm":      record.comm,
		}),
	}
}

func networkAcceptRecordToEvent(source string, record networkRecord) model.Event {
	now := time.Now().UTC()
	pid := pickProcessID(record.pid, record.tgid)
	processInfo, data := enrichProcessEvent(pid, processEventOptions{
		fallbackCommand: record.comm,
		fallbackImage:   "",
		fallbackName:    strings.TrimSpace(record.comm),
		preferProcState: true,
	})
	networkInfo := enrichNetworkAcceptEvent(pid, record.fd)
	data = mergeData(data, networkInfo.data)

	return model.Event{
		Type:      model.EventTypeNetworkAccept,
		Source:    source,
		Timestamp: now,
		Tags:      []string{"network", "ebpf", "inbound", "accept"},
		Process:   &processInfo,
		Network: &model.Network{
			Protocol:        networkInfo.protocol,
			SourceAddress:   networkInfo.sourceAddress,
			DestAddress:     networkInfo.destAddress,
			SourcePort:      networkInfo.sourcePort,
			DestPort:        networkInfo.destPort,
			ConnectionState: networkInfo.connectionState,
		},
		Data: mergeData(data, map[string]any{
			"thread_id": int(record.pid),
			"tgid":      int(record.tgid),
			"fd":        int(record.fd),
			"comm":      record.comm,
		}),
	}
}

func networkStateRecordToEvent(source string, record networkStateRecord) model.Event {
	now := time.Now().UTC()
	sourceAddress := formatNetworkAddress(record.family, record.sourceAddrLen, record.sourceAddr)
	destAddress := formatNetworkAddress(record.family, record.destAddrLen, record.destAddr)
	sourcePort := int(binary.BigEndian.Uint16(record.sourcePortRaw[:]))
	destPort := int(binary.BigEndian.Uint16(record.destPortRaw[:]))
	protocol := protocolFromIPProtocolNumber(record.protocol)
	if protocol == "" {
		protocol = "unknown"
	}

	oldState := tcpConnectionStateName(record.oldState)
	newState := tcpConnectionStateName(record.newState)
	data := map[string]any{
		"family":          int(record.family),
		"protocol_number": int(record.protocol),
		"source_address":  sourceAddress,
		"source_port":     sourcePort,
		"dest_address":    destAddress,
		"dest_port":       destPort,
	}
	if oldState != "" {
		data["old_connection_state"] = oldState
		data["previous_connection_state"] = oldState
	} else {
		data["old_connection_state_id"] = int(record.oldState)
	}
	if newState != "" {
		data["new_connection_state"] = newState
		data["connection_state"] = newState
	} else {
		data["new_connection_state_id"] = int(record.newState)
	}

	return model.Event{
		Type:      model.EventTypeNetworkState,
		Source:    source,
		Timestamp: now,
		Tags:      []string{"network", "ebpf", "state"},
		Network: &model.Network{
			Protocol:        protocol,
			SourceAddress:   sourceAddress,
			DestAddress:     destAddress,
			SourcePort:      sourcePort,
			DestPort:        destPort,
			ConnectionState: defaultString(tcpConnectionStateName(record.newState), "unknown"),
		},
		Data: data,
	}
}

type processEventOptions struct {
	fallbackCommand string
	fallbackImage   string
	fallbackName    string
	preferProcState bool
}

type networkEventInfo struct {
	protocol        string
	sourceAddress   string
	destAddress     string
	sourcePort      int
	destPort        int
	connectionState string
	data            map[string]any
}

func enrichProcessEvent(pid int, options processEventOptions) (model.Process, map[string]any) {
	processInfo := model.Process{
		PID:     pid,
		Name:    strings.TrimSpace(options.fallbackName),
		Image:   strings.TrimSpace(options.fallbackImage),
		Command: strings.TrimSpace(options.fallbackCommand),
	}
	if processInfo.Name == "" && processInfo.Image != "" {
		processInfo.Name = filepath.Base(processInfo.Image)
	}

	if pid <= 0 {
		return processInfo, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	process, err := gopsprocess.NewProcessWithContext(ctx, int32(pid))
	if err != nil {
		ppid, parentName := readParentProcessInfo(pid)
		processInfo.ParentPID = ppid
		processInfo.ParentName = parentName
		return processInfo, nil
	}

	parentPID, _ := process.Ppid()
	processInfo.ParentPID = int(parentPID)
	if parentPID > 0 {
		if parent, err := gopsprocess.NewProcessWithContext(ctx, parentPID); err == nil {
			parentName, _ := parent.NameWithContext(ctx)
			processInfo.ParentName = strings.TrimSpace(parentName)
		}
	}

	name, _ := process.NameWithContext(ctx)
	username, _ := process.UsernameWithContext(ctx)
	exe, _ := process.ExeWithContext(ctx)
	cmdline, _ := process.CmdlineWithContext(ctx)
	statuses, _ := process.StatusWithContext(ctx)

	if strings.TrimSpace(name) != "" {
		processInfo.Name = strings.TrimSpace(name)
	}
	processInfo.Username = strings.TrimSpace(username)
	if options.preferProcState {
		if strings.TrimSpace(exe) != "" {
			processInfo.Image = strings.TrimSpace(exe)
		}
		if strings.TrimSpace(cmdline) != "" {
			processInfo.Command = strings.TrimSpace(cmdline)
		}
	} else {
		if processInfo.Command == "" && strings.TrimSpace(cmdline) != "" {
			processInfo.Command = strings.TrimSpace(cmdline)
		}
		if processInfo.Image == "" && strings.TrimSpace(exe) != "" {
			processInfo.Image = strings.TrimSpace(exe)
		}
	}

	if processInfo.Name == "" && processInfo.Image != "" {
		processInfo.Name = filepath.Base(processInfo.Image)
	}
	if processInfo.Command == "" {
		processInfo.Command = processInfo.Image
	}

	data := map[string]any{}
	if processInfo.Username != "" {
		data["username"] = processInfo.Username
	}
	if processInfo.Name != "" {
		data["process_name"] = processInfo.Name
	}
	if len(statuses) > 0 {
		data["process_status"] = strings.Join(statuses, ",")
	}

	return processInfo, data
}

func enrichNetworkEvent(
	pid int,
	fd int32,
	family uint16,
	destAddress string,
	destPort int,
) networkEventInfo {
	info := networkEventInfo{
		protocol:        "unknown",
		connectionState: "attempt",
	}
	if pid <= 0 {
		return info
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	connections, err := gopsnet.ConnectionsPidWithContext(ctx, "all", int32(pid))
	if err != nil {
		return info
	}

	connection, ok := selectConnection(connections, uint32(fd), family, destAddress, uint32(destPort))
	if !ok {
		return info
	}

	info.protocol = protocolFromSocketType(connection.Type)
	if info.protocol == "" {
		info.protocol = "unknown"
	}
	info.sourceAddress = connection.Laddr.IP
	info.sourcePort = int(connection.Laddr.Port)
	if state := strings.TrimSpace(connection.Status); state != "" {
		info.connectionState = state
	}
	info.data = map[string]any{
		"socket_type":      int(connection.Type),
		"source_address":   connection.Laddr.IP,
		"source_port":      int(connection.Laddr.Port),
		"dest_address":     connection.Raddr.IP,
		"dest_port":        int(connection.Raddr.Port),
		"connection_state": strings.TrimSpace(connection.Status),
	}
	return info
}

func enrichNetworkCloseEvent(pid int, fd int32) networkEventInfo {
	info := networkEventInfo{
		protocol:        "unknown",
		connectionState: "closed",
	}
	if pid <= 0 || fd < 0 {
		return info
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	connection, ok := lookupConnectionByFD(ctx, pid, uint32(fd))
	if !ok {
		return info
	}

	info.protocol = protocolFromSocketType(connection.Type)
	if info.protocol == "" {
		info.protocol = "unknown"
	}
	info.sourceAddress = connection.Laddr.IP
	info.destAddress = connection.Raddr.IP
	info.sourcePort = int(connection.Laddr.Port)
	info.destPort = int(connection.Raddr.Port)
	info.data = map[string]any{
		"socket_type":      int(connection.Type),
		"source_address":   connection.Laddr.IP,
		"source_port":      int(connection.Laddr.Port),
		"dest_address":     connection.Raddr.IP,
		"dest_port":        int(connection.Raddr.Port),
		"connection_state": "closed",
		"previous_state":   strings.TrimSpace(connection.Status),
	}
	return info
}

func enrichNetworkAcceptEvent(pid int, fd int32) networkEventInfo {
	info := networkEventInfo{
		protocol:        "unknown",
		connectionState: "accepted",
	}
	if pid <= 0 || fd < 0 {
		return info
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	connection, ok := lookupConnectionByFD(ctx, pid, uint32(fd))
	if !ok {
		return info
	}

	info.protocol = protocolFromSocketType(connection.Type)
	if info.protocol == "" {
		info.protocol = "unknown"
	}
	info.sourceAddress = connection.Laddr.IP
	info.destAddress = connection.Raddr.IP
	info.sourcePort = int(connection.Laddr.Port)
	info.destPort = int(connection.Raddr.Port)
	if state := strings.TrimSpace(connection.Status); state != "" {
		info.connectionState = state
	}
	info.data = map[string]any{
		"socket_type":      int(connection.Type),
		"source_address":   connection.Laddr.IP,
		"source_port":      int(connection.Laddr.Port),
		"dest_address":     connection.Raddr.IP,
		"dest_port":        int(connection.Raddr.Port),
		"connection_state": info.connectionState,
	}
	return info
}

func lookupConnectionByFD(ctx context.Context, pid int, fd uint32) (gopsnet.ConnectionStat, bool) {
	if pid <= 0 || fd == 0 {
		return gopsnet.ConnectionStat{}, false
	}

	connections, err := gopsnet.ConnectionsPidWithContext(ctx, "all", int32(pid))
	if err != nil {
		return gopsnet.ConnectionStat{}, false
	}
	return selectConnectionByFD(connections, fd)
}

func selectConnectionByFD(connections []gopsnet.ConnectionStat, fd uint32) (gopsnet.ConnectionStat, bool) {
	for _, connection := range connections {
		if connection.Fd == fd {
			return connection, true
		}
	}
	return gopsnet.ConnectionStat{}, false
}

func selectConnection(
	connections []gopsnet.ConnectionStat,
	fd uint32,
	family uint16,
	destAddress string,
	destPort uint32,
) (gopsnet.ConnectionStat, bool) {
	var fallback gopsnet.ConnectionStat
	hasFallback := false

	for _, connection := range connections {
		if !matchesConnectionFamily(connection, family) {
			continue
		}
		if connection.Raddr.IP != destAddress || connection.Raddr.Port != destPort {
			continue
		}
		if fd != 0 && connection.Fd == fd {
			return connection, true
		}
		if !hasFallback {
			fallback = connection
			hasFallback = true
		}
	}

	return fallback, hasFallback
}

func matchesConnectionFamily(connection gopsnet.ConnectionStat, family uint16) bool {
	return family == 0 || connection.Family == uint32(family)
}

func protocolFromSocketType(socketType uint32) string {
	switch socketType {
	case 1:
		return "tcp"
	case 2:
		return "udp"
	case 3:
		return "raw"
	default:
		return ""
	}
}

func protocolFromIPProtocolNumber(protocol uint8) string {
	switch protocol {
	case unix.IPPROTO_TCP:
		return "tcp"
	case unix.IPPROTO_UDP:
		return "udp"
	default:
		return ""
	}
}

func tcpConnectionStateName(state uint32) string {
	switch int(state) {
	case tcpStateEstablished:
		return "ESTABLISHED"
	case tcpStateSynSent:
		return "SYN_SENT"
	case tcpStateSynRecv:
		return "SYN_RECV"
	case tcpStateFinWait1:
		return "FIN_WAIT1"
	case tcpStateFinWait2:
		return "FIN_WAIT2"
	case tcpStateTimeWait:
		return "TIME_WAIT"
	case tcpStateClose:
		return "CLOSED"
	case tcpStateCloseWait:
		return "CLOSE_WAIT"
	case tcpStateLastAck:
		return "LAST_ACK"
	case tcpStateListen:
		return "LISTEN"
	case tcpStateClosing:
		return "CLOSING"
	case tcpStateNewSynRecv:
		return "NEW_SYN_RECV"
	default:
		return ""
	}
}

func mergeData(parts ...map[string]any) map[string]any {
	var merged map[string]any
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		if merged == nil {
			merged = make(map[string]any, len(part))
		}
		for key, value := range part {
			merged[key] = value
		}
	}
	return merged
}

func pickProcessID(pid uint32, tgid uint32) int {
	if tgid != 0 {
		return int(tgid)
	}
	return int(pid)
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func formatNetworkAddress(family uint16, addrLen uint32, addr [networkAddrSize]byte) string {
	switch family {
	case 2:
		return net.IP(addr[:4]).String()
	case 10:
		size := int(addrLen)
		if size <= 0 || size > len(addr) {
			size = len(addr)
		}
		return net.IP(addr[:size]).String()
	default:
		return ""
	}
}

func readCurrentProcessImageAndCommand(pid int, fallback string) (string, string) {
	if pid <= 0 {
		return "", fallback
	}

	image, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		image = ""
	}

	command := readProcessCmdline(pid)
	switch {
	case command != "":
		return image, command
	case image != "":
		return image, image
	default:
		return image, fallback
	}
}

func readParentProcessInfo(pid int) (int, string) {
	if pid <= 0 {
		return 0, ""
	}

	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, ""
	}

	for _, line := range strings.Split(string(raw), "\n") {
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, ""
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil || ppid <= 0 {
			return 0, ""
		}
		parentNameRaw, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", ppid))
		if err != nil {
			return ppid, ""
		}
		return ppid, strings.TrimSpace(string(parentNameRaw))
	}

	return 0, ""
}

func readProcessCmdline(pid int) string {
	if pid <= 0 {
		return ""
	}

	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil || len(raw) == 0 {
		return ""
	}

	parts := strings.FieldsFunc(string(raw), func(r rune) bool {
		return r == 0
	})
	return strings.TrimSpace(strings.Join(parts, " "))
}

func trimCString(raw []byte) string {
	for i, value := range raw {
		if value == 0 {
			return string(raw[:i])
		}
	}
	return string(raw)
}
