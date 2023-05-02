package scanfpcmd

import (
	"context"
	"fmt"
	"github.com/urfave/cli"
	"net"
	"os"
	"sync"
	"time"
	"yaklang.io/yaklang/common/fp"
	"yaklang.io/yaklang/common/fp/webfingerprint"
	"yaklang.io/yaklang/common/hybridscan"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/synscan"
	"yaklang.io/yaklang/common/utils"
)

var SynScanCmd = cli.Command{
	Name:      "synscan",
	ShortName: "syn",
	Usage:     "SYN 端口扫描",
	Before:    nil,
	After:     nil,

	OnUsageError: nil,
	Subcommands:  nil,
	Flags: []cli.Flag{
		cli.StringFlag{
			Name: "target,host,t",
		},
		cli.StringFlag{
			Name:  "port,p",
			Value: "22,80,443,3389,3306,8080-8082,9000-9002,7000-7002",
		},
		cli.IntFlag{
			Name:  "wait,waiting",
			Usage: "在 SYN 包发送完毕之后等待多长时间进行收尾（Seconds）",
			Value: 5,
		},

		// 指纹识别相关配置
		cli.BoolFlag{
			Name:  "fingerprint,fp,x",
			Usage: "开启指纹扫描",
		},
		cli.IntFlag{
			Name:  "request-timeout",
			Usage: "单个请求的超时时间（Seconds）",
			Value: 10,
		},
		cli.StringFlag{
			Name:  "rule-path,rule,r",
			Usage: "手动加载规则文件/文件夹",
		},
		cli.BoolFlag{
			Name:  "only-rule",
			Usage: "只加载这个文件夹中的 Web 指纹",
		},
		cli.StringFlag{
			Name:  "fp-json,fpo",
			Usage: "详细结果输出 json 到文件",
		},

		// 输出实时的开放端口信息
		cli.StringFlag{
			Name:  "output",
			Usage: "输出端口开放的信息到文件",
		},

		cli.StringFlag{
			Name:  "output-line-prefix",
			Value: "",
			Usage: "输出 OUTPUT 每一行的前缀，例如：https:// http://",
		},

		cli.IntFlag{
			Name:  "fingerprint-concurrent,fc",
			Value: 20,
			Usage: "设置指纹扫描的并发量(同时进行多少个指纹扫描模块)",
		},
	},

	Action: func(c *cli.Context) {
		target := c.String("target")
		targetList := utils.ParseStringToHosts(target)
		if len(targetList) <= 0 {
			log.Errorf("empty target: %s", c.String("target"))
			return
		}

		var sampleTarget string
		if len(targetList) == 1 {
			sampleTarget = targetList[0]
		} else {
			for _, target := range targetList {
				if !utils.IsLoopback(target) {
					sampleTarget = target
					break
				}
			}
			if sampleTarget == "" {
				sampleTarget = targetList[1]
			}
		}

		options, err := synscan.CreateConfigOptionsByTargetNetworkOrDomain(sampleTarget, 10*time.Second)
		if err != nil {
			log.Errorf("init syn scanner failed: %s", err)
			return
		}
		synScanConfig, err := synscan.NewConfig(options...)
		if err != nil {
			log.Errorf("create synscan config failed: %s", err)
			return
		}

		log.Infof("default config: \n    iface:%v src:%v gateway:%v", synScanConfig.Iface.Name, synScanConfig.SourceIP, synScanConfig.GatewayIP)

		// 解析指纹配置
		// web rule
		webRules, _ := fp.GetDefaultWebFingerprintRules()
		userRule := webfingerprint.FileOrDirToWebRules(c.String("rule-path"))

		if c.Bool("only-rule") {
			webRules = userRule
		} else {
			webRules = append(webRules, userRule...)
		}

		fingerprintMatchConfigOptions := []fp.ConfigOption{
			// 主动探测模式 - 主动发送符合条件的包
			fp.WithActiveMode(true),

			// 每一个指纹探测请求的超时时间
			fp.WithProbeTimeout(time.Second * time.Duration(c.Int("request-timeout"))),

			// web 指纹火力全开
			fp.WithWebFingerprintUseAllRules(true),

			// web 指纹
			fp.WithWebFingerprintRule(webRules),

			// 打开 Web 指纹识别
			fp.WithForceEnableWebFingerprint(true),

			// 开启 TCP 扫描
			fp.WithTransportProtos(fp.TCP),
		}
		fpConfig := fp.NewConfig(fingerprintMatchConfigOptions...)

		scanCenterConfig, err := hybridscan.NewDefaultConfigWithSynScanConfig(synScanConfig)
		if err != nil {
			log.Error("default config failed: %s", err)
			return
		}

		// 指纹扫描开关
		// 指纹扫描单独进行扫描
		scanCenterConfig.DisableFingerprintMatch = true

		log.Info("start create hyper scan center...")
		scanCenter, err := hybridscan.NewHyperScanCenter(context.Background(), scanCenterConfig)
		if err != nil {
			log.Error(err)
			return
		}

		log.Info("preparing for result collectors")
		var fpLock = new(sync.Mutex)
		var openPortLock = new(sync.Mutex)

		var fpResults []*fp.MatchResult
		var openPortCount int
		var openResult []string

		//// 分发任务与回调函数
		//err = scanCenter.RegisterMatcherResultHandler("cmd", func(matcherResult *fp.MatchResult, err error) {
		//	fpLock.Lock()
		//	defer fpLock.Unlock()
		//
		//	fpCount++
		//
		//	if matcherResult != nil {
		//		fpResults = append(fpResults, matcherResult)
		//		log.Infof("found open port fp -> %v", utils.HostPort(matcherResult.Target, matcherResult.Port))
		//	}
		//})
		//if err != nil {
		//	log.Error(err)
		//	return
		//}

		// outputfile
		var outputFile *os.File
		if c.String("output") != "" {
			outputFile, err = os.OpenFile(c.String("output"), os.O_RDWR|os.O_CREATE, os.ModePerm)
			if err != nil {
				log.Error("open file %v failed; %s", c.String("output"), err)
			}
			if outputFile != nil {
				defer outputFile.Close()
			}
		}

		log.Infof("start submit task and scan...")
		err = scanCenter.Scan(
			context.Background(),
			c.String("target"), c.String("port"), true, false,
			func(ip net.IP, port int) {
				openPortLock.Lock()
				defer openPortLock.Unlock()

				openPortCount++
				r := utils.HostPort(ip.String(), port)
				log.Debugf("found open port -> tcp://%v", r)
				openResult = append(openResult, r)

				if outputFile != nil {
					//outputFile.Write([]byte(fmt.Sprintf("%v\n", r)))
					outputFile.Write(
						[]byte(fmt.Sprintf(
							"%s%v\n",
							c.String("output-line-prefix"),
							r,
						)),
					)
				}
			},
		)
		if err != nil {
			log.Error(err)
			return
		}
		log.Infof("finished submitting.")

		if c.Bool("fingerprint") {
			fpTargetChan := make(chan *fp.PoolTask)
			go func() {
				defer close(fpTargetChan)
				for _, i := range openResult {
					host, port, err := utils.ParseStringToHostPort(i)
					if err != nil {
						continue
					}

					fpTargetChan <- &fp.PoolTask{
						Host:    host,
						Port:    port,
						Options: fingerprintMatchConfigOptions,
					}
				}
			}()
			pool, err := fp.NewExecutingPool(context.Background(), c.Int("fingerprint-concurrent"), fpTargetChan, fpConfig)
			if err != nil {
				log.Error("create fingerprint execute pool failed: %s", err)
				return
			}
			pool.AddCallback(func(matcherResult *fp.MatchResult, err error) {
				fpLock.Lock()
				defer fpLock.Unlock()

				if matcherResult != nil {
					fpResults = append(fpResults, matcherResult)
					log.Infof("scan fingerprint finished: -> %v", utils.HostPort(matcherResult.Target, matcherResult.Port))
				}
			})
			err = pool.Run()
			if err != nil {
				log.Error("fingerprint execute pool run failed: %v", err)
				return
			}
		}

		analysis := fp.MatcherResultsToAnalysis(fpResults)

		log.Infof("waiting last packet (SYN) for %v seconds", c.Int("waiting"))
		select {
		case <-time.After(time.Second * time.Duration(c.Int("waiting"))):
		}

		hosts := utils.ParseStringToHosts(c.String("target"))
		ports := utils.ParseStringToPorts(c.String("port"))
		analysis.TotalScannedPort = len(hosts) * len(ports)

		if c.Bool("fp") || len(analysis.OpenPortCPEMap) > 0 {
			analysis.Show()
			analysis.ToJson(c.String("fp-json"))
		} else {
			log.Infof("open ports ...\n===================================")
			for _, port := range openResult {
				println(port)
			}
		}
	},
}
