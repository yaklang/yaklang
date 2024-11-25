package yak

import (
	"context"
	_ "embed"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/httptpl"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	// func mirrorHTTPFlow(isHttps, url, request, response, body)
	//     mirror hijacked by filtered http flows
	HOOK_MirrorFilteredHTTPFlow = "mirrorFilteredHTTPFlow"

	// func mirrorHTTPFlow(isHttps, url, request, response, body)
	//     mirror hijacked all
	HOOK_MirrorHTTPFlow = "mirrorHTTPFlow"

	// func mirrorNewWebsite(isHttps, url, request, response, body)
	HOOK_MirrorNewWebsite = "mirrorNewWebsite" // schema + addr

	// func mirrorNewWebsitePath(isHttps, url, request, response, body)
	HOOK_MirrorNewWebsitePath = "mirrorNewWebsitePath" // schema + addr + path (remove params)

	// func mirrorNewWebsitePathParams(isHttps, url, request, response, body)
	HOOK_MirrorNewWebsitePathParams = "mirrorNewWebsitePathParams" // schema + addr + path + param_names

	// func hijackHTTPRequest(isHttps, url, request, forward/*func(modified []byte)*/, drop /*func()*/)
	HOOK_HijackHTTPRequest = "hijackHTTPRequest"

	// func hijackHTTPRequest(isHttps, url, response, forward/*func(modified []byte)*/, drop /*func()*/)
	HOOK_HijackHTTPResponse = "hijackHTTPResponse"
	// func hijackHTTPRequest(isHttps, url, request, response, forward/*func(modified []byte)*/, drop /*func()*/)
	HOOK_HijackHTTPResponseEx = "hijackHTTPResponseEx"

	// func hijackSaveHTTPFlow(record *httpFlow, forward func(*httpFlow), drop func()) return (*httpFlow)
	HOOK_hijackSaveHTTPFlow = "hijackSaveHTTPFlow"

	// func handle(r *fp.MatchResult)
	HOOK_PortScanHandle = "handle"

	// func execNuclei(target)
	HOOK_NucleiScanHandle = "execNuclei"

	HOOK_NaslScanHandle           = "execNasl"
	HOOK_LoadNaslScriptByNameFunc = "loadNaslScriptByNameFunc"

	/*
		hijackSaveHTTPFlow = func(flow, forward, drop) {
		    println(flow.Url)
		    flow.Red()
		    forward(flow)
		}
	*/
)

var MITMAndPortScanHooks = []string{
	HOOK_MirrorFilteredHTTPFlow,
	HOOK_MirrorHTTPFlow,
	HOOK_MirrorNewWebsite,
	HOOK_MirrorNewWebsitePath,
	HOOK_MirrorNewWebsitePathParams,
	HOOK_CLAER,

	HOOK_HijackHTTPRequest,
	HOOK_HijackHTTPResponse,
	HOOK_HijackHTTPResponseEx,
	HOOK_hijackSaveHTTPFlow,

	// port-scan
	HOOK_PortScanHandle,
}

type MixPluginCaller struct {
	ctx context.Context // 整个 mix plugin caller 的上下文

	websiteFilter       filter.Filterable
	websitePathFilter   filter.Filterable
	websiteParamsFilter filter.Filterable

	rawQuestFilter filter.Filterable

	runtimeId string
	proxy     string

	feedbackHandler        func(*ypb.ExecResult) error
	ordinaryFeedback       func(i interface{}, item ...interface{})
	callers                *YakToCallerManager
	fingerprintMatcherOnce sync.Once
	fingerprintMatcher     *fp.Matcher
	swg                    *utils.SizedWaitGroup
	cache                  bool
	pluginScanFilter       *yakit.PluginScanFilter // 插件扫描黑白名单，现在直接使用yakit全局网络配置
}

func (m *MixPluginCaller) LastErr() error {
	return m.callers.Err
}

func (m *MixPluginCaller) SetCache(b bool) {
	m.cache = b
}

func (m *MixPluginCaller) SetCtx(ctx context.Context) {
	m.ctx = ctx
}

func (m *MixPluginCaller) SetRuntimeId(s string) {
	if s == "" {
		return
	}
	if m == nil {
		return
	}
	m.runtimeId = s
	if m.callers != nil {
		m.callers.runtimeId = s
	}
}

