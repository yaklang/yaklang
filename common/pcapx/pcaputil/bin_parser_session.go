package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sort"
)

const binH2Preface = "PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"

func (f *binFlow) detectDirection(dir int, w []byte) {
	// Explicit user bindings retain precedence.
	f.detect(w)
	if f.binding != nil || f.protocol != "" {
		return
	}
	p := probeWire(w, f.a.config.ProbeBytes)
	if p.Verdict != ProbeAccept {
		return
	}
	if f.captureTCP {
		switch p.Protocol {
		case "radius", "dhcp", "ntp", "coap":
			// These profiles describe UDP datagrams, not their distinct TCP
			// variants. Explicit ProtocolSession callers supply their own
			// message framing; captured UDP is handled by datagramFields.
			return
		}
	}
	switch p.Protocol {
	case "vnc":
		f.protocol, f.rfb = "vnc", &binRFB{server: dir, phase: "server-version"}
	case "diameter":
		f.protocol, f.diameter = "diameter", &binDiameter{}
	case "iec104":
		f.protocol, f.iec104 = "iec104", &binIEC104{}
	case "s7comm":
		f.protocol, f.s7 = "s7comm", &binS7{}
	case "opcua":
		f.protocol, f.opcua = "opcua", &binOPCUA{}
	case "rtsp":
		f.protocol, f.rtsp = "rtsp", &binRTSP{}
	case "stun":
		f.protocol, f.stun = "stun", &binSTUN{}
	case "http2":
		f.protocol, f.h2 = "http2", newBinHTTP2(dir)
	case "mysql":
		f.protocol, f.mysql = "mysql", &binMySQL{server: dir, phase: "greeting"}
	case "postgresql":
		f.protocol, f.pg = "postgresql", &binPostgres{frontend: -1}
	case "ldap":
		f.protocol, f.ldap = "ldap", &binLDAP{pending: map[uint64]ldapRequest{}}
	case "redis":
		f.protocol, f.redis = "redis", &binRedis{}
	case "websocket":
		client := dir
		if w[1]&128 == 0 {
			client = 1 - dir
		}
		f.protocol, f.ws = "websocket", &binWebSocket{client: client, phase: wsPhaseFromProbe(w)}
	case "mqtt":
		f.protocol, f.mqtt = "mqtt", &binMQTT{client: dir, level: 5, pending: [2]map[uint16]string{{}, {}}, aliases: [2]map[uint16]string{{}, {}}}
	case "mongodb":
		f.protocol, f.mongo = "mongodb", &binMongo{pending: map[uint32]string{}}
	case "kafka":
		f.protocol, f.kafka = "kafka", &binKafka{client: -1, pending: map[int32]int16{}}
	case "tds":
		f.protocol, f.tds = "tds", &binTDS{client: -1, encrypt: tdsEncryptUnknown}
	case "amqp":
		f.protocol, f.amqp = "amqp", &binAMQP{client: -1, pending: map[uint16][]string{}, delivers: map[uint64]uint16{}, chans: map[uint16]*amqpChan{}}
	case "smb2":
		f.protocol, f.smb2 = "smb2", &binSMB2{client: dir, pending: map[uint64]string{}}
	case "dcerpc":
		f.protocol, f.dcerpc = "dcerpc", &binDCERPC{client: dir, ctx: map[uint16]string{}, uuid: map[uint16][]byte{}, pending: map[uint32]string{}, frags: map[uint32][]byte{}}
	case "ssh":
		f.protocol, f.ssh = "ssh", &binSSH{client: dir}
	case "nfs":
		f.protocol, f.nfs = "nfs", &binNFS{client: dir, pending: map[uint32]string{}}
	case "snmp":
		f.protocol, f.snmp = "snmp", &binSNMP{pending: map[int64]string{}}
	case "rdp":
		f.protocol, f.rdp = "rdp", &binRDP{client: dir}
	case "dot":
		f.protocol, f.dot = "dot", &binDoT{pending: map[uint16]string{}}
	case "sip":
		f.protocol, f.sip = "sip", &binSIP{pending: map[sipTxnKey]string{}, seen: map[sipTxnKey]int{}}
	case "rtp":
		f.protocol, f.rtp = "rtp", &binRTP{sources: map[uint32]*rtpSource{}}
	case "quic":
		f.protocol, f.quic = "quic", &binQUIC{streams: map[uint64]*quicStream{}, keys: cloneQUICKeyring(f.a.quicKeys), decryptedSource: f.a.decryptedQUICSource}
	case "smtp":
		client := dir
		if smtpReplyPrefix(w) {
			client = 1 - dir
		}
		f.protocol, f.smtp = "smtp", &binSMTP{client: client, maxPending: f.a.budget.MaxCollectionElements}
	case "imap":
		f.protocol, f.imap = "imap", &binIMAP{pending: map[string]string{}, maxPending: f.a.budget.MaxCollectionElements}
	case "pop3":
		f.protocol, f.pop3 = "pop3", &binPOP3{}
	case "ftp":
		f.protocol, f.ftp = "ftp", &binFTP{}
	case "tns":
		f.protocol, f.tns = "tns", &binTNS{}
	case "radius":
		f.protocol, f.radius = "radius", &binRADIUS{pending: map[uint8]string{}}
	case "dhcp":
		f.protocol, f.dhcp = "dhcp", &binDHCP{pending: map[uint32]string{}}
	case "ntp":
		f.protocol, f.ntp = "ntp", &binNTP{}
	case "coap":
		f.protocol, f.coap = "coap", &binCoAP{maxPending: f.a.budget.MaxCollectionElements}
	case "modbus":
		f.protocol, f.modbus = "modbus", &binModbus{pending: map[uint16]modbusRequest{}, maxPending: f.a.budget.MaxCollectionElements}
	case "dnp3":
		f.protocol, f.dnp3 = "dnp3", &binDNP3{pending: map[uint32]string{}}
	case "c37118":
		f.protocol, f.c37118 = "c37118", &binC37118{}
	case "goose":
		f.protocol, f.goose = "goose", &binGOOSE{}
	}
}

