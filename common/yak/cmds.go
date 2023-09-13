package yak

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
	"github.com/yaklang/yaklang/common/hybridscan"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/urfave/cli"
)

const (
	catNuclei  = "Nuclei 原生集成 / Nuclei Integration"
	catScanner = "快速扫描 / Scanner"
	catFuzz    = "调试工具 / Utils"
)

var Subcommands = []cli.Command{
	{
		Name:  "tag-stats",
		Usage: "Generate Tag Status",
		Action: func(c *cli.Context) error {
			stats, err := yaklib.NewTagStat()
			if err != nil {
				return err
			}
			for _, v := range stats.All() {
				if v.Count <= 1 {
					continue
				}
				fmt.Printf("TAG:[%v]-%v\n", v.Name, v.Count)
			}
			return nil
		},
	},
	{
		Name:     "update-nuclei-poc",
		Usage:    "更新 nulcei-templates 到本地 / update nuclei-template. (github.com/projectdiscovery/nuclei-templates)",
		Category: catNuclei,
		Action: func(c *cli.Context) error {
			engine := NewScriptEngine(1)
			err := engine.ExecuteMain(
				`log.setLevel("info")

log.info("start to load from github resource...")
die(nuclei.UpdatePoC())`, "main",
			)
			if err != nil {
				log.Errorf("update poc from github resource failed: %s", err)
			}
			return nil
		},
	},
	{
		Name: "update-nuclei-database", Usage: "把本地的 nuclei-templates 更新到数据库 (yakit plugin database)",
		Category: catNuclei,
		Action: func(c *cli.Context) error {
			var err error
			err = NewScriptEngine(1).ExecuteMain(`loglevel("info")
log.info("start to load local database"); 
die(nuclei.UpdateDatabase())`, "main")
			if err != nil {
				log.Errorf("execute nuclei.UpdateDatabase() failed: %s", err)
				return err
			}
			return nil
		},
	},
	{
		Name: "remove-nuclei-database", Usage: "移除本地的 nuclei-templates 数据库",
		Category: catNuclei,
		Action: func(c *cli.Context) error {
			err := tools.RemovePoCDatabase()
			if err != nil {
				log.Errorf("remove pocs failed: %s", err)
			}
			return nil
		},
	},
	{
		Name:     "synscan",
		Usage:    "【快】SYN 扫描端口",
		Category: catScanner,
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
				Value: 60,
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

			scanCenterConfig, err := hybridscan.NewDefaultConfigWithSynScanConfig(
				synScanConfig,
			)
			if err != nil {
				log.Errorf("default config failed: %s", err)
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
					log.Errorf("open file %v failed; %s", c.String("output"), err)
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
					log.Infof("found open port -> tcp://%v", r)
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
					log.Errorf("create fingerprint execute pool failed: %s", err)
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
					log.Errorf("fingerprint execute pool run failed: %v", err)
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
	},
	{
		Name:     "scan-service",
		Usage:    "【精准】指纹扫描",
		Category: catScanner,
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
			userRule := webfingerprint.FileOrDirToWebRules(c.String("rule-path"))

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
				options, fp.WithForceEnableWebFingerprint(true),
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
						fp.WithForceEnableWebFingerprint(true),
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
	},
	{Name: "fuzz", Flags: []cli.Flag{cli.StringFlag{Name: "t,target", Usage: "想要测试的 Fuzz 字符串"}}, Action: func(c *cli.Context) {
		for _, r := range mutate.QuickMutateSimple(c.String("t")) {
			println(r)
		}
	}},
	upgradeCommand,
}

var upgradeCommand = cli.Command{
	Name:  "upgrade",
	Usage: "upgrade / reinstall newest yak.",
	Flags: []cli.Flag{
		cli.IntFlag{
			Name:  "timeout",
			Usage: "连接超时时间",
			Value: 30,
		},
	},
	Action: func(c *cli.Context) error {
		destination, err := os.Executable()
		if err != nil {
			return utils.Errorf("cannot fetch os.Executable()...: %s", err)
		}

		binary := fmt.Sprintf(`https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/yak_%v_%v`, runtime.GOOS, runtime.GOARCH)
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			binary = fmt.Sprintf(`https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/yak_%v_%v`, runtime.GOOS, "amd64")
		} else if runtime.GOOS == "windows" {
			binary = fmt.Sprintf(`https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/yak_%v_%v.exe`, runtime.GOOS, "amd64")
		}

		versionUrl := `https://yaklang.oss-accelerate.aliyuncs.com/yak/latest/version.txt`

		client := utils.NewDefaultHTTPClient()
		client.Timeout = time.Duration(c.Int("timeout")) * time.Second

		rsp, _ := client.Get(versionUrl)
		if rsp != nil && rsp.Body != nil {
			raw, _ := ioutil.ReadAll(rsp.Body)
			if len(utils.ParseStringToLines(string(raw))) <= 3 {
				log.Infof("当前 yak 核心引擎最新版本为 / current latest yak core engine version：%v", string(raw))
			}
		}

		log.Infof("start to download yak: %v", binary)
		rsp, err = client.Get(binary)
		if err != nil {
			log.Errorf("下载 yak 引擎失败：download yak failed: %v", err)
			return err
		}

		// 设置本地缓存
		fd, err := ioutil.TempFile("", "yak-")
		if err != nil {
			log.Errorf("create temp file failed: %v", err)
			return err
		}

		tempFile := fd.Name()
		defer func() {
			os.RemoveAll(tempFile)
			log.Infof("cleaning cache for %v", tempFile)
		}()

		log.Infof("downloading for yak binary to local")
		_, err = io.Copy(fd, rsp.Body)
		if err != nil && err != io.EOF {
			log.Errorf("download failed... %v", err.Error())
			return err
		}
		log.Infof("yak 核心引擎下载成功... / yak engine downloaded")

		err = os.Chmod(tempFile, os.ModePerm)
		if err != nil {
			log.Errorf("chmod +x to[%v] failed: %s", tempFile, err)
			return err
		}

		destPath := destination
		destDir, _ := filepath.Split(destPath)
		oldPath := filepath.Join(destDir, fmt.Sprintf("yak_%s", consts.GetYakVersion()))
		if runtime.GOOS == "windows" {
			oldPath += ".exe"
		}
		log.Infof("backup yak old engine to %s", oldPath)

		log.Infof("origin binary: %s", destination)
		// 备份旧的
		if err := os.Rename(destPath, oldPath); err != nil {
			return utils.Errorf("backup old yak-engine failed: %s, retry re-Install with \n"+
				"    `bash <(curl -sS -L http://oss.yaklang.io/install-latest-yak.sh)`\n\n", err)
		}

		localFile, err := os.OpenFile(destPath, os.O_TRUNC|os.O_CREATE|os.O_WRONLY, 0766)
		if err != nil {
			return fmt.Errorf("open file error, %s", err)
		}
		defer localFile.Close()

		fd.Seek(0, 0)
		_, err = io.Copy(localFile, fd)
		if err != nil {
			return utils.Errorf("install/copy latest yak failed: %s", err)
		}
		fd.Close()

		//cmd := exec.Command(destPath, "version")
		//raw, err := cmd.CombinedOutput()
		//if err != nil {
		//	return err
		//}
		//fmt.Println(string(raw))
		return nil
	},
}
