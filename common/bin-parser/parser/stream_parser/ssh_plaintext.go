package stream_parser

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

// These are initial, uncompressed, no-MAC packet layouts, not a connection
// decoder. Message numbers 30..49 have no meaning without a caller-selected
// exchange context. The 35000-byte total limit is local, not a wire maximum.
func parseSSHPlaintextPacket(node *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
	if !sshPlaintextProfile(profile) {
		return fmt.Errorf("ssh-plaintext: unknown exchange profile")
	}
	bits, bounded, err := parseLengthByLengthConfig(node)
	if err != nil {
		return err
	}
	if !bounded || bits < 16*8 || bits > 35000*8 || bits%64 != 0 {
		return fmt.Errorf("ssh-plaintext: explicit 16..35000 byte, eight-byte-aligned packet boundary required")
	}
	return parseCertificateFieldTree(node, process, func(w []byte) ([]tlsCertificateField, map[string]any, error) {
		return decodeSSHPlaintextPacket(w, profile)
	}, "ssh-plaintext")
}

func sshPlaintextProfile(profile string) bool {
	switch profile {
	case "transport", "dh", "dh-gex", "ecdh-nistp256", "ecdh-curve25519":
		return true
	}
	return false
}

type sshPlaintextReader struct {
	wire    []byte
	at, end int
	err     error
	fields  []tlsCertificateField
}

func (r *sshPlaintextReader) take(name, typ string, size int) []byte {
	if r.err != nil {
		return nil
	}
	if size < 0 || size > r.end-r.at {
		r.err = fmt.Errorf("ssh-plaintext: truncated %s", name)
		return nil
	}
	start := r.at
	r.at += size
	r.fields = append(r.fields, tlsCertificateField{Name: name, Type: typ, Start: start, End: r.at})
	return r.wire[start:r.at]
}

func (r *sshPlaintextReader) u32(name string) uint32 {
	b := r.take(name, "uint32", 4)
	if len(b) != 4 {
		return 0
	}
	return binary.BigEndian.Uint32(b)
}

func (r *sshPlaintextReader) str(name, typ string) []byte {
	size := r.u32(name + " Length")
	if r.err != nil {
		return nil
	}
	// Compare before converting to int, including on 32-bit platforms.
	if uint64(size) > uint64(r.end-r.at) {
		r.err = fmt.Errorf("ssh-plaintext: %s exceeds its enclosing string or payload", name)
		return nil
	}
	return r.take(name, typ, int(size))
}

func (r *sshPlaintextReader) fixedString(name string, size int) {
	b := r.str(name, "raw")
	if r.err == nil && len(b) != size {
		r.err = fmt.Errorf("ssh-plaintext: %s requires %d bytes", name, size)
	}
}

// DH values and RSA public components are nonnegative mpints. Zero is a valid
// encoding (empty), not evidence of a usable key/group. Never do key arithmetic.
func (r *sshPlaintextReader) mpint(name string) {
	b := r.str(name, "raw")
	if len(b) > 0 && (b[0]&0x80 != 0 || b[0] == 0 && (len(b) == 1 || b[1]&0x80 == 0)) {
		r.err = fmt.Errorf("ssh-plaintext: noncanonical nonnegative mpint %s", name)
	}
}

func (r *sshPlaintextReader) done() error {
	if r.err != nil {
		return r.err
	}
	if r.at != r.end {
		return fmt.Errorf("ssh-plaintext: trailing bytes in selected layout")
	}
	return nil
}

func sshPlaintextName(name []byte) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if c <= 32 || c >= 127 || c == ',' {
			return false
		}
	}
	return true
}

func (r *sshPlaintextReader) nameList(name string, empty bool) {
	b := r.str(name, "string")
	if r.err != nil {
		return
	}
	if len(b) == 0 && empty {
		return
	}
	for _, part := range bytes.Split(b, []byte{','}) {
		if !sshPlaintextName(part) {
			r.err = fmt.Errorf("ssh-plaintext: invalid name in %s", name)
			return
		}
	}
}

