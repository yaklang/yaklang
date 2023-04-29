package yak

import (
	"context"
	"fmt"
	"net/url"
	"yaklang/common/consts"
	"yaklang/common/filter"
	"yaklang/common/fp"
	"yaklang/common/log"
	"yaklang/common/mutate"
	"yaklang/common/utils"
	"yaklang/common/yak/httptpl"
	"yaklang/common/yak/yaklib"
	"yaklang/common/yakgrpc/yakit"
	"yaklang/common/yakgrpc/ypb"
	"sort"
	"strings"
	"sync"
	"time"
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

	// func hijackSaveHTTPFlow(record *httpFlow, forward func(*httpFlow), drop func()) return (*httpFlow)
	HOOK_hijackSaveHTTPFlow = "hijackSaveHTTPFlow"

	// func handle(r *fp.MatchResult)
	HOOK_PortScanHandle = "handle"

	// func execNuclei(target)
	HOOK_NucleiScanHandle = "execNuclei"

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
	HOOK_hijackSaveHTTPFlow,

	// port-scan
	HOOK_PortScanHandle,
}

var MixScanHooks = append(MITMAndPortScanHooks, HOOK_NucleiScanHandle)

type MixPluginCaller struct {
	websiteFilter       *filter.StringFilter
	websitePathFilter   *filter.StringFilter
	websiteParamsFilter *filter.StringFilter

	feedbackHandler    func(*ypb.ExecResult) error
	ordinaryFeedback   func(i interface{}, item ...interface{})
	callers            *YakToCallerManager
	fingerprintMatcher *fp.Matcher
	swg                utils.SizedWaitGroup
}

func (m *MixPluginCaller) ReplaceYakToCallerManager(c *YakToCallerManager) {
	resetFilterLock.Lock()
	defer resetFilterLock.Unlock()

	m.callers = c
}

var resetFilterLock = new(sync.Mutex)

var loadTemplateLock = new(sync.Mutex)

const nucleiCodeExecTemplate = `
// 这个脚本需要进行操作，设置 CURRENT_NUCLEI_PLUGIN_NAME 作为变量名
nucleiPoCName = MITM_PARAMS.CURRENT_NUCLEI_PLUGIN_NAME
// nucleiPoCName = "[thinkphp-5023-rce]: ThinkPHP 5.0.23 RCE" // MITM_PARAMS.CURRENT_NUCLEI_PLUGIN_NAME
script, err := db.GetYakitPluginByName(nucleiPoCName)
if err != nil {
	yakit.Error("load yakit-plugin(nuclei) failed: %s", err)
	return
}

script.LocalPath = str.TrimLeft(script.LocalPath, "/")
pocName = file.Join(nuclei.GetPoCDir(), script.LocalPath)
if pocName == "" || (!file.IsExisted(pocName)) {
	f, err := file.TempFile()
	if err != nil {
		yakit.Error("load tempfile to save nuclei poc failed: %s", err)
		return
	}
	pocName = f.Name()
    f.WriteString(script.Content)
    f.Close()	
}

proxy = cli.StringSlice("proxy")

execNuclei = func(target) {
    if len(proxy) > 0 {
        yakit.Info("PROXY: %v", proxy)
    } 
	yakit.Info("开始执行插件: %s [%v]", nucleiPoCName, target)
    
	res, err = nuclei.Scan(
        target, nuclei.templates(pocName),
        nuclei.retry(0), nuclei.stopAtFirstMatch(true), nuclei.timeout(10), 
        nuclei.proxy(proxy...),
    )
	if err != nil {
		yakit.Error("扫描[%v]失败: %s", target, err)
		return
	}
    yakit.Info("开始等待插件: %v 针对: %v 的返回结果", nucleiPoCName, target)
	for pocVul = range res {
		yakit.Output(pocVul)		
		yakit.Output(nuclei.PocVulToRisk(pocVul))		
	}
}
`

func (m *MixPluginCaller) SetFeedback(i func(i *ypb.ExecResult) error) {
	m.feedbackHandler = i
	m.ordinaryFeedback = FeedbackFactory(consts.GetGormProjectDatabase(), m.feedbackHandler, false, "")

}

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
	c := &MixPluginCaller{
		websiteFilter:       filter.NewFilter(),
		websitePathFilter:   filter.NewFilter(),
		websiteParamsFilter: filter.NewFilter(),
		callers:             NewYakToCallerManager(),
		feedbackHandler: func(result *ypb.ExecResult) error {
			return yaklib.GetYakitClientInstance().Output(result)
		},
	}
	c.SetLoadPluginTimeout(10)
	var err error
	c.fingerprintMatcher, err = fp.NewDefaultFingerprintMatcher(fp.NewConfig())
	if err != nil {
		return nil, utils.Errorf("create default fingerprint matcher failed: %s", err)
	}
	c.swg = utils.NewSizedWaitGroup(30)
	return c, nil
}

func (c *MixPluginCaller) SetLoadPluginTimeout(i float64) {
	c.callers.timeout = time.Duration(i * float64(time.Second))
}
func (c *MixPluginCaller) SetConcurrent(i int) error {
	return c.GetNativeCaller().SetConcurrent(i)
}

