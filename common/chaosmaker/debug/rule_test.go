package debug

import (
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/suricata/match"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var workDir = "/Users/z3/Downloads/suricata-test-logs"

func CheckAllRules(rules []string) error {
	dirs := []string{"rules", "pcapfiles", "log", "suricata-fast-log"}
	for _, dir := range dirs {
		logPath := filepath.Join(workDir, dir)
		if ok, _ := utils.PathExists(logPath); !ok {
			os.MkdirAll(logPath, 0755)
		}
	}

	parseLogPath := filepath.Join(workDir, "log", "parse-rule.log")
	parseLogWriter, err := os.OpenFile(parseLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("open parse log file failed: %s", err)
	}
	defer parseLogWriter.Close()
	logger := log.GetLogger("parse-rule")
	logger.SetOutput(io.MultiWriter(logger.Printer, parseLogWriter))
	bunchOfRulesSize := 5
	bunchOfRulesCh := make(chan []string, bunchOfRulesSize)
	go func() {
		for i := 0; i < len(rules); i += bunchOfRulesSize {
			end := i + bunchOfRulesSize
			if end > len(rules) {
				end = len(rules)
			}
			bunchOfRulesCh <- rules[i:end]
		}
		close(bunchOfRulesCh)
	}()
	for bunchOfRule := range bunchOfRulesCh {
		ruleInss := []*surirule.Rule{}
		ruleFile, err := os.CreateTemp(filepath.Join(workDir, "rules"), "*.rule")
		if err != nil {
			return fmt.Errorf("create rule file failed: %s", err)
		}
		ruleFile.Write([]byte(strings.Join(bunchOfRule, "\n")))
		ruleFile.Close()
		for _, s := range bunchOfRule {
			ruleIns, err := surirule.Parse(s)
			if err != nil {
				//logger.Errorf("parse rule `%s` failed: %v", s, err)
				continue
			}
			ruleInss = append(ruleInss, ruleIns...)
		}
		storageRules := lo.Map(ruleInss, func(item *surirule.Rule, index int) *rule.Storage {
			return rule.NewRuleFromSuricata(item)
		})

		file, err := os.CreateTemp(filepath.Join(workDir, "pcapfiles"), "*.pcap")
		//file, err := os.CreateTemp(os.TempDir(), "*.pcapng")
		if err != nil {
			return fmt.Errorf("create pcap file failed: %s", err)
		}
		writer := pcapgo.NewWriter(file)
		writer.WriteFileHeader(65536, layers.LinkTypeEthernet)
		mk := chaosmaker.NewChaosMaker()
		mk.FeedRule(storageRules...)
		for traffic := range mk.Generate() {
			res := match.New(ruleInss[0]).Match(traffic)
			println(res)
			//pk := gopacket.NewPacket(traffic, layers.LinkTypeEthernet, gopacket.Default)
			err = writer.WritePacket(gopacket.CaptureInfo{
				Timestamp:     time.Now(),
				CaptureLength: len(traffic),
				Length:        len(traffic),
			}, traffic)
			if err != nil {
				logger.Errorf("write pcap file failed: %v", err)
			}
		}
		file.Close()

		tmpDir, err := os.MkdirTemp(filepath.Join(workDir, "suricata-fast-log"), "*-log")
		if err != nil {
			return fmt.Errorf("create temp dir failed: %s", err)
		}
		cmd := exec.Command("suricata", "-c", "/Users/z3/Downloads/suricata.yaml", "-r", file.Name(), "-s", ruleFile.Name(), "-l", tmpDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout
		err = cmd.Run()
		if err != nil {
			logger.Errorf("run suricata failed: %v", err)
			continue
		}
		content, err := os.ReadFile(filepath.Join(tmpDir, "fast.log"))
		if err != nil {
			logger.Errorf("read fast.log failed")
			continue
		}
		for _, storageRule := range storageRules {
			if !strings.Contains(string(content), storageRule.Name) {
				logger.Errorf("rule `%s` not match", storageRule.Name)
			}
		}
	}
	return nil
}

func TestAllRules(t *testing.T) {
	allRuleStr := GetAllRules()
	invalidRule := []string{
		`alert http $HOME_NET any -> $EXTERNAL_NET any (msg: "CobaltStrike download.windowsupdate.com C2 Profile"; flow: established; content:"msdownload"; http_uri; pcre:"/\/c\/msdownload\/update\/others\/[\d]{4}/\d{2}/\d{7,8}_[\d\w-_]{50,}\.cab/UR"; reference:url,github.com/bluscreenofjeff/MalleableC2Profiles/blob/master/microsoftupdate_getonly.profile; classtype:exploit-kit; sid: 3016002; rev: 1; metadata:created_at 2018_09_25,by al0ne; )`,
		`alert http any any -> any any (msg:"***Linux wget/curl download .sh script***"; flow:established,to_server; content:".sh"; http_uri;  pcre:"/curl|Wget|linux-gnu/Vi"; classtype:trojan-activity; sid:3013002; rev:1; metadta:by al0ne;)`,
	}
	allRuleStr = lo.Filter(allRuleStr, func(item string, index int) bool {
		rules, err := surirule.Parse(item)
		if err != nil {
			return false
		}
		if len(rules) == 1 {
			r := rules[0]
			for _, contentRule := range r.ContentRuleConfig.ContentRules {
				flowBits := contentRule.FlowBits
				if flowBits != "" {
					return false
				}
			}
		}
		if strings.Contains(item, "flowbits:") {
			return false
		}
		if strings.Contains(item, "threshold:") {
			return false
		}
		if slices.Contains(invalidRule, item) {
			return false
		}
		//ruleFile, err := os.CreateTemp(filepath.Join(workDir, "rules"), "*.rule")
		//if err != nil {
		//	panic(err)
		//}
		//ruleFile.Write([]byte(item))
		//ruleFile.Close()
		//outBuffer := &bytes.Buffer{}
		//cmd := exec.Command("suricata", "-c", "/Users/z3/Downloads/suricata.yaml", "-s", ruleFile.Name(), "-r", "/Users/z3/Downloads/suricata-test-logs/pcapfiles/1974636558.pcap")
		//cmd.Stdout = outBuffer
		//cmd.Stderr = outBuffer
		//cmd.Run()
		//if strings.Contains(outBuffer.String(), "E: detect:") {
		//	println(item)
		//	return false
		//}
		return true
	})
	allRuleStr = []string{`alert http any any -> any 7001 (msg:"Exploit CVE-2020-14750 on Oracle Weblogic Server"; flow:established,to_server; content:"/console/"; startswith; http_uri; pcre:"/^(css|images)\//UR"; content:"2e"; nocase; distance:1; http_raw_uri; content:"console.portal"; distance:1; http_uri; reference:cve,CVE-2020-14750; reference:cve,CVE-2020-14882; classtype:web-application-attack; sid:202014750; rev:1;)`}
	err := CheckAllRules(allRuleStr)
	if err != nil {
		t.Fatal(err)
	}
}
