package buildin_rule

import (
	"context"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

var Cases = []BuildinRuleTestCase{
	{
		Name: "检测Java任意文件下载",
		Rule: `java-springboot-filedownload`,
		FS: map[string]string{
			"download.java": "download.java",
		},
		ContainsAll: []string{"middle", "headers"},
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
	{
		Name: "SAXReader 基础检查(安全)",
		Rule: `java-saxreader-unsafe`,
		FS: map[string]string{
			"saxreazder.java": "saxreader/safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXReader 基础检查(不安全)",
		Rule: `java-saxreader-unsafe`,
		FS: map[string]string{
			"saxreazder.java": "saxreader/unsafe.java",
		},
		ContainsAll: []string{"SAXReader"},
	},

	{
		Name: "XMLReaderFactory 基础检查(不安全)",
		Rule: `java-xmlreader-factory-unsafe`,
		FS: map[string]string{
			"xmlreaderfactory.java": "org-xml-sax-xmlreader/unsafe.java",
		},
		ContainsAll: []string{"createXMLReade", "example.xml"},
	},
	{
		Name: "XMLReaderFactory 基础检查(消极测试)",
		Rule: `java-xmlreader-factory-unsafe`,
		FS: map[string]string{
			"xmlreaderfactory.java": "org-xml-sax-xmlreader/safe.java",
		},
		NegativeTest: true,
	},
}

func TestVerifiedRule(t *testing.T) {
	yakit.InitialDatabase()
	for rule := range sfdb.YieldSyntaxFlowRules(consts.GetGormProfileDatabase(), context.Background()) {
		f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
		if err != nil {
			t.Fatalf("compile rule %s error: %s", rule.RuleName, err)
		}
		if len(f.VerifyFs) > 0 || len(f.NegativeFs) > 0 {
			t.Run(strings.Join(append(strings.Split(rule.Tag, "|"), rule.RuleName), "/"), func(t *testing.T) {
				t.Log("Start to verify: " + rule.RuleName)
				err := ssatest.EvaluateVerifyFilesystem(rule.Content, t)
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
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

	var name = "SAXReader 基础检查(安全)"

	for i := 0; i < len(Cases); i++ {
		c := Cases[i]

		if !utils.MatchAllOfSubString(c.Name, name) {
			t.Log("skip " + c.Name)
			continue
		}
		run(t, c.Name, c)
	}
}

func TestBuildInRule_Verify_DEBUG(t *testing.T) {
	if utils.InGithubActions() {
		t.SkipNow()
		return
	}

	ruleName := "java-servlet-n-spring-concat-command-injection.sf"

	rule, err := sfdb.GetRule(ruleName)
	if err != nil {
		t.Fatal(err)
	}

	f, err := sfvm.NewSyntaxFlowVirtualMachine().Compile(rule.Content)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.VerifyFs) > 0 || len(f.NegativeFs) > 0 {
		t.Run(rule.RuleName, func(t *testing.T) {
			t.Log("Start to verify: " + rule.RuleName)
			err := ssatest.EvaluateVerifyFilesystem(rule.Content, t)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildInRule_Verify_Negative_AlertMin(t *testing.T) {
	err := ssatest.EvaluateVerifyFilesystem(`
desc(
alert_min: '2',
language: yaklang,
'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;

`, t)
	if err == nil {
		t.Fatal("expect error")
	}
}

func TestBuildInRule_Verify_Positive_AlertMin2(t *testing.T) {
	err := ssatest.EvaluateVerifyFilesystem(`
desc(
alert_min: 1,
language: yaklang,
'file://a.yak': <<<EOF
b = () => {
	a = 1;
}
EOF
)

a as $output;
check $output;
alert $output;

`, t)
	if err != nil {
		t.Fatal(err)
	}
}