func (c *MixPluginCaller) Wait() {
	wg := new(sync.WaitGroup)
	wg.Add(1)
	go func() {
		defer wg.Done()

		log.Info("start to wait local mix caller...")
		c.swg.Wait()
		log.Infof("mix caller tasks all done")

		log.Infof("start to wait native caller concurrent")
		c.GetNativeCaller().Wait()
		log.Infof("native caller all done")
	}()
	wg.Wait()
}

func (c *MixPluginCaller) ResetFilter() {
	resetFilterLock.Lock()
	defer resetFilterLock.Unlock()
	c.websiteParamsFilter = filter.NewFilter()
	c.websitePathFilter = filter.NewFilter()
	c.websiteFilter = filter.NewFilter()
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
		//log.Info("Initial ordinary feedback")
		//c.SetFeedback(func(i *ypb.ExecResult) error {
		//	var l yaklib.YakitLog
		//	err := json.Unmarshal(i.Message, &l)
		//	if err != nil {
		//		log.Errorf("unmarshal yakit_output/save msg failed: %s", err)
		//		return err
		//	}
		//
		//	if l.Level != "" {
		//		client := yaklib.GetYakitClientInstance()
		//		if client != nil {
		//			return client.SendRaw(&l)
		//		}
		//	}
		//	return nil
		//})
	}

}

func (c *MixPluginCaller) LoadHotPatch(ctx context.Context, code string) error {
	c.ResetFilter()
	c.FeedbackOrdinary("Initializing HotPatched MITM HOOKS")
	err := c.callers.SetForYakit(ctx, code, YakitCallerIf(c.feedbackHandler), MITMAndPortScanHooks...)
	if err != nil {
		c.FeedbackOrdinary(fmt.Sprintf("Initialized HotPatched MITM HOOKS FAILED: %v", err.Error()))
		return err
	}
	return nil
}

func (m *MixPluginCaller) LoadPlugin(scriptName string, params ...*ypb.ExecParamItem) error {
	return m.LoadPluginByName(context.Background(), scriptName, params)
}

func (m *MixPluginCaller) LoadPluginByName(ctx context.Context, name string, params []*ypb.ExecParamItem, codes ...string) error {
	//loadTemplateLock.Lock()
	//defer loadTemplateLock.Unlock()

	m.FeedbackOrdinary(fmt.Sprintf("Initializing MITM Plugin: %v", name))
	var code string
	if len(codes) > 0 {
		code = codes[0]
	}

	var forNuclei bool
	var forPortScan bool
	var forMitm bool
	if code == "" {
		db := consts.GetGormProfileDatabase()
		if db == nil {
			return utils.Error("no database conn is set / no code set")
		}
		ins, err := yakit.GetYakScriptByName(db, name)
		if err != nil {
			return utils.Errorf("load plugin name (yakScript name: %v) failed: %s", name, err)
		}

		if ins.Type == "port-scan" {
			forPortScan = true
		}

		if ins.Type == "mitm" {
			forMitm = true
		}

		if ins.Type == "nuclei" {
			//var rawTemp templates.Template
			//_ = json.Unmarshal([]byte(ins.Content), &rawTemp)
			//if len(rawTemp.Workflow.Workflows) > 0 || len(rawTemp.Workflows) > 0 || rawTemp.CompiledWorkflow != nil {
			//	return utils.Errorf("cannot load nuclei workflow[%v]: not supported", name)
			//}
			_, err := httptpl.CreateYakTemplateFromNucleiTemplateRaw(ins.Content)
			if err != nil {
				return err
			}

			forNuclei = true
			params = append(params, &ypb.ExecParamItem{
				Key:   "CURRENT_NUCLEI_PLUGIN_NAME",
				Value: ins.ScriptName,
			})
			code = nucleiCodeExecTemplate
		} else {
			code = ins.Content
		}
	}

	if forNuclei {
		err := m.callers.AddForYakit(ctx, name, params, code, YakitCallerIf(m.feedbackHandler), HOOK_NucleiScanHandle)
		if err != nil {
			m.FeedbackOrdinary(fmt.Sprintf("Initailzed Nuclei Plugin[%v] Failed: %v", name, err))
			return nil
		}
		return nil
	}

	var hooks []string
	switch true {
	case forMitm:
		hooks = []string{HOOK_MirrorFilteredHTTPFlow, HOOK_MirrorHTTPFlow, HOOK_MirrorNewWebsite, HOOK_MirrorNewWebsitePath, HOOK_MirrorNewWebsitePathParams}
	case forPortScan:
		hooks = []string{HOOK_PortScanHandle}
	default:
		hooks = MITMAndPortScanHooks
	}

	err := m.callers.AddForYakit(ctx, name, params, code, YakitCallerIf(m.feedbackHandler), hooks...)
	if err != nil {
		m.FeedbackOrdinary(fmt.Sprintf("Initailzed MITM/ScanPort Plugin[%v] Failed: %v", name, err))
		return err
	}
	return nil
}

