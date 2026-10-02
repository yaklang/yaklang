package main

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

func kv(pairs ...string) string {
	if len(pairs)%2 != 0 {
		panic("odd kv")
	}
	var b strings.Builder
	for i := 0; i < len(pairs); i += 2 {
		fmt.Fprintf(&b, "%s=%s\n", pairs[i], pairs[i+1])
	}
	return b.String()
}

func pcapOf(fn func(*lab)) []byte {
	l := newLab()
	fn(l)
	return l.pcapng()
}

func oneTCP(frames []Frame, port uint16) (conn, error) {
	cs, err := tcpConns(frames, port)
	if err != nil {
		return conn{}, err
	}
	if len(cs) != 1 {
		return conn{}, fmt.Errorf("port %d: got %d tcp conns", port, len(cs))
	}
	return cs[0], nil
}

func buildSOCKS(l *lab) {
	c := l.tcp(41080, 1080)
	c.client([]byte{0x05, 0x01, 0x00})
	c.server([]byte{0x05, 0x00})
	c.client([]byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1, 0x00, 0x50})
	c.server([]byte{0x05, 0x00, 0x00, 0x01, 127, 0, 0, 1, 0x04, 0x38})
	c.client(httpRaw("GET /health HTTP/1.1", []hdr{{"Host", "lab.invalid"}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "text/plain"}}, []byte("ok")))
	c.close()
}

func parseSOCKS(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 1080)
	if err != nil {
		return "", err
	}
	c, s := &rd{b: cn.c2s}, &rd{b: cn.s2c}
	ver, err := c.u8()
	if err != nil || ver != 5 {
		return "", fmt.Errorf("socks version")
	}
	n, err := c.u8()
	if err != nil || n != 1 {
		return "", fmt.Errorf("socks nmethods")
	}
	methods, err := c.bytes(int(n))
	if err != nil || methods[0] != 0 {
		return "", fmt.Errorf("socks method offer")
	}
	if sv, err := s.u8(); err != nil || sv != 5 {
		return "", fmt.Errorf("socks reply version")
	}
	method, err := s.u8()
	if err != nil || method != 0 {
		return "", fmt.Errorf("socks chosen method")
	}
	req, err := c.bytes(4)
	if err != nil || req[0] != 5 || req[1] != 1 || req[2] != 0 || req[3] != 1 {
		return "", fmt.Errorf("socks connect header")
	}
	addr, err := c.bytes(4)
	if err != nil {
		return "", err
	}
	port, err := c.be16()
	if err != nil {
		return "", err
	}
	rep, err := s.bytes(4)
	if err != nil || rep[0] != 5 || rep[1] != 0 || rep[3] != 1 {
		return "", fmt.Errorf("socks connect reply")
	}
	if _, err = s.bytes(4); err != nil {
		return "", err
	}
	if _, err = s.be16(); err != nil {
		return "", err
	}
	reqs, err := readAllHTTP(c.b[c.i:])
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(s.b[s.i:])
	if err != nil {
		return "", err
	}
	if len(reqs) != 1 || len(reps) != 1 || reqs[0].Start != "GET /health HTTP/1.1" {
		return "", fmt.Errorf("socks relay http")
	}
	parts := strings.Split(reps[0].Start, " ")
	if len(parts) < 2 || string(reps[0].Body) == "" {
		return "", fmt.Errorf("socks relay status")
	}
	return kv(
		"protocol", "socks5",
		"version", "5",
		"auth_method", strconv.Itoa(int(method)),
		"command", "CONNECT",
		"dst", fmt.Sprintf("%d.%d.%d.%d:%d", addr[0], addr[1], addr[2], addr[3], port),
		"relay_status", parts[1],
		"relay_body", string(reps[0].Body),
	), nil
}

func buildFinger(l *lab) {
	c := l.tcp(41079, 79)
	c.client([]byte("/W alice\r\n"))
	c.server([]byte("Login: alice          Name: Alice Lab\r\nPlan:\r\nWindows lab finger fixture for yaklang PR 5013.\r\n"))
	c.close()
}

func parseFinger(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 79)
	if err != nil {
		return "", err
	}
	if string(cn.c2s) != "/W alice\r\n" {
		return "", fmt.Errorf("finger query")
	}
	plan := "Windows lab finger fixture for yaklang PR 5013."
	if !strings.Contains(string(cn.s2c), "Login: alice") || !strings.Contains(string(cn.s2c), plan) {
		return "", fmt.Errorf("finger reply")
	}
	return kv("protocol", "finger", "user", "alice", "plan", plan), nil
}

