package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func buildCalDAV(l *lab) {
	c := l.tcp(48080, 8080)
	c.client(httpRaw("OPTIONS /cal/alice/ HTTP/1.1", []hdr{{"Host", "lab.invalid"}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"DAV", "1, 2, 3, calendar-access"}, {"Allow", "OPTIONS, GET, PUT, DELETE, PROPFIND, REPORT"}}, nil))
	propfind := `<?xml version="1.0"?><propfind xmlns="DAV:"><prop><displayname/></prop></propfind>`
	c.client(httpRaw("PROPFIND /cal/alice/ HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Depth", "0"}, {"Content-Type", "application/xml"}}, []byte(propfind)))
	prop := `<?xml version="1.0"?><multistatus xmlns="DAV:"><response><propstat><prop><displayname>Lab Calendar</displayname></prop></propstat></response></multistatus>`
	c.server(httpRaw("HTTP/1.1 207 Multi-Status", []hdr{{"Content-Type", "application/xml"}}, []byte(prop)))
	report := `<?xml version="1.0"?><calendar-query xmlns="urn:ietf:params:xml:ns:caldav"><filter><comp-filter name="VCALENDAR"/></filter></calendar-query>`
	c.client(httpRaw("REPORT /cal/alice/default/ HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Depth", "1"}, {"Content-Type", "application/xml"}}, []byte(report)))
	cal := "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:lab-evt-5013\r\nSUMMARY:WinLab review\r\nDTSTART:20260923T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	body := `<?xml version="1.0"?><multistatus xmlns="DAV:"><response><calendar-data xmlns="urn:ietf:params:xml:ns:caldav">` + cal + `</calendar-data></response></multistatus>`
	c.server(httpRaw("HTTP/1.1 207 Multi-Status", []hdr{{"Content-Type", "application/xml"}}, []byte(body)))
	c.client(httpRaw("PUT /cal/alice/default/lab-evt-5013.ics HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Content-Type", "text/calendar"}}, []byte(cal)))
	c.server(httpRaw("HTTP/1.1 201 Created", []hdr{{"Location", "/cal/alice/default/lab-evt-5013.ics"}}, nil))
	c.close()
}

func parseCalDAV(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 8080)
	if err != nil {
		return "", err
	}
	reqs, err := readAllHTTP(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(cn.s2c)
	if err != nil {
		return "", err
	}
	var name, uid, summary, putPath string
	for _, m := range reps {
		if n, err := xmlText(m.Body, "displayname"); err == nil {
			name = n
		}
		s := string(m.Body)
		if i := strings.Index(s, "UID:"); i >= 0 {
			uid = strings.TrimSpace(strings.SplitN(s[i+4:], "\r\n", 2)[0])
		}
		if i := strings.Index(s, "SUMMARY:"); i >= 0 {
			summary = strings.TrimSpace(strings.SplitN(s[i+8:], "\r\n", 2)[0])
		}
	}
	for _, m := range reqs {
		if strings.HasPrefix(m.Start, "PUT ") {
			putPath = strings.Split(m.Start, " ")[1]
		}
	}
	if name == "" || uid == "" || summary == "" || putPath == "" {
		return "", fmt.Errorf("caldav fields")
	}
	return kv("protocol", "caldav", "displayname", name, "uid", uid, "summary", summary, "put", putPath), nil
}

