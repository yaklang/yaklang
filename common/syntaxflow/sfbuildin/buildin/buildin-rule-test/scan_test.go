package buildin_rule

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

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
				err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t)
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

	var name = "php-custom_param.sf"

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

	yakit.InitialDatabase()
	ruleName := "java-reflection-for-class-unsafe.sf"

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
			err := ssatest.EvaluateVerifyFilesystemWithRule(rule, t)
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

func TestImport(t *testing.T) {
	err := sfdb.ImportRuleWithoutValid("test.sf", `
desc(
	title: "import test",
	level: "high",
	lang: "php",
)
$a #-> * as $param

alert $param for {"level": "high"}
`, true)
	require.NoError(t, err)
	rule, err := sfdb.GetRule("test.sf")
	require.NoError(t, err)
	var m map[string]*schema.ExtraDescInfo
	fmt.Println(rule.AlertDesc)
	err = json.Unmarshal(codec.AnyToBytes(rule.AlertDesc), &m)
	require.NoError(t, err)
	info, ok := m["param"]
	require.True(t, ok)
	require.True(t, info.Level == schema.SFR_SEVERITY_HIGH)
	err = sfdb.DeleteRuleByRuleName("test.sf")
	require.NoError(t, err)
}

func TestJavaDependencies(t *testing.T) {
	code := `
__dependency__.*fastjson.version as $ver;
$ver?{version_in:(1.2.3,2.3.4]}  as $vulnVersion
alert $vulnVersion for {
	title:"存在fastjson 1.2.3-2.3.4漏洞",
};

desc(
lang: java,
'file://pom.xml': <<<CODE
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.example</groupId>
    <artifactId>vulnerable-fastjson-app</artifactId>
    <version>1.0-SNAPSHOT</version>

    <dependencies>
        <!-- Fastjson dependency with known vulnerabilities -->
        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <!-- An example version with known vulnerabilities, make sure to check for specific vulnerable versions -->
            <version>1.2.24</version>
        </dependency>
    </dependencies>
</project>
CODE
)`
	err := ssatest.EvaluateVerifyFilesystem(code, t)
	if err != nil {
		t.Fatal(err)
	}
}
