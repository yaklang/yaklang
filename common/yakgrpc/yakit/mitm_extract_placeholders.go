package yakit

import (
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

// MITM 提取结果模板中的占位符字面量（与 Yakit / 规则约定一致）。
const (
	MITMExtractPlaceholderURL         = "__url__"
	MITMExtractPlaceholderHost        = "__host__"
	MITMExtractPlaceholderURI         = "__uri__"
	MITMExtractPlaceholderMethod      = "__method__"
	MITMExtractPlaceholderPath        = "__path__"
	MITMExtractPlaceholderScheme      = "__scheme__"
	MITMExtractPlaceholderSchema      = "__schema__" // 与 yak/httptpl 命名一致，展开值同 __scheme__
	MITMExtractPlaceholderPort        = "__port__"
	MITMExtractPlaceholderRuntimeID   = "__runtime_id__"
	MITMExtractPlaceholderHiddenIndex = "__hidden_index__"
	MITMExtractPlaceholderTraceID     = "__trace_id__" // 与 HiddenIndex 相同，对应 extracted_data.trace_id
	MITMExtractPlaceholderRemoteAddr  = "__remote_addr__"
	MITMExtractPlaceholderIP          = "__ip__"
	MITMExtractPlaceholderSourceType  = "__source_type__"
	MITMExtractPlaceholderFlowHash    = "__flow_hash__"
	MITMExtractPlaceholderStatusCode  = "__status_code__"
)

type MITMExtractPlaceholders struct {
	Host        string
	FullURL     string
	URI         string
	Method      string
	Path        string // path only，不含 query
	Scheme      string // http / https 等
	Port        string // 无显式端口时为空
	RuntimeID   string
	HiddenIndex string
	RemoteAddr  string
	IP          string
	SourceType  string
	Hash        string
	StatusCode  string
}

func ExpandMITMExtractPlaceholders(s string, p MITMExtractPlaceholders) string {
	if s == "" {
		return ""
	}
	// 较长 token 优先，降低替换值中偶然包含短字面量时的误伤概率
	replacements := []struct{ from, to string }{
		{MITMExtractPlaceholderHiddenIndex, p.HiddenIndex},
		{MITMExtractPlaceholderRemoteAddr, p.RemoteAddr},
		{MITMExtractPlaceholderSourceType, p.SourceType},
		{MITMExtractPlaceholderRuntimeID, p.RuntimeID},
		{MITMExtractPlaceholderStatusCode, p.StatusCode},
		{MITMExtractPlaceholderFlowHash, p.Hash},
		{MITMExtractPlaceholderTraceID, p.HiddenIndex},
		{MITMExtractPlaceholderURL, p.FullURL},
		{MITMExtractPlaceholderHost, p.Host},
		{MITMExtractPlaceholderURI, p.URI},
		{MITMExtractPlaceholderMethod, p.Method},
		{MITMExtractPlaceholderPath, p.Path},
		{MITMExtractPlaceholderScheme, p.Scheme},
		{MITMExtractPlaceholderSchema, p.Scheme},
		{MITMExtractPlaceholderPort, p.Port},
		{MITMExtractPlaceholderIP, p.IP},
	}
	out := s
	for _, r := range replacements {
		out = strings.ReplaceAll(out, r.from, r.to)
	}
	return out
}

func enrichFromFullURL(p *MITMExtractPlaceholders) {
	raw := strings.TrimSpace(p.FullURL)
	if raw == "" {
		return
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return
	}
	if p.Scheme == "" && u.Scheme != "" {
		p.Scheme = strings.ToLower(u.Scheme)
	}
	if u.Host != "" {
		if p.Host == "" {
			p.Host = u.Host
		}
		if port := u.Port(); port != "" {
			p.Port = port
		}
	}
	if p.URI == "" {
		if ru := u.RequestURI(); ru != "" {
			p.URI = ru
		}
	}
	if p.Path == "" {
		if u.Path != "" {
			p.Path = u.Path
		} else if u.Host != "" {
			p.Path = "/"
		}
	}
}

func pathFromRequestURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" || !strings.HasPrefix(uri, "/") {
		return ""
	}
	u2, err := url.Parse("http://mitm-placeholder.local" + uri)
	if err != nil || u2 == nil {
		return ""
	}
	if u2.Path != "" {
		return u2.Path
	}
	return "/"
}

// BuildMITMExtractPlaceholders 从 HTTPFlow 与（可选）当前请求构造占位符取值。
// req 非空时，URL/Host/Method/Path 等优先采用请求上下文。
func BuildMITMExtractPlaceholders(req *http.Request, flow *schema.HTTPFlow) MITMExtractPlaceholders {
	var p MITMExtractPlaceholders
	if flow != nil {
		p.FullURL = strings.TrimSpace(flow.Url)
		p.Host = strings.TrimSpace(flow.Host)
		p.Method = strings.TrimSpace(flow.Method)
		p.Path = strings.TrimSpace(flow.Path)
		p.RuntimeID = strings.TrimSpace(flow.RuntimeId)
		p.HiddenIndex = strings.TrimSpace(flow.HiddenIndex)
		p.RemoteAddr = strings.TrimSpace(flow.RemoteAddr)
		p.IP = strings.TrimSpace(flow.IPAddress)
		p.SourceType = strings.TrimSpace(flow.SourceType)
		p.Hash = strings.TrimSpace(flow.Hash)
		if flow.StatusCode != 0 {
			p.StatusCode = strconv.FormatInt(flow.StatusCode, 10)
		}
	}
	if req != nil {
		if u := strings.TrimSpace(httpctx.GetRequestURL(req)); u != "" {
			p.FullURL = u
		}
		if h := strings.TrimSpace(req.Host); h != "" {
			p.Host = h
		} else if p.Host == "" && req.URL != nil {
			p.Host = strings.TrimSpace(req.URL.Host)
		}
		if m := strings.TrimSpace(req.Method); m != "" {
			p.Method = m
		}
		if req.URL != nil {
			if ru := req.URL.RequestURI(); ru != "" {
				p.URI = ru
			}
			if req.URL.Path != "" {
				p.Path = req.URL.Path
			}
		}
	}

	enrichFromFullURL(&p)

	if p.URI == "" && flow != nil {
		if fp := strings.TrimSpace(flow.Path); fp != "" {
			p.URI = fp
		}
	}
	if p.Path == "" {
		if derived := pathFromRequestURI(p.URI); derived != "" {
			p.Path = derived
		}
	}

	if p.Port == "" && p.Host != "" {
		if _, port, err := net.SplitHostPort(p.Host); err == nil {
			p.Port = port
		}
	}

	if p.Scheme == "" && flow != nil {
		if flow.IsHTTPS {
			p.Scheme = "https"
		} else if strings.TrimSpace(p.FullURL) != "" || strings.TrimSpace(flow.Url) != "" || strings.TrimSpace(flow.Host) != "" {
			p.Scheme = "http"
		}
	}

	return p
}

func BuildMITMExtractPlaceholdersLowhttp(req *http.Request, lowhttpURL string) MITMExtractPlaceholders {
	var fake *schema.HTTPFlow
	if strings.TrimSpace(lowhttpURL) != "" {
		fake = &schema.HTTPFlow{Url: strings.TrimSpace(lowhttpURL)}
	}
	return BuildMITMExtractPlaceholders(req, fake)
}

func CloneMatchResultWithMITMPlaceholders(match *MatchResult, p MITMExtractPlaceholders) *MatchResult {
	if match == nil {
		return nil
	}
	out := *match
	out.MatchResult = ExpandMITMExtractPlaceholders(match.MatchResult, p)
	return &out
}
