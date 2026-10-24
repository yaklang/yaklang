package pcaputil

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type ftpDataKey struct {
	domain           CaptureDomain
	endpoint, peerIP string
}
type ftpDataBinding struct {
	key                  ftpDataKey
	control, transfer    uint64
	command, file        string
	protected, ambiguous bool
	touched              time.Time
	charge               int64
}

func ftpPassivePort(code int, text string) (int, bool) {
	start, end := strings.IndexByte(text, '('), strings.LastIndexByte(text, ')')
	if start < 0 || end <= start {
		return 0, false
	}
	s := text[start+1 : end]
	if code == 227 {
		parts := strings.Split(s, ",")
		if len(parts) != 6 {
			return 0, false
		}
		var values [6]int
		for i, p := range parts {
			v, e := strconv.Atoi(strings.TrimSpace(p))
			if e != nil || v < 0 || v > 255 {
				return 0, false
			}
			values[i] = v
		}
		p := values[4]*256 + values[5]
		return p, p > 0
	}
	if code == 229 && len(s) > 0 {
		parts := strings.Split(s, s[:1])
		if len(parts) != 5 || parts[0] != "" || parts[1] != "" || parts[2] != "" || parts[4] != "" {
			return 0, false
		}
		p, e := strconv.Atoi(parts[3])
		return p, e == nil && p > 0 && p <= 65535
	}
	return 0, false
}

func (a *binParser) expireFTPData(now time.Time) {
	if now.After(a.ftpClock) {
		a.ftpClock = now
	}
	for k, b := range a.ftpEndpoints {
		if a.ftpClock.Sub(b.touched) > 2*time.Minute {
			a.buffered.Add(-b.charge)
			delete(a.ftpEndpoints, k)
		}
	}
}

// A passive port alone is insufficient. Retain only a bounded observed control
// exchange; bind a data flow once, after an actual transfer command, on the same
// peer addresses and capture domain. No process-global port classifier exists.
func (f *binFlow) observeFTPDataControl(dir int, e *ProtocolEvent) {
	if f.transportFlow == nil || f.ftp == nil || e.Session == nil {
		return
	}
	s := f.ftp
	name, _ := e.Session["Packet Name"].(string)
	role, _ := e.Session["Role"].(string)
	if role == "command" {
		s.client, s.clientKnown = dir, true
		if name == "PROT" {
			s.pendingProtection = strings.TrimSpace(string(e.Raw[len("PROT"):]))
		}
	}
	if !s.clientKnown {
		return
	}
	code, _ := e.Session["Reply Code"].(int)
	replyTo, _ := e.Session["In Reply To"].(string)
	if replyTo == "PROT" && code >= 200 {
		if code == 200 {
			s.dataProtected = strings.EqualFold(s.pendingProtection, "P")
		}
		s.pendingProtection = ""
	}
	a := f.a
	a.ftpMu.Lock()
	defer a.ftpMu.Unlock()
	a.expireFTPData(e.Timestamp)
	if role == "reply" && ((replyTo == "PASV" && code == 227) || (replyTo == "EPSV" && code == 229)) {
		p, ok := ftpPassivePort(code, string(e.Raw))
		if !ok {
			return
		}
		server, _, err := net.SplitHostPort(f.endpoints[dir])
		if err != nil {
			return
		}
		client, _, err := net.SplitHostPort(f.endpoints[1-dir])
		if err != nil {
			return
		}
		key := ftpDataKey{f.domain, net.JoinHostPort(server, strconv.Itoa(p)), client}
		for k, b := range a.ftpEndpoints {
			if b.control == f.id && !b.ambiguous {
				a.buffered.Add(-b.charge)
				delete(a.ftpEndpoints, k)
			}
		}
		if a.ftpEndpoints == nil {
			a.ftpEndpoints = map[ftpDataKey]ftpDataBinding{}
		}
		if old, exists := a.ftpEndpoints[key]; exists && (old.control != f.id || old.ambiguous) {
			old.ambiguous = true
			old.touched = a.ftpClock
			a.ftpEndpoints[key] = old
			return
		}
		if len(a.ftpEndpoints) >= min(256, a.budget.MaxCollectionElements) {
			return
		}
		charge := int64(512 + len(key.endpoint) + len(key.peerIP))
		if !a.reserveEvidence(charge) {
			return
		}
		a.ftpEndpoints[key] = ftpDataBinding{key: key, control: f.id, protected: s.dataProtected, touched: a.ftpClock, charge: charge}
		e.Session["Observed Passive Data Endpoint"] = key.endpoint
	}
	if role == "command" && (name == "RETR" || name == "STOR" || name == "APPE" || name == "LIST" || name == "NLST" || name == "MLSD") {
		if e.ID == 0 {
			e.ID = a.ids.Add(1)
		}
		for key, b := range a.ftpEndpoints {
			if b.control == f.id && !b.ambiguous {
				b.command, b.transfer = name, e.ID
				file := strings.TrimSpace(string(e.Raw[len(name):]))
				delta := int64(len(file) - len(b.file))
				if delta > 0 && !a.reserveEvidence(delta) {
					a.buffered.Add(-b.charge)
					delete(a.ftpEndpoints, key)
					continue
				}
				if delta < 0 {
					a.buffered.Add(delta)
				}
				b.charge += delta
				b.file = file
				b.protected = s.dataProtected
				b.touched = a.ftpClock
				a.ftpEndpoints[key] = b
				e.Session["Data Transfer ID"] = e.ID
			}
		}
	}
}