func (m *MixPluginCaller) SetProxy(s string) {
	if s == "" {
		return
	}
	if m == nil {
		return
	}
	m.proxy = s
	if m.callers != nil {
		m.callers.proxy = s
	}
}

var resetFilterLock = new(sync.Mutex)

var loadTemplateLock = new(sync.Mutex)

const naslCodeExecTemplate = `
naslScriptName = MITM_PARAMS["NASL_SCRIPT_NAME"] // 用于初次加载插件时的预处理操作
proxy = MITM_PARAMS["PROXY"] // 代理
opts = [] // nasl 引擎扫描参数
loadNaslScriptByNameFunc = scriptName => {
	opts.Append(nasl.plugin(scriptName))
}
execNasl = (target)=>{
    if proxy != nil && proxy != ""{
        opts.Append(nasl.proxy(proxy))
    }
	opts.Append(nasl.riskHandle((risk)=>{
		log.info("found risk: %v", risk)
	}))
    kbs ,err = nasl.ScanTarget(target,opts...)
    if err{
        log.error("%v", err)
    }
}
`

//go:embed nuclei_executor.yak
var YAK_TEMPLATE_NUCLEI_EXECUTOR string

func (m *MixPluginCaller) SetFeedback(i func(i *ypb.ExecResult) error) {
	if i == nil {
		return
	}
	m.feedbackHandler = func(result *ypb.ExecResult) error {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("MixPluginCaller Feedback Panic: %v", err)
				utils.Debug(func() {
					utils.PrintCurrentGoroutineRuntimeStack()
				})
				return
			}
		}()
		if i != nil {
			return i(result)
		}
		return nil
	}
	m.ordinaryFeedback = FeedbackFactory(consts.GetGormProjectDatabase(), m.feedbackHandler, false, "")
}

//func (m *MixPluginCaller) SetFeedback(i func(i *ypb.ExecResult) error) {
//	feedBack := m.feedbackHandler
//	m.feedbackHandler = func(result *ypb.ExecResult) error {
//		defer func() {
//			err := feedBack(result)
//			if err != nil {
//				log.Errorf("feedback error: %v", err)
//				return
//			}
//		}()
//		if i != nil {
//			return i(result)
//		}
//		return nil
//	}
//	m.ordinaryFeedback = FeedbackFactory(consts.GetGormProjectDatabase(), m.feedbackHandler, false, "")
//}

func (m *MixPluginCaller) SetDividedContext(b bool) {
	if m.callers == nil {
		return
	}
	m.callers.SetDividedContext(b)
}

func NewMixPluginCaller() (*MixPluginCaller, error) {
	resetFilterLock.Lock()
	defer resetFilterLock.Unlock()
	yaklib.AutoInitYakit()
	webFilter := filter.NewCuckooFilter()
	callerFilter := filter.NewMapFilter()
	c := &MixPluginCaller{
		websiteFilter:       webFilter,
		websitePathFilter:   webFilter,
		websiteParamsFilter: webFilter,
		rawQuestFilter:      webFilter,
		pluginScanFilter:    yakit.GlobalPluginScanFilter,
		callers:             NewYakToCallerManager().WithVulFilter(callerFilter),
		feedbackHandler: func(result *ypb.ExecResult) error {
			return fmt.Errorf("feedback handler not set")
		},
		ctx: context.Background(),
	}
	c.SetLoadPluginTimeout(10)
	c.SetCallPluginTimeout(float64(consts.GetGlobalCallerCallPluginTimeout()))
	c.swg = utils.NewSizedWaitGroup(30)
	return c, nil
}

func NewMixPluginCallerWithFilter(webFilter filter.Filterable) (*MixPluginCaller, error) {
	resetFilterLock.Lock()
	defer resetFilterLock.Unlock()
	yaklib.AutoInitYakit()
	callerFilter := filter.NewMapFilter()
	c := &MixPluginCaller{
		websiteFilter:       webFilter,
		websitePathFilter:   webFilter,
		websiteParamsFilter: webFilter,
		pluginScanFilter:    yakit.GlobalPluginScanFilter,
		callers:             NewYakToCallerManager().WithVulFilter(callerFilter),
		feedbackHandler: func(result *ypb.ExecResult) error {
			return fmt.Errorf("feedback handler not set")
		},
		ctx: context.Background(),
	}
	c.SetLoadPluginTimeout(10)
	c.SetCallPluginTimeout(consts.GetGlobalCallerCallPluginTimeout())
	c.swg = utils.NewSizedWaitGroup(30)
	return c, nil
}

