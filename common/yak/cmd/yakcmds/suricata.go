package yakcmds

import (
	"fmt"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/chaosmaker/rule"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	surirule "github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"strings"
)

var ChaosMakerAIHelperCommand = cli.Command{}

var suricataLoaderCommand = cli.Command{
	Name:     "suricata",
	Usage:    "Load suricata rules to database, for example: yak suricata --rule-dir /tmp/rules --ai --domain api.openai.com --ai-proxy http://127.0.0.1:10808 --ai-token sk-xxx --model gpt-4-0613 --concurrent 5",
	Category: "Suricata Rules Operations",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "rule-file,i",
			Usage: `load suricata`,
		},
		cli.StringFlag{
			Name:  "rule-dir",
			Usage: `load suricata in directory, file ext: .rules`,
		},
		cli.BoolFlag{
			Name:  "ai",
			Usage: "use openai api to generate description for suricata rule",
		},
		cli.StringFlag{
			Name:  "ai-proxy",
			Usage: "use proxy to access openai api",
		},
		cli.StringFlag{
			Name:  "ai-token",
			Usage: "use token to access openai api",
		},
		cli.StringFlag{
			Name:  "model",
			Usage: "use model to access openai api",
		},
		cli.StringFlag{
			Name:  "concurrent",
			Usage: "set concurrent number to load suricata rules",
		},
	},
	Action: func(c *cli.Context) error {
		concurrent := 1
		if c.Int("concurrent") > 0 {
			concurrent = c.Int("concurrent")
		}
		swg := utils.NewSizedWaitGroup(concurrent)
		loadFile := func(i string) error {
			raw, err := os.ReadFile(i)
			if err != nil {
				return err
			}
			log.Infof("start to parse suricata rule: %s", i)
			subRules, err := surirule.Parse(string(raw))
			log.Infof("parse suricata rule: %s, got %d sub rules", i, len(subRules))

			if err != nil {
				return err
			}
			for _, subRule := range subRules {
				swg.Add()
				subRule := subRule
				go func() {
					defer swg.Done()
					r := rule.NewRuleFromSuricata(subRule)
					if c.Bool("ai") {
						log.Infof("start to decorator suricata rule: %s", subRule.Message)
						r.DecoratedByOpenAI("openai", aispec.WithAPIKey(c.String("ai-token")), aispec.WithProxy(c.String("ai-proxy")))
					}
					err := rule.SaveToDB(r)
					if err != nil {
						log.Errorf("save suricata error: %s", err)
					}
				}()
			}
			return nil
		}

		if c.String("rule-file") != "" {
			err := loadFile(c.String("rule-file"))
			if err != nil {
				log.Errorf("load suricata rule failed: %v", err)
			}
		}

		if c.String("rule-dir") != "" {
			log.Infof("start to load suricata rule in dir: %s", c.String("rule-dir"))
			infos, err := utils.ReadFilesRecursively(c.String("rule-dir"))
			if err != nil {
				return utils.Errorf("read dir failed: %v", err)
			}
			for _, i := range infos {
				log.Infof("start to check file: %s", i.Path)
				if strings.HasSuffix(i.Name, ".rules") {
					err := loadFile(i.Path)
					if err != nil {
						log.Errorf("load suricata rule failed: %v", err)
					}
				}
			}
		}
		swg.Wait()
		return nil
	},
}

var chaosMakerCommand = cli.Command{
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
