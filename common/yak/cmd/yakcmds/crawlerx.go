// Package yakcmds
// @Author bcy2007  2024/3/12 16:06
package yakcmds

import (
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/crawlerx"
	"github.com/yaklang/yaklang/common/crawlerx/tools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"net/url"
	"strings"
	"time"
)

var crawlerxCommand = cli.Command{
	Name:  "crawlerx",
	Usage: "click-simulated crawler base on headless browser",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:     "url,u",
			Usage:    "target crawler `URL`",
			Required: true,
		},
		cli.StringFlag{
			Name:  "output,o",
			Usage: "crawler result output `FILE`",
		},
		cli.StringFlag{
			Name:  "browser-path",
			Usage: "crawler browser `EXE_PATH`",
		},
		cli.StringFlag{
			Name:  "ws",
			Usage: "browser `WS_ADDRESS`",
		},
		cli.StringFlag{
			Name:  "proxy",
			Usage: "crawler `PROXY_URL` info",
		},
		cli.IntFlag{
			Name:  "max-url",
			Usage: "crawler `MAX_URL_NUMBER` (0 means unlimited)",
			Value: 0,
		},
		cli.IntFlag{
			Name:  "max-depth",
			Usage: "crawler `MAX_DEPTH_LEVEL`",
			Value: 3,
		},
		cli.IntFlag{
			Name:  "concurrent",
			Usage: "max crawler `CONCURRENT_NUMBER`",
			Value: 2,
		},
		cli.StringFlag{
			Name:  "blacklist",
			Usage: "url `BLACK_LIST_STRING` which divided by ','",
		},
		cli.StringFlag{
			Name:  "whitelist",
			Usage: "url `WHITE_LIST_STRING` which divided by ','",
		},
		cli.IntFlag{
			Name:  "page-timeout",
			Usage: "`TIMEOUT` on single page crawler",
			Value: 30,
		},
		cli.IntFlag{
			Name:  "full-timeout",
			Usage: "`TIMEOUT` on full crawler",
			Value: 1800,
		},
		cli.StringFlag{
			Name:  "form-fill",
			Usage: "form fill when meet input element (format: `KEYWORD:VALUE;`)",
		},
		cli.StringFlag{
			Name:  "file-input",
			Usage: "file input path when meet file input element (format: `KEYWORD:FILEPATH;`)",
		},
		cli.StringFlag{
			Name:  "headers",
			Usage: "request headers set (format: `KEYWORD:VALUE;`)",
		},
		cli.StringFlag{
			Name:  "cookie",
			Usage: "request headers set (format: `KEYWORD:VALUE;`)",
		},
		cli.IntFlag{
			Name:  "range-level",
			Usage: "scan `RANGE_LEVEL` set (0: main domain, 1: subdomain)",
			Value: 0,
		},
		cli.IntFlag{
			Name:  "repeat-level",
			Usage: "scan `REPEAT_LEVEL` set (0-4: unlimited-extreme)",
			Value: 1,
		},
		cli.StringFlag{
			Name:  "ignore-query",
			Usage: "`IGNORE_QUERY` in url when check repeat url (divided by ',')",
		},
		cli.StringFlag{
			Name:  "sensitive-word",
			Usage: "not clicked element that `SENSITIVE_WORD` in inner html (divided by ',')",
		},
		cli.StringFlag{
			Name:  "local-storage",
			Usage: "local storage set on target url (format: `KEYWORD:VALUE;`)",
		},
	},
	Action: func(c *cli.Context) error {
		urlStr := c.String("url")
		proxyStr := c.String("proxy")
		outputFile := c.String("output")
		var proxy *url.URL
		var err error
		if proxyStr == "" {
			proxy = nil
		} else {
			proxy, err = url.Parse(proxyStr)
			if err != nil {
				return utils.Errorf("proxy url %v parse error: %v", proxyStr, err)
			}
		}
		browserInfo := crawlerx.NewBrowserConfig(c.String("browser-path"), c.String("ws"), proxy)
		opts := make([]crawlerx.ConfigOpt, 0)
		opts = append(opts,
			crawlerx.WithMaxUrl(c.Int("max-url")),
			crawlerx.WithMaxDepth(c.Int("max-depth")),
			crawlerx.WithConcurrent(c.Int("concurrent")),
			crawlerx.WithPageTimeout(c.Int("page-timeout")),
			crawlerx.WithFullTimeout(c.Int("full-timeout")),
			crawlerx.WithScanRangeLevel(crawlerx.ScanRangeLevelMap[c.Int("range-level")]),
			crawlerx.WithScanRepeatLevel(crawlerx.RepeatLevelMap[c.Int("repeat-level")]),
			crawlerx.WithBrowserData(browserInfo),
		)
		if c.String("whitelist") != "" {
			opts = append(opts, crawlerx.WithWhiteList(getSliceFromString(c.String("whitelist"))...))
		}
		if c.String("blacklist") != "" {
			opts = append(opts, crawlerx.WithBlackList(getSliceFromString(c.String("blacklist"))...))
		}
		if c.String("form-fill") != "" {
			opts = append(opts, crawlerx.WithFormFill(getMapFromString(c.String("form-fill"))))
		}
		if c.String("file-input") != "" {
			opts = append(opts, crawlerx.WithFileInput(getMapFromString(c.String("file-input"))))
		}
		if c.String("headers") != "" {
			opts = append(opts, crawlerx.WithHeaders(getMapFromString(c.String("headers"))))
		}
		if c.String("cookie") != "" {
			host := ""
			if strings.Contains(urlStr, "://") {
				host = strings.Split(urlStr, "://")[1]
			} else {
				host = urlStr
			}
			host = strings.Split(host, "/")[0]
			opts = append(opts, crawlerx.WithCookies(host, getMapFromString(c.String("cookie"))))
		}
		if c.String("ignore-query") != "" {
			opts = append(opts, crawlerx.WithIgnoreQueryName(getSliceFromString(c.String("ignore-query"))...))
		}
		if c.String("sensitive-word") != "" {
			opts = append(opts, crawlerx.WithSensitiveWords(getSliceFromString(c.String("sensitive-word"))))
		}
		if c.String("local-storage") != "" {
			opts = append(opts, crawlerx.WithLocalStorage(getMapFromString(c.String("local-storage"))))
		}
		ch, err := crawlerx.StartCrawler(urlStr, opts...)
		if err != nil {
			log.Error(err)
			return nil
		}
		log.Info("crawlerx running start!")
		log.Info("NOTICE: It is normal to report Context Canceled Error, so don't worry~")
		if outputFile != "" {
			result := make([]*crawlerx.OutputResult, 0)
			number := 0
			for item := range ch {
				result = append(result, crawlerx.GeneratorOutput(item))
				number++
				if number%10 == 0 {
					log.Infof(`Get %v new requests...`, number)
				}
			}
			log.Infof(`Get %v new requests total`, number)
			time.Sleep(2 * time.Second)
			log.Infof("Generating output file %v...", outputFile)
			resultBytes, _ := json.MarshalIndent(result, "", "\t")
			err := tools.WriteFile(outputFile, resultBytes)
			if err != nil {
				log.Errorf(`Write crawler result to file %s error: %s`, outputFile, err)
			} else {
				log.Info("Generated!")
			}
		} else {
			for item := range ch {
				info := fmt.Sprintf(`%v %d %v`, item.Method(), item.StatusCode(), item.Url())
				log.Info(info)
			}
		}
		return nil
	},
}

func getMapFromString(mapStr string) map[string]string {
	result := make(map[string]string)
	if mapStr == "" {
		return result
	}
	if !strings.Contains(mapStr, ":") {
		return result
	}
	items := strings.Split(mapStr, ";")
	for _, item := range items {
		if !strings.Contains(item, ":") {
			continue
		}
		values := strings.Split(item, ":")
		result[values[0]] = values[1]
	}
	return result
}

func getSliceFromString(sliceStr string) []string {
	if sliceStr == "" {
		return make([]string, 0)
	}
	result := strings.Split(sliceStr, ",")
	return result
}