func (c *MixPluginCaller) SetLoadPluginTimeout(i float64) {
	c.callers.SetLoadPluginTimeout(i)
}

func (c *MixPluginCaller) SetCallPluginTimeout(i float64) {
	c.callers.SetCallPluginTimeout(i)
}

func (c *MixPluginCaller) SetConcurrent(i int) error {
	return c.GetNativeCaller().SetConcurrent(i)
}

func (c *MixPluginCaller) Wait() {
	defer c.websiteFilter.Close()

	waitChan := make(chan struct{})

	go func() {
		defer close(waitChan)

		log.Debugf("start to wait local mix caller...")
		c.swg.Wait()
		log.Debugf("mix caller tasks all done")

		log.Debugf("start to wait native caller concurrent")
		c.GetNativeCaller().Wait()
		log.Debugf("native caller all done")
	}()
	select {
	case <-c.ctx.Done():
	case <-waitChan:
	}
}

func (c *MixPluginCaller) ResetFilter() {
	resetFilterLock.Lock()
	defer resetFilterLock.Unlock()
	c.websiteFilter.Close()

	webFilter := filter.NewCuckooFilter()
	c.websiteParamsFilter = webFilter
	c.websitePathFilter = webFilter
	c.websiteFilter = webFilter
}

func (c *MixPluginCaller) IsPassed(target string) bool {
	if c.pluginScanFilter == nil {
		return true
	}
	f := c.pluginScanFilter

	return utils.IncludeExcludeChecker(f.IncludePluginScanURIs, f.ExcludePluginScanURIs, target)
}

func (c *MixPluginCaller) FeedbackOrdinary(i interface{}) {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
			return
		}
	}()

	if c.ordinaryFeedback != nil {
		c.ordinaryFeedback(i)
	}
}

func (c *MixPluginCaller) LoadHotPatch(ctx context.Context, params []*ypb.ExecParamItem, code string) error {
	c.ResetFilter()
	c.FeedbackOrdinary("Initializing HotPatched MITM HOOKS")
	paramsMap := make(map[string]any)
	for _, param := range params {
		paramsMap[param.GetKey()] = param.GetValue()
	}

	err := c.callers.SetForYakit(ctx, code, paramsMap, YakitCallerIf(c.feedbackHandler), MITMAndPortScanHooks...)
	if err != nil {
		c.FeedbackOrdinary(fmt.Sprintf("Initialized HotPatched MITM HOOKS FAILED: %v", err.Error()))
		return err
	}
	return nil
}

func (m *MixPluginCaller) LoadPlugin(scriptName string, params ...*ypb.ExecParamItem) error {
	if m.ctx == nil {
		m.ctx = context.Background()
	}
	return m.LoadPluginByName(m.ctx, scriptName, params)
}

// LoadPluginByName 基于脚本名加载插件，如果没有指定代码，则从数据库中加载，如果指定了代码，则默认视为mitm插件执行
func (m *MixPluginCaller) LoadPluginByName(ctx context.Context, name string, params []*ypb.ExecParamItem, codes ...string) error {
	var (
		ins  *schema.YakScript
		err  error
		code string
	)
	if len(codes) > 0 {
		code = codes[0]
	}

	if code == "" {
		db := consts.GetGormProfileDatabase()
		// 从数据库加载脚本时，通过脚本名前缀判断脚本类型
		if db == nil {
			return utils.Error("no database conn is set / no code set")
		}
		ins, err = yakit.GetYakScriptByName(db, name)
		if err != nil {
			return utils.Errorf("load plugin name (yakScript name: %v) failed: %s", name, err)
		}
	} else {
		ins = &schema.YakScript{
			ScriptName: name,
			Content:    code,
			Type:       "mitm",
		}
	}

	return m.LoadPluginEx(ctx, ins, params...)
}

