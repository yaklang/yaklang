package pcaputil

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

var builtinBinSpecs = [][2]string{
	{"application-layer.http", "HTTPExact"},
	{"application-layer.mqtt_fields", "MQTT31PacketFields"},
	{"application-layer.mqtt_fields", "MQTT311PacketFields"},
	{"application-layer.memcached_fields", "MemcachedStatsRequestFields"},
	{"application-layer.memcached_fields", "MemcachedStatsResponseFields"},
	{"application-layer.memcached_fields", "MemcachedBinaryGetRequestFields"},
	{"application-layer.cassandra_fields", "CQLOptions4Fields"},
	{"application-layer.cassandra_fields", "CQLSupported4Fields"},
	{"application-layer.cassandra_fields", "CQLStartup4Fields"},
	{"application-layer.kerberos_fields", "KerberosTCPFields"},
	{"application-layer.kerberos_fields", "KerberosMessageFields"},
	{"application-layer.dns", "DNS"},
	{"application-layer.tls", ""},
}

func (f *binFlow) spec(family, entry string) *binSpec {
	return f.a.specs["application-layer."+family+"/"+entry]
}
func (f *binFlow) port(port uint16) bool { return f.ports[0] == port || f.ports[1] == port }

// Detection runs only on the bounded initial prefix. Ports narrow candidates;
// they never choose a negotiated version, phase, or native decoder by themselves.
func (f *binFlow) detect(w []byte) {
	for _, port := range []uint16{0, f.ports[0], f.ports[1]} {
		for _, b := range f.a.bindings[port] {
			if b.Probe(w) {
				f.protocol, f.binding = b.Protocol, b
				return
			}
		}
	}
	for _, prefix := range []string{"GET ", "POST ", "PUT ", "DELETE ", "HEAD ", "OPTIONS ", "PATCH ", "CONNECT ", "TRACE ", "HTTP/1."} {
		if bytes.HasPrefix(w, []byte(prefix)) {
			f.protocol = "http"
			return
		}
	}
	if len(w) >= 5 && w[0] >= 20 && w[0] <= 23 && w[1] == 3 && w[2] <= 4 && binary.BigEndian.Uint16(w[3:5]) <= 18432 {
		f.protocol = "tls"
		return
	}
	if len(w) >= 2 && w[0] == 0x10 {
		_, header, err := mqttLength(w)
		if err == nil && header > 0 && len(w) >= header+3 {
			n := int(binary.BigEndian.Uint16(w[header:]))
			if n <= 6 && len(w) >= header+2+n+1 {
				name, level := string(w[header+2:header+2+n]), w[header+2+n]
				if (name == "MQTT" && level == 4) || (name == "MQIsdp" && level == 3) {
					f.protocol, f.level = "mqtt", level
					return
				}
			}
		}
	}
	if bytes.HasPrefix(w, []byte("stats\r\n")) || bytes.HasPrefix(w, []byte("STAT ")) || (len(w) >= 24 && w[0] == 0x80 && w[1] == 0 && w[4] == 0 && w[5] == 0) {
		f.protocol = "memcached"
		return
	}
	if f.port(9042) && len(w) >= 9 && (w[0] == 4 || w[0] == 0x84) && w[1]&1 == 0 && (w[4] == 1 || w[4] == 5 || w[4] == 6) {
		f.protocol = "cassandra"
		return
	}
	if f.port(88) && len(w) >= 6 && kerberosTag(w[4]) && binary.BigEndian.Uint32(w[:4]) >= 2 {
		f.protocol = "kerberos"
		return
	}
	if f.port(53) && len(w) >= 14 && binary.BigEndian.Uint16(w[:2]) >= 12 && dnsHeader(w[2:]) {
		f.protocol = "dns"
		return
	}
}

func mqttLength(w []byte) (total, header int, err error) {
	if len(w) < 2 {
		return 0, 0, nil
	}
	n, m := 0, 1
	for i := 1; i <= 4; i++ {
		if len(w) <= i {
			return 0, 0, nil
		}
		b := w[i]
		n += int(b&127) * m
		if b&128 == 0 {
			if i > 1 && b == 0 {
				return 0, 0, fmt.Errorf("MQTT non-canonical remaining length")
			}
			return i + 1 + n, i + 1, nil
		}
		m *= 128
	}
	return 0, 0, fmt.Errorf("MQTT remaining length exceeds four bytes")
}

