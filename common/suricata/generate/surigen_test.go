package generate

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"testing"
)

var gen_testcases = []string{
	"alert tcp $HOME_NET any -> $EXTERNAL_NET $HTTP_PORTS (msg:\"ET TROJAN Win32/Agent.NJX Checkin\"; flow:established,to_server; content:\"/checkin.php?\"; http_uri; content:\"User-Agent|3a| Mozilla/4.0 (compatible|3b| MSIE 6.0|3b| Windows NT 5.1|3b| SV1)\"; http_header; fast_pattern:only; content:\"Host|3a| www.51yund.com|0d 0a|\"; http_header; metadata:ruleset community, service http; reference:url,www.threatexpert.com/report.aspx?md5=3d1b0b6a0b0b0b0b0b0b0b0b0b0b0b0b; classtype:trojan-activity; sid:2014144; rev:3;)",
	`alert http any any -> any any (msg:"config.pinyin.sogou";http.server;content:nginx;http.server_body;content:"[setting]|0a|";pcre:"/([a-z]+=\\d+\\s?)+/iRQ";)`,
}

func TestGen(t *testing.T) {
	for _, c := range gen_testcases {
		rules, _ := rule.Parse(c)
		assert.Equal(t, len(rules), 1)
		rule := rules[0]
		generator, _ := NewRulegen(rule)
		assert.NotNil(t, generator)
		output := generator.Gen()
		spew.Dump(output)
	}
}