func buildWhois(l *lab) {
	c := l.tcp(41043, 43)
	c.client([]byte("example.invalid\r\n"))
	c.server([]byte("Domain Name: EXAMPLE.INVALID\r\nReferralServer: whois.lab-invalid:43\r\nRegistry Domain ID: LAB-5013-EXAMPLE\r\n"))
	c.close()
}

func parseWhois(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 43)
	if err != nil {
		return "", err
	}
	q := strings.TrimRight(string(cn.c2s), "\r\n")
	fields := map[string]string{}
	for _, ln := range strings.Split(string(cn.s2c), "\n") {
		ln = strings.TrimRight(ln, "\r")
		k, v, ok := strings.Cut(ln, ":")
		if ok {
			fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	if q == "" || fields["ReferralServer"] == "" || fields["Registry Domain ID"] == "" {
		return "", fmt.Errorf("whois fields")
	}
	return kv("protocol", "whois", "query", q, "referral", fields["ReferralServer"], "id", fields["Registry Domain ID"]), nil
}

func buildGopher(l *lab) {
	menu := "1Lab files\t/files\tlab.invalid\t70\r\n0readme\t/readme\tlab.invalid\t70\r\n.\r\n"
	a := l.tcp(40070, 70)
	a.client([]byte("\r\n"))
	a.server([]byte(menu))
	a.close()
	b := l.tcp(40071, 70)
	b.client([]byte("/readme\r\n"))
	b.server([]byte("yaklang lab gopher item\r\n.\r\n"))
	b.close()
}

func parseGopher(frames []Frame) (string, error) {
	cs, err := tcpConns(frames, 70)
	if err != nil {
		return "", err
	}
	if len(cs) != 2 {
		return "", fmt.Errorf("gopher conns %d", len(cs))
	}
	var menu, body string
	for _, cn := range cs {
		req := string(cn.c2s)
		switch req {
		case "\r\n":
			menu = string(cn.s2c)
		case "/readme\r\n":
			body = strings.TrimSuffix(string(cn.s2c), ".\r\n")
			body = strings.TrimSuffix(body, "\r\n")
		default:
			return "", fmt.Errorf("gopher selector %q", req)
		}
	}
	if !strings.Contains(menu, "1Lab files\t/files\t") || body == "" {
		return "", fmt.Errorf("gopher payload")
	}
	return kv("protocol", "gopher", "menu_item", "Lab files", "menu_selector", "/files", "selector", "/readme", "body", body), nil
}

func buildDICT(l *lab) {
	c := l.tcp(42628, 2628)
	var req, resp strings.Builder
	req.WriteString("CLIENT lab-win/1.0\r\n")
	resp.WriteString("220 lab dict <1.0>\r\n")
	req.WriteString("SHOW DB\r\n")
	resp.WriteString("110 1 databases present\r\nlab-words \"Windows lab dictionary\"\r\n.\r\n250 ok\r\n")
	req.WriteString("DEFINE lab-words protocol\r\n")
	resp.WriteString("150 1 definition retrieved\r\n151 \"protocol\" lab-words \"Windows lab dictionary\"\r\nA lab definition used only as a parse fixture.\r\n.\r\n250 ok\r\n")
	req.WriteString("QUIT\r\n")
	resp.WriteString("221 bye\r\n")
	c.client([]byte(req.String()))
	c.server([]byte(resp.String()))
	c.close()
}

func parseDICT(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 2628)
	if err != nil {
		return "", err
	}
	text := string(cn.c2s) + string(cn.s2c)
	if !strings.Contains(text, "DEFINE lab-words protocol\r\n") || !strings.Contains(text, "SHOW DB\r\n") {
		return "", fmt.Errorf("dict commands")
	}
	const def = "A lab definition used only as a parse fixture."
	if !strings.Contains(string(cn.s2c), def) || !strings.Contains(string(cn.s2c), "lab-words ") {
		return "", fmt.Errorf("dict definition")
	}
	return kv("protocol", "dict", "client", "lab-win/1.0", "database", "lab-words", "word", "protocol", "definition", def), nil
}

