package yakcmds

import (
	"fmt"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/openai"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"os"
	"strconv"
	"strings"
)

var SuricataLoaderCommand = cli.Command{
	Name:  "suricata",
	Usage: "Load suricata rules to database",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "rule-file,i",
			Usage: `load suricata`,
		},
		cli.BoolFlag{
			Name:  "ai",
			Usage: "use openai api to generate description for suricata rule",
		},
		cli.StringFlag{
			Name: "domain",
		},
		cli.StringFlag{
			Name:  "ai-proxy",
			Usage: "use proxy to access openai api",
		},
		cli.StringFlag{
			Name:  "ai-token",
			Usage: "use token to access openai api",
		},
	},
	Action: func(c *cli.Context) error {
		domain := c.String("domain")
		if domain != "" {
			domainRule := strings.Trim(strconv.Quote(domain), ` "'`+"`")
			rule := `alert http any any -> any any (msg:"Domain Fetch: ` + domainRule + `"; content:"` + domainRule + `"; http_header; sid:1; rev:1;)`
			log.Infof("generate suricata rule: %s", rule)
			err := chaosmaker.LoadSuricataToDatabase(rule)
			if err != nil {
				return err
			}
			return nil
		}

		if c.String("rule-file") != "" {
			raw, err := os.ReadFile(c.String("rule-file"))
			if err != nil {
				return err
			}
			log.Infof("start to load suricata rule: %s", c.String("rule-file"))
			subRules, err := surirule.Parse(string(raw))
			if err != nil {
				return err
			}
			for _, subRule := range subRules {
				if c.Bool("ai") {
					subRule.AIDecoration(openai.WithAPIKey(c.String("ai-token")), openai.WithProxy(c.String("ai-proxy")))
				}
				err := rule.SaveSuricata(consts.GetGormProfileDatabase(), subRule)
				if err != nil {
					log.Errorf("save suricata error: %s", err)
				}
			}
		}
		return nil
	},
}

var ChaosMakerCommand = cli.Command{
	Name:    "chaosmaker",
	Aliases: []string{"chaos"},
	Usage:   `Chaos Maker is designed to generate chaos traffic for testing`,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "search",
		},
		cli.StringFlag{
			Name: "remote-addr",
		},
	},
	Action: func(c *cli.Context) error {
		maker := chaosmaker.NewChaosMaker()
		for chaosRule := range chaosmaker.YieldRulesByKeywords(c.String("search")) {
			maker.FeedRule(chaosRule)
		}
		for trafficBytes := range maker.GenerateWithRule() {
			_, ipLayer, tcpLayer, payloads, err := pcapx.ParseEthernetLinkLayer(trafficBytes.Raw)
			if err != nil {
				fmt.Println(string(payloads.Payload()))
				log.Infof("parse traffic failed: %v", err)
				continue
			}
			_ = ipLayer
			_ = tcpLayer
			_ = payloads
		}
		return nil
	},
}