// The outer SSH string becomes a composite without overlapping raw leaves.
func (r *sshPlaintextReader) nested(name string, decode func(*sshPlaintextReader)) {
	b := r.str(name, "raw")
	if r.err != nil {
		return
	}
	sub := &sshPlaintextReader{wire: r.wire, at: r.at - len(b), end: r.at}
	decode(sub)
	if err := sub.done(); err != nil {
		r.err = err
		return
	}
	f := &r.fields[len(r.fields)-1]
	f.Type, f.Children = "", sub.fields
}

func (r *sshPlaintextReader) point(name string) {
	r.nested(name, func(sub *sshPlaintextReader) {
		format := sub.take(name+" Format", "uint8", 1)
		if len(format) == 0 {
			return
		}
		switch format[0] {
		case 4:
			sub.take(name+" X", "raw", 32)
			sub.take(name+" Y", "raw", 32)
		case 2, 3:
			sub.take(name+" X", "raw", 32)
		default:
			sub.err = fmt.Errorf("ssh-plaintext: unsupported P-256 point encoding")
		}
	})
}

func (r *sshPlaintextReader) reply(exchange string, info map[string]any) {
	r.nested("Host Key", func(sub *sshPlaintextReader) {
		format := sub.str("Host Key Format", "string")
		if !sshPlaintextName(format) {
			sub.err = fmt.Errorf("ssh-plaintext: invalid host key format identifier")
			return
		}
		info["Host Key Format"] = string(format)
		switch string(format) {
		case "ssh-rsa":
			sub.mpint("RSA Exponent")
			sub.mpint("RSA Modulus")
			info["Host Key Fields Decoded"] = true
		case "ssh-ed25519":
			sub.fixedString("Ed25519 Public Key", 32)
			info["Host Key Fields Decoded"] = true
		case "ssh-ed448":
			sub.fixedString("Ed448 Public Key", 57)
			info["Host Key Fields Decoded"] = true
		default:
			sub.take("Opaque Host Key Data", "raw", sub.end-sub.at)
		}
	})
	if exchange == "curve25519" {
		b := r.str("Server Curve25519 Public Key", "raw")
		if len(b) != 32 {
			r.err = fmt.Errorf("ssh-plaintext: Curve25519 public key requires 32 bytes")
		}
	} else if exchange == "ecdh" {
		r.point("Server Point")
	} else {
		r.mpint("Server Exchange Value")
	}
	r.nested("Signature", func(sub *sshPlaintextReader) {
		format := sub.str("Signature Format", "string")
		if !sshPlaintextName(format) {
			sub.err = fmt.Errorf("ssh-plaintext: invalid signature format identifier")
			return
		}
		info["Signature Format"] = string(format)
		switch string(format) {
		case "ssh-rsa", "rsa-sha2-256", "rsa-sha2-512":
			sub.str("RSA Signature Bytes", "raw")
			info["Signature Fields Decoded"] = true
		case "ssh-ed25519":
			sub.fixedString("Ed25519 Signature Bytes", 64)
			info["Signature Fields Decoded"] = true
		case "ssh-ed448":
			sub.fixedString("Ed448 Signature Bytes", 114)
			info["Signature Fields Decoded"] = true
		default:
			// Other format identifiers need not have the RSA string grammar.
			sub.take("Opaque Signature Data", "raw", sub.end-sub.at)
		}
	})
}

