package pcaputil

import (
	"encoding/binary"
	"fmt"
)

// binNFS is the M0 session state for NFSv3 over ONC RPC with TCP record
// marking. Port 2049 is never consulted. NFSv4 COMPOUND is out of scope.
type binNFS struct {
	client  int
	pending map[uint32]string
}

const (
	nfsProgram = 100003
	nfsVersion = 3
	nfsRMLast  = 0x80000000
)

func probeNFS(w []byte, limit int) ProbeResult {
	if len(w) < 4 {
		if len(w) > 0 && (w[0] == 0x80 || w[0] == 0) {
			return probeNeed("nfs", "v3", len(w), 24)
		}
		return ProbeResult{Verdict: ProbeReject}
	}
	rm := binary.BigEndian.Uint32(w[:4])
	n := int(rm & 0x7fffffff)
	if n < 20 || n > 1<<20 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if len(w) < 24 {
		return probeNeed("nfs", "v3", len(w), 24)
	}
	if binary.BigEndian.Uint32(w[8:12]) != 0 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if binary.BigEndian.Uint32(w[12:16]) != 2 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if binary.BigEndian.Uint32(w[16:20]) != nfsProgram {
		return ProbeResult{Verdict: ProbeReject}
	}
	if binary.BigEndian.Uint32(w[20:24]) != nfsVersion {
		return ProbeResult{Verdict: ProbeReject}
	}
	_ = limit
	return probeAccept("nfs", "v3", 93)
}

func (f *binFlow) frameNFS(w []byte) (int, *binSpec, error) {
	nstate := f.nfs
	if nstate == nil {
		return 0, nil, sessionContext("NFS session was not observed")
	}
	if err := f.reserveSession(256 + int64(len(nstate.pending))*16); err != nil {
		return 0, nil, err
	}
	at, frags := 0, 0
	for {
		if len(w) < at+4 {
			return 0, nil, nil
		}
		rm := binary.BigEndian.Uint32(w[at : at+4])
		n := int(rm & 0x7fffffff)
		last := rm&nfsRMLast != 0
		if n < 8 {
			return 0, nil, fmt.Errorf("nfs: record mark smaller than RPC header")
		}
		if n > f.a.config.MaxMessageBytes || at+4+n > f.a.config.MaxMessageBytes {
			return f.a.config.MaxMessageBytes + 1, nil, nil
		}
		if at+4+n > len(w) {
			return at + 4 + n, nil, nil
		}
		at += 4 + n
		frags++
		if last {
			break
		}
	}
	_ = frags
	return at, f.a.specs["onc_rpc_tcp/ONCRPCTCP"], nil
}

func nfsAssemble(raw []byte) ([]byte, error) {
	var body []byte
	for at := 0; at < len(raw); {
		if at+4 > len(raw) {
			return nil, fmt.Errorf("nfs: truncated record mark")
		}
		rm := binary.BigEndian.Uint32(raw[at : at+4])
		n := int(rm & 0x7fffffff)
		if at+4+n > len(raw) {
			return nil, fmt.Errorf("nfs: truncated RPC fragment")
		}
		body = append(body, raw[at+4:at+4+n]...)
		at += 4 + n
		if rm&nfsRMLast != 0 {
			if at != len(raw) {
				return nil, fmt.Errorf("nfs: trailing bytes after last record")
			}
			return body, nil
		}
	}
	return nil, fmt.Errorf("nfs: missing last record mark")
}

func (n *binNFS) consume(raw []byte, max int) (map[string]any, error) {
	body, err := nfsAssemble(raw)
	if err != nil {
		return nil, err
	}
	if len(body) < 8 {
		return nil, fmt.Errorf("nfs: truncated RPC header")
	}
	xid := binary.BigEndian.Uint32(body[:4])
	mt := binary.BigEndian.Uint32(body[4:8])
	info := map[string]any{
		"XID":           xid,
		"Context Level": "observed",
		"Program":       uint32(nfsProgram),
		"Version":       uint32(nfsVersion),
	}
	switch mt {
	case 0:
		return n.consumeCall(body, info, max)
	case 1:
		return n.consumeReply(body, info)
	default:
		return nil, fmt.Errorf("nfs: message type must be call or reply")
	}
}