func (f *binFlow) frame(w []byte) (int, *binSpec, error) {
	if f.binding != nil {
		n, err := f.binding.Frame(w)
		return n, f.a.specs[f.binding.Rule+"/"+f.binding.Entry], err
	}
	switch f.protocol {
	case "mqtt":
		n, _, err := mqttLength(w)
		entry := "MQTT311PacketFields"
		if f.level == 3 {
			entry = "MQTT31PacketFields"
		}
		return n, f.spec("mqtt_fields", entry), err
	case "tls":
		if len(w) < 5 {
			return 0, nil, nil
		}
		if w[0] < 20 || w[0] > 23 || w[1] != 3 || w[2] > 4 {
			return 0, nil, fmt.Errorf("invalid TLS record header")
		}
		n := 5 + int(binary.BigEndian.Uint16(w[3:5]))
		if n > 18437 {
			return 0, nil, fmt.Errorf("TLS ciphertext record exceeds limit")
		}
		return n, f.spec("tls", ""), nil
	case "memcached":
		if w[0] == 0x80 {
			if len(w) < 24 {
				return 0, nil, nil
			}
			n := 24 + int(binary.BigEndian.Uint32(w[8:12]))
			if w[1] != 0 {
				return n, nil, nil
			}
			return n, f.spec("memcached_fields", "MemcachedBinaryGetRequestFields"), nil
		}
		if bytes.HasPrefix(w, []byte("stats\r\n")) {
			return 7, f.spec("memcached_fields", "MemcachedStatsRequestFields"), nil
		}
		if bytes.HasPrefix(w, []byte("END\r\n")) {
			return 5, f.spec("memcached_fields", "MemcachedStatsResponseFields"), nil
		}
		if bytes.HasPrefix(w, []byte("STAT ")) {
			if end := bytes.Index(w, []byte("\r\nEND\r\n")); end >= 0 {
				return end + 7, f.spec("memcached_fields", "MemcachedStatsResponseFields"), nil
			}
			return 0, nil, nil
		}
		if len(w) < 7 {
			return 0, nil, nil
		}
		return len(w), nil, nil
	case "cassandra":
		if len(w) < 9 {
			return 0, nil, nil
		}
		n := 9 + int(binary.BigEndian.Uint32(w[5:9]))
		if (w[0] != 4 && w[0] != 0x84) || w[1]&1 != 0 {
			return n, nil, nil
		}
		entry := ""
		switch w[4] {
		case 1:
			entry = "CQLStartup4Fields"
		case 5:
			entry = "CQLOptions4Fields"
		case 6:
			entry = "CQLSupported4Fields"
		}
		if entry == "" {
			return n, nil, nil
		}
		return n, f.spec("cassandra_fields", entry), nil
	case "kerberos":
		if len(w) < 4 {
			return 0, nil, nil
		}
		return 4 + int(binary.BigEndian.Uint32(w[:4])), f.spec("kerberos_fields", "KerberosTCPFields"), nil
	case "dns":
		if len(w) < 2 {
			return 0, nil, nil
		}
		n := 2 + int(binary.BigEndian.Uint16(w[:2]))
		if n < 14 {
			return 0, nil, fmt.Errorf("DNS TCP record is shorter than header")
		}
		return n, f.spec("dns", "DNS"), nil
	}
	return 0, nil, fmt.Errorf("no stream framer for %s", f.protocol)
}

func kerberosTag(tag byte) bool {
	return tag == 0x6a || tag == 0x6b || tag == 0x6c || tag == 0x6d || tag == 0x7e
}
func dnsHeader(w []byte) bool {
	return len(w) >= 12 && (w[2]>>3)&15 <= 5 && binary.BigEndian.Uint16(w[4:6]) <= 256
}

func (a *binParser) datagram(packet gopacket.Packet, udp *layers.UDP) {
	network := packet.NetworkLayer()
	if network == nil {
		return
	}
	if ip, ok := network.(*layers.IPv4); ok && (ip.Version != 4 || ip.FragOffset != 0 || ip.Flags&layers.IPv4MoreFragments != 0) {
		return
	}
	if ip, ok := network.(*layers.IPv6); ok && (ip.Version != 6 || packet.Layer(layers.LayerTypeIPv6Fragment) != nil) {
		return
	}
	a.datagramFields(network, udp, packet.Metadata().CaptureInfo, packet.Metadata().Truncated)
}

// The caller has already excluded IP fragments and validated the network layer.
func (a *binParser) datagramFields(network gopacket.NetworkLayer, udp *layers.UDP, ci gopacket.CaptureInfo, truncated bool) {
	wire := udp.Payload
	a.input.Add(uint64(len(wire)))
	src, dst := network.NetworkFlow().Endpoints()
	e := &BinParserEvent{Timestamp: ci.Timestamp, Transport: "udp", Source: net.JoinHostPort(src.String(), strconv.Itoa(int(udp.SrcPort))), Destination: net.JoinHostPort(dst.String(), strconv.Itoa(int(udp.DstPort))), Length: len(wire), Status: "unrecognized", Summary: "unrecognized UDP datagram"}
	if truncated || ci.CaptureLength < ci.Length || udp.Length < 8 || int(udp.Length) != len(wire)+8 {
		e.Status, e.Summary = "incomplete", "truncated or invalid UDP datagram"
		a.incomplete.Add(1)
	} else if len(wire) > a.config.MaxMessageBytes {
		e.Status, e.Summary = "limited", "UDP datagram exceeds message limit"
		a.limited.Add(uint64(len(wire)))
	} else {
		var spec *binSpec
		if (udp.SrcPort == 53 || udp.DstPort == 53) && dnsHeader(wire) {
			e.Protocol = "dns"
			spec = a.specs["application-layer.dns/DNS"]
		}
		if (udp.SrcPort == 88 || udp.DstPort == 88) && len(wire) >= 2 && kerberosTag(wire[0]) {
			e.Protocol = "kerberos"
			spec = a.specs["application-layer.kerberos_fields/KerberosMessageFields"]
		}
		if spec != nil {
			e.Rule, e.Entry, e.plan = spec.rule, spec.entry, spec.plan
			e.Status, e.Summary = "deferred", e.Protocol
			e.Raw = append([]byte(nil), wire...)
			a.messages.Add(1)
			a.messageBytes.Add(uint64(len(wire)))
			if a.config.Deferred {
				a.deferred.Add(1)
			} else if result, err := e.Decode(); err != nil {
				e.Status, e.Error = "malformed", err.Error()
				a.malformed.Add(1)
			} else {
				e.Status, e.Structured = "decoded", result
				a.decoded.Add(1)
			}
		} else {
			a.unknown.Add(1)
			a.unclassified.Add(uint64(len(wire)))
		}
	}
	if e.Raw == nil {
		e.Raw = append([]byte(nil), wire[:min(len(wire), a.config.ProbeBytes)]...)
	}
	a.emit(e)
}