func buildCardDAV(l *lab) {
	c := l.tcp(48083, 8083)
	c.client(httpRaw("PROPFIND /card/alice/ HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Depth", "0"}, {"Content-Type", "application/xml"}}, []byte(`<?xml version="1.0"?><propfind xmlns="DAV:"><prop><displayname/></prop></propfind>`)))
	c.server(httpRaw("HTTP/1.1 207 Multi-Status", []hdr{{"Content-Type", "application/xml"}}, []byte(`<?xml version="1.0"?><multistatus><response><propstat><prop><displayname>Lab Contacts</displayname></prop></propstat></response></multistatus>`)))
	c.client(httpRaw("REPORT /card/alice/ HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Depth", "1"}, {"Content-Type", "application/xml"}}, []byte(`<?xml version="1.0"?><addressbook-query xmlns="urn:ietf:params:xml:ns:carddav"/>`)))
	card := "BEGIN:VCARD\r\nVERSION:3.0\r\nUID:lab-card-5013\r\nFN:Ada Lab\r\nTEL:+1-555-0100\r\nEND:VCARD\r\n"
	c.server(httpRaw("HTTP/1.1 207 Multi-Status", []hdr{{"Content-Type", "application/xml"}}, []byte(`<multistatus><response><address-data>`+card+`</address-data></response></multistatus>`)))
	c.client(httpRaw("PUT /card/alice/lab-card-5013.vcf HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Content-Type", "text/vcard"}}, []byte(card)))
	c.server(httpRaw("HTTP/1.1 201 Created", nil, nil))
	c.close()
}

func parseCardDAV(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 8083)
	if err != nil {
		return "", err
	}
	_, err = readAllHTTP(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(cn.s2c)
	if err != nil {
		return "", err
	}
	var name, uid, fn, tel string
	for _, m := range reps {
		if n, err := xmlText(m.Body, "displayname"); err == nil {
			name = n
		}
		s := string(m.Body)
		for _, line := range strings.Split(s, "\r\n") {
			switch {
			case strings.HasPrefix(line, "UID:"):
				uid = strings.TrimPrefix(line, "UID:")
			case strings.HasPrefix(line, "FN:"):
				fn = strings.TrimPrefix(line, "FN:")
			case strings.HasPrefix(line, "TEL:"):
				tel = strings.TrimPrefix(line, "TEL:")
			}
		}
	}
	if name == "" || uid == "" || fn == "" || tel == "" {
		return "", fmt.Errorf("carddav fields")
	}
	return kv("protocol", "carddav", "displayname", name, "uid", uid, "fn", fn, "tel", tel), nil
}

func scgiReq(pairs [][2]string, body []byte) []byte {
	var h []byte
	for _, p := range pairs {
		h = append(h, p[0]...)
		h = append(h, 0)
		h = append(h, p[1]...)
		h = append(h, 0)
	}
	out := append([]byte(strconv.Itoa(len(h))+":"), h...)
	out = append(out, ',')
	return append(out, body...)
}

func scgiResp(status, body string) []byte {
	return []byte("Status: " + status + "\r\nContent-Type: text/plain\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n" + body)
}

func buildSCGI(l *lab) {
	c := l.tcp(44000, 4000)
	c.client(scgiReq([][2]string{
		{"CONTENT_LENGTH", "0"},
		{"SCGI", "1"},
		{"REQUEST_METHOD", "GET"},
		{"REQUEST_URI", "/lab/status"},
	}, nil))
	c.server(scgiResp("200 OK", "scgi-ok-5013"))
	body := "point=7&value=42"
	c.client(scgiReq([][2]string{
		{"CONTENT_LENGTH", strconv.Itoa(len(body))},
		{"SCGI", "1"},
		{"REQUEST_METHOD", "POST"},
		{"REQUEST_URI", "/lab/point"},
	}, []byte(body)))
	c.server(scgiResp("200 OK", "stored"))
	c.close()
}

