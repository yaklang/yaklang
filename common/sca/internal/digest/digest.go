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

type Token struct {
	Raw, Algorithm, Canonical, Issue string
}

type Result struct {
	Original  string
	Canonical string
	Tokens    []Token
	Issues    []string
}

// ParseDeclared normalizes SRI (`sha512-<base64>`) and `alg:hex` tokens.
// Same digest with different encodings becomes one canonical `alg:hex` token.
// Original text is always retained; invalid tokens never become Canonical.
func ParseDeclared(raw string) Result {
	original := strings.TrimSpace(raw)
	if original == "" {
		return Result{}
	}
	var canonical []string
	var issues []string
	seen := map[string]bool{}
	out := Result{Original: original}
	for _, token := range strings.Fields(original) {
		item := Token{Raw: token}
		alg, payload, ok := split(token)
		if !ok {
			item.Issue = "malformed digest token"
			issues = append(issues, item.Issue+": "+token)
			out.Tokens = append(out.Tokens, item)
			continue
		}
		item.Algorithm = alg
		size, known := algorithms[alg]
		if !known {
			item.Issue = "unknown digest algorithm " + alg
			issues = append(issues, item.Issue+": "+token)
			out.Tokens = append(out.Tokens, item)
			continue
		}
		sum, ok := decode(payload, size)
		if !ok {
			item.Issue = "illegal digest encoding"
			issues = append(issues, item.Issue+": "+token)
			out.Tokens = append(out.Tokens, item)
			continue
		}
		item.Canonical = alg + ":" + hex.EncodeToString(sum)
		if !seen[item.Canonical] {
			seen[item.Canonical] = true
			canonical = append(canonical, item.Canonical)
		}
		out.Tokens = append(out.Tokens, item)
	}
	sort.Strings(canonical)
	out.Canonical = strings.Join(canonical, " ")
	out.Issues = issues
	return out
}

func split(token string) (string, string, bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", false
	}
	if i := strings.IndexByte(token, ':'); i > 0 {
		return strings.ToLower(token[:i]), token[i+1:], true
	}
	lower := strings.ToLower(token)
	best := ""
	for name := range algorithms {
		if strings.HasPrefix(lower, name+"-") && len(name) > len(best) {
			best = name
		}
	}
	if best == "" {
		return "", "", false
	}
	prefix := best + "-"
	if len(token) < len(prefix) || !strings.EqualFold(token[:len(prefix)], prefix) {
		return "", "", false
	}
	return best, token[len(prefix):], true
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
