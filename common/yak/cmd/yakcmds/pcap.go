package yakcmds

import (
	"fmt"
	"github.com/google/gopacket"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/chaosmaker"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
	"github.com/yaklang/yaklang/common/suricata/match"
	"github.com/yaklang/yaklang/common/suricata/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/tlsutils"
	"net/http"
	"os"
	"strconv"
	"strings"
)

var SuricataLoaderCommand = cli.Command{
	Name:  "suricata",
	Usage: "Load suricata rules to database",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "rule-file",
			Usage: `load suricata`,
		},
		cli.StringFlag{
			Name: "domain",
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
		}

		if c.String("rule-file") != "" {
			raw, err := os.ReadFile(c.String("rule-file"))
			if err != nil {
				return err
			}
			log.Infof("start to load suricata rule: %s", c.String("rule-file"))
			err = chaosmaker.LoadSuricataToDatabase(string(raw))
			if err != nil {
				return err
			}
		}
		return nil
	},
}

var PcapCommand = cli.Command{
	Name:  "pcap",
	Usage: "Sniff network traffic and output to stdout",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "device",
			Usage: "网卡（可选多个,使用逗号分隔）",
		},
		cli.StringFlag{
			Name:  "input",
			Usage: "pcap文件路径",
		},
		cli.StringFlag{
			Name:  "output",
			Usage: "过滤后的流量导出路径",
		},
		cli.BoolFlag{
			Name:  "v",
			Usage: "输出详细信息",
		},
		cli.StringFlag{
			Name:  "suricata",
			Usage: "suricata规则文件路径",
		},
		cli.StringFlag{
			Name:  "suricata-rule-keyword,k",
			Usage: `suricata规则关键字，可选多个，使用逗号分隔`,
		},
	},
	Action: func(c *cli.Context) error {
		var opts []pcaputil.CaptureOption
		if c.Bool("v") {
			opts = append(opts, pcaputil.WithDebug(true))
		}
		if device := c.String("device"); device != "" {
			opts = append(opts, pcaputil.WithDevice(strings.Split(device, ",")...))
		}
		if input := c.String("input"); input != "" {
			opts = append(opts, pcaputil.WithFile(input))
		}
		if output := c.String("output"); output != "" {
			opts = append(opts, pcaputil.WithOutput(output))
		}
		if suricata := c.String("suricata"); suricata != "" {
			//opts = append(opts, pcaputil.WithSuricataFilter(suricata))
		}

		var group *match.Group
		if skw := c.String("suricata-rule-keyword"); skw != "" {
			group = match.NewGroup(
				match.WithGroupOnMatchedCallback(func(packet gopacket.Packet, match *rule.Rule) {
					log.Infof("matched rule: %s", match.Message)
				}))
			err := group.LoadRulesWithQuery(skw)
			if err != nil {
				return err
			}
			defer group.Wait()
		}

		opts = append(
			opts,
			pcaputil.WithTLSClientHello(func(flow *pcaputil.TrafficFlow, hello *tlsutils.HandshakeClientHello) {
				if group == nil {
					log.Infof("%v SNI: %v", flow.String(), hello.SNI())
					return
				}
			}),
			pcaputil.WithHTTPFlow(func(flow *pcaputil.TrafficFlow, req *http.Request, rsp *http.Response) {
				if req == nil {
					return
				}

				if group == nil {
					reqBytes, _ := utils.DumpHTTPRequest(req, true)
					fmt.Println(string(reqBytes))
					fmt.Println("-----------------------------------------")
					rspBytes, _ := utils.DumpHTTPResponse(rsp, true)
					fmt.Println(string(rspBytes))
					fmt.Println("-----------------------------------------")
					return
				}
				reqBytes, _ := utils.DumpHTTPRequest(req, true)
				rspBytes, _ := utils.DumpHTTPResponse(rsp, true)
				group.FeedHTTPFlowBytes(reqBytes, rspBytes)
			}),
		)
		return pcaputil.Start(opts...)
	},
}
