package pcaputil

import (
	"bytes"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type binRedis struct {
	version   string
	pending   int
	client    int
	roleKnown bool
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
	if n == 0 && !bytes.Contains(w, []byte("\r\n")) {
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
	used := 0
	return redisFrameLengthBudget(w, depth, DefaultParserBudget(), &used)
}

func redisFrameLengthBudget(w []byte, depth int, budget ParserBudget, used *int) (int, error) {
	if depth >= budget.MaxRecursionDepth {
		return 0, protocolError(ErrResourceExceeded, "Redis nesting exceeds limit")
	}
	if len(w) == 0 {
		return 0, nil
	}
	nl := bytes.Index(w, []byte("\r\n"))
	if !redisPrefix(w[0]) {
		return 0, fmt.Errorf("redis: invalid RESP prefix")
	}
	if nl < 0 {
		return 0, nil
	}
	value := string(w[1:nl])
	if strings.ContainsAny(value, "\r\n") {
		return 0, fmt.Errorf("redis: invalid line")
	}
	switch w[0] {
	case '+', '-':
		return nl + 2, nil
	case '_':
		if value != "" {
			return 0, fmt.Errorf("redis: invalid null")
		}
		return nl + 2, nil
	case '#':
		if value != "t" && value != "f" {
			return 0, fmt.Errorf("redis: invalid boolean")
		}
		return nl + 2, nil
	case ':', '(':
		digits := value
		if len(digits) > 0 && (digits[0] == '-' || digits[0] == '+') {
			digits = digits[1:]
		}
		if digits == "" || strings.Trim(digits, "0123456789") != "" {
			return 0, fmt.Errorf("redis: invalid integer")
		}
		if w[0] == ':' {
			if _, err := strconv.ParseInt(value, 10, 64); err != nil {
				return 0, fmt.Errorf("redis: integer overflow")
			}
		}
		return nl + 2, nil
	case ',':
		if value != "inf" && value != "-inf" && value != "nan" {
			n, err := strconv.ParseFloat(value, 64)
			if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
				return 0, fmt.Errorf("redis: invalid double")
			}
		}
		return nl + 2, nil
	}
	if value == "-1" && (w[0] == '$' || w[0] == '*') {
		return nl + 2, nil
	}
	if value == "" || strings.Trim(value, "0123456789") != "" {
		return 0, fmt.Errorf("redis: invalid unsigned length")
	}
	count, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("redis: length overflow")
	}
	if w[0] == '$' || w[0] == '!' || w[0] == '=' {
		if count > uint64(budget.MaxMessageBytes) || count+uint64(nl)+4 > uint64(budget.MaxMessageBytes) {
			return 0, protocolError(ErrResourceExceeded, "Redis bulk exceeds message limit")
		}
		need := nl + 4 + int(count)
		if need > len(w) {
			return 0, nil
		}
		if !bytes.Equal(w[need-2:need], []byte("\r\n")) {
			return 0, fmt.Errorf("redis: missing bulk terminator")
		}
		if w[0] == '=' && (count < 4 || w[nl+5] != ':') {
			return 0, fmt.Errorf("redis: invalid verbatim format")
		}
		return need, nil
	}
	multiplier := uint64(1)
	if w[0] == '%' {
		multiplier = 2
	}
	if count > uint64(budget.MaxCollectionElements)/multiplier {
		return 0, protocolError(ErrResourceExceeded, "Redis aggregate exceeds limit")
	}
	count *= multiplier
	if int(count) > budget.MaxCollectionElements-*used {
		return 0, protocolError(ErrResourceExceeded, "Redis total elements exceed limit")
	}
	*used += int(count)
	at := nl + 2
	for i := uint64(0); i < count; i++ {
		n, err := redisFrameLengthBudget(w[at:], depth+1, budget, used)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			return 0, nil
		}
		at += n
	}
	return at, nil
}

func (f *binFlow) frameRedis(w []byte) (int, *binSpec, error) {
	if f.redis == nil {
		return 0, nil, sessionContext("Redis session was not observed")
	}
	if err := f.reserveSession(256); err != nil {
		return 0, nil, err
	}
	used := 0
	n, err := redisFrameLengthBudget(w, 0, f.a.budget, &used)
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

func (r *binRedis) consume(dir int, raw []byte) (map[string]any, error) {
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
	// A push is out-of-band and an array reply is not itself a request.
	if !r.roleKnown && raw[0] == '*' && bytes.Contains(raw, []byte("\r\n$")) {
		r.client = dir
		r.roleKnown = true
	}
	if r.roleKnown && raw[0] != '>' {
		if dir == r.client && raw[0] == '*' {
			r.pending++
		} else if dir != r.client && r.pending > 0 {
			r.pending--
		}
		info["Pipeline"] = r.pending
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
