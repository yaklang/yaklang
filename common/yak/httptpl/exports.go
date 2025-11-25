package httptpl

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/samber/lo"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yak/yaklib/tools"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func init() {
	for k, v := range tools.NucleiOperationsExports {
		Exports[k] = v
	}

	yaklib.FuzzExports["FuzzCalcExpr"] = FuzzCalcExpr
	yaklib.FuzzExports["FuzzCalcExprInt32Safe"] = FuzzCalcExpr2
	yaklib.FuzzExports["FuzzCalcExprInt64Safe"] = FuzzCalcExpr3
}

func FuzzCalcExpr3() map[string]any {
	vars := NewVars()
	// int32 max: 2147483647               (10位)
	// int64 max: 9223372036854775807      (19位)
	powFact := 18
	var num1 int64 = int64(4*math.Pow10(powFact)) + int64(rand.Intn(int(4*math.Pow10(powFact)))) // int64 safe
	var num2 int64 = int64(rand.Intn(int(4*math.Pow10(powFact) - 3)))                            // int64 safe
	vars.Set("num1", fmt.Sprint(num1))
	vars.Set("num2", fmt.Sprint(num2))
	vars.SetAsNucleiTags("expr", `{{num1}}-{{num2}}`)
	vars.Set("result", fmt.Sprint(num1-num2))
	return vars.ToMap()
}

func FuzzCalcExpr2() map[string]any {
	vars := NewVars()
	// int32 max: 2147483647               (10位)
	// int64 max: 9223372036854775807      (19位)
	powFact := 9
	var num1 int64 = int64(4*math.Pow10(powFact)) + int64(rand.Intn(int(4*math.Pow10(powFact)))) // int32 safe
	var num2 int64 = int64(rand.Intn(int(4*math.Pow10(powFact) - 3)))                            // int32 safe
	vars.Set("num1", fmt.Sprint(num1))
	vars.Set("num2", fmt.Sprint(num2))
	vars.SetAsNucleiTags("expr", `{{num1}}-{{num2}}`)
	vars.Set("result", fmt.Sprint(num1-num2))
	return vars.ToMap()
}

func FuzzCalcExpr() map[string]any {
	vars := NewVars()
	var day string
	var month string
	if rand.Intn(2) == 1 {
		day = mutate.MutateQuick("{{zp({{ri(10,28)}}|2)}}")[0]
	} else {
		day = mutate.MutateQuick("{{zp({{ri(0,7)}}|2)}}")[0]
	}
	if rand.Intn(2) == 1 {
		month = mutate.MutateQuick("{{zp({{ri(0,7)}}|2)}}")[0]
	} else {
		month = mutate.MutateQuick("{{zp({{ri(10,12)}}|2)}}")[0]
	}
	year := mutate.MutateQuick("{{zp({{ri(1970," + fmt.Sprint(time.Now().Year()) + ")}}|4)}}")[0]
	result := codec.Atoi(strings.TrimLeft(year, "0")) - codec.Atoi(strings.TrimLeft(month, "0")) - codec.Atoi(strings.TrimLeft(day, "0"))
	vars.AutoSet("year", year)
	vars.AutoSet("month", month)
	vars.AutoSet("day", day)
	vars.AutoSet("expr", `{{year}}-{{month}}-{{day}}`)
	vars.AutoSet("result", fmt.Sprint(result))
	a := vars.ToMap()
	return a
}

func ScanPacket(req []byte, opts ...interface{}) (count uint64) {
	config, lowhttpConfig, lowhttpOpts := toConfig(opts...)
	ctx := context.Background()
	if config.Ctx != nil {
		ctx = config.Ctx
	}
	baseContext, cancel := context.WithCancel(ctx)
	_ = cancel

	if lowhttpConfig.Ctx == nil {
		lowhttpConfig.Ctx = baseContext
	}

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
			if tpl.SelfContained {
				log.Infof("self-contained skipped: %v", tpl.Name)
				continue
			}

			if tpl.ReverseConnectionNeed && !config.EnableReverseConnectionFeature {
				log.Infof("skip template %s because of reverse connection feature is disabled", tpl.Name)
				continue
			}
			if config.OnTemplateLoaded != nil && !config.OnTemplateLoaded(tpl) {
				log.Infof("skipped template %s because of OnTemplateLoaded", tpl.Name)
				continue
			}
			count++
			log.Debugf("start to using template %v", tpl.Name)

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
		log.Debugf("waiting for all templates finished [%v]", urlStr)
		swg.Wait()
		log.Debugf("all templates finished for url[%v]", urlStr)

		return
	case "xray":
	}
	log.Error("not implemented")
	return
}

