package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
)

type binBeanstalk struct {
	client  int
	pending []string
}

func beanstalkUint(s string, max uint64) (uint64, bool) {
	if s == "" || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil && n <= max
}

func beanstalkFirstLine(w []byte) ([]string, int, error) {
	i := bytes.Index(w, []byte("\r\n"))
	if i < 0 {
		if len(w) > 4096 || bytes.ContainsAny(w, "\x00\n") {
			return nil, 0, fmt.Errorf("beanstalkd: invalid or oversized line")
		}
		return nil, 0, nil
	}
	if i > 4096 || i == 0 {
		return nil, 0, fmt.Errorf("beanstalkd: invalid line")
	}
	line := w[:i]
	for _, b := range line {
		if b < 0x20 || b > 0x7e {
			return nil, 0, fmt.Errorf("beanstalkd: control byte in line")
		}
	}
	return strings.Fields(string(line)), i + 2, nil
}

func probeBeanstalk(w []byte, limit int) ProbeResult {
	if !bytes.HasPrefix(w, []byte("put ")) && !bytes.HasPrefix([]byte("put "), w) {
		return ProbeResult{Verdict: ProbeReject}
	}
	fields, _, err := beanstalkFirstLine(w)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if fields == nil {
		if len(w) >= limit {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeNeed("beanstalkd", "text-v1", len(w), limit)
	}
	if len(fields) != 5 || fields[0] != "put" {
		return ProbeResult{Verdict: ProbeReject}
	}
	for i, maximum := range []uint64{^uint64(0), 1 << 32, 1 << 32, 1 << 20} {
		if _, ok := beanstalkUint(fields[i+1], maximum); !ok {
			return ProbeResult{Verdict: ProbeReject}
		}
	}
	return probeAccept("beanstalkd", "text-v1", 96)
}

func (f *binFlow) frameBeanstalk(dir int, w []byte) (int, *binSpec, error) {
	fields, header, err := beanstalkFirstLine(w)
	if err != nil || header == 0 {
		return 0, nil, err
	}
	if len(fields) == 0 {
		return 0, nil, fmt.Errorf("beanstalkd: empty message")
	}
	dataBytes := uint64(0)
	if dir == f.beanstalk.client {
		switch fields[0] {
		case "put":
			if len(fields) != 5 {
				return 0, nil, fmt.Errorf("beanstalkd: invalid put header")
			}
			for i, maximum := range []uint64{^uint64(0), 1 << 32, 1 << 32, uint64(f.a.config.MaxMessageBytes)} {
				n, ok := beanstalkUint(fields[i+1], maximum)
				if !ok {
					return 0, nil, fmt.Errorf("beanstalkd: invalid put number")
				}
				if i == 3 {
					dataBytes = n
				}
			}
		case "reserve":
			if len(fields) != 1 {
				return 0, nil, fmt.Errorf("beanstalkd: invalid reserve")
			}
		case "delete":
			if len(fields) != 2 {
				return 0, nil, fmt.Errorf("beanstalkd: invalid delete")
			}
			if _, ok := beanstalkUint(fields[1], ^uint64(0)); !ok {
				return 0, nil, fmt.Errorf("beanstalkd: invalid job ID")
			}
		default:
			return 0, nil, fmt.Errorf("beanstalkd: unsupported command")
		}
	} else {
		switch fields[0] {
		case "INSERTED":
			if len(fields) != 2 {
				return 0, nil, fmt.Errorf("beanstalkd: invalid INSERTED")
			}
		case "RESERVED":
			if len(fields) != 3 {
				return 0, nil, fmt.Errorf("beanstalkd: invalid RESERVED")
			}
			n, ok := beanstalkUint(fields[2], uint64(f.a.config.MaxMessageBytes))
			if !ok {
				return 0, nil, fmt.Errorf("beanstalkd: invalid reserve body length")
			}
			dataBytes = n
		case "DELETED":
			if len(fields) != 1 {
				return 0, nil, fmt.Errorf("beanstalkd: invalid DELETED")
			}
		default:
			return 0, nil, fmt.Errorf("beanstalkd: unsupported response")
		}
	}
	total := header
	if fields[0] == "put" || fields[0] == "RESERVED" {
		if header+2 > f.a.config.MaxMessageBytes || dataBytes > uint64(f.a.config.MaxMessageBytes-header-2) {
			return 0, nil, fmt.Errorf("beanstalkd: body exceeds frame budget")
		}
		total += int(dataBytes) + 2
		if len(w) >= total && !bytes.Equal(w[total-2:total], []byte("\r\n")) {
			return 0, nil, fmt.Errorf("beanstalkd: missing body delimiter")
		}
	}
	return total, f.a.specs["application-layer.beanstalkd/BeanstalkMessage"], nil
}

func (s *binBeanstalk) consume(dir int, raw []byte, budget int) (map[string]any, error) {
	fields, header, err := beanstalkFirstLine(raw)
	if err != nil || header == 0 || len(fields) == 0 {
		return nil, fmt.Errorf("beanstalkd: invalid complete message")
	}
	out := map[string]any{"Packet Name": strings.ToUpper(fields[0])}
	if dir == s.client {
		if len(s.pending) >= budget {
			return nil, fmt.Errorf("beanstalkd: pending budget exceeded")
		}
		s.pending = append(s.pending, fields[0])
		switch fields[0] {
		case "put":
			priority, _ := strconv.ParseUint(fields[1], 10, 64)
			ttr, _ := strconv.ParseUint(fields[3], 10, 64)
			out["Priority"], out["TTR"] = priority, ttr
			out["Body"] = bytes.Clone(raw[header : len(raw)-2])
		case "delete":
			out["Job ID"] = fields[1]
		}
		return out, nil
	}
	if len(s.pending) == 0 {
		return nil, fmt.Errorf("beanstalkd: unsolicited response")
	}
	want := s.pending[0]
	s.pending = s.pending[1:]
	if want == "put" && fields[0] != "INSERTED" || want == "reserve" && fields[0] != "RESERVED" || want == "delete" && fields[0] != "DELETED" {
		return nil, fmt.Errorf("beanstalkd: response does not match request")
	}
	out["In Reply To"] = want
	if fields[0] == "INSERTED" || fields[0] == "RESERVED" {
		if _, ok := beanstalkUint(fields[1], ^uint64(0)); !ok {
			return nil, fmt.Errorf("beanstalkd: invalid response job ID")
		}
		out["Job ID"] = fields[1]
	}
	if fields[0] == "RESERVED" {
		out["Body"] = bytes.Clone(raw[header : len(raw)-2])
	}
	return out, nil
}
