package pcaputil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type binStratum struct {
	client  int
	pending map[string]string
}

var stratumMethods = map[string]bool{
	"mining.subscribe": true,
	"mining.authorize": true,
	"mining.notify":    true,
	"mining.submit":    true,
}

// A mining JSON-RPC line is admitted only after the complete bounded first
// request has been seen. Generic JSON-RPC and JSON logs on port 3333 remain
// unidentified. Responses become Stratum only within that observed session.
func probeStratum(w []byte, limit int) ProbeResult {
	if len(w) == 0 || w[0] != '{' {
		return ProbeResult{Verdict: ProbeReject}
	}
	end := bytes.IndexByte(w, '\n')
	if end < 0 {
		if len(w) >= limit {
			return ProbeResult{Verdict: ProbeReject}
		}
		return probeNeed("stratum", "json-rpc", len(w), limit)
	}
	line := bytes.TrimSpace(w[:end+1])
	if len(line) == 0 || len(line) > limit {
		return ProbeResult{Verdict: ProbeReject}
	}
	parsed, err := parseStratumLine(line)
	if err != nil || parsed.method == "" {
		return ProbeResult{Verdict: ProbeReject}
	}
	return probeAccept("stratum", "mining-json-rpc", 98)
}

type stratumLine struct {
	id     string
	method string
	params []json.RawMessage
	result json.RawMessage
	errval json.RawMessage
}

func parseStratumLine(raw []byte) (stratumLine, error) {
	var out stratumLine
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 64<<10 || raw[0] != '{' {
		return out, fmt.Errorf("stratum: invalid line length or object")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return out, fmt.Errorf("stratum: malformed JSON: %w", err)
	}
	if root == nil || len(root) > 16 {
		return out, fmt.Errorf("stratum: missing or oversized object")
	}
	if value, ok := root["id"]; ok && !bytes.Equal(value, []byte("null")) {
		if len(value) == 0 || len(value) > 32 || value[0] != '"' && value[0] != '-' && (value[0] < '0' || value[0] > '9') {
			return out, fmt.Errorf("stratum: invalid id")
		}
		out.id = string(value)
	}
	if methodRaw, ok := root["method"]; ok {
		if err := json.Unmarshal(methodRaw, &out.method); err != nil || !stratumMethods[out.method] {
			return out, fmt.Errorf("stratum: unsupported mining method")
		}
		if len(root["params"]) == 0 || json.Unmarshal(root["params"], &out.params) != nil || out.params == nil || len(out.params) > 32 {
			return out, fmt.Errorf("stratum: invalid parameters")
		}
		switch out.method {
		case "mining.subscribe":
			if len(out.params) < 1 || !stratumParamString(out.params[0]) {
				return out, fmt.Errorf("stratum: missing agent")
			}
		case "mining.authorize", "mining.submit":
			if len(out.params) < 2 || !stratumParamString(out.params[0]) || !stratumParamString(out.params[1]) {
				return out, fmt.Errorf("stratum: missing worker or job")
			}
		case "mining.notify":
			if len(out.params) < 1 || !stratumParamString(out.params[0]) {
				return out, fmt.Errorf("stratum: missing job")
			}
		}
		if out.method != "mining.notify" && out.id == "" {
			return out, fmt.Errorf("stratum: request id missing")
		}
		return out, nil
	}
	if out.id == "" || len(root["result"]) == 0 && len(root["error"]) == 0 {
		return out, fmt.Errorf("stratum: response without id or result")
	}
	out.result, out.errval = root["result"], root["error"]
	return out, nil
}

func stratumParamString(raw json.RawMessage) bool {
	var value string
	return json.Unmarshal(raw, &value) == nil && value != "" && len(value) <= 1024
}

func stratumParam(raw json.RawMessage) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}

func (f *binFlow) frameStratum(w []byte) (int, *binSpec, error) {
	if len(w) > 64<<10 {
		return 0, nil, fmt.Errorf("stratum: line exceeds 64 KiB")
	}
	end := bytes.IndexByte(w, '\n')
	if end < 0 {
		return 0, nil, nil
	}
	return end + 1, f.a.specs["application-layer.stratum/StratumLine"], nil
}

func (s *binStratum) consume(dir int, raw []byte, budget int) (map[string]any, error) {
	line, err := parseStratumLine(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if line.method != "" {
		if line.method != "mining.notify" && dir != s.client {
			return nil, fmt.Errorf("stratum: request from unexpected direction")
		}
		out["Packet Name"] = line.method
		out["Message ID"] = line.id
		switch line.method {
		case "mining.subscribe":
			out["Agent"] = stratumParam(line.params[0])
		case "mining.authorize":
			out["Worker"] = stratumParam(line.params[0])
			// Authentication secrets are deliberately not projected into events.
		case "mining.notify":
			out["Job ID"] = stratumParam(line.params[0])
		case "mining.submit":
			out["Worker"], out["Job ID"] = stratumParam(line.params[0]), stratumParam(line.params[1])
		}
		if line.id != "" {
			if len(s.pending) >= budget {
				return nil, fmt.Errorf("stratum: pending request budget exceeded")
			}
			s.pending[line.id] = line.method
		}
		return out, nil
	}
	if dir == s.client {
		return nil, fmt.Errorf("stratum: response from client direction")
	}
	method := s.pending[line.id]
	if method == "" {
		return nil, fmt.Errorf("stratum: response id has no observed request")
	}
	delete(s.pending, line.id)
	out["Packet Name"] = "response"
	out["In Reply To"] = method
	out["Message ID"] = line.id
	if len(line.result) > 0 {
		var value any
		if err := json.Unmarshal(line.result, &value); err != nil {
			return nil, fmt.Errorf("stratum: invalid result")
		}
		out["Result"] = value
	}
	if len(line.errval) > 0 && !strings.EqualFold(string(line.errval), "null") {
		out["Error"] = string(line.errval)
	}
	return out, nil
}