func parseSCGI(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 4000)
	if err != nil {
		return "", err
	}
	r := rd{b: cn.c2s}
	var uris []string
	var bodies []string
	for r.remain() > 0 {
		start := r.i
		for r.i < len(r.b) && r.b[r.i] != ':' {
			r.i++
		}
		if r.i >= len(r.b) {
			return "", fmt.Errorf("scgi netstring")
		}
		n, err := strconv.Atoi(string(r.b[start:r.i]))
		if err != nil {
			return "", err
		}
		r.i++
		hdr, err := r.bytes(n)
		if err != nil {
			return "", err
		}
		comma, err := r.u8()
		if err != nil || comma != ',' {
			return "", fmt.Errorf("scgi comma")
		}
		env := map[string]string{}
		for len(hdr) > 0 {
			z := strings.IndexByte(string(hdr), 0)
			if z < 0 {
				return "", fmt.Errorf("scgi key")
			}
			key := string(hdr[:z])
			hdr = hdr[z+1:]
			z = strings.IndexByte(string(hdr), 0)
			if z < 0 {
				return "", fmt.Errorf("scgi val")
			}
			env[key] = string(hdr[:z])
			hdr = hdr[z+1:]
		}
		cl, _ := strconv.Atoi(env["CONTENT_LENGTH"])
		b, err := r.bytes(cl)
		if err != nil {
			return "", err
		}
		uris = append(uris, env["REQUEST_METHOD"]+" "+env["REQUEST_URI"])
		bodies = append(bodies, string(b))
	}
	reps := string(cn.s2c)
	if len(uris) != 2 || !strings.Contains(reps, "scgi-ok-5013") || !strings.Contains(bodies[1], "point=7") {
		return "", fmt.Errorf("scgi fields")
	}
	return kv("protocol", "scgi", "request1", uris[0], "body1", "scgi-ok-5013", "request2", uris[1], "body2", bodies[1]), nil
}

func hessStr(s string) []byte {
	if len(s) > 31 {
		panic("hess str")
	}
	return append([]byte{byte(len(s))}, s...)
}

func hessInt(v int32) []byte {
	b := []byte{'I', 0, 0, 0, 0}
	binary.BigEndian.PutUint32(b[1:], uint32(v))
	return b
}

func hessCall(method string, args ...[]byte) []byte {
	b := append([]byte{'C'}, hessStr(method)...)
	b = append(b, hessInt(int32(len(args)))...)
	for _, a := range args {
		b = append(b, a...)
	}
	return b
}

func buildHessian(l *lab) {
	c := l.tcp(48082, 8082)
	call1 := hessCall("readPoint", hessInt(7))
	c.client(httpRaw("POST /hessian/lab HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Content-Type", "application/x-hessian"}}, call1))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "application/x-hessian"}}, append([]byte{'R'}, hessInt(42)...)))
	call2 := hessCall("writePoint", hessInt(7), hessInt(42))
	c.client(httpRaw("POST /hessian/lab HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Content-Type", "application/x-hessian"}}, call2))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "application/x-hessian"}}, []byte{'R', 'T'}))
	c.close()
}