func bjnp(dev, cmd byte, seq uint32, session uint16, payload string) []byte {
	b := make([]byte, 16+len(payload))
	copy(b[:4], "BJNP")
	b[4] = dev
	b[5] = cmd
	binary.BigEndian.PutUint32(b[6:10], seq)
	binary.BigEndian.PutUint16(b[10:12], session)
	binary.BigEndian.PutUint32(b[12:16], uint32(len(payload)))
	copy(b[16:], payload)
	return b
}

func buildBJNP(l *lab) {
	l.udp(48611, 8611, true, bjnp(0x01, 0x01, 1, 0, ""))
	l.udp(48611, 8611, false, bjnp(0x81, 0x01, 1, 1, "LAB-PRINTER"))
	l.udp(48611, 8611, true, bjnp(0x01, 0x30, 2, 1, ""))
	l.udp(48611, 8611, false, bjnp(0x81, 0x30, 2, 1, "SN:5013"))
	l.udp(48611, 8611, true, bjnp(0x01, 0x10, 3, 1, "job=7"))
	l.udp(48611, 8611, false, bjnp(0x81, 0x10, 3, 1, "accepted"))
	l.udp(48611, 8611, true, bjnp(0x01, 0x20, 4, 1, ""))
	l.udp(48611, 8611, false, bjnp(0x81, 0x20, 4, 1, "idle"))
}

func parseBJNP(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 8611)
	if err != nil {
		return "", err
	}
	type msg struct {
		dev, cmd byte
		payload  string
	}
	parse := func(p []byte) (msg, error) {
		if len(p) < 16 || string(p[:4]) != "BJNP" {
			return msg{}, fmt.Errorf("bjnp magic")
		}
		n := binary.BigEndian.Uint32(p[12:16])
		if int(n) != len(p)-16 {
			return msg{}, fmt.Errorf("bjnp length")
		}
		return msg{dev: p[4], cmd: p[5], payload: string(p[16:])}, nil
	}
	var cmds []msg
	for _, p := range c2s {
		m, err := parse(p)
		if err != nil {
			return "", err
		}
		cmds = append(cmds, m)
	}
	var reps []msg
	for _, p := range s2c {
		m, err := parse(p)
		if err != nil {
			return "", err
		}
		reps = append(reps, m)
	}
	if len(cmds) < 4 || len(reps) < 4 {
		return "", fmt.Errorf("bjnp count")
	}
	id, status, job := "", "", ""
	for _, m := range reps {
		switch m.cmd {
		case 0x30:
			id = m.payload
		case 0x20:
			status = m.payload
		case 0x10:
			job = m.payload
		}
	}
	if cmds[0].cmd != 0x01 || id == "" || status == "" || job == "" {
		return "", fmt.Errorf("bjnp fields")
	}
	return kv("protocol", "bjnp", "discover", "LAB-PRINTER", "identity", id, "job", "job=7", "job_reply", job, "status", status), nil
}

func zkPacket(body []byte) []byte {
	b := make([]byte, 4+len(body))
	binary.BigEndian.PutUint32(b, uint32(len(body)))
	copy(b[4:], body)
	return b
}

