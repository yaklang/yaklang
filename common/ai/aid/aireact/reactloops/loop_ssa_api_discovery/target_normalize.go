package loop_ssa_api_discovery

import (
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

var (
	reMarkdownLinkURL = regexp.MustCompile(`\[[^\]]*\]\((https?://[^)\s]+)\)`)
	reAngleURL        = regexp.MustCompile(`<?(https?://[^\s>]+)>?`)
)

// NormalizeTargetString trims junk, unwraps markdown/angle URLs, and adds http:// for bare host:port.
// It does not validate reachability; use ProbeTarget after normalization.
func NormalizeTargetString(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`'\"")
	for strings.HasSuffix(raw, ".") || strings.HasSuffix(raw, "，") || strings.HasSuffix(raw, ",") {
		raw = strings.TrimSuffix(raw, ".")
		raw = strings.TrimSuffix(raw, "，")
		raw = strings.TrimSuffix(raw, ",")
		raw = strings.TrimSpace(raw)
	}
	if raw == "" {
		return ""
	}
	if m := reMarkdownLinkURL.FindStringSubmatch(raw); len(m) > 1 {
		raw = strings.TrimSpace(m[1])
	}
	if strings.Contains(raw, "://") {
		if m := reAngleURL.FindStringSubmatch(raw); len(m) > 1 {
			raw = strings.TrimSpace(m[1])
		} else {
			raw = strings.Trim(raw, "<>")
		}
		u, err := url.Parse(raw)
		if err == nil && u.Host != "" {
			u.Fragment = ""
			u.RawQuery = ""
			if u.Path == "/" {
				u.Path = ""
			}
			out := u.String()
			if out == "" {
				return raw
			}
			return out
		}
		return raw
	}

	line := raw
	if host, port, err := utils.ParseStringToHostPort(line); err == nil {
		if port <= 0 {
			return "http://" + line
		}
		return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	}

	if !strings.Contains(line, ":") && line != "" {
		return "http://" + line
	}

	return raw
}

// EffectiveTargetBaseURL prefers a normalized origin (including URL path prefix) from TargetRaw;
// falls back to scheme/host/port from session columns when TargetRaw is not a full URL.
func EffectiveTargetBaseURL(sess *store.DiscoverySession) string {
	if sess == nil {
		return ""
	}
	raw := strings.TrimSpace(sess.TargetRaw)
	if strings.Contains(raw, "://") {
		if u, err := url.Parse(raw); err == nil && u.Scheme != "" && u.Host != "" {
			u.Fragment = ""
			u.RawQuery = ""
			u.User = nil
			s := u.String()
			if s != "" {
				return strings.TrimRight(s, "/")
			}
		}
	}
	return strings.TrimRight(baseTargetURLFromSession(sess), "/")
}

// JoinProbeURL joins discovery target base URL with a path_pattern for HTTP probing.
// If pathPattern is already an absolute http(s) URL, it is returned as-is (trimmed).
func JoinProbeURL(baseURL, pathPattern string) string {
	p := strings.TrimSpace(pathPattern)
	if p == "" {
		return strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	low := strings.ToLower(p)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return p
	}
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	if b == "" {
		return p
	}
	return b + p
}

// baseTargetURLFromSession builds scheme://host:port without path (session scalar fields only).
func baseTargetURLFromSession(sess *store.DiscoverySession) string {
	if sess == nil {
		return ""
	}
	scheme := strings.ToLower(strings.TrimSpace(sess.TargetScheme))
	if scheme == "" {
		scheme = "http"
	}
	host := strings.TrimSpace(sess.TargetHost)
	if host == "" {
		return ""
	}
	port := strings.TrimSpace(sess.TargetPort)
	if port != "" && !strings.Contains(host, ":") {
		host = net.JoinHostPort(host, port)
	}
	return scheme + "://" + host
}

// targetURLHints returns short guidance for models when target is missing or probe failed.
func targetURLHints(sess *store.DiscoverySession) []string {
	if sess == nil {
		return nil
	}
	var out []string
	if strings.TrimSpace(sess.TargetRaw) == "" {
		out = append(out, "未配置 Target：首条用户输入应包含 Target: 行，或调用 discovery_set_target(target=\"http://127.0.0.1:8080\")。")
		return out
	}
	if !sess.TargetReachable {
		out = append(out, "靶机探活失败。请确认进程已监听、端口与协议正确，然后用 discovery_set_target 传入完整 URL（http:// 或 https:// + 主机 + 端口；应用若在子路径，URL 中保留 path）后再次 discovery_get_status。")
	}
	if !strings.Contains(sess.TargetRaw, "://") && strings.TrimSpace(sess.TargetScheme) == "" {
		out = append(out, "当前 Target 为 host:port 形式，已按 http 探测。若实际为 https，请 discovery_set_target(target=\"https://...\")。")
	}
	return out
}