func ScanUrl(u string, opt ...interface{}) (count uint64) {
	https, raw, err := lowhttp.ParseUrlToHttpRequestRaw("GET", u)
	if err != nil {
		return
	}
	opt = append(opt, lowhttp.WithHttps(https))
	return ScanPacket(raw, opt...)
}

func toConfig(opts ...interface{}) (*Config, *lowhttp.LowhttpExecConfig, []lowhttp.LowhttpOpt) {
	var configOpt []ConfigOption
	var lowhttpOpt []lowhttp.LowhttpOpt
	var pocOpt []poc.PocConfigOption
	for _, opt := range opts {
		switch ret := opt.(type) {
		case lowhttp.LowhttpOpt:
			lowhttpOpt = append(lowhttpOpt, ret)
		case poc.PocConfigOption:
			pocOpt = append(pocOpt, ret)
		case ConfigOption:
			configOpt = append(configOpt, ret)
		default:
			log.Errorf("unknown option type: %T", ret)
		}
	}
	pocConfig := poc.NewDefaultPoCConfig()
	for _, opt := range pocOpt {
		opt(pocConfig)
	}
	config := NewConfig(configOpt...)
	if pocConfig.Context == nil {
		pocConfig.Context = config.Ctx
	}
	if config.RuntimeId != "" {
		pocConfig.RuntimeId = config.RuntimeId
	}
	lowhttpConfig := lowhttp.NewLowhttpOption()
	totalConfig := append(lowhttpOpt, pocConfig.ToLowhttpOptions()...)
	for _, opt := range totalConfig {
		opt(lowhttpConfig)
	}
	return config, lowhttpConfig, totalConfig
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

	var tplCount uint64
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
					atomic.AddUint64(&tplCount, ScanPacket([]byte(rawStr), opt...))
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
				atomic.AddUint64(&tplCount, ScanUrl(rawStr, opt...))
			}()
			return
		}

		addrs := utils.ParseStringToUrlsWith3W(rawStr)
		for _, u := range addrs {
			if !utils.IsHttpOrHttpsUrl(u) {
				continue
			}
			u := u
			swg.Add()
			go func() {
				defer func() {
					swg.Done()
				}()
				atomic.AddUint64(&tplCount, ScanUrl(u, opt...))
			}()
		}
	}

	count := 0
	for data := range ch {
		count++
		handleData(data)
	}
	log.Debugf("waiting for ScanStream total: %v(subtask: %v)", count, swg.WaitingEventCount)
	swg.Wait()
	log.Debugf("finished ScanStream total: %v", count)

	if tplCount == 0 {
		log.Warn(`no template executed, use nuclei.PullTemplates or Yakit to pull templates into local directory,
and set nuclei.all(true) / nuclei.rawTemplate("template string") / nuclei.fuzzQueryTemplate("keyword") / nuclei.templates("template name1", "template name2") to select template`)
	}
}

