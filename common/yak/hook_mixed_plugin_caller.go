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
	"github.com/yaklang/yaklang/common/utils/lowhttp"

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

var defaultInvalidSuffix = []string{
	".js",
	".css",
	".xml",
	".jpg", ".jpeg", ".png",
	".mp3", ".mp4", ".ico", ".bmp",
	".flv", ".aac", ".ogg", ".avi",
	".svg", ".gif", ".woff", ".woff2",
	".doc", ".docx", ".pptx",
	".ppt", ".pdf",
	".swf",
	".json",
}

var staticContentTypes = []string{
	// JavaScript
	"text/javascript",
	"application/javascript",
	"application/x-javascript",
	"application/ecmascript",

	// CSS
	"text/css",

	// Images
	"image/jpeg",
	"image/jpg",
	"image/png",
	"image/gif",
	"image/webp",
	"image/svg+xml",
	"image/bmp",
	"image/x-icon", // ICO 文件
	"image/vnd.microsoft.icon",

	// Fonts
	"font/woff",
	"font/woff2",
	"font/ttf",
	"font/otf",
	"application/font-woff",
	"application/font-woff2",
	"application/x-font-ttf",
	"application/x-font-otf",
	"font/opentype",

	// Media (Video & Audio)
	"video/mp4",
	"video/webm",
	"video/ogg",
	"video/x-msvideo",  // AVI
	"video/x-matroska", // MKV
	"audio/mpeg",
	"audio/ogg",
	"audio/wav",
	"audio/webm",
	"audio/x-wav",
}

var dynamicContentTypes = []string{
	// JSON data formats
	"application/json",         // JSON data, usually API responses
	"application/vnd.api+json", // JSON:API format
	"application/hal+json",     // HAL format
	"application/stream+json",  // Streaming JSON

	// XML data formats
	"application/xml", // XML data
	"text/xml",        // Alternative XML format

	// Form and submission data
	"application/x-www-form-urlencoded", // Form submission content type
	"multipart/form-data",               // Form file upload content type

	// Binary and stream data
	"application/octet-stream", // Binary stream, usually file downloads, dynamically generated
	"application/x-protobuf",   // Protobuf binary data
	"application/grpc",         // gRPC communication content type
}

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

	// func mockHTTPRequest(isHttps, url, request, mockResponse /*func(rsp interface{})*/)
	HOOK_MockHTTPRequest = "mockHTTPRequest"

	// func hijackSaveHTTPFlow(record *httpFlow, forward func(*httpFlow), drop func()) return (*httpFlow)
	HOOK_hijackSaveHTTPFlow = "hijackSaveHTTPFlow"

	// func handle(r *fp.MatchResult)
	HOOK_PortScanHandle = "handle"

	// func execNuclei(target)
	HOOK_NucleiScanHandle = "execNuclei"

	HOOK_NaslScanHandle           = "execNasl"
	HOOK_LoadNaslScriptByNameFunc = "loadNaslScriptByNameFunc"

	// beforeRequest, afterRequest
	HOOK_BeforeRequest = "beforeRequest"
	HOOK_AfterRequest  = "afterRequest"

	HOOK_CLAER = "clear"

	// httpflow analyze
	HOOK_Analyze_HTTPFlow        = "analyzeHTTPFlow"
	HOOK_OnAnalyzeHTTPFlowFinish = "onAnalyzeHTTPFlowFinish"

	/*
		hijackSaveHTTPFlow = func(flow, forward, drop) {
		    println(flow.Url)
		    flow.Red()
		    forward(flow)
		}
	*/
)

var (
	MITMAndPortScanHooks = []string{
		HOOK_MirrorFilteredHTTPFlow,
		HOOK_MirrorHTTPFlow,
		HOOK_MirrorNewWebsite,
		HOOK_MirrorNewWebsitePath,
		HOOK_MirrorNewWebsitePathParams,
		HOOK_CLAER,

		HOOK_HijackHTTPRequest,
		HOOK_HijackHTTPResponse,
		HOOK_HijackHTTPResponseEx,
		HOOK_MockHTTPRequest,
		HOOK_hijackSaveHTTPFlow,

		// port-scan
		HOOK_PortScanHandle,

		// beforeRequest, afterRequest
		HOOK_BeforeRequest,
		HOOK_AfterRequest,
		// httpFlow analyze
		HOOK_Analyze_HTTPFlow,
		HOOK_OnAnalyzeHTTPFlowFinish,
	}
	HotPatchScriptName = "@HotPatchCode"
)