func (f *binFlow) consumeSession(dir int, e *ProtocolEvent, result map[string]any) error {
	var err error
	switch f.protocol {
	case "vnc":
		e.Session, err = f.rfb.consume(dir, e.Raw, f.a.budget.MaxCollectionElements, f.a.budget.MaxMessageBytes)
	case "diameter":
		e.Session, err = f.diameter.consume(dir, e.Raw, f.a.budget.MaxCollectionElements, f.a.budget.MaxRecursionDepth)
	case "iec104":
		e.Session, err = f.iec104.consume(dir, e.Raw, f.a.budget.MaxCollectionElements)
	case "s7comm":
		e.Session, err = f.s7.consume(dir, e.Raw, f.a.budget.MaxCollectionElements, f.a.budget.MaxMessageBytes)
	case "opcua":
		e.Session, err = f.opcua.consume(dir, e.Raw, f.a.budget.MaxCollectionElements, f.a.budget.MaxMessageBytes)
	case "rtsp":
		e.Session, err = f.rtsp.consume(dir, e.Raw, e.Timestamp, f.a.budget.MaxCollectionElements)
	case "stun":
		e.Session, err = f.stun.consume(dir, e.Timestamp, e.Raw, f.a.budget.MaxCollectionElements, true)
		if e.Session != nil && e.Session["TURN"] == true {
			e.Protocol = "turn"
		}
	case "http":
		if e.ipp {
			e.Session, err = f.consumeIPPHTTP(dir, e.Raw)
			e.Protocol = "ipp"
		} else {
			e.Session, err = f.consumeDoHHTTP1(e.Raw)
		}
	case "http2":
		var previous *binH2Stream
		if len(e.Raw) >= 9 && !bytes.HasPrefix(e.Raw, []byte(binH2Preface)) {
			previous = f.h2.streams[binary.BigEndian.Uint32(e.Raw[5:9])&0x7fffffff]
		}
		e.Session, err = f.h2.consume(dir, e.Raw)
		if err == nil {
			err = f.consumeGRPC(dir, e, previous)
		}
		if err == nil {
			err = f.consumeDoHH2(dir, e, previous)
		}
	case "mysql":
		e.Session, err = f.mysql.consume(dir, e.Raw, e.Entry, result)
		if err == nil && f.mysql.phase == "tls" {
			f.protocol = "tls"
		}
	case "postgresql":
		e.Session, err = f.pg.consume(dir, e.Raw, e.Entry)
		if err == nil && e.Session["Encrypted"] == true {
			f.protocol, f.pg = "tls", nil
			e.Session["Protocol Transition"] = "postgresql->tls"
		}
	case "ldap":
		e.Session, err = f.ldap.consume(dir, e.Raw, e.Entry, f.a.budget.MaxCollectionElements)
		if err == nil && e.Session["StartTLS"] == true && e.Session["Message Name"] == "ExtendedResponse" {
			f.protocol, f.ldap = "tls", nil
			e.Session["Protocol Transition"] = "ldap->tls"
		}
	case "redis":
		e.Session, err = f.redis.consume(dir, e.Raw)
	case "websocket":
		if e.Entry == "WebSocket" {
			e.Session, err = f.ws.consume(dir, e.Raw)
		} else {
			e.Session = map[string]any{
				"Protocol Transition": "http->websocket",
				"Reason":              "101 Switching Protocols",
			}
		}
	case "mqtt":
		e.Session, err = f.mqtt.consume(dir, e.Raw, result)
	case "mongodb":
		e.Session, err = f.mongo.consume(e.Raw, result)
	case "kafka":
		e.Session, err = f.kafka.consume(dir, e.Raw, result)
	case "tds":
		e.Session, err = f.tds.consume(dir, e.Raw, result, f.a.budget.MaxCollectionElements)
		if err == nil && e.Session["Encrypted"] == true {
			f.protocol, f.tds = "tls", nil
		}
	case "amqp":
		e.Session, err = f.amqp.consume(dir, e.Raw, f.a.budget.MaxCollectionElements)
	case "smb2":
		e.Session, err = f.smb2.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "dcerpc":
		e.Session, err = f.dcerpc.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "ssh":
		e.Session, err = f.ssh.consume(dir, e.Raw)
	case "nfs":
		e.Session, err = f.nfs.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "snmp":
		e.Session, err = f.snmp.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "rdp":
		e.Session, err = f.rdp.consume(e.Raw)
		if err == nil && e.Session["TLS Expected"] == true {
			f.protocol = "tls"
		}
	case "dot":
		e.Session, err = f.dot.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "sip":
		e.Session, err = f.sip.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "rtp":
		e.Session, err = f.rtp.consume(e.Raw, f.directions[dir].ts, f.a.budget.MaxCollectionElements)
	case "quic":
		e.Session, err = f.quic.consume(dir, e.Raw, f.a.budget.MaxCollectionElements)
	case "smtp":
		e.Session, err = f.smtp.consume(dir, e.Raw)
	case "imap":
		e.Session, err = f.imap.consume(dir, e.Raw)
	case "pop3":
		e.Session, err = f.pop3.consume(e.Raw)
	case "ftp":
		e.Session, err = f.ftp.consume(e.Raw)
	case "tns":
		e.Session, err = f.tns.consume(e.Raw)
	case "radius":
		e.Session, err = f.radius.consume(e.Raw, f.a.budget.MaxCollectionElements)
	case "dhcp":
		e.Session, err = f.dhcp.consume(e.Raw)
	case "ntp":
		e.Session, err = f.ntp.consume(e.Raw)
	case "coap":
		e.Session, err = f.coap.consume(dir, e.Raw)
	case "modbus":
		e.Session, err = f.modbus.consume(dir, e.Raw)
	case "dnp3":
		e.Session, err = f.dnp3.consume(e.Raw)
	case "c37118":
		e.Session, err = f.c37118.consume(e.Raw)
	case "goose":
		e.Session, err = f.goose.consume(e.Raw)
	}
	if e.Session != nil {
		switch e.Protocol {
		case "http2":
			e.Summary = fmt.Sprintf("HTTP/2 stream %v frame %v", e.Session["Stream ID"], e.Session["Frame Type"])
			if kind, ok := e.Session["Header Kind"].(string); ok {
				e.Summary = fmt.Sprintf("HTTP/2 stream %v %s", e.Session["Stream ID"], kind)
			}
			if e.Session["GRPC"] == true {
				e.Protocol = "grpc"
				e.Summary = fmt.Sprintf("gRPC stream %v", e.Session["Stream ID"])
			}
		case "mysql":
			e.Summary = fmt.Sprintf("MySQL transaction %v %v", e.Session["Transaction ID"], e.Session["Phase"])
		case "postgresql":
			e.Summary = fmt.Sprintf("PostgreSQL %v", e.Session["Message Name"])
		case "ldap":
			e.Summary = fmt.Sprintf("LDAP %v id %v", e.Session["Message Name"], e.Session["Message ID"])
		case "redis":
			e.Summary = fmt.Sprintf("Redis %v", e.Session["RESP Type"])
		case "websocket":
			e.Summary = fmt.Sprintf("WebSocket %v", e.Session["Opcode Name"])
		case "mqtt":
			e.Summary = fmt.Sprintf("MQTT %v", e.Session["Packet Name"])
		case "mongodb":
			e.Summary = fmt.Sprintf("MongoDB %v id %v", e.Session["Opcode Name"], e.Session["Request ID"])
		case "kafka":
			e.Summary = fmt.Sprintf("Kafka %v corr %v", e.Session["API Name"], e.Session["Correlation ID"])
		case "tds":
			e.Summary = fmt.Sprintf("TDS %v", e.Session["Packet Name"])
		case "amqp":
			e.Summary = fmt.Sprintf("AMQP ch %v %v", e.Session["Channel"], e.Session["Packet Name"])
		case "smb2":
			e.Summary = fmt.Sprintf("SMB2 %v mid %v", e.Session["Packet Name"], e.Session["Message ID"])
		case "dcerpc":
			e.Summary = fmt.Sprintf("DCE/RPC %v call %v", e.Session["Packet Name"], e.Session["Call ID"])
		case "ssh":
			e.Summary = fmt.Sprintf("SSH %v", e.Session["Packet Name"])
		case "nfs":
			e.Summary = fmt.Sprintf("NFS %v xid %v", e.Session["Packet Name"], e.Session["XID"])
		case "snmp":
			e.Summary = fmt.Sprintf("SNMPv3 %v id %v", e.Session["Packet Name"], e.Session["Request ID"])
		case "rdp":
			e.Summary = fmt.Sprintf("RDP %v", e.Session["Packet Name"])
		case "dot":
			e.Summary = fmt.Sprintf("DoT %v id %v %v", e.Session["Packet Name"], e.Session["Transaction ID"], e.Session["QNAME"])
		case "sip":
			e.Summary = fmt.Sprintf("SIP %v %v", e.Session["Packet Name"], e.Session["Call-ID"])
		case "rtp":
			e.Summary = fmt.Sprintf("RTP %v ssrc %v", e.Session["Packet Name"], e.Session["SSRC"])
		case "quic":
			e.Summary = fmt.Sprintf("QUIC %v", e.Session["Packet Name"])
			if e.Session["HTTP3"] == true {
				e.Protocol = "http3"
				e.Summary = fmt.Sprintf("HTTP/3 stream %v %v", e.Session["HTTP3 Stream ID"], e.Session["HTTP3 Stream Kind"])
			}
			if e.Session["DoQ"] == true {
				e.Protocol = "doq"
				e.Summary = fmt.Sprintf("DoQ stream %v %v %v", e.Session["DoQ Stream ID"], e.Session["Packet Name"], e.Session["QNAME"])
			}
		case "smtp":
			e.Summary = fmt.Sprintf("SMTP %v", e.Session["Packet Name"])
		case "imap":
			e.Summary = fmt.Sprintf("IMAP %v %v", e.Session["Packet Name"], e.Session["Tag"])
		case "pop3":
			e.Summary = fmt.Sprintf("POP3 %v", e.Session["Packet Name"])
		case "ftp":
			e.Summary = fmt.Sprintf("FTP %v", e.Session["Packet Name"])
		case "tns":
			e.Summary = fmt.Sprintf("TNS %v", e.Session["Packet Name"])
		case "radius":
			e.Summary = fmt.Sprintf("RADIUS %v id %v", e.Session["Packet Name"], e.Session["Identifier"])
		case "dhcp":
			e.Summary = fmt.Sprintf("DHCP %v xid %v", e.Session["Packet Name"], e.Session["Xid"])
		case "ntp":
			e.Summary = fmt.Sprintf("NTP %v stratum %v", e.Session["Packet Name"], e.Session["Stratum"])
		case "coap":
			e.Summary = fmt.Sprintf("CoAP %v %v mid %v", e.Session["Packet Name"], e.Session["Code"], e.Session["Message ID"])
		case "modbus":
			e.Summary = fmt.Sprintf("Modbus %v tid %v", e.Session["Packet Name"], e.Session["Transaction ID"])
		case "iec104":
			e.Summary = fmt.Sprintf("IEC104 %v", e.Session["Packet Name"])
		case "dnp3":
			e.Summary = fmt.Sprintf("DNP3 %v %v->%v", e.Session["Packet Name"], e.Session["Source"], e.Session["Destination"])
		case "c37118":
			e.Summary = fmt.Sprintf("C37.118 %v id %v", e.Session["Packet Name"], e.Session["ID Code"])
		case "goose":
			e.Summary = fmt.Sprintf("GOOSE st %v sq %v", e.Session["State Number"], e.Session["Sequence Number"])
		}
		if e.Session["DoH"] == true {
			e.Protocol = "doh"
			e.Summary = dohSummary(e.Session)
		}
	}
	return err
}

