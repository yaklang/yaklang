package pcaputil

import (
	"encoding/binary"
	"fmt"
	"net"
)

// KRPC is a complete bencoded UDP datagram. A port hint is insufficient: the
// outer dictionary, transaction, node ID, and method-specific arguments must
// all agree before the datagram can be labeled as BitTorrent DHT.
type dhtBencode struct {
	w        []byte
	at       int
	elements int
	limit    int
}

func (p *dhtBencode) count() error {
	p.elements++
	if p.elements > p.limit {
		return fmt.Errorf("dht: collection budget exceeded")
	}
	return nil
}

func (p *dhtBencode) bytes() ([]byte, error) {
	if p.at >= len(p.w) || p.w[p.at] < '0' || p.w[p.at] > '9' {
		return nil, fmt.Errorf("dht: invalid string length")
	}
	start := p.at
	n := 0
	for p.at < len(p.w) && p.w[p.at] >= '0' && p.w[p.at] <= '9' {
		if n > len(p.w)/10 {
			return nil, fmt.Errorf("dht: string length overflow")
		}
		n = n*10 + int(p.w[p.at]-'0')
		p.at++
	}
	if p.at >= len(p.w) || p.w[p.at] != ':' || p.at-start > 1 && p.w[start] == '0' {
		return nil, fmt.Errorf("dht: noncanonical string length")
	}
	p.at++
	if n > len(p.w)-p.at {
		return nil, fmt.Errorf("dht: truncated string")
	}
	out := p.w[p.at : p.at+n]
	p.at += n
	return out, nil
}

func (p *dhtBencode) value(depth int) (any, error) {
	if depth > 16 || p.at >= len(p.w) {
		return nil, fmt.Errorf("dht: truncated or nested beyond budget")
	}
	if err := p.count(); err != nil {
		return nil, err
	}
	switch p.w[p.at] {
	case 'd':
		p.at++
		out := map[string]any{}
		var previous string
		for p.at < len(p.w) && p.w[p.at] != 'e' {
			if err := p.count(); err != nil {
				return nil, err
			}
			key, err := p.bytes()
			if err != nil || len(key) == 0 || len(out) > 0 && string(key) <= previous {
				return nil, fmt.Errorf("dht: invalid or unordered dictionary key")
			}
			previous = string(key)
			value, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out[previous] = value
		}
		if p.at >= len(p.w) {
			return nil, fmt.Errorf("dht: unterminated dictionary")
		}
		p.at++
		return out, nil
	case 'l':
		p.at++
		var out []any
		for p.at < len(p.w) && p.w[p.at] != 'e' {
			value, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			out = append(out, value)
		}
		if p.at >= len(p.w) {
			return nil, fmt.Errorf("dht: unterminated list")
		}
		p.at++
		return out, nil
	case 'i':
		p.at++
		start := p.at
		negative := p.at < len(p.w) && p.w[p.at] == '-'
		if negative {
			p.at++
		}
		if p.at >= len(p.w) || p.w[p.at] < '0' || p.w[p.at] > '9' ||
			p.w[p.at] == '0' && (negative || p.at+1 < len(p.w) && p.w[p.at+1] != 'e') {
			return nil, fmt.Errorf("dht: invalid integer")
		}
		var n int64
		for p.at < len(p.w) && p.w[p.at] >= '0' && p.w[p.at] <= '9' {
			if n > (1<<63-1)/10-9 {
				return nil, fmt.Errorf("dht: integer overflow")
			}
			n = n*10 + int64(p.w[p.at]-'0')
			p.at++
		}
		if p.at >= len(p.w) || p.w[p.at] != 'e' || p.at == start {
			return nil, fmt.Errorf("dht: unterminated integer")
		}
		p.at++
		if negative {
			n = -n
		}
		return n, nil
	default:
		return p.bytes()
	}
}

func dhtString(fields map[string]any, key string) []byte {
	value, _ := fields[key].([]byte)
	return value
}

func dhtNodeID(fields map[string]any) bool { return len(dhtString(fields, "id")) == 20 }