func buildZK(l *lab) {
	c := l.tcp(42181, 2181)
	req := make([]byte, 45)
	// protocolVersion 0, lastZxid 0 already
	binary.BigEndian.PutUint32(req[12:16], 30000)
	binary.BigEndian.PutUint32(req[24:28], 16) // passwd len at offset 4+8+4+8=24
	c.client(zkPacket(req))
	resp := make([]byte, 37)
	binary.BigEndian.PutUint32(resp[4:8], 30000)
	binary.BigEndian.PutUint64(resp[8:16], 0x5013)
	binary.BigEndian.PutUint32(resp[16:20], 16)
	copy(resp[20:36], []byte("winlab-session!!"))
	c.server(zkPacket(resp))
	ping := make([]byte, 8)
	binary.BigEndian.PutUint32(ping[0:4], 0xfffffffe)
	binary.BigEndian.PutUint32(ping[4:8], 11)
	c.client(zkPacket(ping))
	pong := make([]byte, 16)
	binary.BigEndian.PutUint32(pong[0:4], 0xfffffffe)
	binary.BigEndian.PutUint64(pong[4:12], 2)
	c.server(zkPacket(pong))
	gc := append([]byte{}, 0, 0, 0, 1, 0, 0, 0, 8)
	gc = append(gc, zkBytes("/")...)
	gc = append(gc, 0)
	c.client(zkPacket(gc))
	gr := make([]byte, 16)
	binary.BigEndian.PutUint32(gr[0:4], 1)
	binary.BigEndian.PutUint64(gr[4:12], 2)
	gr = append(gr, 0, 0, 0, 2)
	gr = append(gr, zkBytes("lab")...)
	gr = append(gr, zkBytes("znode")...)
	c.server(zkPacket(gr))
	gd := append([]byte{}, 0, 0, 0, 2, 0, 0, 0, 4)
	gd = append(gd, zkBytes("/lab")...)
	gd = append(gd, 0)
	c.client(zkPacket(gd))
	gdr := make([]byte, 16)
	binary.BigEndian.PutUint32(gdr[0:4], 2)
	binary.BigEndian.PutUint64(gdr[4:12], 3)
	gdr = append(gdr, zkBytes("winlab")...)
	stat := make([]byte, 68)
	binary.BigEndian.PutUint64(stat[0:8], 1)
	binary.BigEndian.PutUint64(stat[8:16], 3)
	binary.BigEndian.PutUint32(stat[52:56], 6)
	gdr = append(gdr, stat...)
	c.server(zkPacket(gdr))
	cl := make([]byte, 8)
	binary.BigEndian.PutUint32(cl[0:4], 3)
	binary.BigEndian.PutUint32(cl[4:8], 0xfffffff5) // opcode -11, CloseSession
	c.client(zkPacket(cl))
	clr := make([]byte, 16)
	binary.BigEndian.PutUint32(clr[0:4], 3)
	binary.BigEndian.PutUint64(clr[4:12], 3)
	c.server(zkPacket(clr))
	c.close()
}

func zkBodies(stream []byte) ([][]byte, error) {
	r := rd{b: stream}
	var out [][]byte
	for r.remain() > 0 {
		n, err := r.be32()
		if err != nil {
			return nil, err
		}
		body, err := r.bytes(int(n))
		if err != nil {
			return nil, err
		}
		out = append(out, append([]byte{}, body...))
	}
	return out, nil
}

func parseZK(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 2181)
	if err != nil {
		return "", err
	}
	creq, err := zkBodies(cn.c2s)
	if err != nil {
		return "", err
	}
	cresp, err := zkBodies(cn.s2c)
	if err != nil {
		return "", err
	}
	if len(creq) < 5 || len(cresp) < 5 {
		return "", fmt.Errorf("zk packets")
	}
	rs := rd{b: cresp[0]}
	if _, err = rs.be32(); err != nil {
		return "", err
	}
	if _, err = rs.be32(); err != nil {
		return "", err
	}
	sid, err := rs.be64()
	if err != nil || sid == 0 {
		return "", fmt.Errorf("zk session")
	}
	var children string
	var value string
	for _, body := range cresp[1:] {
		r := rd{b: body}
		xid, err := r.be32()
		if err != nil {
			return "", err
		}
		if _, err = r.be64(); err != nil {
			return "", err
		}
		zerr, err := r.be32()
		if err != nil || zerr != 0 {
			return "", fmt.Errorf("zk err")
		}
		if xid == 1 {
			n, err := r.be32()
			if err != nil {
				return "", err
			}
			var names []string
			for i := uint32(0); i < n; i++ {
				s, err := r.zkString()
				if err != nil {
					return "", err
				}
				names = append(names, s)
			}
			children = strings.Join(names, ",")
		}
		if xid == 2 {
			b, err := r.zkBytes()
			if err != nil {
				return "", err
			}
			value = string(b)
		}
	}
	if children == "" || value == "" {
		return "", fmt.Errorf("zk data")
	}
	return kv("protocol", "zookeeper", "session_id", fmt.Sprintf("0x%x", sid), "children", children, "path", "/lab", "value", value), nil
}

func mqttsn(payload []byte) []byte {
	if len(payload)+1 > 255 {
		panic("mqttsn too long")
	}
	return append([]byte{byte(len(payload) + 1)}, payload...)
}

