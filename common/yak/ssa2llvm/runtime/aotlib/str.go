package aotlib

import (
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func StrSplit(s, sep string) []string { return strings.Split(s, sep) }
func StrTrim(s, cutset string) string { return strings.Trim(s, cutset) }
func StrTrimSpace(s string) string    { return strings.TrimSpace(s) }
func StrJoin(parts []string, sep string) string {
	return strings.Join(parts, sep)
}
func StrToLower(s string) string         { return strings.ToLower(s) }
func StrToUpper(s string) string         { return strings.ToUpper(s) }
func StrHasPrefix(s, prefix string) bool { return strings.HasPrefix(s, prefix) }
func StrHasSuffix(s, suffix string) bool { return strings.HasSuffix(s, suffix) }
func StrContains(s, substr string) bool  { return strings.Contains(s, substr) }
func StrReplace(s, old, new string, n int) string {
	return strings.Replace(s, old, new, n)
}
func StrRepeat(s string, count int) string { return strings.Repeat(s, count) }
func StrIndex(s, substr string) int        { return strings.Index(s, substr) }
func StrLen(s string) int                  { return len(s) }
func StrReplaceAll(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}
func StrPathJoin(parts []string) string {
	return strings.Join(parts, "/")
}
func StrMatchAllOfSubString(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func StrCount(s, substr string) int { return strings.Count(s, substr) }

const strRandCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func StrRandStr(length int) string {
	if length <= 0 {
		return ""
	}
	out := make([]byte, length)
	max := big.NewInt(int64(len(strRandCharset)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			out[i] = strRandCharset[0]
			continue
		}
		out[i] = strRandCharset[n.Int64()]
	}
	return string(out)
}

func StrHostPort(host string, port any) string {
	return fmt.Sprintf("%v:%v", host, port)
}

// StrParseStringToHostPort mirrors yaklib's str.ParseStringToHostPort for the
// common "host:port" and URL forms.
func StrParseStringToHostPort(raw string) (string, int, error) {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", 0, err
		}
		port := 0
		if p := u.Port(); p != "" {
			if n, err := strconv.Atoi(p); err == nil && n > 0 {
				port = n
			}
		}
		if port == 0 {
			switch u.Scheme {
			case "http", "ws":
				port = 80
			case "https", "wss":
				port = 443
			}
		}
		return u.Hostname(), port, nil
	}
	host, portStr, err := net.SplitHostPort(raw)
	if err != nil {
		return "", 0, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", 0, err
	}
	return host, port, nil
}

func StrExtractHostPort(raw string) string {
	host, port, err := StrParseStringToHostPort(raw)
	if err == nil && host != "" && port > 0 {
		return StrHostPort(host, port)
	}
	return raw
}

// StrIsTLSServer mirrors yaklib's str.IsTLSServer: report whether the address
// speaks TLS.
func StrIsTLSServer(addr string) (bool, error) {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	tlsConn := tls.Client(conn, &tls.Config{InsecureSkipVerify: true})
	_ = tlsConn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := tlsConn.Handshake(); err != nil {
		return false, nil
	}
	return true, nil
}

var strURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// StrParseStringToUrls mirrors yaklib's str.ParseStringToUrls: extract URLs
// from the input strings.
func StrParseStringToUrls(targets ...string) []string {
	var out []string
	for _, t := range targets {
		for _, m := range strURLPattern.FindAllString(t, -1) {
			out = append(out, m)
		}
	}
	return out
}

// StringsExports mirrors the str module's export table (the AOT-supported
// subset). Entries match common/yak/yaklib.StringsExport signatures.
var StringsExports = map[string]any{
	"Split":               StrSplit,
	"Trim":                StrTrim,
	"TrimSpace":           StrTrimSpace,
	"Join":                StrJoin,
	"ToLower":             StrToLower,
	"ToUpper":             StrToUpper,
	"HasPrefix":           StrHasPrefix,
	"HasSuffix":           StrHasSuffix,
	"Contains":            StrContains,
	"Replace":             StrReplace,
	"Repeat":              StrRepeat,
	"Index":               StrIndex,
	"Len":                 StrLen,
	"ReplaceAll":          StrReplaceAll,
	"PathJoin":            StrPathJoin,
	"MatchAllOfSubString": StrMatchAllOfSubString,
	"Count":               StrCount,
	"RandStr":             StrRandStr,
	"HostPort":            StrHostPort,
	"ParseStringToHostPort": StrParseStringToHostPort,
	"ExtractHostPort":       StrExtractHostPort,
	"IsTLSServer":           StrIsTLSServer,
	"ParseStringToUrls":     StrParseStringToUrls,
}