func (m *MixPluginCaller) CallHijackRequest(
	isHttps bool, u string, getRequest func() interface{},
	reject func() interface{},
	drop func() interface{},
) {
	m.callers.CallByNameExSync(
		HOOK_HijackHTTPRequest,
		func() interface{} {
			return isHttps
		},
		func() interface{} {
			return u
		},
		getRequest,
		reject, drop,
	)
}

func (m *MixPluginCaller) CallHijackResponse(
	isHttps bool, u string, getResponse,
	reject, drop func() interface{},
) {
	m.callers.CallByNameExSync(
		HOOK_HijackHTTPResponse,
		func() interface{} { return isHttps },
		func() interface{} { return u }, getResponse, reject, drop,
	)
}

func calcWebsitePathParamsHash(urlIns *url.URL, host, port interface{}, req []byte) string {
	freq, err := mutate.NewFuzzHTTPRequest(req)
	if err != nil {
		return ""
	}
	var params []string
	for _, r := range freq.GetCommonParams() {
		params = append(params, utils.CalcSha1(r.Name(), r.Position()))
	}
	sort.Strings(params)
	return utils.CalcSha1(urlIns.Scheme, host, port, strings.Join(params, ","), "path-params")
}

func (m *MixPluginCaller) HandleServiceScanResult(r *fp.MatchResult) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic from port-scan plugin: %s", err)
		}
	}()
	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("HandleServiceScanResult call HOOK_PortScanHandle failed: %v", err)
			}
		}()
		m.GetNativeCaller().CallByName(HOOK_PortScanHandle, r)
	}()
	go func() {
		defer wg.Done()
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("HandleServiceScanResult call HOOK_NucleiScanHandle failed: %v", err)
			}
		}()
		m.GetNativeCaller().CallByName(HOOK_NucleiScanHandle, utils.HostPort(r.Target, r.Port))
	}()
	wg.Wait()
}

func (m *MixPluginCaller) MirrorHTTPFlow(
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool) {
	m.MirrorHTTPFlowEx(true, isHttps, u, req, rsp, body, filters...)
}

func (m *MixPluginCaller) MirrorHTTPFlowEx(
	scanPort bool,
	isHttps bool, u string, req, rsp, body []byte,
	filters ...bool) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("panic from mirror httpflow ex: %s", err)
		}
	}()

	m.callers.CallByName(HOOK_MirrorHTTPFlow, isHttps, u, req, rsp, body)
	urlObj, err := url.Parse(u)
	if err != nil {
		yaklib.YakitInfo(yaklib.GetYakitClientInstance())("解析 URL 失败：%v 原因: %v", u, err)
	}
	if urlObj != nil {
		host, port, _ := utils.ParseStringToHostPort(u)
		websiteHash := utils.CalcSha1("website", urlObj.Scheme, host, port)
		websitePathHash := utils.CalcSha1("path", urlObj.Scheme, host, port, urlObj.EscapedPath())
		websitePathParamsHash := calcWebsitePathParamsHash(urlObj, host, port, req)
		if !m.websiteFilter.Exist(websiteHash) {
			m.websiteFilter.Insert(websiteHash)
			m.callers.CallByName(HOOK_MirrorNewWebsite, isHttps, u, req, rsp, body)

			if scanPort {
				host, port, _ = utils.ParseStringToHostPort(u)
				if host != "" && port > 0 {
					m.swg.Add()
					go func() {
						defer m.swg.Done()
						addr := utils.HostPort(host, port)
						log.Infof("(port/mitm) start to match %v", addr)
						matchResult, err := m.fingerprintMatcher.Match(host, port)
						if err != nil {
							return
						}
						log.Infof("%v", matchResult.String())
						m.callers.CallByName(HOOK_PortScanHandle, matchResult)
					}()
				}
			}

			// 异步+并发限制执行 Nuclei
			m.swg.Add()
			go func() {
				defer m.swg.Done()
				m.callers.CallByName(HOOK_NucleiScanHandle, urlObj.String())
			}()
		}

		if !m.websitePathFilter.Exist(websitePathHash) {
			m.websitePathFilter.Insert(websitePathHash)
			m.callers.CallByName(HOOK_MirrorNewWebsitePath, isHttps, u, req, rsp, body)
		}

		if !m.websiteParamsFilter.Exist(websitePathParamsHash) {
			m.websiteParamsFilter.Insert(websitePathParamsHash)
			m.callers.CallByName(HOOK_MirrorNewWebsitePathParams, isHttps, u, req, rsp, body)
		}
	}

	for _, i := range filters {
		if !i {
			return
		}
	}
	m.callers.CallByName(HOOK_MirrorFilteredHTTPFlow, isHttps, u, req, rsp, body)
}

func (m *MixPluginCaller) HijackSaveHTTPFlow(flow *yakit.HTTPFlow, reject func(httpFlow *yakit.HTTPFlow), drop func()) {
	m.callers.CallByName(HOOK_hijackSaveHTTPFlow, flow, reject, drop)
}

func (m *MixPluginCaller) GetNativeCaller() *YakToCallerManager {
	return m.callers
}
