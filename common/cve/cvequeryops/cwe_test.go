package cvequeryops

import (
	"bytes"
	"github.com/antchfx/xmlquery"
	"github.com/davecgh/go-spew/spew"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/cve/cveresources"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ziputil"
)

func TestCWE_Download2(t *testing.T) {
	f, err := DownloadCWE()
	if err != nil {
		panic(err)
	}
	println(f)
}

func TestCWE_XMLHANDLER(t *testing.T) {
	var targetPath string = `/Users/v1ll4n/yakit-projects/temp/cwe/cwec_v4.10.xml`
	if utils.GetFirstExistedFile(targetPath) == "" {
		extracted := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "cwe")
		err := ziputil.DeCompress(`/Users/v1ll4n/yakit-projects/temp/cwe-latest-4146988470.zip`, extracted)
		if err != nil {
			panic(err)
		}

		infos, err := utils.ReadDir(extracted)
		if err != nil {
			panic(err)
		}
		for _, i := range infos {
			if i.IsDir {
				continue
			}
			matched, _ := regexp.MatchString(`cwec_(.*?)\.xml`, i.Name)
			if matched {
				targetPath = i.Path
				break
			}
		}
		println(targetPath)
	}
	raw, err := ioutil.ReadFile(targetPath)
	if err != nil {
		panic(err)
	}
	node, err := xmlquery.Parse(bytes.NewBuffer(raw))
	if err != nil {
		panic(err)
	}
	var cwes []*cveresources.CWE
	xmlquery.FindEach(node, `//Weaknesses/Weakness`, func(i int, cweInstance *xmlquery.Node) {
		cwe := &cveresources.CWE{}
		cwe.IdStr = cweInstance.SelectAttr("ID")
		cwe.Id, _ = strconv.Atoi(cwe.IdStr)
		cwe.Name = cweInstance.SelectAttr("Name")
		cwe.Abstraction = cweInstance.SelectAttr("Abstraction")
		cwe.Status = cweInstance.SelectAttr("Status")

		if ret := xmlquery.FindOne(cweInstance, `//Description`); ret != nil {
			cwe.Description = ret.InnerText()
		}
		var descEx []string
		xmlquery.FindEach(cweInstance, `//Extended_Description`, func(i int, node *xmlquery.Node) {
			descEx = append(descEx, node.InnerText())
		})
		xmlquery.FindEach(cweInstance, `//Extended_Description/p`, func(i int, node *xmlquery.Node) {
			descEx = append(descEx, node.InnerText())
		})
		cwe.ExtendedDescription = strings.Join(descEx, "\n")
		cwe.ExtendedDescription = strings.TrimSpace(cwe.ExtendedDescription)

		var children []string
		var inferTo []string
		var siblings []string
		var requires []string
		xmlquery.FindEach(cweInstance, `//Related_Weaknesses/Related_Weakness`, func(i int, node *xmlquery.Node) {
			idStr := strings.TrimSpace(node.SelectAttr(`CWE_ID`))
			var id, _ = strconv.Atoi(idStr)
			if id <= 0 {
				return
			}
			switch ret := strings.ToLower(node.SelectAttr("Nature")); ret {
			case "children", "childof":
				children = append(children, idStr)
			case "peerof", "canalsobe":
				siblings = append(siblings, idStr)
			case "canprecede":
				inferTo = append(inferTo, idStr)
			case "requires", "startswith":
				requires = append(requires, idStr)
			default:
				log.Infof("unhandled relation")
				return
			}
		})
		cwe.InferTo = strings.Join(inferTo, ",")
		cwe.Siblings = strings.Join(siblings, ",")
		cwe.Requires = strings.Join(requires, ",")

		var langs []string
		xmlquery.FindEach(cweInstance, `//Applicable_Platforms/Language`, func(i int, node *xmlquery.Node) {
			if a := node.SelectAttr("Name"); a != "" {
				langs = append(langs, a)
			}
		})
		cwe.RelativeLanguage = strings.Join(langs, ",")
		var cves []string
		xmlquery.FindEach(cweInstance, `//Observed_Examples/Observed_Example/Reference`, func(i int, node *xmlquery.Node) {
			if ret := strings.TrimSpace(node.InnerText()); ret != "" {
				cves = append(cves, ret)
			}
		})
		cwe.CVEExamples = strings.Join(cves, ",")
		var capec []string
		xmlquery.FindEach(cweInstance, `//Related_Attack_Patterns/Related_Attack_Pattern`, func(i int, node *xmlquery.Node) {
			if ret := node.SelectAttr("CAPEC_ID"); ret != "" {
				id, _ := strconv.Atoi(ret)
				if id > 0 {
					capec = append(capec, ret)
				}
			}
		})
		cwe.CAPECVectors = strings.Join(capec, ",")
		cwes = append(cwes, cwe)
	})
	spew.Dump(len(cwes))
}
