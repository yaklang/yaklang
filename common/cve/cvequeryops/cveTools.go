package cvequeryops

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"yaklang.io/yaklang/common/cve/cveresources"
	"yaklang.io/yaklang/common/log"
)

const scriptFormat = `
# port scan plugin
yakit.AutoInitYakit()

CVE = []
addCVE = func(cve, rules, score, severity, titleZh, titleEn) {
    CVE = append(CVE, {
        "cve": cve,
        "rule": rules,
        "score": score,
        "severity": severity,
        "titleZh": titleZh,
        "titleEn": titleEn, 
    })
}

%s


handle = func(result /* *fp.MatchResult */) {
    // handle match result
    if !result.IsOpen() {
        return
    }

    if result.Fingerprint == nil {
        return
    }

    if str.MatchAllOfRegexp(result.GetServiceName(), "%s") {
        x.Foreach(CVE, func(i) {
            defer func{
                err = recover()
                if err != nil {
                    yakit.Error("%s CVE 合规检查失败：%%v", err)
                }
            }
            
            flag = true

            for _, cpes := range result.Fingerprint.CPEFromUrls{
                for _, cpe := range cpes{
                    if cpe.Product == "%s" && cpe.Version != "*"{
                        if RuleCompare(cpe.Version,i.rule){
                            flag = false
                        }
                    }
                }
                
            }

            if flag {
                return
            }

            target := str.HostPort(result.Target, result.Port)
            targetOutput := sprintf("(%%v)", target)
            risk.NewRisk(
                target, 
                risk.title(sprintf("%%v:%%v%%v", i.cve, i["titleEn"], targetOutput)),
                risk.titleVerbose(sprintf("%%v:%%v%%v", i.cve, i["titleZh"], targetOutput)),
                risk.type("cve-baseline"),
                risk.typeVerbose("CVE基线检查"),
                risk.parameter(result.Fingerprint.Banner),
                risk.potential(true),
                risk.level(i.severity),
                risk.details(i),
                risk.cve(i.cve),
            )
            println(i.cve)
        })
    }
}

func RuleCompare(version, versionRule){
    for _, ruleMap := range versionRule {
		flag = true
        for ruleName, rule := range ruleMap {
            flag = flag && VersionRuleCompare(version,rule,ruleName)
        }
        if flag {
            return true
        }
    }
    return false
}

func VersionRuleCompare(version, BoundaryVerison, Op){
    switch Op{
        case "versionEndIncluding":
            if str.VersionLessEqual(version, BoundaryVerison) {
                return true
            }
        case "versionStartIncluding":
            if str.VersionGreaterEqual(version, BoundaryVerison) {
                return true
            }
        case "versionStartExcluding":
            if str.VersionGreater(version, BoundaryVerison)  {
                return true
            }
        case "versionEndExcluding":
            if str.VersionLess(version, BoundaryVerison)  {
                return true
            }
        case "current":
            if version == BoundaryVerison{
                return true
            }
    }
    return false
}
`

// MakeCtScript 生成合规插件脚本，要求输入产品名，数据库路径，服务名(扫描获取的服务名)，脚本输出路径
func MakeCtScript(product, dbName, serverName, scriptPath string) {
	//! 设置合规脚本目标产品

	var addRuleStrs []string
	formatString := "addCVE(\"%s\", %s, \"%.2f\", \"%s\", \"%s\", %s)\n"
	CVEs, _ := Query(dbName, Product(product))
	for _, cve := range CVEs {
		//// todo 漏洞中文名暂时不用cnnvd，
		//cnnvd, _ := cve.CNNVD(dbName)
		var config cveresources.Configurations
		err := json.Unmarshal(cve.CPEConfigurations, &config)
		if err != nil {
			log.Errorf("config json error:%#v", err)
			return
		}
		var version []map[string]string
		for _, node := range config.Nodes {
			version = append(version, node.GetProductVersion(product)...)
		}

		if len(version) == 0 {
			continue
		}

		mapFormatStr := `"%s":"%s"`
		var versionRuleStr string
		var ruleListStrs []string
		for _, m := range version {

			var insideMapStrs []string
			for k, v := range m {
				insideMapStrs = append(insideMapStrs, fmt.Sprintf(mapFormatStr, k, v))
			}
			ruleListStrs = append(ruleListStrs, "{"+strings.Join(insideMapStrs, ",")+"}")
		}
		versionRuleStr = "[" + strings.Join(ruleListStrs, ",") + "]"

		addRuleStrs = append(addRuleStrs, fmt.Sprintf(formatString, cve.CVE.CVE, versionRuleStr, cve.BaseCVSSv2Score, cve.Severity, GetAKA(cve.DescriptionMain), strconv.Quote(cve.DescriptionMain)))
	}
	addRule := strings.Join(addRuleStrs, "\n")
	script := fmt.Sprintf(scriptFormat, addRule, serverName, product, product)
	outputPath := path.Join(scriptPath, product+".yak")
	err := os.WriteFile(outputPath, []byte(script), 0666)
	if err != nil {
		panic(err)
	}
}

func GetAKA(descriptions string) string {
	if !strings.Contains(descriptions, "aka") {
		return ""
	} else {
		compileRegex := regexp.MustCompile("aka \"(.*?)\"")
		matchAka := compileRegex.FindStringSubmatch(descriptions)
		if len(matchAka) > 0 {
			return matchAka[len(matchAka)-1]
		}
		return ""
	}
}
