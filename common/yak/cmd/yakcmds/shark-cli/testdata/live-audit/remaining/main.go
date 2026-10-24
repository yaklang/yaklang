// Explicit Windows integration peer. Standard protocol clients and independent
// libraries exchange fresh socket traffic; pcaputil is never used to generate
// or validate a reply. Small database/LDAP/UDP peers are protocol scenarios,
// not claims to implement a database, directory, DHCP lease, or time service.
package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-ldap/ldap"
	_ "github.com/go-sql-driver/mysql"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/gorilla/websocket"
	"github.com/gosnmp/gosnmp"
	_ "github.com/lib/pq"
	"github.com/yaklang/pcap"
	"golang.org/x/crypto/ssh"
	ber "gopkg.in/asn1-ber.v1"
)

func must(e error) {
	if e != nil {
		panic(e)
	}
}

type op struct {
	Protocol, Kind, Hash, Error string
	Bytes                       int
}
type manifest struct {
	Nonce                string
	Ports                map[string]int
	Operations           []op
	Frames, Bytes, Drops int
	Filter               string
}

var result manifest

func note(protocol, kind string, b []byte) {
	h := sha256.Sum256(b)
	result.Operations = append(result.Operations, op{Protocol: protocol, Kind: kind, Hash: hex.EncodeToString(h[:]), Bytes: len(b)})
}
func exercise(name string, f func()) {
	defer func() {
		if p := recover(); p != nil {
			result.Operations = append(result.Operations, op{Protocol: name, Error: fmt.Sprint(p)})
			fmt.Fprintln(os.Stderr, name, p)
		}
	}()
	f()
}
func tcp(f func(net.Conn)) net.Listener {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	must(e)
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); _ = c.SetDeadline(time.Now().Add(20 * time.Second)); f(c) }()
		}
	}()
	return l
}
func udp(f func([]byte) []byte) *net.UDPConn {
	c, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(e)
	go func() {
		b := make([]byte, 65535)
		for {
			n, a, e := c.ReadFromUDP(b)
			if e != nil {
				return
			}
			if out := f(b[:n]); out != nil {
				_, _ = c.WriteToUDP(out, a)
			}
		}
	}()
	return c
}
func port(a net.Addr) int {
	_, p, e := net.SplitHostPort(a.String())
	must(e)
	n, e := strconv.Atoi(p)
	must(e)
	return n
}
func address(p int) string { return net.JoinHostPort("127.0.0.1", strconv.Itoa(p)) }
func readN(r io.Reader, n int) []byte {
	b := make([]byte, n)
	_, e := io.ReadFull(r, b)
	must(e)
	return b
}
func pgMessage(c io.Writer, t byte, b []byte) {
	w := append([]byte{t}, binary.BigEndian.AppendUint32(nil, uint32(4+len(b)))...)
	_, e := c.Write(append(w, b...))
	must(e)
}
func pgCString(s string) []byte { return append([]byte(s), 0) }
func pgCommand(c net.Conn, query string) {
	if !strings.HasPrefix(query, "SELECT '") {
		pgMessage(c, 'C', pgCString(strings.Fields(query)[0]))
		return
	}
	v := strings.TrimSuffix(strings.TrimPrefix(query, "SELECT '"), "'")
	col := binary.BigEndian.AppendUint16(nil, 1)
	col = append(col, pgCString("nonce")...)
	col = append(col, make([]byte, 6)...)
	col = binary.BigEndian.AppendUint32(col, 25)
	col = binary.BigEndian.AppendUint16(col, 65535)
	col = binary.BigEndian.AppendUint32(col, 0xffffffff)
	col = binary.BigEndian.AppendUint16(col, 0)
	pgMessage(c, 'T', col)
	row := binary.BigEndian.AppendUint16(nil, 1)
	row = binary.BigEndian.AppendUint32(row, uint32(len(v)))
	pgMessage(c, 'D', append(row, []byte(v)...))
	pgMessage(c, 'C', pgCString("SELECT 1"))
}
func pgPeer(c net.Conn) {
	defer func() { _ = recover() }()
	n := binary.BigEndian.Uint32(readN(c, 4))
	if n < 8 || n > 4096 {
		return
	}
	readN(c, int(n)-4)
	pgMessage(c, 'R', []byte{0, 0, 0, 0})
	for _, kv := range [][2]string{{"server_version", "16.0"}, {"client_encoding", "UTF8"}, {"standard_conforming_strings", "on"}} {
		pgMessage(c, 'S', append(pgCString(kv[0]), pgCString(kv[1])...))
	}
	pgMessage(c, 'K', []byte{0, 0, 0, 42, 1, 2, 3, 4})
	pgMessage(c, 'Z', []byte{'I'})
	for {
		t := readN(c, 1)[0]
		n := binary.BigEndian.Uint32(readN(c, 4))
		if n < 4 || n > 1<<20 {
			return
		}
		b := readN(c, int(n)-4)
		if t == 'X' {
			return
		}
		if t != 'Q' {
			return
		}
		pgCommand(c, strings.TrimSuffix(string(b), "\x00"))
		pgMessage(c, 'Z', []byte{'I'})
	}
}
func mysqlPacket(c io.Writer, seq byte, b []byte) {
	w := []byte{byte(len(b)), byte(len(b) >> 8), byte(len(b) >> 16), seq}
	_, e := c.Write(append(w, b...))
	must(e)
}
func mysqlRead(c io.Reader) []byte {
	h := readN(c, 4)
	n := int(h[0]) | int(h[1])<<8 | int(h[2])<<16
	if n > 1<<20 {
		panic("mysql packet budget")
	}
	return readN(c, n)
}
func mysqlString(b []byte) []byte {
	if len(b) > 250 {
		panic("test string budget")
	}
	return append([]byte{byte(len(b))}, b...)
}
func mysqlPeer(c net.Conn) {
	defer func() { _ = recover() }()
	const caps uint32 = 1 | 4 | 512 | 8192 | 32768 | 524288
	salt := make([]byte, 20)
	_, e := rand.Read(salt)
	must(e)
	for i, b := range salt {
		if b == 0 {
			salt[i] = 1
		}
	}
	b := append([]byte{10}, []byte("8.0.36-audit\x00")...)
	b = binary.LittleEndian.AppendUint32(b, 42)
	b = append(b, salt[:8]...)
	b = append(b, 0, byte(caps&255), byte(caps>>8&255), 33, 2, 0, byte(caps>>16), byte(caps>>24), 21)
	b = append(b, make([]byte, 10)...)
	b = append(b, salt[8:]...)
	b = append(b, 0)
	b = append(b, []byte("mysql_native_password\x00")...)
	mysqlPacket(c, 0, b)
	r := mysqlRead(c)
	if len(r) < 34 {
		return
	}
	at := 32
	for at < len(r) && r[at] != 0 {
		at++
	}
	at++
	if at >= len(r) {
		return
	}
	n := int(r[at])
	at++
	if n != 20 || at+n > len(r) {
		return
	}
	password := []byte("pw-" + result.Nonce)
	s1 := sha1.Sum(password)
	s2 := sha1.Sum(s1[:])
	h := sha1.New()
	h.Write(salt)
	h.Write(s2[:])
	scramble := h.Sum(nil)
	for i := range scramble {
		scramble[i] ^= s1[i]
	}
	if !bytes.Equal(scramble, r[at:at+n]) {
		return
	}
	ok := []byte{0, 0, 0, 2, 0, 0, 0}
	eof := []byte{0xfe, 0, 0, 2, 0}
	mysqlPacket(c, 2, ok)
	for {
		q := mysqlRead(c)
		if len(q) == 0 || q[0] == 1 {
			return
		}
		if q[0] == 14 {
			mysqlPacket(c, 1, ok)
			continue
		}
		if q[0] != 3 {
			return
		}
		query := string(q[1:])
		if !strings.HasPrefix(query, "SELECT '") {
			mysqlPacket(c, 1, ok)
			continue
		}
		v := strings.TrimSuffix(strings.TrimPrefix(query, "SELECT '"), "'")
		mysqlPacket(c, 1, []byte{1})
		var col []byte
		for _, s := range []string{"def", "audit", "", "", "nonce", ""} {
			col = append(col, mysqlString([]byte(s))...)
		}
		col = append(col, 12, 33, 0, 255, 0, 0, 0, 0xfd, 0, 0, 0, 0, 0)
		mysqlPacket(c, 2, col)
		mysqlPacket(c, 3, eof)
		mysqlPacket(c, 4, mysqlString([]byte(v)))
		mysqlPacket(c, 5, eof)
	}
}
func ldapString(s string) *ber.Packet {
	return ber.NewString(ber.ClassUniversal, ber.TypePrimitive, ber.TagOctetString, s, "")
}
func ldapMessage(c net.Conn, id int64, p *ber.Packet) {
	root := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
	root.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagInteger, id, ""))
	root.AppendChild(p)
	_, e := c.Write(root.Bytes())
	must(e)
}
func ldapDone(c net.Conn, id int64, tag ber.Tag) {
	p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, tag, nil, "")
	p.AppendChild(ber.NewInteger(ber.ClassUniversal, ber.TypePrimitive, ber.TagEnumerated, 0, ""))
	p.AppendChild(ldapString(""))
	p.AppendChild(ldapString(""))
	ldapMessage(c, id, p)
}
func ldapPeer(c net.Conn) {
	defer func() { _ = recover() }()
	for {
		root, e := ber.ReadPacket(c)
		if e != nil {
			return
		}
		if len(root.Children) < 2 {
			return
		}
		id := root.Children[0].Value.(int64)
		switch root.Children[1].Tag {
		case 0:
			ldapDone(c, id, 1)
		case 2:
			return
		case 3:
			p := ber.Encode(ber.ClassApplication, ber.TypeConstructed, 4, nil, "")
			p.AppendChild(ldapString("cn=" + result.Nonce + ",dc=audit,dc=invalid"))
			attrs := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
			for _, kv := range [][2]string{{"cn", result.Nonce}, {"description", "SSH-2.0-body-is-not-a-new-protocol"}} {
				a := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSequence, nil, "")
				a.AppendChild(ldapString(kv[0]))
				v := ber.Encode(ber.ClassUniversal, ber.TypeConstructed, ber.TagSet, nil, "")
				v.AppendChild(ldapString(kv[1]))
				a.AppendChild(v)
				attrs.AppendChild(a)
			}
			p.AppendChild(attrs)
			ldapMessage(c, id, p)
			ldapDone(c, id, 5)
		default:
			return
		}
	}
}
func main() {
	if len(os.Args) != 2 {
		panic("usage: remaining-peer OUTPUT_DIRECTORY")
	}
	out := os.Args[1]
	b, e := os.ReadFile(filepath.Join(out, "config.json"))
	must(e)
	must(json.Unmarshal(b, &result))
	var closers []io.Closer
	defer func() {
		for _, c := range closers {
			c.Close()
		}
	}()
	registerTCP := func(name string, f func(net.Conn)) {
		l := tcp(f)
		closers = append(closers, l)
		result.Ports[name] = port(l.Addr())
	}
	registerUDP := func(name string, f func([]byte) []byte) {
		l := udp(f)
		closers = append(closers, l)
		result.Ports[name] = port(l.LocalAddr())
	}
	registerTCP("postgresql", pgPeer)
	registerTCP("mysql", mysqlPeer)
	registerTCP("ldap", ldapPeer)
	key, e := os.ReadFile(filepath.Join(out, "server.key"))
	must(e)
	signer, e := ssh.ParsePrivateKey(key)
	must(e)
	sshConfig := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "pw-"+result.Nonce {
			return nil, fmt.Errorf("test authentication failed")
		}
		return nil, nil
	}}
	sshConfig.AddHostKey(signer)
	sshConfig.RekeyThreshold = 4096
	registerTCP("ssh", func(c net.Conn) {
		conn, channels, requests, e := ssh.NewServerConn(c, sshConfig)
		if e != nil {
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(requests)
		for ch := range channels {
			if ch.ChannelType() != "session" {
				ch.Reject(ssh.UnknownChannelType, "session only")
				continue
			}
			s, reqs, e := ch.Accept()
			if e != nil {
				return
			}
			go func() {
				defer s.Close()
				for req := range reqs {
					if req.Type != "exec" {
						req.Reply(false, nil)
						continue
					}
					req.Reply(true, nil)
					var cmd struct{ Command string }
					if ssh.Unmarshal(req.Payload, &cmd) != nil {
						return
					}
					_, _ = s.Write([]byte(strings.Repeat(result.Nonce+"|"+cmd.Command+"\n", 400)))
					_, _ = s.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					s.CloseWrite()
					return
				}
			}()
		}
	})
	wsListener, e := net.Listen("tcp4", "127.0.0.1:0")
	must(e)
	closers = append(closers, wsListener)
	result.Ports["websocket"] = port(wsListener.Addr())
	upgrader := websocket.Upgrader{EnableCompression: true}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, e := upgrader.Upgrade(w, r, nil)
		if e != nil {
			return
		}
		defer c.Close()
		for {
			t, b, e := c.ReadMessage()
			if e != nil {
				return
			}
			if c.WriteMessage(t, b) != nil {
				return
			}
		}
	})}
	defer server.Close()
	go server.Serve(wsListener)
	registerUDP("snmp", func(w []byte) []byte {
		p, e := (&gosnmp.GoSNMP{}).SnmpDecodePacket(w)
		if e != nil {
			return nil
		}
		p.PDUType = gosnmp.GetResponse
		for i := range p.Variables {
			p.Variables[i].Type = gosnmp.OctetString
			p.Variables[i].Value = []byte(result.Nonce)
		}
		b, e := p.MarshalMsg()
		if e != nil {
			return nil
		}
		return b
	})
	registerUDP("ntp", func(w []byte) []byte {
		if len(w) != 48 {
			return nil
		}
		r := make([]byte, 48)
		r[0], r[1], r[2], r[3] = 0x24, 2, 6, 236
		copy(r[12:16], "LOCL")
		copy(r[24:32], w[40:48])
		now := time.Now()
		sec := uint32(now.Unix() + 2208988800)
		for _, at := range []int{16, 32, 40} {
			binary.BigEndian.PutUint32(r[at:], sec)
			binary.BigEndian.PutUint32(r[at+4:], uint32(uint64(now.Nanosecond())<<32/1000000000))
		}
		return r
	})
	registerUDP("dhcp", func(w []byte) []byte {
		var d layers.DHCPv4
		if d.DecodeFromBytes(w, gopacket.NilDecodeFeedback) != nil {
			return nil
		}
		typ := byte(2)
		for _, o := range d.Options {
			if o.Type == layers.DHCPOptMessageType && len(o.Data) > 0 && o.Data[0] == 3 {
				typ = 5
			}
		}
		d.Operation = layers.DHCPOpReply
		d.YourClientIP = net.IPv4(192, 0, 2, 20)
		d.NextServerIP = net.IPv4(127, 0, 0, 1)
		d.Options = layers.DHCPOptions{{Type: layers.DHCPOptMessageType, Length: 1, Data: []byte{typ}}, {Type: layers.DHCPOptServerID, Length: 4, Data: []byte{127, 0, 0, 1}}}
		buf := gopacket.NewSerializeBuffer()
		if gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true}, &d) != nil {
			return nil
		}
		return buf.Bytes()
	})
	var filters []string
	for name, p := range result.Ports {
		carrier := "tcp"
		if name == "ntp" || name == "snmp" || name == "dhcp" {
			carrier = "udp"
		}
		filters = append(filters, fmt.Sprintf("%s port %d", carrier, p))
	}
	result.Filter = strings.Join(filters, " or ")
	handle, e := pcap.OpenLive(`\Device\NPF_Loopback`, 65535, false, 100*time.Millisecond)
	must(e)
	defer handle.Close()
	must(handle.SetBPFFilter(result.Filter))
	file, e := os.OpenFile(filepath.Join(out, "live.pcap"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	must(e)
	writer := pcapgo.NewWriterNanos(file)
	must(writer.WriteFileHeader(65535, handle.LinkType()))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			b, ci, e := handle.ReadPacketData()
			if e == pcap.NextErrorTimeoutExpired {
				continue
			}
			if e != nil {
				panic(e)
			}
			must(writer.WritePacket(ci, b))
			result.Frames++
			result.Bytes += len(b)
		}
	}()
	b, e = json.Marshal(result.Ports)
	must(e)
	must(os.WriteFile(filepath.Join(out, "capture-ready.json"), b, 0600))
	exercise("ssh", func() {
		cfg := &ssh.ClientConfig{User: "audit", Auth: []ssh.AuthMethod{ssh.Password("pw-" + result.Nonce)}, HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()), Timeout: 5 * time.Second}
		c, e := ssh.Dial("tcp", address(result.Ports["ssh"]), cfg)
		must(e)
		defer c.Close()
		for i := 0; i < 2; i++ {
			s, e := c.NewSession()
			must(e)
			data, e := s.Output(fmt.Sprintf("audit-%d", i))
			must(e)
			s.Close()
			note("ssh", "exec", data)
		}
	})
	exercise("websocket", func() {
		d := websocket.Dialer{EnableCompression: true}
		c, _, e := d.Dial("ws://"+address(result.Ports["websocket"])+"/"+result.Nonce, nil)
		must(e)
		defer c.Close()
		for _, size := range []int{3, 2048, 70000} {
			seed := result.Nonce + "SSH-2.0-payload/"
			data := []byte(strings.Repeat(seed, size/len(seed)+1))
			data = data[:min(len(data), size)]
			must(c.WriteMessage(websocket.BinaryMessage, data))
			_, reply, e := c.ReadMessage()
			must(e)
			if !bytes.Equal(data, reply) {
				panic("websocket echo")
			}
			note("websocket", "compressed-echo", reply)
		}
		must(c.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "done"), time.Now().Add(time.Second)))
	})
	exercise("ldap", func() {
		c, e := ldap.Dial("tcp", address(result.Ports["ldap"]))
		must(e)
		defer c.Close()
		must(c.Bind("cn=audit,dc=audit,dc=invalid", "pw-"+result.Nonce))
		for i := 0; i < 3; i++ {
			r, e := c.Search(ldap.NewSearchRequest("dc=audit,dc=invalid", ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, "(objectClass=*)", []string{"cn", "description"}, nil))
			must(e)
			if len(r.Entries) != 1 || r.Entries[0].GetAttributeValue("cn") != result.Nonce {
				panic("LDAP value")
			}
			note("ldap", "search", []byte(r.Entries[0].GetAttributeValue("cn")))
		}
	})
	for _, name := range []string{"postgresql", "mysql"} {
		name := name
		exercise(name, func() {
			driver, dsn := "postgres", fmt.Sprintf("host=127.0.0.1 port=%d user=audit dbname=audit sslmode=disable connect_timeout=3", result.Ports[name])
			if name == "mysql" {
				driver = "mysql"
				dsn = fmt.Sprintf("audit:pw-%s@tcp(%s)/?timeout=3s", result.Nonce, address(result.Ports[name]))
			}
			db, e := sql.Open(driver, dsn)
			must(e)
			defer db.Close()
			db.SetMaxOpenConns(1)
			must(db.Ping())
			for i := 0; i < 3; i++ {
				want := result.Nonce + "-" + strconv.Itoa(i)
				var got string
				must(db.QueryRow("SELECT '" + want + "'").Scan(&got))
				if got != want {
					panic("database value")
				}
				note(name, "select-reuse", []byte(got))
			}
		})
	}
	exercise("snmp", func() {
		for _, version := range []gosnmp.SnmpVersion{gosnmp.Version1, gosnmp.Version2c} {
			s := &gosnmp.GoSNMP{Target: "127.0.0.1", Port: uint16(result.Ports["snmp"]), Community: "public-" + result.Nonce, Version: version, Timeout: 2 * time.Second, Retries: 0}
			must(s.Connect())
			for i := 0; i < 3; i++ {
				r, e := s.Get([]string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.5.0"})
				must(e)
				for _, v := range r.Variables {
					value, ok := v.Value.([]byte)
					if !ok || string(value) != result.Nonce {
						panic("SNMP value")
					}
					note("snmp", "get", value)
				}
			}
			s.Conn.Close()
		}
	})
	exercise("dhcp", func() {
		c, e := net.DialTimeout("udp", address(result.Ports["dhcp"]), 3*time.Second)
		must(e)
		defer c.Close()
		must(c.SetDeadline(time.Now().Add(3 * time.Second)))
		mac := make([]byte, 6)
		_, e = rand.Read(mac)
		must(e)
		mac[0] = 2
		xid := 0x10000000 | uint32(time.Now().UnixNano())&0xffffff
		for _, typ := range []byte{1, 3} {
			d := &layers.DHCPv4{Operation: layers.DHCPOpRequest, HardwareType: layers.LinkTypeEthernet, HardwareLen: 6, Xid: xid, ClientHWAddr: mac, ClientIP: net.IPv4zero, YourClientIP: net.IPv4zero, NextServerIP: net.IPv4zero, RelayAgentIP: net.IPv4zero, Options: layers.DHCPOptions{{Type: layers.DHCPOptMessageType, Length: 1, Data: []byte{typ}}}}
			buf := gopacket.NewSerializeBuffer()
			must(gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true}, d))
			_, e = c.Write(buf.Bytes())
			must(e)
			w := make([]byte, 2048)
			n, e := c.Read(w)
			must(e)
			var reply layers.DHCPv4
			must(reply.DecodeFromBytes(w[:n], gopacket.NilDecodeFeedback))
			if reply.Xid != d.Xid {
				panic("DHCP transaction")
			}
			note("dhcp", "observed-exchange-no-lease", w[:n])
		}
	})
	// Python's independent FTP/SMTP/SMB/MQTT/NTP clients run while this capture
	// stays open. EOF on our private stdin ends the bounded integration run.
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	time.Sleep(250 * time.Millisecond)
	close(stop)
	wg.Wait()
	file.Close()
	if s, e := handle.Stats(); e == nil {
		result.Drops = s.PacketsDropped
	}
	b, e = json.MarshalIndent(result, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(out, "go-manifest.json"), b, 0600))
	fmt.Printf("captured %d frames; %d operations; kernel drops=%d\n", result.Frames, len(result.Operations), result.Drops)
}
