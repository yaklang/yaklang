// Package cmd
// @Author bcy2007  2023/9/18 11:39
package main

import (
	"encoding/json"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/bas/send"
	"github.com/yaklang/yaklang/common/log"

	basUtils "github.com/yaklang/yaklang/common/bas/utils"
	"github.com/yaklang/yaklang/common/utils"
	"os"
)

func main() {
	app := cli.NewApp()
	app.Name = "Packet Sender"
	app.Version = "v0.2"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "ip,i",
			Usage: "packet receive target",
		},
		cli.StringFlag{
			Name:  "ruleFilePath,r",
			Usage: "rule file path",
		},
		cli.BoolFlag{
			Name:  "test",
			Usage: "test",
		},
	}
	app.Action = func(c *cli.Context) error {
		ipaddress := c.String("ip")
		if ipaddress == "" {
			return utils.Error("ipaddress blank")
		}
		ruleFilePath := c.String("ruleFilePath")
		if ruleFilePath == "" {
			return utils.Errorf("rule file path blank")
		}
		return sending(ipaddress, ruleFilePath)
	}
	err := app.Run(os.Args)
	if err != nil {
		log.Errorf("packet sender running error: %s", err)
		return
	}
}

func sending(ipaddress, ruleFilePath string) error {
	rules, err := ParseRuleInfo(ruleFilePath)
	if err != nil {
		return utils.Errorf("parse rule info error: %v", err)
	}
	if len(rules) == 0 {
		return utils.Error("rule info length 0")
	}
	sender, err := send.CreateSender(ipaddress, rules)
	if err != nil {
		return utils.Errorf("create sender error: %v", err)
	}
	err = sender.SendPack()
	if err != nil {
		return utils.Errorf("packet send error: %v", err)
	}
	return nil
}

func ParseRuleInfo(ruleFilePath string) (map[int]string, error) {
	ruleJson, err := readRule(ruleFilePath)
	if err != nil {
		return nil, utils.Errorf("read rule error: %v", err)
	}
	var ruleBlocks []send.RuleFormat
	if err := json.Unmarshal(ruleJson, &ruleBlocks); err != nil {
		return nil, utils.Errorf("unmarshal rules error: %v", err)
	}
	ruleInfoMap := make(map[int]string)
	for _, r := range ruleBlocks {
		ruleInfoMap[r.RuleID] = r.Content
	}
	return ruleInfoMap, nil
}

func readRule(rulePath string) ([]byte, error) {
	ruleContentBytes, err := basUtils.ReadFile(rulePath)
	if err != nil {
		return nil, utils.Errorf("file %v read error: %v", rulePath, err)
	}
	defer func() {
		err := basUtils.RemoveFile(rulePath)
		if err != nil {
			log.Errorf("file %v remove error: %v", rulePath, err)
		}
	}()
	return ruleContentBytes, nil
}
