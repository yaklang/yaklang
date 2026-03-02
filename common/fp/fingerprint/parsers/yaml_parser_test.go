package parsers

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/embed"
	"testing"
)

func TestMatcher(t *testing.T) {
	content, err := embed.Asset("data/fingerprint-rules.yml.gz")
	if err != nil {
		t.Fatal(err)
	}
	rules, err := ParseYamlRule(string(content))
	if err != nil {
		t.Fatal(err)
	}
	_ = rules
}

func TestConvertOldYamlWebRuleToGeneralRule_ShouldNotMixCPEAcrossMatchers(t *testing.T) {
	webRules := []*webfingerprint.WebRule{{
		Methods: []*webfingerprint.WebMatcherMethods{{
			Keywords: []*webfingerprint.KeywordMatcher{{
				CPE:          webfingerprint.CPE{Vendor: "apache", Product: "http_server"},
				Regexp:       `Apache/([0-9.]+)`,
				VersionIndex: 1,
			}},
			HTTPHeaders: []*webfingerprint.HTTPHeaderMatcher{{
				HeaderName: "Server",
				HeaderValue: webfingerprint.KeywordMatcher{
					CPE:          webfingerprint.CPE{Product: "iis"},
					Regexp:       `Microsoft-IIS/([0-9.]+)`,
					VersionIndex: 1,
				},
			}},
		}},
	}}

	rules, err := ConvertOldYamlWebRuleToGeneralRule(webRules)
	if err != nil {
		t.Fatal(err)
	}
	assert.Len(t, rules, 2)

	rsp := []byte("HTTP/1.1 200 OK\r\nServer: Microsoft-IIS/8.5\r\n\r\n")
	for _, r := range rules {
		info, err := rule.Execute(func(string) (*rule.MatchResource, error) {
			return rule.NewHttpResource(rsp), nil
		}, r)
		if err != nil {
			t.Fatal(err)
		}
		if info == nil {
			continue
		}
		info.Init()
		assert.False(t, info.Vendor == "apache" && info.Product == "iis")
	}
}