func nucleiOptionDummy(n string) func(i ...any) any {
	return func(i ...any) any {
		return ConfigOption(func(config *Config) {
			// log.Errorf("option: nuclei %s is not implemented or abandoned", n)
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

func WithOnRisk(target string, onRisk func(i *schema.Risk)) ConfigOption {
	vCh := make(chan *tools.PocVul)
	filterVul := filter.NewMapFilter()
	i := processVulnerability(target, filterVul, vCh, onRisk)

	return func(config *Config) {
		_callback(i)(config)
		_tcpCallback(i)(config)
		go func() {
			defer filterVul.Close()
			<-vCh
		}()
	}
}

func processVulnerability(target any, filterVul filter.Filterable, vCh chan *tools.PocVul, handlers ...func(i *schema.Risk)) func(i map[string]interface{}) {
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
			runtimeId := utils.MapGetString(i, "runtimeId")
			if len(tpl.HTTPRequestSequences) > 0 {
				resp := i["responses"].([]*lowhttp.LowhttpResponse)
				reqBulk := i["requests"].(*YakRequestBulkConfig)
				if runtimeId != "" {
					runtimeId = resp[0].RuntimeId
				}
				// 根据 payload , tpl 名称 , target 条件过滤
				// calcSha1 = utils.CalcSha1(tpl.Name, resp[0].RawRequest, target)
				var urls []string
				calcSha1 = utils.CalcSha1(tpl.Name, resp[0].RemoteAddr, target)
				if len(resp) == 1 {
					urls = append(urls, resp[0].Url)
					details["request"] = string(resp[0].RawRequest)
					details["response"] = string(resp[0].RawPacket)
				} else {
					for idx, r := range resp {
						urls = append(urls, r.Url)
						details[fmt.Sprintf("request_%d", idx+1)] = string(r.RawRequest)
						details[fmt.Sprintf("response_%d", idx+1)] = string(r.RawPacket)
					}
				}
				currTarget = strings.Join(urls, ",")
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
				RuntimeId:     runtimeId,
				UUID:          tpl.UUID,
				ScriptName:    tpl.ScriptName,
			}
			if !filterVul.Exist(calcSha1) {
				filterVul.Insert(calcSha1)
				risk := tools.PocVulToRisk(pv)
				risk.RuntimeId = runtimeId
				for _, h := range handlers {
					h(risk)
				}
				err = yakit.SaveRisk(risk)
				if err != nil {
					log.Errorf("save risk failed: %s", err)
				}
				vCh <- pv
			}

		}
	}
}

func ScanLegacy(target any, opt ...interface{}) (chan *tools.PocVul, error) {
	opts := make([]ConfigOption, 0, len(opt))
	lo.Filter(opt, func(item interface{}, index int) bool {
		_, ok := item.(ConfigOption)
		if ok {
			opts = append(opts, item.(ConfigOption))
		}
		return ok
	})
	return ScanLegacyWithFilter(target, NewConfig(opts...).defaultFilter, opt...)
}

func ScanLegacyWithFilter(target any, filterVul filter.Filterable, opt ...interface{}) (chan *tools.PocVul, error) {
	if filterVul == nil {
		filterVul = defaultFilter
	}
	vCh := make(chan *tools.PocVul)

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
}

var Exports = map[string]interface{}{
	"Scan":     ScanLegacy,
	"ScanAuto": ScanAuto,

	// params
	"customVulnFilter":        WithCustomVulnFilter,
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
	"interactshTimeout":       WithOOBTimeout,
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
	"exactTemplateIns":        WithExactTemplateInstance,
	"all":                     WithAllTemplate,
	"vars":                    withCustomVariablesFromInterface,
	// "runtimeId":               lowhttp.WithRuntimeId,
	"runtimeId":         WithHttpTplRuntimeId,
	"mode":              WithMode,
	"resultCallback":    _callback,
	"tcpResultCallback": _tcpCallback,
	"https":             lowhttp.WithHttps,
	"http2":             lowhttp.WithHttp2,
	"http3":             lowhttp.WithHttp3,
	"fromPlugin":        lowhttp.WithFromPlugin,
	"context":           WithContext,
}

func WithHttpTplRuntimeId(id string) ConfigOption {
	return func(config *Config) {
		config.RuntimeId = id
	}
}

func _callback(handler func(i map[string]interface{})) ConfigOption {
	return WithResultCallback(func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		var runtimeId string
		if len(rsp) > 0 {
			runtimeId = rsp[0].RuntimeId
		}
		handler(map[string]interface{}{
			"template":  y,
			"requests":  reqBulk,
			"responses": rsp,
			"response":  rsp,
			"match":     result,
			"extractor": extractor,
			"runtimeId": runtimeId,
		})
	})
}

func _tcpCallback(handler func(i map[string]interface{})) ConfigOption {
	return WithTCPResultCallback(func(y *YakTemplate, reqBulk *YakNetworkBulkConfig, rsp []*NucleiTcpResponse, result bool, extractor map[string]interface{}) {
		var runtimeId string
		if len(rsp) > 0 {
			runtimeId = rsp[0].RuntimeId
		}
		handler(map[string]interface{}{
			"template":  y,
			"requests":  reqBulk,
			"responses": rsp,
			"response":  rsp,
			"match":     result,
			"extractor": extractor,
			"runtimeId": runtimeId,
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