func (m *MixPluginCaller) LoadPluginEx(ctx context.Context, script *schema.YakScript, params ...*ypb.ExecParamItem) error {
	if script.Type == "yak" || script.Type == "codec" {
		return utils.Errorf("cannot load yak script[%v] - %v: not supported", script.ScriptName, script.Type)
	}

	if script.ForceInteractive {
		log.Infof("script[%v] is interactive, skip load", script.ScriptName)
		return nil
	}
	if m.ctx == nil {
		m.ctx = context.Background()
	}
	var (
		paramMap    = make(map[string]any)
		code        = script.Content
		name        = script.ScriptName
		forNuclei   bool
		forPortScan bool
		forMitm     bool
		forNasl     bool
	)
	for _, p := range params {
		paramMap[p.Key] = p.Value
	}

	ctx = context.WithValue(ctx, "ctx_info", map[string]interface{}{})
	m.FeedbackOrdinary(fmt.Sprintf("Initializing MITM Plugin: %v", name))

	if strings.HasSuffix(strings.ToLower(name), ".nasl") {
		forNasl = true
		code = naslCodeExecTemplate
		params = append(params, &ypb.ExecParamItem{
			Key:   "NASL_SCRIPT_NAME",
			Value: name,
		})
	}

	if script.Type == "port-scan" {
		forPortScan = true
	}

	if script.Type == "mitm" {
		forMitm = true
	}

	if script.Type == "nuclei" {
		_, err := httptpl.CreateYakTemplateFromYakScript(script)
		if err != nil {
			return err
		}

		forNuclei = true
		paramMap["CURRENT_NUCLEI_PLUGIN"] = script
		code = YAK_TEMPLATE_NUCLEI_EXECUTOR
	}

	if forNuclei {
		err := m.callers.AddForYakit(ctx, script, paramMap, code, YakitCallerIf(m.feedbackHandler), HOOK_NucleiScanHandle)
		if err != nil {
			m.FeedbackOrdinary(fmt.Sprintf("Initailzed Nuclei Plugin[%v] Failed: %v", name, err))
			return nil
		}
		return nil
	}
	if forNasl {
		ctx.Value("ctx_info").(map[string]interface{})["isNaslScript"] = true
		err := m.callers.AddForYakit(ctx, script, paramMap, code, YakitCallerIf(m.feedbackHandler), HOOK_NaslScanHandle)
		if err != nil {
			m.FeedbackOrdinary(fmt.Sprintf("Initailzed Nasl Plugin[%v] Failed: %v", name, err))
			return nil
		}
		return nil
	}
	var hooks []string
	switch true {
	case forMitm:
		hooks = []string{
			HOOK_MirrorFilteredHTTPFlow,
			HOOK_MirrorHTTPFlow,
			HOOK_MirrorNewWebsite,
			HOOK_MirrorNewWebsitePath,
			HOOK_MirrorNewWebsitePathParams,
			HOOK_HijackHTTPRequest,
			HOOK_HijackHTTPResponse,
			HOOK_HijackHTTPResponseEx,
			HOOK_hijackSaveHTTPFlow,
		}
	case forPortScan:
		hooks = []string{HOOK_PortScanHandle}
	default:
		hooks = MITMAndPortScanHooks
	}

	err := m.callers.AddForYakit(ctx, script, paramMap, code, YakitCallerIf(m.feedbackHandler), hooks...)
	if err != nil {
		m.FeedbackOrdinary(fmt.Sprintf("Initailzed MITM/ScanPort Plugin[%v] Failed: %v", name, err))
		return err
	}
	return nil
}

func (m *MixPluginCaller) CallHijackRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string, getRequest func() interface{},
	reject func() interface{},
	drop func() interface{},
) {
	if !m.IsPassed(u) {
		log.Infof("call HijackRequest error: url[%v] not passed", u)
		return
	}
	callers := m.callers
	if callers.ShouldCallByName(HOOK_HijackHTTPRequest) {
		callers.Call(
			HOOK_HijackHTTPRequest,
			WithCallConfigForceSync(true),
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItemFuncs(
				func() any { return isHttps },
				func() any { return u },
				getRequest, reject, drop,
			),
		)
	}
}

