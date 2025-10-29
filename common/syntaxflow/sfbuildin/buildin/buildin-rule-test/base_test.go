package buildin_rule

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/syntaxflow/sfdb"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample
var samples embed.FS

type BuildinRuleTestCase struct {
	Name string
	Rule string
	FS   map[string]string

	// if negative test set, the result is empty or error
	// it means no vuln / result found
	NegativeTest bool

	ContainsAll    []string
	NotContainsAny []string
}

var Cases = []BuildinRuleTestCase{
	{
		Name: "检测Java任意文件下载(attachment)",
		// Rule: `java-filedownload-attachment-filename`,
		Rule: "Checking [Filename Attachment when Filedownloading]",
		FS: map[string]string{
			"download.java": "download.java",
		},
		ContainsAll: []string{"attachment", "filename"},
	},
	{
		Name: "XStream 基础使用",
		// Rule: `java-xstream-unsafe`,
		Rule: "XStream 未明确设置安全策略（.setMode(XStream.NO_REFERENCES)）",
		FS: map[string]string{
			"xstream.java": "xstream.java",
		},
		ContainsAll: []string{"xstream.fromXML"},
	},

	{
		Name: "XStream 类成员 - 基础使用",
		// Rule: `java-xstream-unsafe`,
		Rule: "XStream 未明确设置安全策略（.setMode(XStream.NO_REFERENCES)）",
		FS: map[string]string{
			"xstream-2.java": "xstream-2.java",
		},
		ContainsAll: []string{"xstreamInstance.fromXML"},
	},

	{
		Name: "XStream 类成员(空成员) - 基础使用",
		// Rule: `java-xstream-unsafe`,
		Rule: "XStream 未明确设置安全策略（.setMode(XStream.NO_REFERENCES)）",
		FS: map[string]string{
			"xstream-3.java": "xstream-3.java",
		},
		ContainsAll: []string{"xstreamInstance.fromXML"},
	},

	{
		Name: "XStream 基础使用(negative)",
		// Rule: `java-xstream-unsafe`,
		Rule: "XStream 未明确设置安全策略（.setMode(XStream.NO_REFERENCES)）",
		FS: map[string]string{
			"xstream-safe.java": "xstream-safe.java",
		},
		NegativeTest: true,
	},

	{
		Name: "SAXBuilder 基础使用(安全)",
		// Rule: `java-saxbuilder-unsafe`,
		Rule: "SAXBuilder 未明确设置安全策略（.setFeature(...)）",
		FS: map[string]string{
			"saxbuilder-safe.java": "saxbuilder-safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXBuilder 基础使用不安全",
		// Rule: `java-saxbuilder-unsafe`,
		Rule: "SAXBuilder 未明确设置安全策略（.setFeature(...)）",
		FS: map[string]string{
			"saxbuilder-unsafe.java": "saxbuilder-unsafe.java",
		},
		NegativeTest: false,
		ContainsAll:  []string{"SAXBuilder"},
	},
	{
		Name: "SAXParserFactory 基础检查",
		// Rule: `java-saxparser-factory-unsafe`,
		Rule: "检测 SAXParserFactory() 不安全使用",
		FS: map[string]string{
			"saxparser-factory-unsafe.java": "saxparser-factory-unsafe.java",
		},
		NegativeTest: false,
		ContainsAll:  []string{"SAXParserFactory"},
	},
	{
		Name: "SAXParserFactory 基础检查(安全)",
		// Rule: `java-saxparser-factory-unsafe`,
		Rule: "检测 SAXParserFactory() 不安全使用",
		FS: map[string]string{
			"saxparser-factory-safe.java": "saxparser-factory-safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXReader 基础检查(安全)",
		// Rule: `java-saxreader-unsafe`,
		Rule: "SAXReader 未明确设置安全策略（.setFeature(...) ...）",
		FS: map[string]string{
			"saxreazder.java": "saxreader/safe.java",
		},
		NegativeTest: true,
	},
	{
		Name: "SAXReader 基础检查(不安全)",
		// Rule: `java-saxreader-unsafe`,
		Rule: "SAXReader 未明确设置安全策略（.setFeature(...) ...）",
		FS: map[string]string{
			"saxreazder.java": "saxreader/unsafe.java",
		},
		ContainsAll: []string{"SAXReader"},
	},

	{
		Name: "XMLReaderFactory 基础检查(不安全)",
		// Rule: `java-xmlreader-factory-unsafe`,
		Rule: "检查 XMLReaderFactory.createXMLReader() 不安全使用",
		FS: map[string]string{
			"xmlreaderfactory.java": "org-xml-sax-xmlreader/unsafe.java",
		},
		ContainsAll: []string{"createXMLReade", "example.xml"},
	},
	{
		Name: "XMLReaderFactory 基础检查(消极测试)",
		// Rule: `java-xmlreader-factory-unsafe`,
		Rule: "检查 XMLReaderFactory.createXMLReader() 不安全使用",
		FS: map[string]string{
			"xmlreaderfactory.java": "org-xml-sax-xmlreader/safe.java",
		},
		NegativeTest: true,
	},
}

func TestBuildInRule(t *testing.T) {
	t.Skip("AI修正了title合desc，不需要再测试了")
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

func run(t *testing.T, name string, c BuildinRuleTestCase) {
	t.Run(name, func(t *testing.T) {
		rules, err := sfdb.GetRules(c.Rule)
		if err != nil {
			t.Fatal(err)
		}
		if len(rules) <= 0 {
			t.Fatal("no rule found")
		}
		vfs := filesys.NewVirtualFs()
		for k, v := range c.FS {
			filesys.Recursive(".", filesys.WithEmbedFS(samples), filesys.WithFileStat(func(s string, info fs.FileInfo) error {
				_, name := path.Split(s)
				if utils.MatchAllOfGlob(name, v) {
					raw, err := samples.ReadFile(s)
					if err != nil {
						log.Warnf("read file %s error: %s", s, err)
						t.Fatal("load file error: " + v)
					}
					vfs.AddFile(k, string(raw))
				}

				if strings.HasSuffix(s, v) {
					raw, err := samples.ReadFile(s)
					if err != nil {
						log.Warnf("read file %s error: %s", s, err)
						t.Fatal("load file error: " + v)
					}
					vfs.AddFile(k, string(raw))
				}
				return nil
			}))
		}
		for _, r := range rules {
			ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
				if len(programs) <= 0 {
					t.Fatal("no program found")
				}
				for _, prog := range programs {
					result, err := prog.SyntaxFlowWithError(r.Content)
					if !c.NegativeTest {
						if err != nil || result.GetErrors() != nil {
							if err != nil {
								t.Fatal(err)
							} else {
								t.Fatal(result.GetErrors())
							}
						}
					} else {
						if err == nil && len(result.GetErrors()) == 0 {
							count := 0
							for _, i := range result.GetAlertVariables() {
								count += len(result.GetValues(i))
							}
							if count > 0 {
								t.Fatal("no alert variables should, negative test failed")
							}
						}

						if errors.Is(err, sfvm.CriticalError) {
							t.Fatal("cannot accept critical error: " + err.Error())
						}

						if len(result.GetAlertVariables()) > 0 {
							count := 0
							for _, i := range result.GetAlertVariables() {
								// i.Recursive(func(operator sfvm.ValueOperator) error {
								count += len(result.GetValues(i))
								// 	return nil
								// })
							}
							if count > 0 {
								t.Fatal("no alert variables should, negative test failed")
							}
						}
						return nil
					}

					if len(result.GetAlertVariables()) >= 0 {
						for _, name := range result.GetAlertVariables() {
							val := result.GetValues(name)
							msg := fmt.Sprintf("%v\n%s\n%s\n\n", r.Severity, name, val)
							t.Logf(msg)
							if len(c.ContainsAll) > 0 {
								if !utils.MatchAllOfSubString(msg, c.ContainsAll...) {
									t.Fatal("not all contains")
								}
							}
							if len(c.NotContainsAny) > 0 {
								if utils.MatchAnyOfSubString(msg, c.NotContainsAny...) {
									t.Fatal("contain any")
								}
							}
							programName := ""
							result := ssadb.CreateResult()
							defer ssadb.DeleteResultByID(result.ID)
							val.Recursive(func(operator sfvm.ValueOperator) error {
								switch ret := operator.(type) {
								case *ssaapi.Value:
									if ret.GetProgramName() == "" {
										return nil
									}
									programName = ret.GetProgramName()
									return ssaapi.SaveValue(
										ret,
										ssaapi.OptionSaveValue_ResultID(result.ID),
										ssaapi.OptionSaveValue_RuleTitle(r.Title),
										ssaapi.OptionSaveValue_ProgramName(programName),
										ssaapi.OptionSaveValue_RuleName(r.RuleName),
									)
								}
								return nil
							})
							count := 0
							for node := range ssadb.YieldAuditNodeByResultId(ssadb.GetDB(), result.ID) {
								count++
								_ = node
							}
							if programName != "" {
								assert.Greater(t, count, 0)
							}
						}
					} else {
						t.Fatal("no alert found no result found")
					}
				}
				return nil
			}, ssaapi.WithLanguage(ssaconfig.JAVA))
		}
	})
}
