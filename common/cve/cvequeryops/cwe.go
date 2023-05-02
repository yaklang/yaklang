package cvequeryops

import (
	"bytes"
	"github.com/antchfx/xmlquery"
	"github.com/jinzhu/gorm"
	"io"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"yaklang.io/yaklang/common/consts"
	"yaklang.io/yaklang/common/cve/cveresources"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/ziputil"
)

func DownloadCWE() (string, error) {
	fp, err := consts.TempFile("cwe-latest-*.zip")
	if err != nil {
		return "", err
	}
	defer fp.Close()
	// https://cwe.mitre.org/data/xml/cwec_latest.xml.zip
	rsp, err := http.Get(`https://cwe.mitre.org/data/xml/cwec_latest.xml.zip`)
	if err != nil {
		log.Errorf("download mitre cwe failed: %s", err)
		return "", err
	}
	io.Copy(fp, rsp.Body)
	return fp.Name(), nil
}

func SaveCWE(db *gorm.DB, cwes []*cveresources.CWE) {
	for _, i := range cwes {
		//log.Infof("start save cwe: %v", i.CWEString())
		if d := db.Model(&cveresources.CWE{}).Save(i); d.Error != nil {
			log.Errorf("save error: %s", d.Error)
		}
	}
}

func LoadCWE(cweXMLPath string) ([]*cveresources.CWE, error) {
	extracted := filepath.Join(consts.GetDefaultYakitBaseTempDir(), "cwe")
	err := ziputil.DeCompress(cweXMLPath, extracted)
	if err != nil {
		return nil, err
	}

	var targetPath string
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
	if targetPath == "" {
		return nil, utils.Errorf("Target Path: %v is not existed or un-zip failed", cweXMLPath)
	}

	raw, err := ioutil.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}
	node, err := xmlquery.Parse(bytes.NewBuffer(raw))
	if err != nil {
		return nil, err
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

		var inferTo []string
		var siblings []string
		var requires []string
		var parent []string
		xmlquery.FindEach(cweInstance, `//Related_Weaknesses/Related_Weakness`, func(i int, node *xmlquery.Node) {
			idStr := strings.TrimSpace(node.SelectAttr(`CWE_ID`))
			var id, _ = strconv.Atoi(idStr)
			if id <= 0 {
				return
			}
			switch ret := strings.ToLower(node.SelectAttr("Nature")); ret {
			case "childof":
				if !utils.StringArrayContains(parent, idStr) {
					parent = append(parent, idStr)
				}
			case "peerof", "canalsobe":
				if !utils.StringArrayContains(siblings, idStr) {
					siblings = append(siblings, idStr)
				}
			case "canprecede":
				if !utils.StringArrayContains(inferTo, idStr) {
					inferTo = append(inferTo, idStr)
				}
			case "requires", "startswith":
				if !utils.StringArrayContains(requires, idStr) {
					requires = append(requires, idStr)
				}
			default:
				log.Infof("unhandled relation")
				return
			}
		})
		cwe.InferTo = strings.Join(inferTo, ",")
		cwe.Siblings = strings.Join(siblings, ",")
		cwe.Requires = strings.Join(requires, ",")
		cwe.Parent = strings.Join(parent, ",")

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
	return cwes, nil
}