func (n *binNFS) consumeCall(body []byte, info map[string]any, max int) (map[string]any, error) {
	if len(body) < 40 {
		return nil, fmt.Errorf("nfs: truncated CALL")
	}
	if binary.BigEndian.Uint32(body[8:12]) != 2 {
		return nil, protocolError(ErrUnsupportedVersion, "ONC RPC version must be 2")
	}
	prog := binary.BigEndian.Uint32(body[12:16])
	vers := binary.BigEndian.Uint32(body[16:20])
	proc := binary.BigEndian.Uint32(body[20:24])
	if prog != nfsProgram || vers != nfsVersion {
		return nil, protocolError(ErrUnsupportedFeature, "NFS session requires program 100003 version 3")
	}
	name := nfsProcName(proc)
	info["Message Type"] = "CALL"
	info["Procedure"] = proc
	info["Packet Name"] = name
	i := 24
	var err error
	if i, err = nfsSkipAuth(body, i); err != nil {
		return nil, err
	}
	if i, err = nfsSkipAuth(body, i); err != nil {
		return nil, err
	}
	if err := nfsParseCallArgs(proc, body[i:], info); err != nil {
		return nil, err
	}
	if max <= 0 {
		max = 4096
	}
	if len(n.pending) >= max {
		return nil, protocolError(ErrResourceExceeded, "NFS outstanding XIDs exceed budget")
	}
	xid := info["XID"].(uint32)
	n.pending[xid] = name
	info["Outstanding"] = true
	return info, nil
}

func (n *binNFS) consumeReply(body []byte, info map[string]any) (map[string]any, error) {
	if len(body) < 12 {
		return nil, fmt.Errorf("nfs: truncated REPLY")
	}
	info["Message Type"] = "REPLY"
	xid := info["XID"].(uint32)
	if want, ok := n.pending[xid]; ok {
		delete(n.pending, xid)
		info["Matched Request"] = want
		info["Association Status"] = "matched"
		info["Packet Name"] = want
	} else {
		info["Unmatched"] = true
		info["Association Status"] = "missing-request"
		info["Context Level"] = "partial"
		info["Packet Name"] = "REPLY"
	}
	replyStat := binary.BigEndian.Uint32(body[8:12])
	info["Reply Stat"] = replyStat
	if replyStat != 0 {
		info["RPC Error"] = "MSG_DENIED"
		return info, nil
	}
	i, err := nfsSkipAuth(body, 12)
	if err != nil {
		return nil, err
	}
	if i+4 > len(body) {
		return nil, fmt.Errorf("nfs: truncated accept_stat")
	}
	accept := binary.BigEndian.Uint32(body[i : i+4])
	i += 4
	info["Accept Stat"] = accept
	if accept != 0 {
		info["RPC Error"] = nfsAcceptName(accept)
		return info, nil
	}
	if i+4 > len(body) {
		return info, nil
	}
	status := binary.BigEndian.Uint32(body[i : i+4])
	i += 4
	info["NFS Status"] = status
	if status != 0 {
		info["NFS Error"] = nfsStatName(status)
		return info, nil
	}
	name, _ := info["Packet Name"].(string)
	_ = nfsParseReplyOK(name, body[i:], info)
	return info, nil
}

func nfsProcName(proc uint32) string {
	switch proc {
	case 0:
		return "NULL"
	case 1:
		return "GETATTR"
	case 3:
		return "LOOKUP"
	case 6:
		return "READ"
	case 7:
		return "WRITE"
	}
	return fmt.Sprintf("PROC_%d", proc)
}

func nfsAcceptName(st uint32) string {
	switch st {
	case 1:
		return "PROG_UNAVAIL"
	case 2:
		return "PROG_MISMATCH"
	case 3:
		return "PROC_UNAVAIL"
	case 4:
		return "GARBAGE_ARGS"
	case 5:
		return "SYSTEM_ERR"
	}
	return fmt.Sprintf("accept_%d", st)
}

func nfsStatName(st uint32) string {
	switch st {
	case 2:
		return "NFS3ERR_NOENT"
	case 13:
		return "NFS3ERR_ACCES"
	case 70:
		return "NFS3ERR_STALE"
	}
	return fmt.Sprintf("nfsstat_%d", st)
}

func nfsSkipAuth(b []byte, i int) (int, error) {
	if i+8 > len(b) {
		return i, fmt.Errorf("nfs: truncated opaque_auth")
	}
	n := int(binary.BigEndian.Uint32(b[i+4 : i+8]))
	i += 8
	if n < 0 || i+n > len(b) {
		return i, fmt.Errorf("nfs: truncated auth body")
	}
	i += n + nfsXDRPad(n)
	if i > len(b) {
		return i, fmt.Errorf("nfs: truncated auth padding")
	}
	return i, nil
}

