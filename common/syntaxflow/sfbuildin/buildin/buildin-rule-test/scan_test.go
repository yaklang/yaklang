package buildin_rule

import (
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

var Cases = []BuildinRuleTestCase{
	{
		Name: "检测Java任意文件下载",
		Rule: `java-springboot-filedownload`,
		FS: map[string]string{
			"download.java": "download.java",
		},
		ContainsAll: []string{"middle", "FileSystemResource"},
	},
	{
		Name: "检测Java任意文件下载(attachment)",
		Rule: `java-filedownload-attachment-filename`,
		FS: map[string]string{
			"download.java": "download.java",
		},
		ContainsAll: []string{"attachment", "filename"},
	},
	{
		Name: "XStream 基础使用",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream.java": "xstream.java",
		},
		ContainsAll: []string{"xstream.fromXML"},
	},

	{
		Name: "XStream 基础使用(negative)",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-safe.java": "xstream-safe.java",
		},
		NegativeTest: true,
	},
}

func TestBuildInRule(t *testing.T) {
	for i := 0; i < len(Cases); i++ {
		c := Cases[i]
		run(t, c.Name, c)
	}
}

func TestBuildInRule_DEBUG(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
		return
	}
	for i := 0; i < len(Cases); i++ {
		c := Cases[i]
		if !utils.MatchAllOfSubString(c.Rule, `java-xstream-unsafe`) {
			t.Log("skip " + c.Rule)
			continue
		}
		run(t, c.Name, c)
	}
}