func (a *binParser) matchFTPData(f *binFlow, ts time.Time) *ftpDataBinding {
	a.ftpMu.Lock()
	defer a.ftpMu.Unlock()
	a.expireFTPData(ts)
	for key, b := range a.ftpEndpoints {
		if key.domain != f.domain || b.transfer == 0 || b.ambiguous {
			continue
		}
		for d := 0; d < 2; d++ {
			peer, _, err := net.SplitHostPort(f.endpoints[1-d])
			if err == nil && f.endpoints[d] == key.endpoint && peer == key.peerIP {
				a.buffered.Add(-b.charge)
				delete(a.ftpEndpoints, key)
				if f.reserveSession(b.charge) != nil {
					return nil
				}
				copy := b
				return &copy
			}
		}
	}
	return nil
}

func (a *binParser) forgetFTPControl(id uint64) {
	a.ftpMu.Lock()
	defer a.ftpMu.Unlock()
	for k, b := range a.ftpEndpoints {
		if b.control == id && !b.ambiguous {
			a.buffered.Add(-b.charge)
			delete(a.ftpEndpoints, k)
		}
	}
}
func (a *binParser) closeFTPEndpoints() {
	a.ftpMu.Lock()
	defer a.ftpMu.Unlock()
	for k, b := range a.ftpEndpoints {
		a.buffered.Add(-b.charge)
		delete(a.ftpEndpoints, k)
	}
}

func (f *binFlow) emitFTPData(dir int, data []byte, ts time.Time) {
	// A TCP arrival can exceed the configured message budget. Data transfer
	// chunks are observations, not message boundaries, so split them without
	// copying or retaining the full transfer in session state.
	limit := min(f.a.config.MaxMessageBytes, f.a.budget.MaxFrameBytes)
	for len(data) > 0 {
		n := min(len(data), limit)
		f.emitFTPDataChunk(dir, data[:n], ts)
		data = data[n:]
	}
}

func (f *binFlow) emitFTPDataChunk(dir int, data []byte, ts time.Time) {
	if len(data) == 0 {
		return
	}
	d := &f.directions[dir]
	d.ts = ts
	e := f.event(dir, data, "decoded", "FTP data stream bytes")
	b := f.ftpData
	e.Profile, e.Admission, e.Completeness = "ftp-observed-passive-transfer", "observed-control-command-and-data-endpoint", "stream-segment"
	e.TransactionID = b.transfer
	e.Session = map[string]any{"Packet Name": "DATA", "Transfer Command": b.command, "File Name": b.file, "Control Flow ID": b.control, "Data Transfer ID": b.transfer, "Stream Offset": d.offset, "Payload Bytes": len(data), "Transfer Completion": "not established by this chunk", "Data Protection": b.protected, "Payload Decrypted": f.byteSource == "decrypted", "Association": "control command + negotiated passive endpoint + peer addresses + capture domain"}
	e.semanticFields = cloneSession(e.Session)
	e.Summary = fmt.Sprintf("FTP DATA %s %s · offset %d · %d bytes", b.command, b.file, d.offset, len(data))
	d.offset += uint64(len(data))
	d.pruneContributions(f.a)
	f.a.messages.Add(1)
	f.a.messageBytes.Add(uint64(len(data)))
	f.a.decoded.Add(1)
	f.a.emit(e)
}
