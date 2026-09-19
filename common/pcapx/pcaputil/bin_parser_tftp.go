package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// TFTP is datagram-only and changes server TID after RRQ/WRQ. File bytes are
// never accumulated: the transfer state retains sequence/digest metadata only.
type binTFTP struct {
	write                            bool
	request                          [32]byte
	filename, mode                   string
	options                          map[string]string
	blockSize                        int
	expected, block                  uint16
	pending, final, done, negotiated bool
	hasData                          bool
	digest                           [32]byte
	server                           string
}

func tftpStrings(w []byte, max int) ([]string, error) {
	if len(w) == 0 || w[len(w)-1] != 0 {
		return nil, fmt.Errorf("tftp: unterminated string")
	}
	parts := bytes.Split(w[:len(w)-1], []byte{0})
	if len(parts) > max {
		return nil, protocolError(ErrResourceExceeded, "TFTP option budget")
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		if len(p) == 0 {
			return nil, fmt.Errorf("tftp: empty string")
		}
		out[i] = string(p)
	}
	return out, nil
}
func tftpOptions(parts []string) (map[string]string, error) {
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("tftp: incomplete option pair")
	}
	out := map[string]string{}
	for i := 0; i < len(parts); i += 2 {
		k := strings.ToLower(parts[i])
		if _, ok := out[k]; ok {
			return nil, fmt.Errorf("tftp: duplicate option")
		}
		out[k] = parts[i+1]
	}
	return out, nil
}
func probeTFTP(w []byte, max int) bool {
	if len(w) < 6 || w[0] != 0 || (w[1] != 1 && w[1] != 2) {
		return false
	}
	p, err := tftpStrings(w[2:], max*2+2)
	if err != nil || len(p) < 2 {
		return false
	}
	mode := strings.ToLower(p[1])
	if mode != "octet" && mode != "netascii" {
		return false
	}
	_, err = tftpOptions(p[2:])
	return err == nil
}
func (s *binTFTP) consume(dir int, w []byte, max int) (map[string]any, error) {
	if len(w) < 2 {
		return nil, fmt.Errorf("tftp: missing opcode")
	}
	op := binary.BigEndian.Uint16(w)
	out := map[string]any{"Opcode": op, "Filename": s.filename, "Mode": s.mode}
	switch op {
	case 1, 2:
		if dir != 0 {
			return nil, fmt.Errorf("tftp: server sent transfer request")
		}
		parts, err := tftpStrings(w[2:], max*2+2)
		if err != nil {
			return nil, err
		}
		if len(parts) < 2 {
			return nil, fmt.Errorf("tftp: missing mode")
		}
		opts, err := tftpOptions(parts[2:])
		if err != nil {
			return nil, err
		}
		mode := strings.ToLower(parts[1])
		if mode != "octet" && mode != "netascii" {
			return nil, protocolError(ErrUnsupportedFeature, "TFTP mode")
		}
		digest := sha256.Sum256(w)
		if s.filename != "" {
			if s.request != digest {
				return nil, protocolError(ErrContextRequired, "different TFTP request on active transfer")
			}
			out["Retransmission"] = true
		} else {
			s.filename, s.mode, s.options, s.request, s.write, s.blockSize, s.expected = parts[0], mode, opts, digest, op == 2, 512, 1
			if s.write {
				s.pending = true
				s.block = 0
			}
		}
		out["Packet Name"] = map[uint16]string{1: "RRQ", 2: "WRQ"}[op]
		out["Filename"] = s.filename
		out["Mode"] = s.mode
		out["Options"] = cloneSessionStringMap(opts)
	case 6:
		if dir != 1 {
			return nil, fmt.Errorf("tftp: client OACK")
		}
		parts, err := tftpStrings(w[2:], max*2)
		if err != nil {
			return nil, err
		}
		opts, err := tftpOptions(parts)
		if err != nil {
			return nil, err
		}
		bs := 512
		for k, v := range opts {
			asked, ok := s.options[k]
			if !ok {
				return nil, fmt.Errorf("tftp: unrequested option %s", k)
			}
			n, err := strconv.ParseUint(v, 10, 32)
			if err != nil {
				return nil, fmt.Errorf("tftp: invalid option value")
			}
			switch k {
			case "blksize":
				a, e := strconv.ParseUint(asked, 10, 32)
				if e != nil || n < 8 || n > 65464 || n > a {
					return nil, fmt.Errorf("tftp: invalid negotiated block size")
				}
				bs = int(n)
			case "timeout":
				if v != asked || n < 1 || n > 255 {
					return nil, fmt.Errorf("tftp: invalid timeout")
				}
			case "tsize":
				if s.write && v != asked {
					return nil, fmt.Errorf("tftp: transfer size mismatch")
				}
			case "windowsize":
				if n != 1 {
					return nil, protocolError(ErrUnsupportedFeature, "TFTP windowed transfer")
				}
			default:
				return nil, protocolError(ErrUnsupportedFeature, "TFTP option "+k)
			}
		}
		if s.hasData {
			return nil, fmt.Errorf("tftp: OACK after transfer started")
		}
		s.blockSize = bs
		s.negotiated = true
		s.pending = !s.write
		s.block = 0
		out["Packet Name"] = "OACK"
		out["Options"] = cloneSessionStringMap(opts)
	case 3:
		dataDir := 1
		if s.write {
			dataDir = 0
		}
		if dir != dataDir || len(w) < 4 {
			return nil, fmt.Errorf("tftp: invalid DATA direction/header")
		}
		block := binary.BigEndian.Uint16(w[2:])
		size := len(w) - 4
		if size > s.blockSize {
			return nil, fmt.Errorf("tftp: DATA exceeds negotiated block size")
		}
		digest := sha256.Sum256(w[4:])
		if block == s.block && s.hasData {
			if digest != s.digest {
				return nil, protocolError(ErrDesynchronized, "TFTP retransmission changed bytes")
			}
			out["Retransmission"] = true
		} else {
			if s.done || s.pending || block != s.expected {
				return nil, protocolError(ErrContextRequired, "TFTP DATA missing preceding exchange")
			}
			s.block, s.digest, s.pending, s.final = block, digest, true, size < s.blockSize
			s.hasData = true
		}
		out["Packet Name"] = "DATA"
		out["Block"] = block
		out["Data Length"] = size
		out["Final Block"] = s.final
	case 4:
		ackDir := 0
		if s.write {
			ackDir = 1
		}
		if dir != ackDir || len(w) != 4 {
			return nil, fmt.Errorf("tftp: invalid ACK")
		}
		block := binary.BigEndian.Uint16(w[2:])
		if block != s.block || (!s.hasData && !s.negotiated && !s.write) {
			return nil, protocolError(ErrContextRequired, "TFTP ACK does not match DATA")
		}
		if !s.pending {
			out["Retransmission"] = true
		} else {
			s.pending = false
			s.expected = block + 1
			if s.final {
				s.done = true
			}
		}
		out["Packet Name"] = "ACK"
		out["Block"] = block
		out["Transfer Complete"] = s.done
	case 5:
		if len(w) < 5 || w[len(w)-1] != 0 {
			return nil, fmt.Errorf("tftp: invalid ERROR")
		}
		out["Packet Name"] = "ERROR"
		out["Error Code"] = binary.BigEndian.Uint16(w[2:])
		out["Error Message"] = string(w[4 : len(w)-1])
		s.done = true
	default:
		return nil, fmt.Errorf("tftp: unknown opcode")
	}
	return out, nil
}
func cloneSessionStringMap(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