func (f *binFlow) invalidateSession(dir int) {
	d := &f.directions[dir]
	if d.stopped {
		return
	}
	if len(d.buffer) > 0 {
		f.stop(dir, d.buffer, "context-required", "peer failure invalidated connection state")
	} else {
		d.stopped = true
		f.release(d)
	}
}

func (f *binFlow) closeSession() {
	f.stun, f.tftp, f.rtsp, f.ipp = nil, nil, nil, nil
	f.diameter, f.iec104, f.s7, f.opcua = nil, nil, nil, nil
	f.rfb = nil
	f.dnp3, f.c37118, f.goose = nil, nil, nil
	f.h2, f.mysql, f.pg, f.ws, f.ldap, f.redis, f.mqtt, f.mongo, f.kafka, f.tds, f.amqp, f.smb2, f.dcerpc, f.ssh, f.nfs, f.snmp, f.rdp, f.dot, f.doh, f.sip, f.rtp, f.quic, f.smtp, f.imap, f.pop3, f.ftp, f.tns, f.radius, f.dhcp, f.ntp, f.coap, f.modbus = nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil
	f.a.buffered.Add(-f.sessionBytes)
	f.sessionBytes = 0
}

func (f *binFlow) finishSession(reason TrafficFlowCloseReason) {
	emit := func(dir int, session map[string]any, why string) {
		e := f.event(dir, nil, "incomplete", string(reason)+": "+why)
		e.Session = session
		f.a.incomplete.Add(1)
		f.a.emit(e)
	}
	if h := f.h2; h != nil && f.protocol == "http2" {
		ids := make([]int, 0, len(h.streams))
		for id := range h.streams {
			ids = append(ids, int(id))
		}
		sort.Ints(ids)
		for _, id := range ids {
			emit(h.client, map[string]any{"Stream ID": uint32(id)}, "HTTP/2 exchange did not reach END_STREAM in both directions")
		}
	}
	if m := f.mysql; m != nil && f.protocol == "mysql" && m.phase != "command" && m.phase != "closed" {
		emit(m.server, map[string]any{"Phase": m.phase, "Transaction ID": m.transaction}, "MySQL exchange ended before its expected response")
	}
	if ws := f.ws; ws != nil {
		for dir, op := range ws.opcode {
			if op != 0 {
				emit(dir, map[string]any{"Opcode": uint64(op)}, "WebSocket message ended before its final continuation")
			}
		}
	}
	if p := f.pg; p != nil && p.pending > 0 {
		emit(max(p.frontend, 0), map[string]any{"Outstanding": p.pending}, "PostgreSQL exchange ended with unmatched extended-query messages")
	}
	if l := f.ldap; l != nil && len(l.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(l.pending)}, "LDAP exchange ended with unmatched MessageIDs")
	}
	if q := f.mqtt; q != nil {
		n := len(q.pending[0]) + len(q.pending[1])
		if n > 0 {
			emit(q.client, map[string]any{"Outstanding": n}, "MQTT exchange ended with unmatched packet identifiers")
		}
	}
	if g := f.mongo; g != nil && len(g.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(g.pending)}, "MongoDB exchange ended with unmatched request IDs")
	}
	if k := f.kafka; k != nil && len(k.pending) > 0 {
		emit(max(k.client, 0), map[string]any{"Outstanding": len(k.pending)}, "Kafka exchange ended with unmatched correlation IDs")
	}
	if d := f.tds; d != nil && len(d.pending) > 0 {
		emit(max(d.client, 0), map[string]any{"Outstanding": len(d.pending)}, "TDS exchange ended with unmatched requests")
	}
	if q := f.amqp; q != nil {
		n := len(q.delivers)
		for _, list := range q.pending {
			n += len(list)
		}
		if n > 0 {
			emit(max(q.client, 0), map[string]any{"Outstanding": n}, "AMQP exchange ended with unmatched methods or deliveries")
		}
	}
	if s := f.smb2; s != nil && len(s.pending) > 0 {
		emit(max(s.client, 0), map[string]any{"Outstanding": len(s.pending)}, "SMB2 exchange ended with unmatched MessageIds")
	}
	if d := f.dcerpc; d != nil && (len(d.pending) > 0 || len(d.frags) > 0) {
		emit(max(d.client, 0), map[string]any{"Outstanding": len(d.pending) + len(d.frags)}, "DCE/RPC exchange ended with unmatched calls or fragments")
	}
	if sh := f.ssh; sh != nil && !sh.encrypted {
		if !sh.banner[0] || !sh.banner[1] || !sh.haveKEX[0] || !sh.haveKEX[1] || !sh.newkeys[0] || !sh.newkeys[1] {
			emit(max(sh.client, 0), map[string]any{"Identification": sh.ident}, "SSH handshake ended before NEWKEYS")
		}
	}
	if n := f.nfs; n != nil && len(n.pending) > 0 {
		emit(max(n.client, 0), map[string]any{"Outstanding": len(n.pending)}, "NFS exchange ended with unmatched XIDs")
	}
	if q := f.snmp; q != nil && len(q.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(q.pending)}, "SNMPv3 exchange ended with unmatched request-ids")
	}
	if r := f.rdp; r != nil && r.sawCR && !r.sawCC {
		emit(max(r.client, 0), map[string]any{"Cookie": r.cookie}, "RDP exchange ended before X.224 Connection Confirm")
	}
	if d := f.dot; d != nil && len(d.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(d.pending)}, "DoT exchange ended with unmatched DNS transaction IDs")
	}
	if h := f.doh; h != nil && len(h.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(h.pending)}, "DoH exchange ended with unmatched DNS transaction IDs")
	}
	if p := f.sip; p != nil && len(p.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(p.pending)}, "SIP exchange ended with unmatched transactions")
	}
	if s := f.smtp; s != nil && s.pendingCount() > 0 && !s.encrypted {
		emit(0, map[string]any{"Outstanding": s.pending[s.pendingHead:]}, "SMTP exchange ended with unmatched command")
	}
	if im := f.imap; im != nil && len(im.pending) > 0 && !im.encrypted {
		emit(0, map[string]any{"Outstanding": len(im.pending)}, "IMAP exchange ended with unmatched tags")
	}
	if p := f.pop3; p != nil && p.pending != "" && !p.encrypted {
		emit(0, map[string]any{"Outstanding": p.pending}, "POP3 exchange ended with unmatched command")
	}
	if t := f.ftp; t != nil && t.pending != "" && !t.encrypted {
		emit(0, map[string]any{"Outstanding": t.pending}, "FTP exchange ended with unmatched command")
	}
	if n := f.tns; n != nil && n.pending > 0 {
		emit(0, map[string]any{"Outstanding": n.pending}, "TNS exchange ended with unmatched Connect")
	}
	if r := f.radius; r != nil && len(r.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(r.pending)}, "RADIUS exchange ended with unmatched Identifiers")
	}
	if d := f.dhcp; d != nil && len(d.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(d.pending)}, "DHCP exchange ended with unmatched xids")
	}
	if c := f.coap; c != nil && len(c.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(c.pending)}, "CoAP exchange ended with unmatched Message IDs")
	}
	if m := f.modbus; m != nil && len(m.pending) > 0 {
		emit(0, map[string]any{"Outstanding": len(m.pending)}, "Modbus exchange ended with unmatched Transaction IDs")
	}
}

