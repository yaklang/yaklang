package yak

import (
	"testing"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	yaklangspec "github.com/yaklang/yaklang/common/yak/yaklang/spec"
)

func TestCounter(t *testing.T) {
	_ = yaklang.New()
	var count = 0
	for key, value := range yaklangspec.Fntable {
		val, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		switch key {
		case "env", "sync", "codec",
			"mmdb", "crawler", "mitm", "tls",
			"xhtml", "cli", "fuzz", "httpool", "http", "httpserver",
			"dictutil", "tools", "synscan", "tcp", "servicescan",
			"subdomain", "exec", "brute", "dns", "ping", "spacengine",
			"json", "dyn", "nuclei", "yakit", "jwt", "java", "poc", "csrf",
			"risk", "report", "xpath", "hook", "yso", "facades", "t3",
			"iiop", "js", "smb", "ldap", "redis", "rdp", "crawlerx":
			count += len(val)
		}
	}
	println(count)
}