func nfsXDRPad(n int) int { return (4 - n%4) % 4 }

func nfsOpaque(b []byte, i int) ([]byte, int, error) {
	if i+4 > len(b) {
		return nil, i, fmt.Errorf("nfs: truncated opaque length")
	}
	n := int(binary.BigEndian.Uint32(b[i : i+4]))
	i += 4
	if n < 0 || i+n > len(b) {
		return nil, i, fmt.Errorf("nfs: truncated opaque")
	}
	s := b[i : i+n]
	i += n + nfsXDRPad(n)
	if i > len(b) {
		return nil, i, fmt.Errorf("nfs: truncated opaque padding")
	}
	return s, i, nil
}

func nfsParseCallArgs(proc uint32, stub []byte, info map[string]any) error {
	switch proc {
	case 1, 6, 7: // GETATTR / READ / WRITE start with fh
		fh, i, err := nfsOpaque(stub, 0)
		if err != nil {
			return err
		}
		info["File Handle Bytes"] = len(fh)
		if proc == 6 || proc == 7 {
			if i+12 > len(stub) {
				return fmt.Errorf("nfs: truncated READ/WRITE args")
			}
			info["Offset"] = binary.BigEndian.Uint64(stub[i : i+8])
			info["Count"] = binary.BigEndian.Uint32(stub[i+8 : i+12])
			i += 12
			if proc == 7 {
				if i+4 > len(stub) {
					return fmt.Errorf("nfs: truncated WRITE stable")
				}
				info["Stable"] = binary.BigEndian.Uint32(stub[i : i+4])
				data, _, err := nfsOpaque(stub, i+4)
				if err != nil {
					return err
				}
				info["Data Bytes"] = len(data)
			}
		}
	case 3: // LOOKUP: diropargs3
		fh, i, err := nfsOpaque(stub, 0)
		if err != nil {
			return err
		}
		info["File Handle Bytes"] = len(fh)
		name, _, err := nfsOpaque(stub, i)
		if err != nil {
			return err
		}
		info["Name"] = string(name)
	}
	return nil
}

func nfsParseReplyOK(name string, stub []byte, info map[string]any) error {
	switch name {
	case "LOOKUP":
		fh, _, err := nfsOpaque(stub, 0)
		if err != nil {
			return err
		}
		info["Object Handle Bytes"] = len(fh)
	case "GETATTR":
		if len(stub) >= 28 {
			info["File Type"] = binary.BigEndian.Uint32(stub[:4])
			info["Size"] = binary.BigEndian.Uint64(stub[20:28])
		}
	case "READ":
		i, err := nfsSkipPostOpAttr(stub, 0)
		if err != nil {
			return err
		}
		if i+12 > len(stub) {
			return fmt.Errorf("nfs: truncated READ res")
		}
		info["Count"] = binary.BigEndian.Uint32(stub[i : i+4])
		info["EOF"] = binary.BigEndian.Uint32(stub[i+4:i+8]) != 0
		data, _, err := nfsOpaque(stub, i+8)
		if err != nil {
			return err
		}
		info["Data Bytes"] = len(data)
	case "WRITE":
		i, err := nfsSkipWccData(stub, 0)
		if err != nil {
			return err
		}
		if i+4 <= len(stub) {
			info["Count"] = binary.BigEndian.Uint32(stub[i : i+4])
		}
	}
	return nil
}

func nfsSkipPostOpAttr(b []byte, i int) (int, error) {
	if i+4 > len(b) {
		return i, fmt.Errorf("nfs: truncated post_op_attr")
	}
	if binary.BigEndian.Uint32(b[i:i+4]) == 0 {
		return i + 4, nil
	}
	if i+4+84 > len(b) {
		return i, fmt.Errorf("nfs: truncated fattr3")
	}
	return i + 4 + 84, nil
}

func nfsSkipWccData(b []byte, i int) (int, error) {
	// pre_op_attr: bool + optional size/mtime/ctime (8+8+8=24)
	if i+4 > len(b) {
		return i, fmt.Errorf("nfs: truncated wcc_data")
	}
	if binary.BigEndian.Uint32(b[i:i+4]) != 0 {
		i += 4 + 24
	} else {
		i += 4
	}
	return nfsSkipPostOpAttr(b, i)
}
