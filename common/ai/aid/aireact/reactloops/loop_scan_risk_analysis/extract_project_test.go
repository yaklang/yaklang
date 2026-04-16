package loop_scan_risk_analysis

import "testing"

func TestExtractProjectNameForScanAnalysis(t *testing.T) {
	zh := "\u5206\u6790 go-sec-code \u7684\u9879\u76ee\u626b\u63cf\u7ed3\u679c"
	latest := "go-sec-code \u5206\u6790\u6700\u65b0\u4e00\u6b21\u7684\u626b\u63cf"
	analyzeLatestDe := "\u5206\u6790 go-sec-code \u6700\u65b0\u7684\u4e00\u6b21\u626b\u63cf"
	cases := []struct {
		in   string
		want string
	}{
		{in: zh, want: "go-sec-code"},
		{in: latest, want: "go-sec-code"},
		{in: analyzeLatestDe, want: "go-sec-code"},
		{in: "\u8bf7\u5206\u6790 my-proj_name \u7684\u9879\u76ee\u626b\u63cf", want: "my-proj_name"},
		{in: "no project phrase here", want: ""},
	}
	for _, c := range cases {
		got := extractProjectNameForScanAnalysis(c.in)
		if got != c.want {
			t.Fatalf("extractProjectNameForScanAnalysis(%q)=%q want=%q", c.in, got, c.want)
		}
	}
}

func TestParseStrictProjectNameLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "project_name=go-sec-code", want: "go-sec-code"},
		{in: "project_name = my_repo-1", want: "my_repo-1"},
		{in: "Project_Name=\"yak-core\"", want: "yak-core"},
		{in: "project_name=   go-sec-code   ", want: "go-sec-code"},
		{in: "project_name=\n\t\"go-sec-code\"\n", want: "go-sec-code"},
		{in: "project_name=***go-sec-code***", want: "go-sec-code"},
		{in: "project_name=【go-sec-code】", want: "go-sec-code"},
		{in: "project_name=go-sec-code\"}", want: "go-sec-code"},
		{in: "```text\nproject_name=go-sec-code\"}\n```", want: "go-sec-code"},
		{in: "go-sec-code分析扫描结果", want: ""},
		{in: `{"suggestion":"go-sec-code"}`, want: ""},
		{in: `result: {"suggestion":"go-sec-code"}`, want: ""},
		{in: "go-sec-code", want: ""},
		{in: "", want: ""},
	}
	for _, c := range cases {
		got := parseStrictProjectNameLine(c.in)
		if got != c.want {
			t.Fatalf("parseStrictProjectNameLine(%q)=%q want=%q", c.in, got, c.want)
		}
	}
}

func TestParseOptionalPlainProjectSlug(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "go-sec-code", want: "go-sec-code"},
		{in: "  my_repo-1  ", want: "my_repo-1"},
		{in: "project_name=go-sec-code", want: ""},
		{in: "go sec", want: ""},
		{in: "a\nb", want: ""},
		{in: "", want: ""},
	}
	for _, c := range cases {
		got := parseOptionalPlainProjectSlug(c.in)
		if got != c.want {
			t.Fatalf("parseOptionalPlainProjectSlug(%q)=%q want=%q", c.in, got, c.want)
		}
	}
}

func TestParseInteractiveProjectNameReply(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "project_name=go-sec-code", want: "go-sec-code"},
		{in: `{"suggestion":"go-sec-code"}`, want: "go-sec-code"},
		{in: `result: {"suggestion":"go-sec-code"}`, want: "go-sec-code"},
		{in: `{"result":{"project_name":"go-sec-code"}}`, want: "go-sec-code"},
		{in: "go-sec-code", want: ""},
	}
	for _, c := range cases {
		got := parseInteractiveProjectNameReply(c.in)
		if got != c.want {
			t.Fatalf("parseInteractiveProjectNameReply(%q)=%q want=%q", c.in, got, c.want)
		}
	}
}
