package loop_ssa_api_discovery

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const loopKeyCredentialTransformCache = "ssa_credential_transform_cache"

var hexCredentialOutputRE = regexp.MustCompile(`^[0-9a-fA-F]+$`)

type credentialTransformCache map[string]string // key: algorithm|input -> output

func credentialTransformCacheKey(algorithm, input string) string {
	return strings.ToLower(strings.TrimSpace(algorithm)) + "|" + input
}

func recordCredentialTransform(loop *reactloops.ReActLoop, res *credentialTransformResult) {
	if loop == nil || res == nil || strings.TrimSpace(res.Output) == "" {
		return
	}
	cache := loadCredentialTransformCache(loop)
	cache[credentialTransformCacheKey(res.Algorithm, res.Input)] = res.Output
	saveCredentialTransformCache(loop, cache)
}

func loadCredentialTransformCache(loop *reactloops.ReActLoop) credentialTransformCache {
	if loop == nil {
		return credentialTransformCache{}
	}
	raw := strings.TrimSpace(loop.Get(loopKeyCredentialTransformCache))
	if raw == "" {
		return credentialTransformCache{}
	}
	var cache credentialTransformCache
	if json.Unmarshal([]byte(raw), &cache) != nil {
		return credentialTransformCache{}
	}
	return cache
}

func saveCredentialTransformCache(loop *reactloops.ReActLoop, cache credentialTransformCache) {
	if loop == nil || cache == nil {
		return
	}
	b, err := json.Marshal(cache)
	if err != nil {
		return
	}
	loop.Set(loopKeyCredentialTransformCache, string(b))
}

func lookupStoredCredentialTransform(loop *reactloops.ReActLoop, algorithm, input string) string {
	if loop == nil || input == "" {
		return ""
	}
	return loadCredentialTransformCache(loop)[credentialTransformCacheKey(algorithm, input)]
}

func lookupPlaintextPasswordForUsername(rt *Runtime, username string) string {
	if rt == nil || strings.TrimSpace(username) == "" {
		return ""
	}
	for _, g := range rt.UserCredentialGroups() {
		for _, a := range g.Accounts {
			if a.Username == username {
				return a.Password
			}
		}
	}
	if rt.UserAuthUsername == username {
		return strings.TrimSpace(rt.UserAuthPassword)
	}
	return ""
}

func parseLoginFormValues(postParams, body string) url.Values {
	for _, src := range []string{postParams, body} {
		if strings.TrimSpace(src) == "" {
			continue
		}
		vals, err := url.ParseQuery(strings.ReplaceAll(src, ";", "&"))
		if err == nil && len(vals) > 0 {
			return vals
		}
	}
	return url.Values{}
}

func extractPasswordFromLoginRequest(postParams, body string) string {
	vals := parseLoginFormValues(postParams, body)
	for _, key := range []string{"password", "passwd", "pwd", "pass"} {
		if v := strings.TrimSpace(vals.Get(key)); v != "" {
			return v
		}
	}
	return ""
}

func extractEncodingFromLoginRequest(postParams, body string) string {
	vals := parseLoginFormValues(postParams, body)
	return strings.ToLower(strings.TrimSpace(vals.Get("encoding")))
}

func expectedHexLengthForTransform(algorithm string) int {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "md5", "hmac-md5":
		return 32
	case "sha1", "hmac-sha1":
		return 40
	case "sha256", "hmac-sha256":
		return 64
	case "sha512", "hmac-sha512":
		return 128
	default:
		return 0
	}
}

func isValidHexCredentialOutput(output, algorithm string) bool {
	output = strings.TrimSpace(output)
	want := expectedHexLengthForTransform(algorithm)
	if want == 0 {
		return true
	}
	if len(output) != want || !hexCredentialOutputRE.MatchString(output) {
		return false
	}
	return !looksLikeFakeRepeatingHash(output)
}

func looksLikeFakeRepeatingHash(hex string) bool {
	if len(hex) < 16 {
		return false
	}
	// Detect low-entropy repeating patterns like 3c5e7a5b0f8d9e2a1c4b6d8f0e2a4c6 repeated.
	for block := 4; block <= 32; block += 4 {
		if len(hex)%block != 0 {
			continue
		}
		chunk := hex[:block]
		repeats := true
		for i := block; i < len(hex); i += block {
			if hex[i:i+block] != chunk {
				repeats = false
				break
			}
		}
		if repeats && len(hex)/block >= 3 {
			return true
		}
	}
	return false
}

func loginPathFromRequestURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	if u, err := url.Parse(rawURL); err == nil && u.Path != "" {
		return normURLPath(u.Path)
	}
	return normURLPath(rawURL)
}

func resolveLoginPasswordTransform(rt *Runtime, authRealm, loginPath, postParams, body string) string {
	if enc := extractEncodingFromLoginRequest(postParams, body); enc != "" && enc != "none" {
		return enc
	}
	if rt == nil || rt.WorkDir == "" {
		return ""
	}
	ev, err := loadAuthEvidenceFromWorkDir(rt.WorkDir)
	if err != nil || ev == nil {
		return ""
	}
	authRealm = NormalizeAuthRealm(authRealm)
	loginPath = normURLPath(loginPath)
	for _, ep := range ev.LoginEndpoints {
		t := strings.ToLower(strings.TrimSpace(ep.PasswordTransform))
		if t == "" || t == "none" {
			continue
		}
		if authRealm != "" && NormalizeAuthRealm(ep.AuthRealm) != authRealm {
			continue
		}
		if loginPath != "" && normURLPath(ep.Path) != loginPath {
			continue
		}
		return t
	}
	for _, ep := range ev.LoginEndpoints {
		t := strings.ToLower(strings.TrimSpace(ep.PasswordTransform))
		if t != "" && t != "none" {
			return t
		}
	}
	return ""
}

// checkLoginPasswordTransformBlocked rejects login POSTs with hand-written or invalid password hashes.
func checkLoginPasswordTransformBlocked(rt *Runtime, loop *reactloops.ReActLoop, authRealm string, action *aicommon.Action) string {
	if action == nil {
		return ""
	}
	method := strings.ToUpper(strings.TrimSpace(action.GetString("method")))
	if method != "POST" && method != "PUT" {
		return ""
	}
	loginURL := action.GetString("url")
	if !looksLikeLoginRequestURL(loginURL) {
		return ""
	}
	postParams := action.GetString("post-params")
	body := action.GetString("body")
	transform := resolveLoginPasswordTransform(rt, authRealm, loginPathFromRequestURL(loginURL), postParams, body)
	if transform == "" || transform == "none" {
		return ""
	}
	password := extractPasswordFromLoginRequest(postParams, body)
	if password == "" {
		return ""
	}
	if !isValidHexCredentialOutput(password, transform) {
		want := expectedHexLengthForTransform(transform)
		return fmt.Sprintf(
			"login POST blocked: password field must be %q output from transform_credential (%d lowercase hex chars); "+
				"got len=%d. Call transform_credential with algorithm=%q and the account plaintext password, then paste the tool output into post-params.",
			transform, want, len(password), transform,
		)
	}
	username := extractUsernameFromLoginRequest(postParams, body)
	plaintext := lookupPlaintextPasswordForUsername(rt, username)
	if plaintext == "" {
		if lookupStoredTransformByOutput(loop, transform, password) == "" {
			return fmt.Sprintf(
				"login POST blocked: password hash not produced by transform_credential in this loop. "+
					"Call transform_credential algorithm=%q input=<plaintext> first, then use the exact output value.",
				transform,
			)
		}
		return ""
	}
	expected, err := transformCredentialGoParams(transform, plaintext, "", "", "", "", false)
	if err != nil {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(password), strings.TrimSpace(expected.Output)) {
		return fmt.Sprintf(
			"login POST blocked: password hash does not match transform_credential(%q, %q). "+
				"Expected prefix %s... Call transform_credential with input=%q for username=%q, then POST the tool output.",
			transform, plaintext, shortHashPrefix(expected.Output), plaintext, username,
		)
	}
	stored := lookupStoredCredentialTransform(loop, transform, plaintext)
	if stored == "" || !strings.EqualFold(stored, password) {
		return fmt.Sprintf(
			"login POST blocked: must call transform_credential before login POST (algorithm=%q input=%q for username=%q). "+
				"Do not hand-write SHA512/hash strings; only use transform_credential output.",
			transform, plaintext, username,
		)
	}
	return ""
}

func lookupStoredTransformByOutput(loop *reactloops.ReActLoop, algorithm, output string) string {
	if loop == nil {
		return ""
	}
	algorithm = strings.ToLower(strings.TrimSpace(algorithm))
	for key, val := range loadCredentialTransformCache(loop) {
		if !strings.HasPrefix(key, algorithm+"|") {
			continue
		}
		if strings.EqualFold(val, output) {
			return key
		}
	}
	return ""
}

func shortHashPrefix(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) <= 16 {
		return hash
	}
	return hash[:16]
}