func (m *MixPluginCaller) CallHijackRequest(
	isHttps bool, u string, getRequest func() interface{},
	reject func() interface{},
	drop func() interface{},
) {
	m.CallHijackRequestWithCtx(context.Background(), isHttps, u, getRequest, reject, drop)
}

func (m *MixPluginCaller) CallHijackResponseWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string, getResponse,
	reject, drop func() interface{},
) {
	if !m.IsPassed(u) {
		log.Infof("call HijackResponse error: url[%v] not passed", u)
		return
	}
	callers := m.callers
	if callers.ShouldCallByName(HOOK_HijackHTTPResponse) {
		callers.Call(
			HOOK_HijackHTTPResponse,
			WithCallConfigForceSync(true),
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItemFuncs(
				func() any { return isHttps },
				func() any { return u },
				getResponse, reject, drop,
			),
		)
	}
}

func (m *MixPluginCaller) CallHijackResponse(
	isHttps bool, u string, getResponse,
	reject, drop func() interface{},
) {
	m.CallHijackResponseWithCtx(context.Background(), isHttps, u, getResponse, reject, drop)
}

func (m *MixPluginCaller) CallHijackResponseExWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string, getRequest, getResponse,
	reject, drop func() interface{},
) {
	if !m.IsPassed(u) {
		log.Infof("call HijackResponseEx error: url[%v] not passed", u)
		return
	}
	callers := m.callers
	if callers.ShouldCallByName(HOOK_HijackHTTPResponseEx) {
		callers.Call(HOOK_HijackHTTPResponseEx,
			WithCallConfigForceSync(true),
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItemFuncs(
				func() any { return isHttps },
				func() any { return u },
				getRequest, getResponse, reject, drop,
			),
		)
	}
}

func (m *MixPluginCaller) CallHijackResponseEx(
	isHttps bool, u string, getRequest, getResponse,
	reject, drop func() interface{},
) {
	m.CallHijackResponseExWithCtx(context.Background(), isHttps, u, getRequest, getResponse, reject, drop)
}

func calcWebsitePathParamsHash(urlIns *url.URL, host, port interface{}, req []byte) string {
	freq, err := getFuzzHTTPRequestByCache(req)
	if err != nil {
		return ""
	}
	var params []string
	fuzzParams := freq.GetCommonParams()
	for _, r := range fuzzParams {
		params = append(params, utils.CalcMd5(r.Position(), r.Name()))
	}
	sort.Strings(params)
	return utils.CalcSha1(urlIns.Scheme, freq.GetMethod(), host, port, freq.GetPathWithoutQuery(), strings.Join(params, ","), "path-params")
}

func calcWebsitePathHash(urlIns *url.URL, host, port interface{}, req []byte) string {
	freq, err := getFuzzHTTPRequestByCache(req)
	if err != nil {
		return ""
	}
	return utils.CalcSha1(urlIns.Scheme, freq.GetMethod(), host, port, freq.GetPathWithoutQuery(), "path")
}

var ttlHTTPRequestCache = utils.NewTTLCache[*mutate.FuzzHTTPRequest](30 * time.Minute)

func getFuzzHTTPRequestByCache(req []byte) (*mutate.FuzzHTTPRequest, error) {
	hash := utils.CalcSha1(req)
	reqIns, ok := ttlHTTPRequestCache.Get(hash)
	if ok {
		return reqIns, nil
	}
	reqIns, err := mutate.NewFuzzHTTPRequest(req)
	if err != nil {
		return nil, err
	}
	ttlHTTPRequestCache.SetWithTTL(hash, reqIns, 30*time.Minute)
	return reqIns, nil
}

func calcNewWebsiteHash(urlIns *url.URL, host, port interface{}, req []byte) string {
	freq, err := getFuzzHTTPRequestByCache(req)
	if err != nil {
		return ""
	}
	return utils.CalcSha1(urlIns.Scheme, host, port, freq.GetMethod(), "new-website")
}

func (m *MixPluginCaller) GetFingerprintMatcher() *fp.Matcher {
	m.fingerprintMatcherOnce.Do(func() {
		m.fingerprintMatcher, _ = fp.NewDefaultFingerprintMatcher(fp.NewConfig(fp.WithDatabaseCache(true), fp.WithCache(true)))
	})
	return m.fingerprintMatcher
}

