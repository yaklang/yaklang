package yakit

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

type MITMExtractPlaceholders struct {
	Host    string
	FullURL string
	URI     string
}

func ExpandMITMExtractPlaceholders(s string, p MITMExtractPlaceholders) string {
	if s == "" {
		return ""
	}
	out := strings.ReplaceAll(s, "__url__", p.FullURL)
	out = strings.ReplaceAll(out, "__host__", p.Host)
	out = strings.ReplaceAll(out, "__uri__", p.URI)
	return out
}

func BuildMITMExtractPlaceholders(req *http.Request, flow *schema.HTTPFlow) MITMExtractPlaceholders {
	var p MITMExtractPlaceholders
	if flow != nil {
		p.FullURL = strings.TrimSpace(flow.Url)
		p.Host = strings.TrimSpace(flow.Host)
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
		if req.URL != nil {
			if ru := req.URL.RequestURI(); ru != "" {
				p.URI = ru
			}
		}
	}
	if p.URI == "" && p.FullURL != "" {
		if u, err := url.Parse(p.FullURL); err == nil {
			p.URI = u.RequestURI()
			if p.Host == "" {
				p.Host = u.Host
			}
		}
	}
	if p.URI == "" && flow != nil && flow.Path != "" {
		p.URI = flow.Path
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