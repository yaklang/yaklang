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
		Name: "XStream 类成员 - 基础使用",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-2.java": "xstream-2.java",
		},
		ContainsAll: []string{"xstreamInstance.fromXML"},
	},

	{
		Name: "XStream 类成员(空成员) - 基础使用",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-3.java": "xstream-3.java",
		},
		ContainsAll: []string{"xstreamInstance.fromXML"},
	},

	{
		Name: "XStream 基础使用(negative)",
		Rule: `java-xstream-unsafe`,
		FS: map[string]string{
			"xstream-safe.java": "xstream-safe.java",
		},
		NegativeTest: true,
	},

	{
		Name: "SAXBuilder 基础使用(安全)",
		Rule: `java-saxbuilder-unsafe`,
		FS: map[string]string{
			"saxbuilder-safe.java": "saxbuilder-safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXBuilder 基础使用不安全",
		Rule: `java-saxbuilder-unsafe`,
		FS: map[string]string{
			"saxbuilder-unsafe.java": "saxbuilder-unsafe.java",
		},
		NegativeTest: false,
		ContainsAll:  []string{"SAXBuilder"},
	},
	{
		Name: "SAXParserFactory 基础检查",
		Rule: `java-saxparser-factory-unsafe`,
		FS: map[string]string{
			"saxparser-factory-unsafe.java": "saxparser-factory-unsafe.java",
		},
		NegativeTest: false,
		ContainsAll:  []string{"SAXParserFactory"},
	},
	{
		Name: "SAXParserFactory 基础检查(安全)",
		Rule: `java-saxparser-factory-unsafe`,
		FS: map[string]string{
			"saxparser-factory-safe.java": "saxparser-factory-safe.java",
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

	var name = "SAXParserFactory"

	for i := 0; i < len(Cases); i++ {
		c := Cases[i]

		if !utils.MatchAllOfSubString(c.Name, name) {
			t.Log("skip " + c.Name)
			continue
		}
		run(t, c.Name, c)
	}
}