func parseHessValue(r *rd) (string, error) {
	t, err := r.u8()
	if err != nil {
		return "", err
	}
	switch t {
	case 'I':
		v, err := r.be32()
		if err != nil {
			return "", err
		}
		return strconv.Itoa(int(int32(v))), nil
	case 'T':
		return "true", nil
	case 'F':
		return "false", nil
	default:
		if t < 32 {
			b, err := r.bytes(int(t))
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
		return "", fmt.Errorf("hessian type %q", t)
	}
}

func parseHessian(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 8082)
	if err != nil {
		return "", err
	}
	reqs, err := readAllHTTP(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(cn.s2c)
	if err != nil {
		return "", err
	}
	if len(reqs) != 2 || len(reps) != 2 {
		return "", fmt.Errorf("hessian http")
	}
	parseCall := func(body []byte) (string, []string, error) {
		r := rd{b: body}
		k, err := r.u8()
		if err != nil || k != 'C' {
			return "", nil, fmt.Errorf("hessian call")
		}
		method, err := parseHessValue(&r)
		if err != nil {
			return "", nil, err
		}
		ns, err := parseHessValue(&r)
		if err != nil {
			return "", nil, err
		}
		n, _ := strconv.Atoi(ns)
		var args []string
		for i := 0; i < n; i++ {
			a, err := parseHessValue(&r)
			if err != nil {
				return "", nil, err
			}
			args = append(args, a)
		}
		return method, args, nil
	}
	m1, a1, err := parseCall(reqs[0].Body)
	if err != nil {
		return "", err
	}
	m2, a2, err := parseCall(reqs[1].Body)
	if err != nil {
		return "", err
	}
	parseReply := func(body []byte) (string, error) {
		r := rd{b: body}
		k, err := r.u8()
		if err != nil || k != 'R' {
			return "", fmt.Errorf("hessian reply")
		}
		return parseHessValue(&r)
	}
	v1, err := parseReply(reps[0].Body)
	if err != nil {
		return "", err
	}
	v2, err := parseReply(reps[1].Body)
	if err != nil {
		return "", err
	}
	return kv("protocol", "hessian2", "method1", m1, "arg1", strings.Join(a1, ","), "result1", v1, "method2", m2, "arg2", strings.Join(a2, ","), "result2", v2), nil
}

func buildMsgpack(l *lab) {
	c := l.tcp(49850, 19850)
	req1 := mpArray(4, []byte{0x00}, []byte{0x01}, mpFixStr("lab.status"), []byte{0x90})
	rep1 := mpArray(4, []byte{0x01}, []byte{0x01}, []byte{0xc0}, mpFixStr("ok-5013"))
	req2 := mpArray(4, []byte{0x00}, []byte{0x02}, mpFixStr("lab.set"), mpArray(3, mpFixStr("point"), []byte{0x07}, []byte{0x2a}))
	rep2 := mpArray(4, []byte{0x01}, []byte{0x02}, []byte{0xc0}, []byte{0xc3})
	note := mpArray(3, []byte{0x02}, mpFixStr("lab.event"), mpArray(2, mpFixStr("job"), mpFixStr("7f3a")))
	c.client(req1)
	c.server(rep1)
	c.client(req2)
	c.server(rep2)
	c.client(note)
	c.close()
}

func parseMsgpack(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 19850)
	if err != nil {
		return "", err
	}
	client, err := mpAll(cn.c2s)
	if err != nil {
		return "", err
	}
	server, err := mpAll(cn.s2c)
	if err != nil {
		return "", err
	}
	asSlice := func(v any) ([]any, error) {
		a, ok := v.([]any)
		if !ok {
			return nil, fmt.Errorf("msgpack not array")
		}
		return a, nil
	}
	var status, method string
	var point, value string
	var job string
	for _, v := range client {
		a, err := asSlice(v)
		if err != nil {
			return "", err
		}
		if len(a) == 4 && a[0] == 0 {
			method, _ = a[2].(string)
			if method == "lab.set" {
				args, _ := a[3].([]any)
				if len(args) == 3 {
					point = fmt.Sprint(args[0])
					value = fmt.Sprint(args[2])
				}
			}
		}
		if len(a) == 3 && a[0] == 2 {
			args, _ := a[2].([]any)
			if len(args) == 2 {
				job = fmt.Sprint(args[1])
			}
		}
	}
	for _, v := range server {
		a, err := asSlice(v)
		if err != nil {
			return "", err
		}
		if len(a) == 4 && a[0] == 1 && a[1] == 1 {
			status = fmt.Sprint(a[3])
		}
	}
	if status == "" || method == "" || point == "" || job == "" {
		return "", fmt.Errorf("msgpack fields")
	}
	return kv("protocol", "msgpack-rpc", "status_result", status, "set_method", method, "set_point", point, "set_value", value, "event_job", job), nil
}

func buildClickHouse(l *lab) {
	c := l.tcp(49000, 9000)
	var client []byte
	client = append(client, chVarint(0)...)
	client = append(client, chString("winlab-ch")...)
	client = append(client, chVarint(23)...)
	client = append(client, chVarint(8)...)
	client = append(client, chVarint(54401)...)
	client = append(client, chString("lab")...)
	client = append(client, chString("winlab")...)
	client = append(client, chString("")...)
	client = append(client, chVarint(4)...)
	var server []byte
	server = append(server, chString("lab-clickhouse")...)
	server = append(server, chVarint(23)...)
	server = append(server, chVarint(8)...)
	server = append(server, chVarint(54401)...)
	server = append(server, chString("UTC")...)
	server = append(server, chString("lab")...)
	server = append(server, chVarint(1)...)
	server = append(server, chVarint(4)...)
	c.client(client)
	c.server(server)
	c.close()
}