func decodeDHTMessage(w []byte, maxElements int) (map[string]any, error) {
	if len(w) < 18 || len(w) > 1<<20 || w[0] != 'd' {
		return nil, fmt.Errorf("dht: invalid datagram length or prefix")
	}
	if maxElements <= 0 {
		maxElements = 4096
	}
	p := &dhtBencode{w: w, limit: maxElements}
	value, err := p.value(0)
	if err != nil {
		return nil, err
	}
	if p.at != len(w) {
		return nil, fmt.Errorf("dht: invalid or trailing bencode")
	}
	root, ok := value.(map[string]any)
	if !ok || len(dhtString(root, "t")) == 0 || len(dhtString(root, "t")) > 16 || len(dhtString(root, "y")) != 1 {
		return nil, fmt.Errorf("dht: missing transaction or kind")
	}
	out := map[string]any{"Transaction ID": append([]byte(nil), dhtString(root, "t")...)}
	switch string(dhtString(root, "y")) {
	case "q":
		args, ok := root["a"].(map[string]any)
		if !ok || !dhtNodeID(args) {
			return nil, fmt.Errorf("dht: request lacks 20-byte node ID")
		}
		method := string(dhtString(root, "q"))
		switch method {
		case "ping":
		case "find_node":
			if len(dhtString(args, "target")) != 20 {
				return nil, fmt.Errorf("dht: find_node target length")
			}
			out["Target"] = append([]byte(nil), dhtString(args, "target")...)
		case "get_peers":
			if len(dhtString(args, "info_hash")) != 20 {
				return nil, fmt.Errorf("dht: get_peers info hash length")
			}
			out["Info Hash"] = append([]byte(nil), dhtString(args, "info_hash")...)
		case "announce_peer":
			if len(dhtString(args, "info_hash")) != 20 || len(dhtString(args, "token")) == 0 || len(dhtString(args, "token")) > 64 {
				return nil, fmt.Errorf("dht: announce_peer fields")
			}
			port, portOK := args["port"].(int64)
			implied, impliedOK := args["implied_port"].(int64)
			if !portOK && implied != 1 || portOK && (port < 0 || port > 65535) || impliedOK && implied != 0 && implied != 1 {
				return nil, fmt.Errorf("dht: announce_peer port")
			}
			out["Info Hash"] = append([]byte(nil), dhtString(args, "info_hash")...)
			out["Token"] = append([]byte(nil), dhtString(args, "token")...)
			out["Port"] = port
		default:
			return nil, fmt.Errorf("dht: unsupported query %q", method)
		}
		out["Packet Name"], out["Node ID"] = method, append([]byte(nil), dhtString(args, "id")...)
	case "r":
		response, ok := root["r"].(map[string]any)
		if !ok || !dhtNodeID(response) {
			return nil, fmt.Errorf("dht: response lacks 20-byte node ID")
		}
		if nodes := dhtString(response, "nodes"); nodes != nil {
			if len(nodes)%26 != 0 {
				return nil, fmt.Errorf("dht: compact nodes length")
			}
			out["Nodes Length"] = len(nodes)
			if len(nodes) >= 26 {
				out["First Node ID"] = append([]byte(nil), nodes[:20]...)
				out["First Node IP"] = net.IP(nodes[20:24]).String()
				out["First Node Port"] = binary.BigEndian.Uint16(nodes[24:26])
			}
		}
		if rawPeers, exists := response["values"]; exists {
			peers, ok := rawPeers.([]any)
			if !ok || len(peers) == 0 || len(peers) > maxElements {
				return nil, fmt.Errorf("dht: invalid compact peer list")
			}
			for _, rawPeer := range peers {
				peer, ok := rawPeer.([]byte)
				if !ok || len(peer) != 6 {
					return nil, fmt.Errorf("dht: invalid compact peer address")
				}
			}
			out["Peer Count"] = len(peers)
		}
		if token := dhtString(response, "token"); token != nil {
			if len(token) == 0 || len(token) > 64 {
				return nil, fmt.Errorf("dht: invalid response token")
			}
			out["Token"] = append([]byte(nil), token...)
		}
		out["Packet Name"], out["Node ID"] = "response", append([]byte(nil), dhtString(response, "id")...)
	case "e":
		failure, ok := root["e"].([]any)
		if !ok || len(failure) != 2 {
			return nil, fmt.Errorf("dht: invalid error response")
		}
		code, codeOK := failure[0].(int64)
		message, messageOK := failure[1].([]byte)
		if !codeOK || code < 201 || code > 204 || !messageOK || len(message) == 0 || len(message) > 256 {
			return nil, fmt.Errorf("dht: invalid error code or message")
		}
		out["Packet Name"], out["Error Code"], out["Error Message"] = "error", code, string(message)
	default:
		return nil, fmt.Errorf("dht: invalid message kind")
	}
	return out, nil
}

func validDHTMessage(w []byte) bool {
	_, err := decodeDHTMessage(w, 4096)
	return err == nil
}