// Conservative capacity accounting includes dictionaries and map/queue slots.
// Reservations are released on close; repeated streams reuse the high-water budget.
func (f *binFlow) reserveSession(target int64) error {
	if target <= f.sessionBytes {
		return nil
	}
	delta := target - f.sessionBytes
	for {
		current := f.a.buffered.Load()
		if current+delta > int64(f.a.config.MaxBufferedBytes) {
			return fmt.Errorf("%w: %w", errBinContext, protocolError(ErrResourceExceeded, "connection state exceeds capture memory budget"))
		}
		if f.a.buffered.CompareAndSwap(current, current+delta) {
			f.sessionBytes = target
			for peak := f.a.peak.Load(); current+delta > peak; peak = f.a.peak.Load() {
				if f.a.peak.CompareAndSwap(peak, current+delta) {
					break
				}
			}
			return nil
		}
	}
}

func sessionCollectionLimit(limit int) int {
	if limit <= 0 {
		return DefaultParserBudget().MaxCollectionElements
	}
	return limit
}

func sessionContext(why string) error { return fmt.Errorf("%w: %s", errBinContext, why) }

// Context snapshots contain only these value types. No live decoder, pooled
// connection or mutable dictionary is retained by an event or inspector.
func cloneSessionValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, v := range x {
			out[k] = cloneSessionValue(v)
		}
		return out
	case []map[string]any:
		out := make([]map[string]any, len(x))
		for i, v := range x {
			out[i] = cloneSessionValue(v).(map[string]any)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, v := range x {
			out[i] = cloneSessionValue(v)
		}
		return out
	case []int32:
		return append([]int32(nil), x...)
	case []string:
		return append([]string(nil), x...)
	case []byte:
		return bytes.Clone(x)
	default:
		return v
	}
}