func parseClickHouse(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 9000)
	if err != nil {
		return "", err
	}
	cr := rd{b: cn.c2s}
	typ, err := cr.varint()
	if err != nil || typ != 0 {
		return "", fmt.Errorf("ch hello")
	}
	name, err := cr.chString()
	if err != nil {
		return "", err
	}
	if _, err = cr.varint(); err != nil {
		return "", err
	}
	if _, err = cr.varint(); err != nil {
		return "", err
	}
	rev, err := cr.varint()
	if err != nil {
		return "", err
	}
	db, err := cr.chString()
	if err != nil {
		return "", err
	}
	user, err := cr.chString()
	if err != nil {
		return "", err
	}
	if _, err = cr.chString(); err != nil {
		return "", err
	}
	ping, err := cr.varint()
	if err != nil || ping != 4 || cr.remain() != 0 {
		return "", fmt.Errorf("ch ping")
	}
	sr := rd{b: cn.s2c}
	sname, err := sr.chString()
	if err != nil {
		return "", err
	}
	if _, err = sr.varint(); err != nil {
		return "", err
	}
	if _, err = sr.varint(); err != nil {
		return "", err
	}
	srev, err := sr.varint()
	if err != nil {
		return "", err
	}
	tz, err := sr.chString()
	if err != nil {
		return "", err
	}
	if _, err = sr.chString(); err != nil {
		return "", err
	}
	if _, err = sr.varint(); err != nil {
		return "", err
	}
	pong, err := sr.varint()
	if err != nil || pong != 4 || sr.remain() != 0 || srev != rev {
		return "", fmt.Errorf("ch pong")
	}
	return kv("protocol", "clickhouse", "client", name, "user", user, "database", db, "server", sname, "revision", strconv.FormatUint(rev, 10), "timezone", tz, "ping", "pong"), nil
}

func id20(s string) string {
	if len(s) != 20 {
		panic("node id " + s + " len " + strconv.Itoa(len(s)))
	}
	return bencStr(s)
}

func buildDHT(l *lab) {
	self := id20("winlab-node-5013!!!!")
	peer := id20("lab-peer-node-5013!!")
	target := id20("target-node-5013!!!!")
	info := id20("lab-infohash-5013!!!")
	q := func(t, name, arg string) string {
		return bencDict([][2]string{{"a", arg}, {"q", bencStr(name)}, {"t", bencStr(t)}, {"y", bencStr("q")}})
	}
	r := func(t, arg string) string {
		return bencDict([][2]string{{"r", arg}, {"t", bencStr(t)}, {"y", bencStr("r")}})
	}
	l.udp(46881, 6881, true, []byte(q("aa", "ping", bencDict([][2]string{{"id", self}}))))
	l.udp(46881, 6881, false, []byte(r("aa", bencDict([][2]string{{"id", peer}}))))
	l.udp(46881, 6881, true, []byte(q("ab", "find_node", bencDict([][2]string{{"id", self}, {"target", target}}))))
	nodes := make([]byte, 26)
	copy(nodes[:20], []byte("lab-peer-node-5013!!"))
	copy(nodes[20:24], []byte{198, 51, 100, 9})
	binary.BigEndian.PutUint16(nodes[24:26], 6881)
	l.udp(46881, 6881, false, []byte(r("ab", bencDict([][2]string{{"id", peer}, {"nodes", bencStr(string(nodes))}}))))
	l.udp(46881, 6881, true, []byte(q("ac", "get_peers", bencDict([][2]string{{"id", self}, {"info_hash", info}}))))
	l.udp(46881, 6881, false, []byte(r("ac", bencDict([][2]string{{"id", peer}, {"token", bencStr("lab")}}))))
	ann := bencDict([][2]string{{"id", self}, {"implied_port", bencInt(0)}, {"info_hash", info}, {"port", bencInt(6881)}, {"token", bencStr("lab")}})
	l.udp(46881, 6881, true, []byte(q("ad", "announce_peer", ann)))
	l.udp(46881, 6881, false, []byte(r("ad", bencDict([][2]string{{"id", peer}}))))
}