type MixPluginCaller struct {
	ctx context.Context // 整个 mix plugin caller 的上下文

	websiteFilter       filter.Filterable
	websitePathFilter   filter.Filterable
	websiteParamsFilter filter.Filterable
	targetFilter        filter.Filterable
	rawQuestFilter      filter.Filterable

	runtimeId string
	proxy     []string
	extraVars map[string]any // 额外的变量传递给插件

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

func (m *MixPluginCaller) SetVar(key string, value any) {
	if m == nil {
		return
	}
	if m.extraVars == nil {
		m.extraVars = make(map[string]any)
	}
	m.extraVars[key] = value
}

func (m *MixPluginCaller) SetProxy(s ...string) {
	if s == nil || len(s) == 0 {
		return
	}
	if m == nil {
		return
	}
	m.proxy = s
	if m.callers != nil {
		m.callers.proxy = strings.Join(s, ",")
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
		targetFilter:        webFilter,
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
		targetFilter:        webFilter,
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

// SetLongRunningThreshold 设置插件长时间运行阈值（秒）
func (c *MixPluginCaller) SetLongRunningThreshold(seconds int) {
	if c.callers != nil {
		c.callers.SetLongRunningThreshold(seconds)
	}
}

// GetLongRunningThreshold 获取插件长时间运行阈值（秒）
func (c *MixPluginCaller) GetLongRunningThreshold() int {
	if c.callers != nil {
		return c.callers.GetLongRunningThreshold()
	}
	return consts.PluginCallDurationThresholdSeconds
}

func (c *MixPluginCaller) Wait() {
	defer c.websiteFilter.Close()

	waitChan := make(chan struct{})

	go func() {
		defer func() {
			close(waitChan)
			if r := recover(); r != nil {
				if errMsg := utils.InterfaceToString(r); errMsg != "" {
					log.Error(errMsg)
				}
			}
		}()

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
	c.targetFilter = webFilter
	c.websiteFilter = webFilter
}

func (c *MixPluginCaller) IsPassed(target string) bool {
	if c.pluginScanFilter == nil {
		return true
	}
	f := c.pluginScanFilter
	return utils.IncludeExcludeChecker(f.IncludePluginScanURIs, f.ExcludePluginScanURIs, target)
}

func (c *MixPluginCaller) IsStatic(rawUrl string, req, rsp []byte) bool {
	for _, d := range defaultInvalidSuffix {
		if strings.HasSuffix(strings.Split(strings.ToLower(rawUrl), "?")[0], d) {
			return true
		}
	}

	contentTypeReq := lowhttp.GetHTTPPacketHeader(req, "Content-Type")
	contentTypeRsp := lowhttp.GetHTTPPacketHeader(rsp, "Content-Type")
	// acceptHeader := lowhttp.GetHTTPPacketHeader(request, "Accept")

	// hasDynamicType := false
	// hasStaticType := false
	// if acceptHeader != "" {
	// 	contentTypes := lowhttp.SplitContentTypesFromAcceptHeader(acceptHeader)
	// 	if len(contentTypes) > 0 {
	// 		for _, ct := range contentTypes {
	// 			if isStaticContentType(ct) {
	// 				hasStaticType = true
	// 			}
	// 			if isDynamicContentType(ct) {
	// 				hasDynamicType = true
	// 			}
	// 		}
	// 	}
	// 	if hasDynamicType && hasStaticType {
	// 		return false
	// 	}
	// 	if hasStaticType && !hasDynamicType {
	// 		return true
	// 	}
	// }

	check := func(contentType string) bool {
		if contentType != "" {
			if isDynamicContentType(contentType) {
				return false
			}
			if isStaticContentType(contentType) {
				return true
			}
		}
		return false
	}

	if check(contentTypeReq) {
		return true
	}
	if check(contentTypeRsp) {
		return true
	}

	return false
}

func isDynamicContentType(contentType string) bool {
	for _, dynamicType := range dynamicContentTypes {
		if strings.Contains(contentType, dynamicType) {
			return true
		}
	}
	return false
}

func isStaticContentType(contentType string) bool {
	for _, staticType := range staticContentTypes {
		if strings.Contains(contentType, staticType) {
			return true
		}
	}
	return false
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
	c.callers.Remove(&ypb.RemoveHookParams{
		HookName:     MITMAndPortScanHooks,
		RemoveHookID: []string{HotPatchScriptName},
	})

	err := c.callers.AddForYakit(ctx, &schema.YakScript{
		ScriptName: HotPatchScriptName,
	}, paramsMap, code, YakitCallerIf(c.feedbackHandler), MITMAndPortScanHooks...)
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

	// 添加额外的变量
	if m.extraVars != nil {
		for k, v := range m.extraVars {
			paramMap[k] = v
		}
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

func (m *MixPluginCaller) CallBeforeRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	originReq, req []byte,
) []byte {
	if !m.IsPassed(u) {
		log.Infof("call HijackRequest error: url[%v] not passed", u)
		return nil
	}
	callers := m.callers
	if callers.ShouldCallByName(HOOK_BeforeRequest) {
		results := callers.Call(
			HOOK_BeforeRequest,
			WithCallConfigForceSync(true),
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItems(
				isHttps, originReq, req,
			),
		)
		if len(results) > 0 {
			return utils.InterfaceToBytes(results[0])
		}
	}
	return nil
}

func (m *MixPluginCaller) CallAfterRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string,
	originReq, req []byte,
	originRsp, rsp []byte,
) []byte {
	if !m.IsPassed(u) {
		log.Infof("call HijackRequest error: url[%v] not passed", u)
		return nil
	}
	callers := m.callers
	if callers.ShouldCallByName(HOOK_AfterRequest) {
		results := callers.Call(
			HOOK_AfterRequest,
			WithCallConfigForceSync(true),
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItems(
				isHttps, originReq, req, originRsp, rsp,
			),
		)
		if len(results) > 0 {
			return utils.InterfaceToBytes(results[0])
		}
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

func (m *MixPluginCaller) CallMockHTTPRequestWithCtx(
	runtimeCtx context.Context,
	isHttps bool, u string, getRequest func() interface{},
	mockResponse func(rsp interface{}),
) {
	if !m.IsPassed(u) {
		log.Infof("call MockHTTPRequest error: url[%v] not passed", u)
		return
	}
	callers := m.callers
	if callers.ShouldCallByName(HOOK_MockHTTPRequest) {
		callers.Call(
			HOOK_MockHTTPRequest,
			WithCallConfigForceSync(true),
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItemFuncs(
				func() any { return isHttps },
				func() any { return u },
				getRequest,
				func() any { return mockResponse },
			),
		)
	}
}

func (m *MixPluginCaller) CallMockHTTPRequest(
	isHttps bool, u string, getRequest func() interface{},
	mockResponse func(rsp interface{}),
) {
	m.CallMockHTTPRequestWithCtx(context.Background(), isHttps, u, getRequest, mockResponse)
}

func (m *MixPluginCaller) CallAnalyzeHTTPFlow(
	runtimeCtx context.Context,
	flow *schema.HTTPFlow,
	extract func(ruleName string, httpFlow *schema.HTTPFlow, content ...string),
) {
	if m.callers.ShouldCallByName(HOOK_Analyze_HTTPFlow) {
		m.callers.Call(
			HOOK_Analyze_HTTPFlow,
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItems(flow, extract),
		)
	}
}

func (m *MixPluginCaller) CallOnAnalyzeHTTPFlowFinish(
	runtimeCtx context.Context,
	totalCount int64,
	matchedCount int64,
) {
	if m.callers.ShouldCallByName(HOOK_OnAnalyzeHTTPFlowFinish) {
		m.callers.Call(
			HOOK_OnAnalyzeHTTPFlowFinish,
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigItems(totalCount, matchedCount),
		)
	}
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
		targetHash := calcTargetHash(host, port)
		if !m.targetFilter.Exist(targetHash) {
			m.targetFilter.Insert(targetHash)
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
								fp.WithProxy(m.proxy...),
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
		}
		if !m.websiteFilter.Exist(websiteHash) {
			m.websiteFilter.Insert(websiteHash)
			if callers.ShouldCallByName(HOOK_MirrorNewWebsite) {
				callers.Call(HOOK_MirrorNewWebsite,
					WithCallConfigRuntimeCtx(runtimeCtx),
					WithCallConfigForceSync(forceSync),
					WithCallConfigItems(isHttps, u, req, rsp, body),
				)
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
		if m.IsStatic(u, req, rsp) {
			return
		}
		callers.Call(HOOK_MirrorFilteredHTTPFlow,
			WithCallConfigRuntimeCtx(runtimeCtx),
			WithCallConfigForceSync(forceSync),
			WithCallConfigItems(isHttps, u, req, rsp, body),
		)
	}
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

func calcTargetHash(host, port interface{}) string {
	return utils.CalcSha1(host, port)
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

// EnableExecutionTracing 启用插件执行跟踪
func (m *MixPluginCaller) EnableExecutionTracing(enable bool) {
	if m.callers != nil {
		m.callers.EnableExecutionTracing(enable)
	}
}

// IsExecutionTracingEnabled 检查是否启用了插件执行跟踪
func (m *MixPluginCaller) IsExecutionTracingEnabled() bool {
	if m.callers != nil {
		return m.callers.IsExecutionTracingEnabled()
	}
	return false
}

// GetExecutionTracker 获取执行跟踪器
func (m *MixPluginCaller) GetExecutionTracker() *PluginExecutionTracker {
	if m.callers != nil {
		return m.callers.GetExecutionTracker()
	}
	return nil
}

// AddExecutionTraceCallback 添加执行跟踪回调
func (m *MixPluginCaller) AddExecutionTraceCallback(callback func(*PluginExecutionTrace)) (callbackID string, remove func()) {
	if m.callers != nil {
		return m.callers.AddExecutionTraceCallback(callback)
	}
	return "", nil
}

// GetAllExecutionTraces 获取所有执行跟踪
func (m *MixPluginCaller) GetAllExecutionTraces() []*PluginExecutionTrace {
	if m.callers != nil {
		return m.callers.GetAllExecutionTraces()
	}
	return nil
}

// GetExecutionTracesByPlugin 根据插件ID获取执行跟踪
func (m *MixPluginCaller) GetExecutionTracesByPlugin(pluginID string) []*PluginExecutionTrace {
	if m.callers != nil {
		return m.callers.GetExecutionTracesByPlugin(pluginID)
	}
	return nil
}

// GetExecutionTracesByHook 根据Hook名获取执行跟踪
func (m *MixPluginCaller) GetExecutionTracesByHook(hookName string) []*PluginExecutionTrace {
	if m.callers != nil {
		return m.callers.GetExecutionTracesByHook(hookName)
	}
	return nil
}

// GetRunningExecutionTraces 获取正在运行的执行跟踪
func (m *MixPluginCaller) GetRunningExecutionTraces() []*PluginExecutionTrace {
	if m.callers != nil {
		return m.callers.GetRunningExecutionTraces()
	}
	return nil
}

// CancelExecutionTrace 取消指定的执行跟踪
func (m *MixPluginCaller) CancelExecutionTrace(traceID string) bool {
	if m.callers != nil {
		return m.callers.CancelExecutionTrace(traceID)
	}
	return false
}

// CancelAllExecutionTraces 取消所有执行跟踪
func (m *MixPluginCaller) CancelAllExecutionTraces() {
	if m.callers != nil {
		m.callers.CancelAllExecutionTraces()
	}
}

// CleanupCompletedExecutionTraces 清理已完成的执行跟踪
func (m *MixPluginCaller) CleanupCompletedExecutionTraces(olderThan time.Duration) {
	if m.callers != nil {
		m.callers.CleanupCompletedExecutionTraces(olderThan)
	}
}

func (m *MixPluginCaller) HaveTheHookFunc(name string) bool {
	if m == nil {
		return false
	}
	return m.callers.ShouldCallByName(name)
}
