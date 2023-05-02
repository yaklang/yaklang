package httptpl

import (
	"context"
	"strings"
	"yaklang.io/yaklang/common/go-funk"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

func ScanPacket(req []byte, opts ...interface{}) {
	config, lowhttpConfig, lowhttpOpts := toConfig(opts...)
	baseContext, cancel := context.WithCancel(context.Background())
	_ = cancel

	if lowhttpConfig.Ctx == nil {
		lowhttpConfig.Ctx = baseContext
	}

	config.AppendResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		if result {
			log.Infof("httptpl.YakTemplate matched response: %v", y.Name)
		}
	})

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

		for _, u := range utils.ParseStringToUrlsWith3W(rawStr) {
			if !utils.IsHttp(u) {
				continue
			}
			swg.Add()
			go func() {
				defer func() {
					swg.Done()
				}()
				ScanUrl(u, opt...)
			}()
			return
		}
	}

	count := 0
	for data := range ch {
		count++
		handleData(data)
	}
	log.Infof("waiting for ScanStream total: %v", count)
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

var Exports = map[string]interface{}{
	"Scan": ScanAuto,

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
	"https":                   lowhttp.WithHttps,
	"http2":                   lowhttp.WithHttp2,
}

func _callback(handler func(i map[string]interface{})) ConfigOption {
	return WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		handler(map[string]interface{}{
			"template":  y,
			"requests":  reqBulk,
			"responses": rsp,
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
