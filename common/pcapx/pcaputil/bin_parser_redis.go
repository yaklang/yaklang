package pcaputil

import (
	"bytes"
	"fmt"
	"strconv"
)

type binRedis struct {
	version string
	pending int
}

func probeRedis(w []byte, limit int) ProbeResult {
	if len(w) < 1 {
		return ProbeResult{Verdict: ProbeReject}
	}
	if !redisPrefix(w[0]) {
		return ProbeResult{Verdict: ProbeReject}
	}
	n, err := redisFrameLength(w, 0)
	if err != nil {
		return ProbeResult{Verdict: ProbeReject}
	}
	if n == 0 {
		return probeNeed("redis", redisVersion(w[0]), len(w), min(limit, len(w)+16))
	}
	return probeAccept("redis", redisVersion(w[0]), 80)
}

func redisPrefix(b byte) bool {
	switch b {
	case '*', '$', '+', '-', ':', '_', '#', ',', '(', '!', '=', '%', '~', '>':
		return true
	}
	return false
}

func redisVersion(b byte) string {
	switch b {
	case '_', '#', ',', '(', '!', '=', '%', '~', '>':
		return "RESP3"
	default:
		return "RESP2"
	}
}

func redisFrameLength(w []byte, depth int) (int, error) {
	if depth > 64 {
		return 0, fmt.Errorf("redis: nesting exceeds limit")
	}
	if len(w) < 1 {
		return 0, nil
	}
	nl := bytes.Index(w, []byte("\r\n"))
	switch w[0] {
	case '+', '-', ':', '_', '#', ',':
		if nl < 0 {
			return 0, nil
		}
		return nl + 2, nil
	case '$', '!', '=', '*', '%', '~', '>', '(':
		if nl < 0 {
			if err := redisLengthDigits(w[1:]); err != nil {
				return 0, err
			}
			return 0, nil
		}
		if w[0] == '(' {
			return nl + 2, nil
		}
	default:
		return 0, fmt.Errorf("redis: invalid RESP prefix")
	}
	switch w[0] {
	case '$', '!', '=':
		n, err := strconv.Atoi(string(w[1:nl]))
		if err != nil {
			return 0, fmt.Errorf("redis: invalid bulk length")
		}
		if n < 0 {
			return nl + 2, nil
		}
		need := nl + 2 + n + 2
		if need > len(w) {
			return 0, nil
		}
		return need, nil
	case '*', '%', '~', '>':
		if nl < 0 {
			return 0, nil
		}
		count, err := strconv.Atoi(string(w[1:nl]))
		if err != nil {
			return 0, fmt.Errorf("redis: invalid aggregate count")
		}
		if count < 0 {
			return nl + 2, nil
		}
		if w[0] == '%' {
			count *= 2
		}
		if count > 4096 {
			return 0, fmt.Errorf("redis: aggregate exceeds limit")
		}
		at := nl + 2
		for i := 0; i < count; i++ {
			if at >= len(w) {
				return 0, nil
			}
			n, err := redisFrameLength(w[at:], depth+1)
			if err != nil {
				return 0, err
			}
			if n == 0 {
				return 0, nil
			}
			at += n
		}
		return at, nil
	default:
		return 0, fmt.Errorf("redis: invalid RESP prefix")
	}
}

func redisLengthDigits(w []byte) error {
	if len(w) == 0 {
		return nil
	}
	i := 0
	if w[0] == '-' {
		i = 1
	}
	if i >= len(w) {
		return nil
	}
	for ; i < len(w); i++ {
		if w[i] == '\r' {
			return nil
		}
		if w[i] < '0' || w[i] > '9' {
			return fmt.Errorf("redis: invalid length digits")
		}
	}
	return nil
}

func (f *binFlow) frameRedis(w []byte) (int, *binSpec, error) {
	if f.redis == nil {
		return 0, nil, sessionContext("Redis session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	n, err := redisFrameLength(w, 0)
	if err != nil {
		return 0, nil, err
	}
	if n == 0 {
		return 0, nil, nil
	}
	if n > f.a.config.MaxMessageBytes {
		return f.a.config.MaxMessageBytes + 1, nil, nil
	}
	return n, f.spec("redis", "Redis"), nil
}

func (r *binRedis) consume(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("redis: empty value")
	}
	ver := redisVersion(raw[0])
	if r.version == "" {
		r.version = ver
	}
	info := map[string]any{
		"RESP Type":     redisTypeName(raw[0]),
		"Version":       ver,
		"Context Level": "observed",
	}
	if raw[0] == '*' || raw[0] == '>' {
		r.pending++
		info["Pipeline"] = r.pending
	} else if raw[0] == '+' || raw[0] == '-' || raw[0] == ':' || raw[0] == '$' {
		if r.pending > 0 {
			r.pending--
		}
	}
	if raw[0] == '>' {
		info["Push"] = true
	}
	return info, nil
}

func redisTypeName(b byte) string {
	switch b {
	case '*':
		return "Array"
	case '$':
		return "Bulk"
	case '+':
		return "Simple"
	case '-':
		return "Error"
	case ':':
		return "Integer"
	case '_':
		return "Null"
	case '#':
		return "Boolean"
	case ',':
		return "Double"
	case '(':
		return "BigNumber"
	case '!':
		return "BlobError"
	case '=':
		return "Verbatim"
	case '%':
		return "Map"
	case '~':
		return "Set"
	case '>':
		return "Push"
	}
	return "Unknown"
}
