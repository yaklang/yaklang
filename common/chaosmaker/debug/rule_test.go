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
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/maps"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

var workDir = "/Users/z3/Downloads/suricata-test-logs"

func CheckAllRules(rules []string) ([]string, error) {
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
		return nil, fmt.Errorf("open parse log file failed: %s", err)
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
	swg := utils.NewSizedWaitGroup(20)
	lock := sync.Mutex{}
	failedRules := []string{}
	for bunchOfRule := range bunchOfRulesCh {
		ruleInss := []*surirule.Rule{}
		ruleFile, err := os.CreateTemp(filepath.Join(workDir, "rules"), "*.rule")
		if err != nil {
			return nil, fmt.Errorf("create rule file failed: %s", err)
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
		swg.Add()
		go func() {
			res, err := func() ([]string, error) {
				failedRules := []string{}
				file, err := os.CreateTemp(filepath.Join(workDir, "pcapfiles"), "*.pcap")
				//file, err := os.CreateTemp(os.TempDir(), "*.pcapng")
				if err != nil {
					return nil, fmt.Errorf("create pcap file failed: %s", err)
				}
				writer := pcapgo.NewWriter(file)
				writer.WriteFileHeader(65536, layers.LinkTypeEthernet)
				mk := chaosmaker.NewChaosMaker()
				mk.FeedRule(storageRules...)
				for traffic := range mk.Generate() {
					//res := match.New(ruleInss[0]).Match(traffic)
					//println(res)
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
					return nil, fmt.Errorf("create temp dir failed: %s", err)
				}
				cmd := exec.Command("suricata", "-c", "/Users/z3/Downloads/suricata.yaml", "-r", file.Name(), "-s", ruleFile.Name(), "-l", tmpDir)
				//cmd.Stdout = os.Stdout
				//cmd.Stderr = os.Stdout
				err = cmd.Run()
				if err != nil {
					logger.Errorf("run suricata failed: %v", err)
					return nil, err
				}
				content, err := os.ReadFile(filepath.Join(tmpDir, "fast.log"))
				if err != nil {
					logger.Errorf("read fast.log failed")
					return nil, err
				}
				for _, storageRule := range storageRules {
					if !strings.Contains(string(content), storageRule.Name) {
						logger.Errorf("rule `%s` not match", storageRule.Name)
						failedRules = append(failedRules, storageRule.SuricataRaw)
					}
				}
				return failedRules, nil
			}()
			if err != nil {
				log.Errorf("check rule failed: %v", err)
				res = append(res, lo.Map(storageRules, func(item *rule.Storage, index int) string {
					return item.SuricataRaw
				})...)
			}
			swg.Done()
			lock.Lock()
			failedRules = append(failedRules, res...)
			lock.Unlock()
		}()
	}
	swg.Wait()
	return failedRules, nil
}

func TestAllRules(t *testing.T) {
	allRuleStr := GetAllRules()
	invalidRule := []string{
		`alert http $HOME_NET any -> $EXTERNAL_NET any (msg: "CobaltStrike download.windowsupdate.com C2 Profile"; flow: established; content:"msdownload"; http_uri; pcre:"/\/c\/msdownload\/update\/others\/[\d]{4}/\d{2}/\d{7,8}_[\d\w-_]{50,}\.cab/UR"; reference:url,github.com/bluscreenofjeff/MalleableC2Profiles/blob/master/microsoftupdate_getonly.profile; classtype:exploit-kit; sid: 3016002; rev: 1; metadata:created_at 2018_09_25,by al0ne; )`,
		`alert http any any -> any any (msg:"***Linux wget/curl download .sh script***"; flow:established,to_server; content:".sh"; http_uri;  pcre:"/curl|Wget|linux-gnu/Vi"; classtype:trojan-activity; sid:3013002; rev:1; metadta:by al0ne;)`,
	}
	ruleToKeyMap := map[string]map[string]string{}
	allRuleStr = lo.Filter(allRuleStr, func(item string, index int) bool {
		rules, err := surirule.Parse(item)
		if err != nil {
			return false
		}
		if len(rules) == 1 {
			r := rules[0]
			if r.Protocol != "icmp" {
				return false
			}
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
		if strings.Contains(item, "base64_decode:") {
			return false
		}
		if strings.Contains(item, "http_raw_uri") && strings.Contains(item, "http_uri") {
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
		ruleToKeyMap[item] = rules[0].SettingMap
		return true
	})
	//allRuleStr = []string{`alert http any any -> any any (msg:"Exploit CVE-2020-17141 on Microsoft Exchange Server"; flow:to_server,established; content:"POST"; http_method; content:"/ews/Exchange.asmx"; startswith; http_uri; content:"<m:RouteComplaint "; http_client_body; content:"<m:Data>"; distance:0; http_client_body; base64_decode:bytes 300, offset 0, relative; base64_data; content:"<!DOCTYPE"; content:"SYSTEM"; distance:0; reference:cve,CVE-2020-17141; classtype:web-application-attack; sid:202017141; rev:1;)`}
	failedRule, err := CheckAllRules(allRuleStr)
	if err != nil {
		t.Fatal(err)
	}
	failedRuleKeys := []string{}
	successfulRuleKeys := []string{}
	lo.ForEach(failedRule, func(item string, index int) {
		failedRuleKeys = append(failedRuleKeys, maps.Keys(ruleToKeyMap[item])...)
	})
	lo.ForEach(allRuleStr, func(item string, index int) {
		if !slices.Contains(failedRule, item) {
			successfulRuleKeys = append(successfulRuleKeys, maps.Keys(ruleToKeyMap[item])...)
		}
	})

	fatalKeys := []string{}
	lo.ForEach(failedRuleKeys, func(item string, index int) {
		if !slices.Contains(successfulRuleKeys, item) {
			fatalKeys = append(fatalKeys, item)
		}
	})
	fatalKeys = utils.NewSet(fatalKeys).List()
	println("fatal keys:")
	println(strings.Join(fatalKeys, "\n"))
	failedRuleKeys = utils.NewSet(failedRuleKeys).List()
	println("failed keys:")
	println(strings.Join(failedRuleKeys, "\n"))
}

func TestFatalErrorKeyRuleStatistics(t *testing.T) {
	fatalErrorKeysStr := `http_header
content
byte_jump
http.cookie
http_client_body
http.start
tls.certs
file_data
http.content_type
http.uri.raw
http.stat_code
dotprefix
http.header
http.request_line
app-layer-event
http_method
http.request_body
nocase
classtype
within
byte_extract
http.content_len
tag
pcre
http_uri
metadata
sid
urilen
rev
startswith
http.header_names
http.response_body
depth
distance
fast_pattern
pkt_data
dsize
tls.sni
reference
http.method
http.referer
flow
offset
byte_test
endswith
http.uri
file.data
xbits
noalert
isdataat
http_server_body
dns.query
bsize
http.server
msg`
	fatalErrorKeys := lo.Map(strings.Split(fatalErrorKeysStr, "\n"), func(item string, index int) string {
		return strings.TrimSpace(item)
	})
	allRuleStr := GetAllRules()
	keyToRuleNumber := map[string]int{}
	for _, ruleStr := range allRuleStr {
		rules, err := surirule.Parse(ruleStr)
		if err != nil {
			continue
		}
		for _, rule := range rules {
			for key := range rule.SettingMap {
				keyToRuleNumber[key]++
			}
		}
	}
	pairs := [][2]any{}
	for key, number := range keyToRuleNumber {
		pairs = append(pairs, [2]any{key, number})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i][1].(int) > pairs[j][1].(int)
	})
	for _, pair := range pairs {
		key := pair[0].(string)
		if !slices.Contains(fatalErrorKeys, key) {
			continue
		}
		println(key, keyToRuleNumber[key])
	}
}