func (m *MixPluginCaller) HandleServiceScanResult(r *fp.MatchResult) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic from port-scan plugin: %s", err)
		}
	}()
	callers := m.callers
	wg := new(sync.WaitGroup)
	if callers.ShouldCallByName(HOOK_PortScanHandle) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("HandleServiceScanResult call HOOK_PortScanHandle failed: %v", err)
				}
			}()
			callers.CallByName(HOOK_PortScanHandle, r)
		}()
	}
	if callers.ShouldCallByName(HOOK_NucleiScanHandle) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("HandleServiceScanResult call HOOK_NucleiScanHandle failed: %v", err)
				}
			}()
			callers.CallByName(HOOK_NucleiScanHandle, utils.HostPort(r.Target, r.Port))
		}()
	}
	if callers.ShouldCallByName(HOOK_NaslScanHandle) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if err := recover(); err != nil {
					log.Errorf("HandleServiceScanResult call HOOK_NaslScanHandle failed: %v", err)
				}
			}()
			callers.CallByName(HOOK_NaslScanHandle, utils.HostPort(r.Target, r.Port))
		}()
	}
	wg.Wait()
}

func (m *MixPluginCaller) MirrorHTTPFlowWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool,
) {
	m.mirrorHTTPFlow(runtimeCtx, false, true, isHttps, u, req, rsp, body, filters...)
}

func (m *MixPluginCaller) MirrorHTTPFlow(
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool,
) {
	m.mirrorHTTPFlow(context.Background(), false, true, isHttps, u, req, rsp, body, filters...)
}

func (m *MixPluginCaller) MirrorHTTPFlowEx(
	scanPort bool,
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool,
) {
	m.mirrorHTTPFlow(context.Background(), false, scanPort, isHttps, u, req, rsp, body, filters...)
}

func (m *MixPluginCaller) MirrorHTTPFlowExSync(
	scanPort bool,
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool,
) {
	m.mirrorHTTPFlow(context.Background(), true, scanPort, isHttps, u, req, rsp, body, filters...)
}

