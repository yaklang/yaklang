package httptpl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"strings"
	"time"
)

func init() {
	for k, v := range tools.NucleiOperationsExports {
		Exports[k] = v
	}

	yaklib.FuzzExports["FuzzCalcExpr"] = FuzzCalcExpr
}

func FuzzCalcExpr() map[string]interface{} {
	vars := NewVars()
	day := mutate.MutateQuick("{{ri(1-28|2)}}")[0]
	vars.AutoSet("year", "{{rand_int(2000,2020)}}")
	vars.AutoSet("month", "0{{rand_int(1,7)}}")
	vars.AutoSet("day", day)
	vars.AutoSet("expr", `{{year}}-{{month}}-{{day}}`)
	vars.AutoSet("result", `{{to_number(year)-to_number(month)-to_number(day)}}`)
	var a = vars.ToMap()
	return a
}

func ScanPacket(req []byte, opts ...interface{}) {
	config, lowhttpConfig, lowhttpOpts := toConfig(opts...)
	baseContext, cancel := context.WithCancel(context.Background())
	_ = cancel

	if lowhttpConfig.Ctx == nil {
		lowhttpConfig.Ctx = baseContext
	}

	//config.AppendHTTPResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
	//	if result {
	//		log.Infof("httptpl.YakTemplate matched response: %v", y.Name)
	//	}
	//})

	var urlStr string
	u, _ := lowhttp.ExtractURLFromHTTPRequestRaw(req, lowhttpConfig.Https)
	if u != nil {
		urlStr = u.String()
	}

	switch strings.ToLower(strings.TrimSpace(config.Mode)) {
	case "nuclei":
		templateConcurrent := config.ConcurrentTemplates
		if templateConcurrent <= 0 {
			templateConcurrent = 10
		}
		swg := utils.NewSizedWaitGroup(templateConcurrent)

		tplChan, err := config.GenerateYakTemplate()
		if err != nil {
			log.Errorf("generate yak template failed: %s", err)
			return
		}
		for tpl := range tplChan {
			if tpl.ReverseConnectionNeed && !config.EnableReverseConnectionFeature {
				log.Infof("skip template %s because of reverse connection feature is disabled", tpl.Name)
				continue
			}

			tpl := tpl
			err := swg.AddWithContext(lowhttpConfig.Ctx)
			if err != nil {
				continue
			}

			if config.Verbose {
				log.Infof("start to execute [%v] for url[%v]", tpl.Name, urlStr)
			}

			go func() {
				defer func() {
					swg.Done()
					if err := recover(); err != nil {
						log.Errorf("execute template failed: %v", err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}

					if config.Verbose {
						log.Infof("finished executing [%v] for url[%v]", tpl.Name, urlStr)
					}
				}()

				_, err := tpl.Exec(config, lowhttpConfig.Https, req, lowhttpOpts...)
				if err != nil {
					log.Errorf("execute template failed: %s", err)
				}
			}()
		}
		log.Infof("waiting for all templates finished [%v]", urlStr)
		swg.Wait()
		log.Infof("all templates finished for url[%v]", urlStr)

		return
	case "xray":
	}
	log.Error("not implemented")
	return
}

func ScanUrl(u string, opt ...interface{}) {
	https, raw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", u)
	if err != nil {
		return
	}
	opt = append(opt, lowhttp.WithHttps(https))
	ScanPacket(raw, opt...)
}

func toConfig(opts ...interface{}) (*Config, *lowhttp.LowhttpExecConfig, []lowhttp.LowhttpOpt) {
	var configOpt []ConfigOption
	var lowhttpOpt []lowhttp.LowhttpOpt
	for _, opt := range opts {
		switch ret := opt.(type) {
		case lowhttp.LowhttpOpt:
			lowhttpOpt = append(lowhttpOpt, ret)
		case ConfigOption:
			configOpt = append(configOpt, ret)
		}
	}
	config := NewConfig(configOpt...)
	lowhttpConfig := lowhttp.NewLowhttpOption()
	for _, opt := range lowhttpOpt {
		opt(lowhttpConfig)
	}
	return config, lowhttpConfig, lowhttpOpt
}

func ScanAuto(items any, opt ...interface{}) {
	switch items.(type) {
	case string, []byte:
		ScanAuto([]any{items}, opt...)
		return
	}

	ch := make(chan any, 100)
	go func() {
		defer func() {
			close(ch)
		}()
		funk.ForEach(items, func(item any) {
			ch <- utils.InterfaceToString(item)
		})
	}()
	_scanStream(ch, opt...)
}

func _scanStream(ch chan any, opt ...interface{}) {
	config, _, _ := toConfig(opt...)

	swg := utils.NewSizedWaitGroup(config.ConcurrentTarget)
	handleData := func(data any) {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("_scanStream.execute template failed: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()

		rawStr := utils.InterfaceToString(data)
		for _, methodPrefix := range []string{"GET", "POST", "HEAD", "OPTIONS", "PUT", "DELETE", "TRACE", "CONNECT"} {
			methodPrefix := methodPrefix
			if strings.HasPrefix(rawStr, methodPrefix+" ") {
				swg.Add()
				go func() {
					defer func() {
						swg.Done()
					}()
					ScanPacket([]byte(rawStr), opt...)
				}()
				return
			}
		}

		if strings.HasPrefix(rawStr, "http://") || strings.HasPrefix(rawStr, "https://") {
			swg.Add()
			go func() {
				defer func() {
					swg.Done()
				}()
				ScanUrl(rawStr, opt...)
			}()
			return
		}

		addrs := utils.ParseStringToUrlsWith3W(rawStr)
		for _, u := range addrs {
			if !utils.IsHttp(u) {
				continue
			}
			u := u
			swg.Add()
			go func() {
				defer func() {
					swg.Done()
				}()
				ScanUrl(u, opt...)
			}()
		}
	}

	count := 0
	for data := range ch {
		count++
		handleData(data)
	}
	log.Infof("waiting for ScanStream total: %v(subtask: %v)", count, swg.WaitingEventCount)
	swg.Wait()
	log.Infof("finished ScanStream total: %v", count)
}

func nucleiOptionDummy(n string) func(i ...any) any {
	return func(i ...any) any {
		return ConfigOption(func(config *Config) {
			log.Errorf("option: nuclei %s is not implemented or abandoned", n)
		})
	}
}

func httpPayloadsToString(payloads *YakPayloads) (string, error) {
	result := make(map[string]string)
	for key, value := range payloads.raw {
		result[key] = fmt.Sprintf("%+v - %+v", value.FromFile, value.Data)
	}
	jsonBytes, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return string(jsonBytes), nil
}

func processVulnerability(target any, filterVul *filter.StringFilter, vCh chan *tools.PocVul) func(i map[string]interface{}) {
	return func(i map[string]interface{}) {
		if i["match"].(bool) {
			tpl := i["template"].(*YakTemplate)
			var (
				currTarget string
				payloads   string
				err        error
				calcSha1   string
			)
			details := make(map[string]interface{}, 2)
			if len(tpl.HTTPRequestSequences) > 0 {
				resp := i["responses"].([]*lowhttp.LowhttpResponse)
				currTarget = resp[0].RemoteAddr
				reqBulk := i["requests"].(*YakRequestBulkConfig)
				// 根据 payload , tpl 名称 , target 条件过滤
				calcSha1 = utils.CalcSha1(tpl.Name, resp[0].RawRequest, target)
				if len(resp) == 1 {
					details["request"] = string(resp[0].RawRequest)
					details["response"] = string(resp[0].RawPacket)
				} else {
					for idx, r := range resp {
						details[fmt.Sprintf("request_%d", idx+1)] = string(r.RawRequest)
						details[fmt.Sprintf("response_%d", idx+1)] = string(r.RawPacket)
					}
				}
				payloads, err = httpPayloadsToString(reqBulk.Payloads)
				if err != nil {
					log.Errorf("httpPayloadsToString failed: %v", err)
				}
			}

			if len(tpl.TCPRequestSequences) > 0 {
				resp := i["responses"].([]*NucleiTcpResponse)
				calcSha1 = utils.CalcSha1(tpl.Name, resp[0].RawRequest, target)

				currTarget = resp[0].RemoteAddr
				if len(resp) == 1 {
					details["request"] = spew.Sdump(resp[0].RawRequest)
					details["response"] = spew.Sdump(resp[0].RawPacket)
				} else {
					for idx, r := range resp {
						details[fmt.Sprintf("request_%d", idx+1)] = spew.Sdump(r.RawRequest)
						details[fmt.Sprintf("response_%d", idx+1)] = spew.Sdump(r.RawPacket)
					}
				}
			}

			pv := &tools.PocVul{
				Source:        "nuclei",
				Target:        currTarget,
				PocName:       tpl.Name,
				MatchedAt:     utils.DatetimePretty(),
				Tags:          strings.Join(tpl.Tags, ","),
				Timestamp:     time.Now().Unix(),
				Severity:      tpl.Severity,
				Details:       details,
				CVE:           tpl.CVE,
				DescriptionZh: tpl.DescriptionZh,
				Description:   tpl.Description,
				Payload:       payloads,
			}
			if !filterVul.Exist(calcSha1) {
				filterVul.Insert(calcSha1)
				risk := tools.PocVulToRisk(pv)
				err = yakit.SaveRisk(risk)
				if err != nil {
					log.Errorf("save risk failed: %s", err)
				}
				vCh <- pv
			}

		}
	}
}

var Exports = map[string]interface{}{
	"Scan": func(target any, opt ...interface{}) (chan *tools.PocVul, error) {
		var vCh = make(chan *tools.PocVul)
		filterVul := filter.NewFilter()
		i := processVulnerability(target, filterVul, vCh)
		opt = append(opt, _callback(i))
		opt = append(opt, _tcpCallback(i))

		c, _, _ := toConfig(opt...)
		if strings.TrimSpace(c.SingleTemplateRaw) != "" {
			tpl, err := CreateYakTemplateFromNucleiTemplateRaw(c.SingleTemplateRaw)
			if err != nil {
				log.Errorf("create yak template failed (raw): %s", err)
				close(vCh)
				return vCh, err
			}
			_ = tpl
		}

		go func() {
			defer close(vCh)
			ScanAuto(target, opt...)
		}()

		return vCh, nil
	},
	"ScanAuto": ScanAuto,

	// params
	"tags":                    WithTags,
	"excludeTags":             nucleiOptionDummy("excludeTags"),
	"workflows":               nucleiOptionDummy("workflows"),
	"templates":               WithTemplateName,
	"excludeTemplates":        WithExcludeTemplates,
	"templatesDir":            nucleiOptionDummy("templatesDir"),
	"headers":                 nucleiOptionDummy("headers"),
	"severity":                nucleiOptionDummy("severity"),
	"output":                  nucleiOptionDummy("output"),
	"proxy":                   lowhttp.WithProxy,
	"logFile":                 nucleiOptionDummy("logFile"),
	"reportingDB":             nucleiOptionDummy("reportingDB"),
	"reportingConfig":         nucleiOptionDummy("reportingConfig"),
	"bulkSize":                WithConcurrentTemplates,
	"templatesThreads":        WithConcurrentInTemplates,
	"timeout":                 _timeout,
	"pageTimeout":             _timeout,
	"retry":                   lowhttp.WithRetryTimes,
	"rateLimit":               rateLimit,
	"headless":                nucleiOptionDummy("headless"),
	"showBrowser":             nucleiOptionDummy("showBrowser"),
	"dnsResolver":             lowhttp.WithDNSServers,
	"systemDnsResolver":       nucleiOptionDummy("systemDnsResolver"),
	"metrics":                 nucleiOptionDummy("metrics"),
	"debug":                   WithDebug,
	"debugRequest":            WithDebugRequest,
	"debugResponse":           WithDebugResponse,
	"silent":                  nucleiOptionDummy("silent"),
	"version":                 nucleiOptionDummy("version"),
	"verbose":                 WithVerbose,
	"noColor":                 nucleiOptionDummy("noColor"),
	"updateTemplates":         nucleiOptionDummy("updateTemplates"),
	"templatesVersion":        nucleiOptionDummy("templatesVersion"),
	"templateList":            nucleiOptionDummy("templateList"),
	"stopAtFirstMatch":        nucleiOptionDummy("stopAtFirstMatch"),
	"noMeta":                  nucleiOptionDummy("noMeta"),
	"newTemplates":            nucleiOptionDummy("newTemplates"),
	"noInteractsh":            noInteractsh,
	"reverseUrl":              nucleiOptionDummy("reverseUrl"),
	"enableReverseConnection": WithEnableReverseConnectionFeature,
	"targetConcurrent":        WithConcurrentTarget,
	"rawTemplate":             WithTemplateRaw,
	"fuzzQueryTemplate":       WithFuzzQueryTemplate,
	"mode":                    WithMode,
	"resultCallback":          _callback,
	"tcpResultCallback":       _tcpCallback,
	"https":                   lowhttp.WithHttps,
	"http2":                   lowhttp.WithHttp2,
}

func _callback(handler func(i map[string]interface{})) ConfigOption {
	return WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		handler(map[string]interface{}{
			"template":  y,
			"requests":  reqBulk,
			"responses": rsp,
			"response":  rsp,
			"match":     result,
			"extractor": extractor,
		})
	})
}

func _tcpCallback(handler func(i map[string]interface{})) ConfigOption {
	return WithTCPResultCallback(func(y *YakTemplate, reqBulk *YakNetworkBulkConfig, rsp []*NucleiTcpResponse, result bool, extractor map[string]interface{}) {
		handler(map[string]interface{}{
			"template":  y,
			"requests":  reqBulk,
			"responses": rsp,
			"response":  rsp,
			"match":     result,
			"extractor": extractor,
		})
	})
}

func noInteractsh(b bool) ConfigOption {
	return WithEnableReverseConnectionFeature(!b)
}

func rateLimit(i float64) lowhttp.LowhttpOpt {
	return lowhttp.WithRetryWaitTime(utils.FloatSecondDuration(i))
}

func _timeout(i float64) lowhttp.LowhttpOpt {
	return func(o *lowhttp.LowhttpExecConfig) {
		o.Timeout = utils.FloatSecondDuration(i)
	}
}
