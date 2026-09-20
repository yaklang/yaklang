// Package digest parses declared package integrity/checksum evidence.
// It does not fetch or hash package bytes.
package digest

import (
	"encoding/base64"
	"encoding/hex"
	"sort"
	"strings"
)

var algorithms = map[string]int{
	"md5": 16, "sha1": 20, "sha256": 32, "sha384": 48, "sha512": 64,
	"sha3-256": 32, "sha3-384": 48, "sha3-512": 64,
	"blake2b-256": 32, "blake2b-384": 48, "blake2b-512": 64, "blake3": 32,
}

type Result struct {
	Canonical string
	Issues    []string
}

// ParseDeclared normalizes SRI (`sha512-<base64>`) and `alg:hex` tokens.
// Same digest with different encodings becomes one canonical `alg:hex` token.
func ParseDeclared(raw string) Result {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Result{}
	}
	var tokens []string
	var issues []string
	seen := map[string]bool{}
	for _, token := range strings.Fields(raw) {
		alg, payload, ok := split(token)
		if !ok {
			issues = append(issues, "malformed digest token")
			continue
		}
		size, known := algorithms[alg]
		if !known {
			issues = append(issues, "unknown digest algorithm "+alg)
			continue
		}
		sum, ok := decode(payload, size)
		if !ok {
			issues = append(issues, "illegal digest encoding")
			continue
		}
		item := alg + ":" + hex.EncodeToString(sum)
		if seen[item] {
			continue
		}
		seen[item] = true
		tokens = append(tokens, item)
	}
	sort.Strings(tokens)
	return Result{Canonical: strings.Join(tokens, " "), Issues: issues}
}

func split(token string) (string, string, bool) {
	lower := strings.ToLower(strings.TrimSpace(token))
	if lower == "" {
		return "", "", false
	}
	if i := strings.IndexByte(lower, ':'); i > 0 {
		return lower[:i], token[i+1:], true
	}
	best := ""
	for name := range algorithms {
		if strings.HasPrefix(lower, name+"-") && len(name) > len(best) {
			best = name
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, token[len(best)+1:], true
}

func decode(payload string, size int) ([]byte, bool) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return nil, false
	}
	if b, err := hex.DecodeString(payload); err == nil && len(b) == size {
		return b, true
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(payload); err == nil && len(b) == size {
			return b, true
		}
	}
	return nil, false
}