func (m *MixPluginCaller) mirrorHTTPFlow(
	runtimeCtx context.Context,
	forceSync bool,
	scanPort bool,
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool,
) {
	if !m.IsPassed(u) {
		log.Infof("call MirrorHTTPFlow error: url[%v] not passed", u)
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic from mirror httpflow ex: %s", err)
		}
	}()
	callers := m.callers

	if !utils.IsHttpOrHttpsUrl(u) {
		host, port, _ := utils.ParseStringToHostPort(u)
		if host == "" {
			host = u
		}
		if port == 443 {
			u = fmt.Sprintf("https://%s", host)
		} else {
			u = fmt.Sprintf("http://%s", host)
		}
	}
	if callers.ShouldCallByName(HOOK_MirrorHTTPFlow) {
		callers.Call(HOOK_MirrorHTTPFlow,
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigForceSync(forceSync),
			WithCallConfigItems(isHttps, u, req, rsp, body),
		)
	}

	urlObj, err := url.Parse(u)
	if err != nil {
		yaklib.GetYakitClientInstance().YakitInfo("解析 URL 失败：%v 原因: %v", u, err)
	}
	if urlObj != nil {
		host, port, _ := utils.ParseStringToHostPort(u)
		websiteHash := calcNewWebsiteHash(urlObj, host, port, req)
		websitePathHash := calcWebsitePathHash(urlObj, host, port, req)
		websitePathParamsHash := calcWebsitePathParamsHash(urlObj, host, port, req)
		if !m.websiteFilter.Exist(websiteHash) {
			m.websiteFilter.Insert(websiteHash)
			if callers.ShouldCallByName(HOOK_MirrorNewWebsite) {
				callers.Call(HOOK_MirrorNewWebsite,
					WithCallConfigRuntimeCtx(runtimeCtx),
					WithCallConfigForceSync(forceSync),
					WithCallConfigItems(isHttps, u, req, rsp, body),
				)
			}

			if callers.ShouldCallByName(HOOK_PortScanHandle) {
				var (
					matchResult *fp.MatchResult = &fp.MatchResult{State: fp.OPEN}
					err         error
				)
				host, port, _ = utils.ParseStringToHostPort(u)
				if host != "" && port > 0 {
					m.swg.Add()
					go func() {
						defer m.swg.Done()
						if scanPort {
							addr := utils.HostPort(host, port)
							log.Debugf("(port/mitm) start to match %v", addr)
							matchResult, err = m.GetFingerprintMatcher().Match(
								host, port, fp.WithCache(m.cache), fp.WithDatabaseCache(true),
								fp.WithProxy(m.proxy),
							)
							if err != nil {
								return
							}
							log.Debugf("%v", matchResult.String())
						}
						callers.Call(HOOK_PortScanHandle,
							WithCallConfigRuntimeCtx(runtimeCtx),
							WithCallConfigForceSync(forceSync),
							WithCallConfigItems(matchResult),
						)
					}()
				}
			}

			// 异步+并发限制执行 Nuclei
			if callers.ShouldCallByName(HOOK_NucleiScanHandle) {
				m.swg.Add()
				go func() {
					defer m.swg.Done()
					callers.Call(HOOK_NucleiScanHandle,
						WithCallConfigRuntimeCtx(runtimeCtx),
						WithCallConfigForceSync(forceSync),
						WithCallConfigItems(urlObj.String()),
					)
				}()
			}
			if callers.ShouldCallByName(HOOK_NaslScanHandle) {
				m.swg.Add()
				go func() {
					defer m.swg.Done()
					callers.Call(HOOK_NaslScanHandle,
						WithCallConfigRuntimeCtx(runtimeCtx),
						WithCallConfigForceSync(forceSync),
						WithCallConfigItems(urlObj.String()),
					)
				}()
			}
		}

		if !m.websitePathFilter.Exist(websitePathHash) {
			m.websitePathFilter.Insert(websitePathHash)
			if callers.ShouldCallByName(HOOK_MirrorNewWebsitePath) {
				callers.Call(HOOK_MirrorNewWebsitePath,
					WithCallConfigRuntimeCtx(runtimeCtx),
					WithCallConfigForceSync(forceSync),
					WithCallConfigItems(isHttps, u, req, rsp, body),
				)
			}
		}

		if !m.websiteParamsFilter.Exist(websitePathParamsHash) {
			m.websiteParamsFilter.Insert(websitePathParamsHash)
			if callers.ShouldCallByName(HOOK_MirrorNewWebsitePathParams) {
				callers.Call(HOOK_MirrorNewWebsitePathParams,
					WithCallConfigRuntimeCtx(runtimeCtx),
					WithCallConfigForceSync(forceSync),
					WithCallConfigItems(isHttps, u, req, rsp, body),
				)
			}
		}
	}

	for _, i := range filters {
		if !i {
			return
		}
	}
	if callers.ShouldCallByName(HOOK_MirrorFilteredHTTPFlow) {
		callers.Call(HOOK_MirrorFilteredHTTPFlow,
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigForceSync(forceSync),
			WithCallConfigItems(isHttps, u, req, rsp, body),
		)
	}
}

func (m *MixPluginCaller) HijackSaveHTTPFlow(flow *schema.HTTPFlow, reject func(httpFlow *schema.HTTPFlow), drop func()) {
	if !m.IsPassed(flow.Url) {
		log.Infof("call HijackSaveHTTPFlow error: url[%v] not passed", flow.Url)
		return
	}
	if m.callers.ShouldCallByName(HOOK_hijackSaveHTTPFlow) {
		m.callers.CallByName(HOOK_hijackSaveHTTPFlow, flow, reject, drop)
	}
}

func (m *MixPluginCaller) HijackSaveHTTPFlowEx(runtimeCtx context.Context, flow *schema.HTTPFlow, callback func(), reject func(httpFlow *schema.HTTPFlow), drop func()) {
	if !m.IsPassed(flow.Url) {
		log.Infof("call HijackSaveHTTPFlow error: url[%v] not passed", flow.Url)
		return
	}
	if m.callers.ShouldCallByName(HOOK_hijackSaveHTTPFlow, callback) {
		m.callers.Call(
			HOOK_hijackSaveHTTPFlow,
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigCallback(callback),
			WithCallConfigItems(flow, reject, drop),
		)
	}
}

func (m *MixPluginCaller) GetNativeCaller() *YakToCallerManager {
	return m.callers
}
