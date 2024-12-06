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
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	err := CheckAllRules(allRuleStr)
	if err != nil {
		t.Fatal(err)
	}
}