func decodeSSHPlaintextPacket(wire []byte, profile string) ([]tlsCertificateField, map[string]any, error) {
	if !sshPlaintextProfile(profile) || len(wire) < 16 || len(wire) > 35000 || len(wire)%8 != 0 {
		return nil, nil, fmt.Errorf("ssh-plaintext: invalid profile or packet boundary")
	}
	if uint64(binary.BigEndian.Uint32(wire)) != uint64(len(wire)-4) || wire[4] < 4 || int(wire[4])+6 > len(wire) {
		return nil, nil, fmt.Errorf("ssh-plaintext: invalid packet or padding length")
	}
	end := len(wire) - int(wire[4])
	r := &sshPlaintextReader{wire: wire, at: 5, end: end}
	msg := r.take("Message Number", "uint8", 1)[0]
	info := map[string]any{
		"Profile": "SSH initial plaintext packet", "Exchange Layout": profile,
		"Plaintext Context Supplied By Caller": true, "Payload Layout Decoded": true,
		"Host Key Fields Decoded": false, "Signature Fields Decoded": false,
		"Negotiation Validated": false, "Key Mathematics Validated": false,
		"Signature Validated": false, "Peer Identity Validated": false,
		"Handshake Completion Validated": false, "TCP Reassembly Performed": false,
		"Structured Generation Supported": false,
	}
	switch {
	case msg == 20:
		r.take("Cookie", "raw", 16)
		for i, name := range []string{"Kex Algorithms", "Host Key Algorithms", "Encryption C2S", "Encryption S2C", "MAC C2S", "MAC S2C", "Compression C2S", "Compression S2C", "Languages C2S", "Languages S2C"} {
			r.nameList(name, i >= 8)
		}
		// RFC 4251 boolean: zero is false, every nonzero value is true.
		r.take("First Kex Packet Follows", "uint8", 1)
		r.u32("Reserved") // ignored on receipt, retained verbatim
	case msg == 21:
		// NEWKEYS has no additional payload. It does not prove a transition.
	case profile == "dh-gex" && msg == 34:
		r.u32("Minimum Group Bits")
		r.u32("Preferred Group Bits")
		r.u32("Maximum Group Bits")
	case profile == "dh-gex" && msg == 30:
		r.u32("Preferred Group Bits")
	case profile == "dh-gex" && msg == 31:
		r.mpint("Group Prime")
		r.mpint("Group Generator")
	case profile == "dh-gex" && msg == 32 || profile == "dh" && msg == 30:
		r.mpint("Client Exchange Value")
	case profile == "dh-gex" && msg == 33 || profile == "dh" && msg == 31:
		r.reply("dh", info)
	case profile == "ecdh-nistp256" && msg == 30:
		r.point("Client Point")
	case profile == "ecdh-nistp256" && msg == 31:
		r.reply("ecdh", info)
	case profile == "ecdh-curve25519" && msg == 30:
		b := r.str("Client Curve25519 Public Key", "raw")
		if len(b) != 32 {
			r.err = fmt.Errorf("ssh-plaintext: Curve25519 public key requires 32 bytes")
		}
	case profile == "ecdh-curve25519" && msg == 31:
		r.reply("curve25519", info)
	default:
		info["Payload Layout Decoded"] = false
		r.take("Opaque Message Data", "raw", r.end-r.at)
	}
	if err := r.done(); err != nil {
		return nil, nil, err
	}
	return []tlsCertificateField{
		{Name: "Packet Length", Type: "uint32", Start: 0, End: 4},
		{Name: "Padding Length", Type: "uint8", Start: 4, End: 5},
		{Name: "Payload", Start: 5, End: end, Children: r.fields},
		{Name: "Padding", Type: "raw", Start: end, End: len(wire)},
	}, info, nil
}

// DecodeSSHInitialPacket validates exactly the grammar used by the public
// bounded YAML entry. The caller must supply the initial, unencrypted boundary.
// It does not authenticate a host key, signature, or negotiated session.
// Wire Fields maps retain their direct literal fields' half-open byte ranges
// relative to this owned PDU, not to a contributing captured frame.
func DecodeSSHInitialPacket(wire []byte, profile string) (map[string]any, error) {
	fields, info, err := decodeSSHPlaintextPacket(wire, profile)
	if err != nil {
		return nil, err
	}
	var values func([]tlsCertificateField) map[string]any
	values = func(fs []tlsCertificateField) map[string]any {
		out := map[string]any{}
		ranges := map[string]any{}
		for _, f := range fs {
			if len(f.Children) > 0 {
				out[f.Name] = values(f.Children)
				continue
			}
			b := wire[f.Start:f.End]
			switch f.Type {
			case "uint8":
				out[f.Name] = b[0]
			case "uint32":
				out[f.Name] = binary.BigEndian.Uint32(b)
			case "string":
				out[f.Name] = string(b)
			default:
				out[f.Name] = bytes.Clone(b)
			}
			// The staged grammar already owns these positions. Keep each
			// occurrence in its immediate map, including zero-width strings;
			// equal values elsewhere cannot establish its wire location.
			ranges[f.Name] = []any{uint64(f.Start), uint64(f.End)}
		}
		out["Wire Field Ranges"] = ranges
		return out
	}
	info["Wire Fields"] = values(fields)
	return info, nil
}