func buildBITS(l *lab) {
	c := l.tcp(48081, 8081)
	sid := "{LAB-SESSION-5013}"
	c.client(httpRaw("POST /lab/bits/job HTTP/1.1", []hdr{
		{"Host", "lab.invalid"},
		{"BITS-Packet-Type", "Create-Session"},
		{"BITS-Supported-Protocols", "{7df0354d-249b-430f-820d-3d2a9bef4931}"},
	}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{
		{"BITS-Packet-Type", "Ack"},
		{"BITS-Protocol", "{7df0354d-249b-430f-820d-3d2a9bef4931}"},
		{"BITS-Session-Id", sid},
	}, nil))
	c.client(httpRaw("POST /lab/bits/job HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"BITS-Packet-Type", "Ping"}, {"BITS-Session-Id", sid}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"BITS-Packet-Type", "Ack"}, {"BITS-Session-Id", sid}}, nil))
	frag := []byte("lab-bits-payload")
	c.client(httpRaw("POST /lab/bits/job HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"BITS-Packet-Type", "Fragment"}, {"BITS-Session-Id", sid}, {"Content-Range", "bytes 0-15/16"}}, frag))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"BITS-Packet-Type", "Ack"}, {"BITS-Session-Id", sid}}, nil))
	c.client(httpRaw("POST /lab/bits/job HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"BITS-Packet-Type", "Close-Session"}, {"BITS-Session-Id", sid}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"BITS-Packet-Type", "Ack"}, {"BITS-Session-Id", sid}}, nil))
	c.close()
}

func parseBITS(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 8081)
	if err != nil {
		return "", err
	}
	reqs, err := readAllHTTP(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(cn.s2c)
	if err != nil {
		return "", err
	}
	var sid, rng, frag string
	var types []string
	for _, m := range reqs {
		types = append(types, m.Headers["bits-packet-type"])
		if m.Headers["content-range"] != "" {
			rng = m.Headers["content-range"]
			frag = string(m.Body)
		}
	}
	for _, m := range reps {
		if m.Headers["bits-session-id"] != "" {
			sid = m.Headers["bits-session-id"]
		}
	}
	if sid == "" || frag == "" || len(types) != 4 {
		return "", fmt.Errorf("bits fields")
	}
	return kv("protocol", "ms-bits", "session", sid, "packets", strings.Join(types, ","), "range", rng, "fragment", frag), nil
}

func buildConsul(l *lab) {
	c := l.tcp(48500, 8500)
	c.client(httpRaw("GET /v1/status/leader HTTP/1.1", []hdr{{"Host", "lab.invalid"}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "application/json"}}, []byte(`"192.0.2.20:8300"`)))
	c.client(httpRaw("GET /v1/catalog/services HTTP/1.1", []hdr{{"Host", "lab.invalid"}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "application/json"}}, []byte(`{"lab-win":["win","pcap"]}`)))
	c.client(httpRaw("GET /v1/kv/lab/point?raw HTTP/1.1", []hdr{{"Host", "lab.invalid"}}, nil))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "text/plain"}}, []byte("42")))
	c.client(httpRaw("PUT /v1/kv/lab/point HTTP/1.1", []hdr{{"Host", "lab.invalid"}, {"Content-Type", "text/plain"}}, []byte("42")))
	c.server(httpRaw("HTTP/1.1 200 OK", []hdr{{"Content-Type", "application/json"}}, []byte("true")))
	c.close()
}

