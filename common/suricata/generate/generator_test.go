package generate

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"strings"
	"testing"
)

const ruleraw = `alert http any any -> any any (msg:httptest;content:"/";http.uri;content:"/";http.uri.raw;content:GET;http.method;content:HTTP/1.1;http.protocol;content:"GET / HTTP/1.1|0d 0a|";http.request_line;content:"Mozilla/5.0 (Windows NT; Windows NT 10.0; zh-CN) WindowsPowerShell/5.1.22621.1778";http.user_agent;endswith;content:"|0d 0a|Accept-Encoding|0d 0a|Host|0d 0a|User-Agent|0d 0a 0d 0a|";http.header_names;)`

func TestNewSurigen(t *testing.T) {
	rules := strings.SplitN(ruleraw, "\n", -1)
	for _, r := range rules {
		r, err := rule.Parse(r)
		if err != nil {
			t.Error(err)
		}
		surigen, err := New(r[0])
		if err != nil {
			t.Error(err)
		}
		gen := surigen.Gen()
		if err != nil {
			return
		}
		fmt.Println(string(gen))
	}
}

var gen_testcases = []string{
	"alert tcp $HOME_NET any -> $EXTERNAL_NET $HTTP_PORTS (msg:\"ET TROJAN Win32/Agent.NJX Checkin\"; flow:established,to_server; content:\"/checkin.php?\"; http_uri; content:\"User-Agent|3a| Mozilla/4.0 (compatible|3b| MSIE 6.0|3b| Windows NT 5.1|3b| SV1)\"; http_header; fast_pattern:only; content:\"Host|3a| www.51yund.com|0d 0a|\"; http_header; metadata:ruleset community, service http; reference:url,www.threatexpert.com/report.aspx?md5=3d1b0b6a0b0b0b0b0b0b0b0b0b0b0b0b; classtype:trojan-activity; sid:2014144; rev:3;)",
	`alert http any any -> any any (msg:"config.pinyin.sogou";http.server;content:nginx;http.server_body;content:"[setting]|0a|";pcre:"/([a-z]+=\\d+\\s?)+/iRQ";)`,
}

func TestGen(t *testing.T) {
	for _, c := range gen_testcases {
		rules, _ := rule.Parse(c)
		assert.Equal(t, len(rules), 1)
		rule := rules[0]
		generator, _ := New(rule)
		assert.NotNil(t, generator)
		output := generator.Gen()
		spew.Dump(output)
	}
}