func cloneSession(v map[string]any) map[string]any {
	if v == nil {
		return nil
	}
	return cloneSessionValue(v).(map[string]any)
}

func sessionSnapshotBytes(v any) int { return sessionSnapshotSize(v, true) }

// History budgets count logical owned byte lengths, not spare input capacity.
// Both forms remain approximate budgets rather than exact Go heap sizes.
func sessionSnapshotSize(v any, byteCapacity bool) int {
	switch x := v.(type) {
	case map[string]any:
		if x == nil {
			return 0
		}
		n := 64
		for k, v := range x {
			n += len(k) + 32 + sessionSnapshotSize(v, byteCapacity)
		}
		return n
	case []map[string]any:
		n := 24 + len(x)*8
		for _, v := range x {
			n += sessionSnapshotSize(v, byteCapacity)
		}
		return n
	case []any:
		n := 24 + len(x)*16
		for _, v := range x {
			n += sessionSnapshotSize(v, byteCapacity)
		}
		return n
	case []int32:
		return 24 + len(x)*4
	case []string:
		n := 24 + len(x)*16
		for _, v := range x {
			n += len(v)
		}
		return n
	case string:
		return len(x) + 16
	case []byte:
		if byteCapacity {
			return cap(x) + 24
		}
		return len(x) + 24
	case nil:
		return 0
	default:
		return 16
	}
}

func protocolHistoryBytes(e *ProtocolEvent) int {
	return len(e.Raw) + sessionSnapshotSize(e.Session, false)
}