func parseConsul(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 8500)
	if err != nil {
		return "", err
	}
	reqs, err := readAllHTTP(cn.c2s)
	if err != nil {
		return "", err
	}
	reps, err := readAllHTTP(cn.s2c)
	if err != nil {
		return "", err
	}
	if len(reqs) != 4 || len(reps) != 4 {
		return "", fmt.Errorf("consul count")
	}
	var leader string
	if err := json.Unmarshal(reps[0].Body, &leader); err != nil || leader == "" {
		return "", fmt.Errorf("consul leader")
	}
	var services map[string][]string
	if err := json.Unmarshal(reps[1].Body, &services); err != nil || len(services["lab-win"]) == 0 {
		return "", fmt.Errorf("consul services")
	}
	if string(reps[2].Body) != "42" || string(reps[3].Body) != "true" {
		return "", fmt.Errorf("consul kv")
	}
	if !strings.Contains(reqs[2].Start, "/v1/kv/lab/point") || !strings.HasPrefix(reqs[3].Start, "PUT ") {
		return "", fmt.Errorf("consul paths")
	}
	return kv("protocol", "consul", "leader", leader, "service", "lab-win", "tags", strings.Join(services["lab-win"], ","), "kv", "lab/point", "value", string(reps[2].Body)), nil
}

func gearPkt(res bool, typ uint32, data string) []byte {
	b := make([]byte, 12+len(data))
	if res {
		copy(b[:4], []byte{0, 'R', 'E', 'S'})
	} else {
		copy(b[:4], []byte{0, 'R', 'E', 'Q'})
	}
	binary.BigEndian.PutUint32(b[4:8], typ)
	binary.BigEndian.PutUint32(b[8:12], uint32(len(data)))
	copy(b[12:], data)
	return b
}

func buildGearman(l *lab) {
	w := l.tcp(47002, 4730)
	w.client(gearPkt(false, 1, "lab.reverse\x00"))
	w.client(gearPkt(false, 9, ""))
	w.server(gearPkt(true, 10, ""))
	w.client(gearPkt(false, 4, ""))
	cl := l.tcp(47001, 4730)
	cl.client(gearPkt(false, 7, "lab.reverse\x00\x00winlab"))
	cl.server(gearPkt(true, 8, "H:lab:7\x00"))
	w.server(gearPkt(true, 6, ""))
	w.client(gearPkt(false, 9, ""))
	w.server(gearPkt(true, 11, "H:lab:7\x00lab.reverse\x00winlab"))
	w.client(gearPkt(false, 13, "H:lab:7\x00balniw"))
	cl.server(gearPkt(true, 13, "H:lab:7\x00balniw"))
	w.close()
	cl.close()
}

func parseGearman(frames []Frame) (string, error) {
	cs, err := tcpConns(frames, 4730)
	if err != nil {
		return "", err
	}
	type pkt struct {
		res bool
		typ uint32
		dat string
	}
	walk := func(b []byte) ([]pkt, error) {
		var out []pkt
		for len(b) > 0 {
			if len(b) < 12 {
				return nil, fmt.Errorf("gearman short")
			}
			magic := string(b[:4])
			res := magic == "\x00RES"
			if magic != "\x00REQ" && !res {
				return nil, fmt.Errorf("gearman magic")
			}
			typ := binary.BigEndian.Uint32(b[4:8])
			n := binary.BigEndian.Uint32(b[8:12])
			if int(n) > len(b)-12 {
				return nil, fmt.Errorf("gearman length")
			}
			out = append(out, pkt{res: res, typ: typ, dat: string(b[12 : 12+n])})
			b = b[12+n:]
		}
		return out, nil
	}
	var fn, handle, workload, result string
	for _, cn := range cs {
		for _, side := range [][]byte{cn.c2s, cn.s2c} {
			pkts, err := walk(side)
			if err != nil {
				return "", err
			}
			for _, p := range pkts {
				parts := strings.Split(p.dat, "\x00")
				switch p.typ {
				case 7:
					if len(parts) >= 3 {
						fn = parts[0]
						workload = parts[2]
					}
				case 8:
					if len(parts) > 0 {
						handle = parts[0]
					}
				case 11:
					if len(parts) >= 3 {
						fn = parts[1]
						workload = parts[2]
						handle = parts[0]
					}
				case 13:
					if len(parts) >= 2 && parts[1] != "" {
						result = parts[1]
						handle = parts[0]
					}
				}
			}
		}
	}
	if fn == "" || handle == "" || workload == "" || result == "" {
		return "", fmt.Errorf("gearman fields")
	}
	return kv("protocol", "gearman", "function", fn, "handle", handle, "workload", workload, "result", result), nil
}

