package stream_parser

import (
	"bytes"
	"encoding/base64"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SCRAM-SHA-256 syntax only. No password derivation, proof/nonce comparison,
// channel-binding verification, SASLprep or identity assertion is performed.
func postgresqlSCRAMName(b []byte, empty bool) bool {
	if len(b) == 0 {
		return empty
	}
	if !utf8.Valid(b) {
		return false
	}
	for i := 0; i < len(b); i++ {
		if b[i] == 0 || b[i] == ',' {
			return false
		}
		if b[i] == '=' {
			if i+2 >= len(b) || string(b[i:i+3]) != "=2C" && string(b[i:i+3]) != "=3D" {
				return false
			}
			i += 2
		}
	}
	return true
}
func (r *postgresqlFieldsReader) scram(info map[string]any, phase string) {
	if r.err != nil {
		return
	}
	start, index := r.at, len(r.fields)
	info["SCRAM Phase"] = phase
	info["SCRAM Proof Verified"] = false
	info["Channel Binding Verified"] = false
	if phase == "client-first" {
		n := bytes.IndexByte(r.wire[r.at:r.end], ',')
		if n < 0 {
			r.fail("missing GS2 flag delimiter")
			return
		}
		flag := r.take("GS2 Binding Flag", "raw", n)
		r.take("SCRAM Comma", "raw", 1)
		plus := info["Mechanism"] == "SCRAM-SHA-256-PLUS"
		if bytes.HasPrefix(flag, []byte("p=")) {
			if len(flag) == 2 || !plus {
				r.fail("GS2 binding flag/mechanism mismatch")
			}
			for _, c := range flag[2:] {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '-') {
					r.fail("invalid GS2 binding name")
				}
			}
		} else if plus || !bytes.Equal(flag, []byte("n")) && !bytes.Equal(flag, []byte("y")) {
			r.fail("invalid GS2 flag")
		}
		if r.err != nil {
			return
		}
		n = bytes.IndexByte(r.wire[r.at:r.end], ',')
		if n < 0 {
			r.fail("missing GS2 identity delimiter")
			return
		}
		identity := r.take("GS2 Authorization Identity", "raw", n)
		r.take("SCRAM Comma", "raw", 1)
		if len(identity) > 0 && (!bytes.HasPrefix(identity, []byte("a=")) || !postgresqlSCRAMName(identity[2:], false)) {
			r.fail("invalid GS2 identity")
		}
	}
	attrsStart, attrsIndex := r.at, len(r.fields)
	attrs := make([]map[string]any, 0)
	keys := make([]byte, 0)
	seen := map[byte]bool{}
	for r.err == nil && r.at < r.end {
		if len(attrs) == postgresqlFieldsMaxItems {
			r.fail("SCRAM attributes resource limit")
			break
		}
		a, f := r.at, len(r.fields)
		if r.end-r.at < 2 {
			r.fail("incomplete SCRAM attribute")
			break
		}
		key := byte(r.uint("SCRAM Attribute Name", 1))
		if !(key >= 'a' && key <= 'z' || key >= 'A' && key <= 'Z') {
			r.fail("invalid SCRAM attribute name")
		}
		if r.uint("SCRAM Equals", 1) != '=' {
			r.fail("missing SCRAM equals")
		}
		if seen[key] || key == 'm' {
			r.fail("duplicate or mandatory unsupported SCRAM attribute")
		}
		seen[key] = true
		if r.err != nil {
			break
		}
		n := bytes.IndexByte(r.wire[r.at:r.end], ',')
		if n < 0 {
			n = r.end - r.at
		}
		s := r.at
		v := r.take("SCRAM Attribute Value", "raw", n)
		if !utf8.Valid(v) || bytes.IndexByte(v, 0) >= 0 || len(v) == 0 && !(phase == "client-first" && key == 'n') {
			r.fail("invalid/empty SCRAM value")
		}
		attr := map[string]any{"Name": string([]byte{key}), "Value": bytes.Clone(v), "Relative Byte Range": [2]int{s, r.at}}
		switch key {
		case 'n':
			if !postgresqlSCRAMName(v, true) {
				r.fail("invalid SCRAM name escaping")
			}
			info["Empty SCRAM Username Observed"] = len(v) == 0
		case 'r':
			for _, c := range v {
				if c < 33 || c > 126 || c == ',' {
					r.fail("invalid SCRAM nonce")
				}
			}
		case 'i':
			if len(v) == 0 || v[0] < '1' || v[0] > '9' {
				r.fail("invalid SCRAM iterations")
			}
			for _, c := range v {
				if c < '0' || c > '9' {
					r.fail("invalid SCRAM iterations")
				}
			}
			i, err := strconv.ParseUint(string(v), 10, 32)
			if err != nil {
				r.fail("iteration integer range")
			}
			attr["Integer"] = i
		case 'c', 's', 'p', 'v':
			decoded, err := base64.StdEncoding.Strict().DecodeString(string(v))
			if err != nil || base64.StdEncoding.EncodeToString(decoded) != string(v) {
				r.fail("noncanonical SCRAM base64")
			}
			if (key == 'p' || key == 'v') && len(decoded) != 32 {
				r.fail("SCRAM-SHA-256 proof/verifier length")
			}
			attr["Decoded Octet Count"] = len(decoded) // decoded secret-related bytes are not published
		}
		attrs = append(attrs, attr)
		keys = append(keys, key)
		if r.at < r.end {
			r.take("SCRAM Comma", "raw", 1)
			if r.at == r.end {
				r.fail("trailing SCRAM comma")
			}
		}
		r.group("SCRAM Attribute", a, f, false)
	}
	keyString := string(keys)
	required := ""
	switch phase {
	case "client-first":
		required = "nr"
	case "server-first":
		required = "rsi"
	case "client-final":
		required = "cr"
	case "server-final":
		if strings.HasPrefix(keyString, "e") {
			required = "e"
		} else {
			required = "v"
		}
	}
	if !strings.HasPrefix(keyString, required) || required == "" {
		r.fail("SCRAM attribute order")
	}
	ext := keyString
	if len(ext) >= len(required) {
		ext = ext[len(required):]
	}
	if phase == "client-final" {
		if !strings.HasSuffix(ext, "p") {
			r.fail("missing final SCRAM proof")
		} else {
			ext = ext[:len(ext)-1]
		}
	}
	for _, key := range ext {
		if strings.ContainsRune("acemnprsviy", key) {
			r.fail("unexpected reserved SCRAM attribute")
		}
	}
	info["SCRAM Attributes"] = attrs
	r.group("SCRAM Attributes", attrsStart, attrsIndex, true)
	r.group("SCRAM Fields", start, index, false)
}