func buildMQTTSN(l *lab) {
	l.udp(41883, 1883, true, mqttsn([]byte{0x01, 0x01}))
	l.udp(41883, 1883, false, mqttsn([]byte{0x02, 0x01}))
	conn := []byte{0x04, 0x04, 0x01, 0x00, 0x3c}
	conn = append(conn, []byte("winlab")...)
	l.udp(41883, 1883, true, mqttsn(conn))
	l.udp(41883, 1883, false, mqttsn([]byte{0x05, 0x00}))
	reg := []byte{0x0A, 0x00, 0x00, 0x00, 0x01}
	reg = append(reg, []byte("lab/temp")...)
	l.udp(41883, 1883, true, mqttsn(reg))
	l.udp(41883, 1883, false, mqttsn([]byte{0x0B, 0x00, 0x01, 0x00, 0x01, 0x00}))
	pub := []byte{0x0C, 0x20, 0x00, 0x01, 0x00, 0x02}
	pub = append(pub, []byte("23.5")...)
	l.udp(41883, 1883, true, mqttsn(pub))
	l.udp(41883, 1883, false, mqttsn([]byte{0x0D, 0x00, 0x01, 0x00, 0x02, 0x00}))
	l.udp(41883, 1883, true, mqttsn([]byte{0x18}))
}

func parseMQTTSN(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 1883)
	if err != nil {
		return "", err
	}
	body := func(p []byte) ([]byte, error) {
		if len(p) < 2 || int(p[0]) != len(p) {
			return nil, fmt.Errorf("mqttsn length")
		}
		return p[1:], nil
	}
	var client, topic, data string
	var topicID int
	for _, p := range c2s {
		b, err := body(p)
		if err != nil {
			return "", err
		}
		switch b[0] {
		case 0x04:
			if len(b) < 6 {
				return "", fmt.Errorf("mqttsn connect")
			}
			client = string(b[5:])
		case 0x0A:
			if len(b) < 6 {
				return "", fmt.Errorf("mqttsn register")
			}
			topic = string(b[5:])
		case 0x0C:
			if len(b) < 6 {
				return "", fmt.Errorf("mqttsn publish")
			}
			topicID = int(binary.BigEndian.Uint16(b[2:4]))
			data = string(b[6:])
		}
	}
	ack := false
	for _, p := range s2c {
		b, err := body(p)
		if err != nil {
			return "", err
		}
		if b[0] == 0x05 && len(b) > 1 && b[1] == 0 {
			ack = true
		}
	}
	if client == "" || topic == "" || data == "" || !ack || topicID == 0 {
		return "", fmt.Errorf("mqttsn fields")
	}
	return kv("protocol", "mqtt-sn", "client_id", client, "topic", topic, "topic_id", strconv.Itoa(topicID), "qos", "1", "payload", data), nil
}

func stunType(method, class uint16) uint16 {
	return (method & 0x0f) | ((class & 0x1) << 4) | ((method & 0x70) << 1) | ((class & 0x2) << 7) | ((method & 0xf80) << 2)
}

func stunAttr(typ uint16, val []byte) []byte {
	b := make([]byte, 4+len(val))
	binary.BigEndian.PutUint16(b[0:2], typ)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(val)))
	copy(b[4:], val)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func xorAddr(ip [4]byte, port uint16) []byte {
	xp := port ^ 0x2112
	b := []byte{0x00, 0x01, byte(xp >> 8), byte(xp)}
	magic := []byte{0x21, 0x12, 0xA4, 0x42}
	for i := 0; i < 4; i++ {
		b = append(b, ip[i]^magic[i])
	}
	return b
}

func stunMsg(method, class uint16, tx string, attrs ...[]byte) []byte {
	if len(tx) != 12 {
		panic("txid")
	}
	var body []byte
	for _, a := range attrs {
		body = append(body, a...)
	}
	b := make([]byte, 20+len(body))
	binary.BigEndian.PutUint16(b[0:2], stunType(method, class))
	binary.BigEndian.PutUint16(b[2:4], uint16(len(body)))
	binary.BigEndian.PutUint32(b[4:8], 0x2112A442)
	copy(b[8:20], tx)
	copy(b[20:], body)
	return b
}

func channelData(ch uint16, payload string) []byte {
	b := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint16(b[0:2], ch)
	binary.BigEndian.PutUint16(b[2:4], uint16(len(payload)))
	copy(b[4:], payload)
	for len(b)%4 != 0 {
		b = append(b, 0)
	}
	return b
}

