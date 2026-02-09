package yakcmds

import (
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"sync"
	"time"
)

var servicescanCommand = cli.Command{
	Name:  "scan-service",
	Usage: "ServiceScan means Fingerprint Scan, it will create a tcp/udp connection to the target and wait for the response",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "hosts,target,t",
			Usage: "输入扫描主机，以逗号分隔例如：(192.168.1.1/24,192.168.1.1-23,10.1.1.2)",
		},
		cli.StringFlag{
			Name:  "port,tcp-port,p",
			Usage: "输入想要扫描的端口，支持单个端口和范围，例如（80,443,21-25,8080-8082）",
			Value: "22,80,443,3389,3306,8080-8082,9000-9002,7000-7002",
		},
		cli.StringFlag{
			Name:  "udp-port",
			Usage: "想要扫描的 UDP 端口，支持单个端口和范围",
		},
		cli.StringFlag{
			Name:  "rule-path,rule,r",
			Usage: "手动加载规则文件/文件夹",
		},
		cli.BoolFlag{
			Name:  "only-rule",
			Usage: "只加载这个文件夹中的 Web 指纹",
		},
		cli.IntFlag{
			Name:  "concurrent,thread,c",
			Usage: "并发速度，同时有多少个扫描过程进行？",
			Value: 60,
		},
		//cli.IntFlag{
		//	Name:  "timeout",
		//	Usage: "超时时间(Seconds)",
		//	Value: 3600,
		//},
		cli.BoolFlag{
			Name:  "web",
			Usage: "主动开启 web 扫描模式",
		},
		cli.IntFlag{
			Name:  "request-timeout",
			Usage: "单个请求的超时时间（Seconds）",
			Value: 10,
		},
		cli.StringFlag{
			Name:  "json,o",
			Usage: "详细结果输出 json 到文件",
		},
	},
	Action: func(c *cli.Context) error {
		var options []fp.ConfigOption

		// web rule
		webRules, _ := fp.GetDefaultWebFingerprintRules()
		userRule := fp.FileOrDirToWebRules(c.String("rule-path"))

		if c.Bool("only-rule") {
			webRules = userRule
		} else {
			webRules = append(webRules, userRule...)
		}

		options = append(
			options,

			// 主动探测模式 - 主动发送符合条件的包
			fp.WithActiveMode(true),

			// 每一个指纹探测请求的超时时间
			fp.WithProbeTimeout(time.Second*time.Duration(c.Int("request-timeout"))),

			// web 指纹火力全开
			fp.WithWebFingerprintUseAllRules(true),

			// web 指纹
			fp.WithWebFingerprintRule(webRules),
		)
		options = append(
			options, fp.WithForceEnableAllFingerprint(true),
		)

		config := fp.NewConfig(options...)

		matcher, err := fp.NewDefaultFingerprintMatcher(config)
		if err != nil {
			return err
		}

		// udp/tcp
		portSwg := utils.NewSizedWaitGroup(c.Int("concurrent"))

		// 结果处理的同步锁
		resultLock := new(sync.Mutex)

		var res []*fp.MatchResult

		scanCore := func(tHost string, tPort int, opts ...fp.ConfigOption) {
			defer portSwg.Done()

			log.Infof("start scan %v", utils.HostPort(tHost, tPort))
			result, err := matcher.Match(
				tHost, tPort,
				opts...,
			)
			if err != nil {
				log.Errorf("scan %v failed: %s", utils.HostPort(tHost, tPort), err)
				return
			}
			resultLock.Lock()
			defer resultLock.Unlock()

			log.Infof("[%6s] %s://%s cpe: %v", result.State, result.GetProto(), utils.HostPort(result.Target, result.Port), result.GetCPEs())
			res = append(res, result)
		}

		for _, host := range utils.ParseStringToHosts(c.String("hosts")) {
			host := host
			for _, tcpPort := range utils.ParseStringToPorts(c.String("port")) {
				tcpPort := tcpPort

				portSwg.Add()
				go scanCore(
					host, tcpPort,
					fp.WithForceEnableAllFingerprint(true),
					fp.WithOnlyEnableWebFingerprint(c.Bool("web")),
					fp.WithTransportProtos(fp.TCP),
				)
			}

			for _, udpPort := range utils.ParseStringToPorts(c.String("udp-port")) {
				udpPort := udpPort

				portSwg.Add()
				go scanCore(host, udpPort, fp.WithDisableWebFingerprint(true),
					fp.WithTransportProtos(fp.UDP))
			}

		}
		portSwg.Wait()

		analysis := fp.MatcherResultsToAnalysis(res)

		analysis.Show()
		analysis.ToJson(c.String("json"))

		return nil
	},
}