func buildBeanstalk(l *lab) {
	c := l.tcp(41300, 11300)
	c.client([]byte("put 1024 0 60 12\r\nhello-winlab\r\n"))
	c.server([]byte("INSERTED 7\r\n"))
	c.client([]byte("reserve\r\n"))
	c.server([]byte("RESERVED 7 12\r\nhello-winlab\r\n"))
	c.client([]byte("delete 7\r\n"))
	c.server([]byte("DELETED\r\n"))
	c.close()
}

func parseBeanstalk(frames []Frame) (string, error) {
	cn, err := oneTCP(frames, 11300)
	if err != nil {
		return "", err
	}
	s := string(cn.s2c)
	if !strings.HasPrefix(string(cn.c2s), "put 1024 0 60 12\r\n") || !strings.Contains(s, "INSERTED 7\r\n") || !strings.Contains(s, "DELETED\r\n") {
		return "", fmt.Errorf("beanstalk commands")
	}
	const mark = "RESERVED 7 12\r\n"
	i := strings.Index(s, mark)
	if i < 0 {
		return "", fmt.Errorf("beanstalk reserved")
	}
	body := s[i+len(mark):]
	if !strings.HasPrefix(body, "hello-winlab\r\n") {
		return "", fmt.Errorf("beanstalk body")
	}
	return kv("protocol", "beanstalkd", "job_id", "7", "priority", "1024", "ttr", "60", "body", "hello-winlab", "deleted", "true"), nil
}

func dhtDict(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("dht not dict")
	}
	return m, nil
}

func parseDHT(frames []Frame) (string, error) {
	c2s, s2c, err := udpByPort(frames, 6881)
	if err != nil {
		return "", err
	}
	var pingID, target, info, token string
	var port int
	for _, p := range c2s {
		v, rest, err := bdecode(string(p))
		if err != nil || rest != "" {
			return "", fmt.Errorf("dht bencode")
		}
		m, err := dhtDict(v)
		if err != nil {
			return "", err
		}
		if m["y"] != "q" {
			continue
		}
		a, err := dhtDict(m["a"])
		if err != nil {
			return "", err
		}
		switch m["q"] {
		case "ping":
			pingID, _ = a["id"].(string)
		case "find_node":
			target, _ = a["target"].(string)
		case "get_peers":
			info, _ = a["info_hash"].(string)
		case "announce_peer":
			token, _ = a["token"].(string)
			if n, ok := a["port"].(int); ok {
				port = n
			}
		}
	}
	sawNodes := false
	for _, p := range s2c {
		v, rest, err := bdecode(string(p))
		if err != nil || rest != "" {
			return "", fmt.Errorf("dht bencode")
		}
		m, err := dhtDict(v)
		if err != nil {
			return "", err
		}
		if m["y"] != "r" {
			return "", fmt.Errorf("dht reply")
		}
		body, err := dhtDict(m["r"])
		if err != nil {
			return "", err
		}
		if nodes, ok := body["nodes"].(string); ok {
			if len(nodes) == 0 || len(nodes)%26 != 0 {
				return "", fmt.Errorf("dht nodes")
			}
			sawNodes = true
		}
	}
	if len(pingID) != 20 || len(target) != 20 || len(info) != 20 || token == "" || port == 0 || !sawNodes {
		return "", fmt.Errorf("dht fields")
	}
	return kv("protocol", "bittorrent-dht", "ping_id", pingID, "find_target", target, "info_hash", info, "announce_token", token, "announce_port", strconv.Itoa(port)), nil
}