func buildTURN(l *lab) {
	life := []byte{0, 0, 0x02, 0x58}
	relay := [4]byte{192, 0, 2, 55}
	peer := [4]byte{198, 51, 100, 8}
	mapped := clientIP
	l.udp(43478, 3478, true, stunMsg(0x003, 0, "winlabTURN01", stunAttr(0x0019, []byte{0x11, 0, 0, 0})))
	l.udp(43478, 3478, false, stunMsg(0x003, 2, "winlabTURN01",
		stunAttr(0x0016, xorAddr(relay, 50000)),
		stunAttr(0x000D, life),
		stunAttr(0x0020, xorAddr(mapped, 43478)),
	))
	l.udp(43478, 3478, true, stunMsg(0x008, 0, "winlabTURN02", stunAttr(0x0012, xorAddr(peer, 3478))))
	l.udp(43478, 3478, false, stunMsg(0x008, 2, "winlabTURN02"))
	ch := []byte{0x40, 0x01, 0, 0}
	l.udp(43478, 3478, true, stunMsg(0x009, 0, "winlabTURN03", stunAttr(0x000C, ch), stunAttr(0x0012, xorAddr(peer, 3478))))
	l.udp(43478, 3478, false, stunMsg(0x009, 2, "winlabTURN03"))
	l.udp(43478, 3478, true, channelData(0x4001, "lab-turn"))
	l.udp(43478, 3478, false, channelData(0x4001, "peer-ok"))
	l.udp(43478, 3478, true, stunMsg(0x004, 0, "winlabTURN04", stunAttr(0x000D, life)))
	l.udp(43478, 3478, false, stunMsg(0x004, 2, "winlabTURN04", stunAttr(0x000D, life)))
}

func parseTURN(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 3478)
	if err != nil {
		return "", err
	}
	decodeXOR := func(v []byte) (string, error) {
		if len(v) < 8 || v[1] != 1 {
			return "", fmt.Errorf("xor addr")
		}
		port := binary.BigEndian.Uint16(v[2:4]) ^ 0x2112
		ip := []byte{v[4] ^ 0x21, v[5] ^ 0x12, v[6] ^ 0xA4, v[7] ^ 0x42}
		return fmt.Sprintf("%d.%d.%d.%d:%d", ip[0], ip[1], ip[2], ip[3], port), nil
	}
	var relay, peer, data string
	var lifetime uint32
	var channel int
	walk := func(pkts [][]byte) error {
		for _, p := range pkts {
			if len(p) >= 4 && binary.BigEndian.Uint16(p[0:2]) >= 0x4000 && binary.BigEndian.Uint16(p[0:2]) <= 0x7fff {
				n := int(binary.BigEndian.Uint16(p[2:4]))
				if 4+n > len(p) {
					return fmt.Errorf("channel data")
				}
				channel = int(binary.BigEndian.Uint16(p[0:2]))
				if data == "" {
					data = string(p[4 : 4+n])
				} else {
					data = data + "/" + string(p[4:4+n])
				}
				continue
			}
			if len(p) < 20 || binary.BigEndian.Uint32(p[4:8]) != 0x2112A442 {
				return fmt.Errorf("stun header")
			}
			alen := int(binary.BigEndian.Uint16(p[2:4]))
			attrs := p[20:]
			if len(attrs) < alen {
				return fmt.Errorf("stun attrs")
			}
			attrs = attrs[:alen]
			for len(attrs) >= 4 {
				typ := binary.BigEndian.Uint16(attrs[0:2])
				n := int(binary.BigEndian.Uint16(attrs[2:4]))
				if 4+n > len(attrs) {
					return fmt.Errorf("stun attr")
				}
				val := attrs[4 : 4+n]
				pad := (4 - n%4) % 4
				attrs = attrs[4+n+pad:]
				switch typ {
				case 0x0016:
					s, err := decodeXOR(val)
					if err != nil {
						return err
					}
					relay = s
				case 0x0012:
					s, err := decodeXOR(val)
					if err != nil {
						return err
					}
					peer = s
				case 0x000D:
					if len(val) != 4 {
						return fmt.Errorf("lifetime")
					}
					lifetime = binary.BigEndian.Uint32(val)
				case 0x000C:
					if len(val) < 2 {
						return fmt.Errorf("channel")
					}
					channel = int(binary.BigEndian.Uint16(val[0:2]))
				}
			}
		}
		return nil
	}
	if err := walk(c2s); err != nil {
		return "", err
	}
	if err := walk(s2c); err != nil {
		return "", err
	}
	if relay == "" || peer == "" || lifetime == 0 || channel == 0 || !strings.Contains(data, "lab-turn") {
		return "", fmt.Errorf("turn fields")
	}
	return kv("protocol", "turn", "relayed", relay, "peer", peer, "channel", fmt.Sprintf("0x%04x", channel), "lifetime", strconv.Itoa(int(lifetime)), "payload", data), nil
}
