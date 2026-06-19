package httptpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

type (
	ResultCallback     func(y *YakTemplate, reqBulk any /**YakRequestBulkConfig / YakNetworkBulkConfig*/, rsp any /*[]*lowhttp.LowhttpResponse / [][]byte*/, result bool, extractor map[string]interface{})
	HTTPResultCallback func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{})
	TCPResultCallback  func(y *YakTemplate, reqBulk *YakNetworkBulkConfig, rsp []*NucleiTcpResponse, result bool, extractor map[string]interface{})
)

func HTTPResultCallbackWrapper(callback HTTPResultCallback) ResultCallback {
	return func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		bulk, ok := reqBulk.(*YakRequestBulkConfig)
		if !ok {
			return
		}

		results, ok := rsp.([]*lowhttp.LowhttpResponse)
		if !ok {
			return
		}

		callback(y, bulk, results, result, extractor)
	}
}

func TCPResultCallbackWrapper(callback TCPResultCallback) ResultCallback {
	return func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		bulk, ok := reqBulk.(*YakNetworkBulkConfig)
		if !ok {
			return
		}

		results, ok := rsp.([]*NucleiTcpResponse)
		if !ok {
			return
		}

		callback(y, bulk, results, result, extractor)
	}
}

type ConfigOption func(*Config)

type Config struct {
	// Templates 内部 HTTP 网络并发
	ConcurrentInTemplates int
	// Templates 外部 HTTP 网络并发
	ConcurrentTemplates int
	// ConcurrentTarget 批量扫描的并发
	ConcurrentTarget int

	Callback ResultCallback

	// runtime id for match task
	RuntimeId string
	Ctx       context.Context

	// nuclei / xray
	Mode string

	EnableReverseConnectionFeature bool

	// 搜索 yakit.YakScript
	SingleTemplateRaw      string
	ExactTemplateInstances []*schema.YakScript
	TemplateName           []string
	FuzzQueryTemplate      []string
	ExcludeTemplates       []string
	Tags                   []string
	QueryAll               bool

	// DebugMode
	Debug         bool
	DebugRequest  bool
	DebugResponse bool

	Verbose bool

	OOBTimeout                float64
	OOBRequireCallback        func(...float64) (string, string, error)
	OOBRequireCheckingTrigger func(string, string, ...float64) (string, []byte)

	CustomVariables map[string]any

	// onTempalteLoaded
	OnTemplateLoaded  func(*YakTemplate) bool
	BeforeSendPackage func(data []byte, isHttps bool) []byte
	defaultFilter     filter.Filterable

	mockHTTPRequest func(isHttps bool, urlStr string, req []byte, mockResponse func(rsp interface{}))
}

// mockHTTPRequest 设置一个自定义的 HTTP 请求模拟函数，用于在不发起真实请求的情况下测试模板
// 参数:
//   - f: 模拟请求处理函数，接收是否 HTTPS、URL、原始请求与设置响应的回调
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用自定义请求模拟函数
// res, err = nuclei.Scan("http://example.com", nuclei.mockHTTPRequest(func(isHttps, url, req, setRsp) { setRsp("HTTP/1.1 200 OK\r\n\r\n") }))
// die(err)
// ```
func WithMockHTTPRequest(f func(isHttps bool, urlStr string, req []byte, mockResponse func(rsp interface{}))) ConfigOption {
	return func(config *Config) {
		config.mockHTTPRequest = f
	}
}

// customVulnFilter 设置一个自定义的漏洞去重过滤器，用于控制扫描结果的去重逻辑
// 参数:
//   - f: 实现了 Filterable 接口的过滤器
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置自定义漏洞过滤器
// res, err = nuclei.Scan("http://example.com", nuclei.customVulnFilter(filter.NewFilter()))
// die(err)
// ```
func WithCustomVulnFilter(f filter.Filterable) ConfigOption {
	return func(config *Config) {
		if f == nil {
			return
		}
		config.defaultFilter = f
	}
}

func WithOOBRequireCallback(f func(...float64) (string, string, error)) ConfigOption {
	return func(config *Config) {
		config.OOBRequireCallback = f
	}
}

// context 设置 nuclei 扫描使用的 context，用于取消或超时控制
// 参数:
//   - c: 上下文对象
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用可取消的 context 控制扫描
// ctx, cancel = context.WithCancel(context.Background())
// defer cancel()
// res, err = nuclei.Scan("http://example.com", nuclei.context(ctx))
// die(err)
// ```
func WithContext(c context.Context) ConfigOption {
	return func(config *Config) {
		config.Ctx = c
	}
}

func WithOOBRequireCheckingTrigger(f func(string, string, ...float64) (string, []byte)) ConfigOption {
	return func(config *Config) {
		config.OOBRequireCheckingTrigger = f
	}
}

// debug 设置是否开启调试模式，开启后会同时打印请求与响应报文
// 参数:
//   - b: 是否开启调试模式
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：开启调试模式
// res, err = nuclei.Scan("http://example.com", nuclei.debug(true))
// die(err)
// ```
func WithDebug(b bool) ConfigOption {
	return func(config *Config) {
		config.Debug = b
		config.DebugResponse = b
		config.DebugRequest = b
	}
}

// interactshTimeout 设置反连(OOB)平台等待回连结果的超时时间
// 参数:
//   - f: 超时时间，单位为秒
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置反连等待超时
// res, err = nuclei.Scan("http://example.com", nuclei.enableReverseConnection(true), nuclei.interactshTimeout(10))
// die(err)
// ```
func WithOOBTimeout(f float64) ConfigOption {
	return func(config *Config) {
		config.OOBTimeout = f
	}
}

// verbose 设置是否开启详细日志输出，打印每个模板的执行过程
// 参数:
//   - b: 是否开启详细日志
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：开启详细日志
// res, err = nuclei.Scan("http://example.com", nuclei.verbose(true))
// die(err)
// ```
func WithVerbose(b bool) ConfigOption {
	return func(config *Config) {
		config.Verbose = b
	}
}

func WithCustomVariables(vars map[string]any) ConfigOption {
	return func(config *Config) {
		if len(vars) == 0 {
			return
		}
		if config.CustomVariables == nil {
			config.CustomVariables = make(map[string]any, len(vars))
		}
		for k, v := range vars {
			config.CustomVariables[k] = v
		}
	}
}

// vars 设置 nuclei 扫描时使用的自定义变量，会注入到模板渲染过程中
// 参数:
//   - raw: 自定义变量，通常为 map 结构
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：注入自定义变量
// res, err = nuclei.Scan("http://example.com", nuclei.vars({"username": "admin"}))
// die(err)
// ```
func withCustomVariablesFromInterface(raw interface{}) ConfigOption {
	if raw == nil {
		return func(*Config) {}
	}
	m := utils.InterfaceToMapInterface(raw)
	if len(m) == 0 {
		return func(*Config) {}
	}
	copied := make(map[string]any, len(m))
	for k, v := range m {
		copied[k] = v
	}
	return WithCustomVariables(copied)
}

// debugRequest 设置是否打印调试请求报文
// 参数:
//   - b: 是否打印请求报文
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：打印请求报文
// res, err = nuclei.Scan("http://example.com", nuclei.debugRequest(true))
// die(err)
// ```
func WithDebugRequest(b bool) ConfigOption {
	return func(config *Config) {
		config.DebugRequest = b
		config.Debug = b
	}
}

// debugResponse 设置是否打印调试响应报文
// 参数:
//   - b: 是否打印响应报文
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：打印响应报文
// res, err = nuclei.Scan("http://example.com", nuclei.debugResponse(true))
// die(err)
// ```
func WithDebugResponse(b bool) ConfigOption {
	return func(config *Config) {
		config.Debug = b
		config.DebugResponse = b
	}
}

// tags 设置仅运行带有指定标签的模板，可传入一个或多个标签
// 参数:
//   - f: 一个或多个模板标签，例如 cve、rce
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：按标签筛选模板
// res, err = nuclei.Scan("http://example.com", nuclei.tags("cve", "rce"))
// die(err)
// ```
func WithTags(f ...string) ConfigOption {
	return func(config *Config) {
		config.Tags = f
	}
}

// targetConcurrent 设置同时扫描的目标并发数
// 参数:
//   - i: 目标并发数
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置目标并发
// res, err = nuclei.Scan("http://example.com", nuclei.targetConcurrent(10))
// die(err)
// ```
func WithConcurrentTarget(i int) ConfigOption {
	return func(config *Config) {
		config.ConcurrentTarget = i
	}
}

// enableReverseConnection 设置是否启用反连(OOB)检测功能，用于检测无回显漏洞
// 参数:
//   - b: 是否启用反连检测
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：开启反连检测
// res, err = nuclei.Scan("http://example.com", nuclei.enableReverseConnection(true))
// die(err)
// ```
func WithEnableReverseConnectionFeature(b bool) ConfigOption {
	return func(config *Config) {
		config.EnableReverseConnectionFeature = b
	}
}

// rawTemplate 设置直接使用传入的单个 nuclei 模板原始内容进行扫描
// 参数:
//   - b: nuclei 模板的原始字符串内容
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用原始模板内容扫描
// res, err = nuclei.Scan("http://example.com", nuclei.rawTemplate(templateContent))
// die(err)
// ```
func WithTemplateRaw(b string) ConfigOption {
	return func(config *Config) {
		config.SingleTemplateRaw = b
	}
}

// templates 设置按名称选择要运行的模板，可传入一个或多个模板名称
// 参数:
//   - s: 一个或多个模板名称
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：按名称选择模板
// res, err = nuclei.Scan("http://example.com", nuclei.templates("template-name-1", "template-name-2"))
// die(err)
// ```
func WithTemplateName(s ...string) ConfigOption {
	return func(config *Config) {
		config.TemplateName = s
	}
}

// exactTemplateIns 设置使用一个精确的 YakScript 模板实例进行扫描
// 参数:
//   - script: YakScript 模板实例
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用精确模板实例
// res, err = nuclei.Scan("http://example.com", nuclei.exactTemplateIns(scriptIns))
// die(err)
// ```
func WithExactTemplateInstance(script *schema.YakScript) ConfigOption {
	return func(config *Config) {
		config.ExactTemplateInstances = append(config.ExactTemplateInstances, script)
	}
}

// fuzzQueryTemplate 设置按关键词模糊查询并选择匹配的模板
// 参数:
//   - s: 一个或多个用于模糊查询模板的关键词
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：按关键词模糊匹配模板
// res, err = nuclei.Scan("http://example.com", nuclei.fuzzQueryTemplate("weblogic"))
// die(err)
// ```
func WithFuzzQueryTemplate(s ...string) ConfigOption {
	return func(config *Config) {
		config.FuzzQueryTemplate = s
	}
}

// all 设置是否使用全部本地模板进行扫描
// 参数:
//   - b: 是否使用全部模板
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用全部模板扫描
// res, err = nuclei.Scan("http://example.com", nuclei.all(true))
// die(err)
// ```
func WithAllTemplate(b bool) ConfigOption {
	return func(config *Config) {
		config.QueryAll = b
	}
}

func WithOnTemplateLoaded(f func(template *YakTemplate) bool) ConfigOption {
	return func(config *Config) {
		config.OnTemplateLoaded = f
	}
}

// excludeTemplates 设置扫描时需要排除的模板名称，可传入一个或多个
// 参数:
//   - s: 一个或多个需要排除的模板名称
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：排除指定模板
// res, err = nuclei.Scan("http://example.com", nuclei.all(true), nuclei.excludeTemplates("noisy-template"))
// die(err)
// ```
func WithExcludeTemplates(s ...string) ConfigOption {
	return func(config *Config) {
		config.ExcludeTemplates = s
	}
}

// templatesThreads 设置单个模板内部的执行并发数
// 参数:
//   - i: 单个模板内部的并发数
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置模板内部并发
// res, err = nuclei.Scan("http://example.com", nuclei.templatesThreads(10))
// die(err)
// ```
func WithConcurrentInTemplates(i int) ConfigOption {
	return func(config *Config) {
		config.ConcurrentInTemplates = i
	}
}

// bulkSize 设置同时执行的模板并发数
// 参数:
//   - i: 同时执行的模板数量
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置模板并发数
// res, err = nuclei.Scan("http://example.com", nuclei.bulkSize(20))
// die(err)
// ```
func WithConcurrentTemplates(i int) ConfigOption {
	return func(config *Config) {
		config.ConcurrentTemplates = i
	}
}

// mode 设置扫描模式，目前主要支持 nuclei 模式
// 参数:
//   - s: 扫描模式字符串，例如 "nuclei"
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置扫描模式
// res, err = nuclei.Scan("http://example.com", nuclei.mode("nuclei"))
// die(err)
// ```
func WithMode(s string) ConfigOption {
	return func(config *Config) {
		config.Mode = s
	}
}

// runtimeId 设置本次 nuclei 扫描的运行时 ID，用于关联扫描任务与结果
// 参数:
//   - id: 运行时 ID 字符串
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置运行时 ID
// res, err = nuclei.Scan("http://example.com", nuclei.runtimeId("task-001"))
// die(err)
// ```
func WithHttpTplRuntimeId(id string) ConfigOption {
	return func(config *Config) {
		config.RuntimeId = id
	}
}

// resultCallback 设置 HTTP 模板命中时的结果回调函数
// 参数:
//   - handler: 回调函数，入参为包含 template、requests、responses、match 等键的结果字典
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置结果回调
// res, err = nuclei.Scan("http://example.com", nuclei.resultCallback(func(i) { println(i["match"]) }))
// die(err)
// ```
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

// tcpResultCallback 设置 TCP 模板命中时的结果回调函数
// 参数:
//   - handler: 回调函数，入参为包含 template、requests、responses、match 等键的结果字典
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 TCP 结果回调
// res, err = nuclei.Scan("tcp://example.com:8080", nuclei.tcpResultCallback(func(i) { println(i["match"]) }))
// die(err)
// ```
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

// noInteractsh 设置是否禁用 interactsh 反连检测，传入 true 表示禁用
// 参数:
//   - b: 是否禁用 interactsh 反连检测
//
// 返回值:
//   - 一个 nuclei.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：禁用反连检测
// res, err = nuclei.Scan("http://example.com", nuclei.noInteractsh(true))
// die(err)
// ```
func noInteractsh(b bool) ConfigOption {
	return WithEnableReverseConnectionFeature(!b)
}

func WithBeforeSendPackage(f func(data []byte, isHttps bool) []byte) ConfigOption {
	return func(config *Config) {
		config.BeforeSendPackage = f
	}
}

func WithResultCallback(f HTTPResultCallback) ConfigOption {
	return func(config *Config) {
		if config.Callback != nil {
			originCallback := config.Callback
			config.Callback = func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("httptpl execute result callback failed: %v", err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				originCallback(y, reqBulk, rsp, result, extractor)
				HTTPResultCallbackWrapper(f)(y, reqBulk, rsp, result, extractor)
			}
		} else {
			config.Callback = HTTPResultCallbackWrapper(f)
		}
	}
}

func WithTCPResultCallback(f TCPResultCallback) ConfigOption {
	return func(config *Config) {
		if config.Callback != nil {
			originCallback := config.Callback
			config.Callback = func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
				defer func() {
					if err := recover(); err != nil {
						log.Errorf("(WithCallback) httptpl execute result callback failed: %v", err)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				originCallback(y, reqBulk, rsp, result, extractor)
				TCPResultCallbackWrapper(f)(y, reqBulk, rsp, result, extractor)
			}
		} else {
			config.Callback = TCPResultCallbackWrapper(f)
		}
	}
}

func (c *Config) ExecuteResultCallback(y *YakTemplate, bulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
	if c == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("httptpl execute result callback failed: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	if c.Callback != nil {
		c.Callback(y, bulk, rsp, result, extractor)
	}
}

func (c *Config) ExecuteTCPResultCallback(y *YakTemplate, bulk *YakNetworkBulkConfig, rsp []*NucleiTcpResponse, result bool, extractor map[string]interface{}) {
	if c == nil {
		return
	}
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("httptpl execute result callback failed: %v", err)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()
	if c.Callback != nil {
		c.Callback(y, bulk, rsp, result, extractor)
	}
}

// NewConfig 创建一个默认的配置
var defaultFilter = filter.NewFilter()

func NewConfig(opts ...ConfigOption) *Config {
	c := &Config{
		ConcurrentInTemplates:          20,
		ConcurrentTemplates:            20,
		ConcurrentTarget:               10,
		Mode:                           "nuclei",
		EnableReverseConnectionFeature: true,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.defaultFilter == nil {
		c.defaultFilter = defaultFilter
	}
	return c
}

func (c *Config) IsNuclei() bool {
	return strings.ToLower(strings.TrimSpace(c.Mode)) == "nuclei"
}

func (c *Config) AppendResultCallback(handler ResultCallback) {
	if c.Callback == nil {
		c.Callback = handler
		return
	}

	origin := c.Callback
	c.Callback = func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		origin(y, reqBulk, rsp, result, extractor)
		handler(y, reqBulk, rsp, result, extractor)
	}
}

func (c *Config) AppendHTTPResultCallback(handler HTTPResultCallback) {
	handlerRaw := HTTPResultCallbackWrapper(handler)
	if c.Callback == nil {
		c.Callback = handlerRaw
		return
	}

	origin := c.Callback
	c.Callback = func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		origin(y, reqBulk, rsp, result, extractor)
		handlerRaw(y, reqBulk, rsp, result, extractor)
	}
}

func (c *Config) AppendTCPResultCallback(handler TCPResultCallback) {
	handlerRaw := TCPResultCallbackWrapper(handler)
	if c.Callback == nil {
		c.Callback = handlerRaw
		return
	}

	origin := c.Callback
	c.Callback = func(y *YakTemplate, reqBulk any, rsp any, result bool, extractor map[string]interface{}) {
		origin(y, reqBulk, rsp, result, extractor)
		handlerRaw(y, reqBulk, rsp, result, extractor)
	}
}

func (c *Config) GenerateYakTemplate() (chan *YakTemplate, error) {
	if c.IsNuclei() {
		ch := make(chan *YakTemplate)
		go func() {
			defer close(ch)

			defer func() {
				if err := recover(); err != nil {
					log.Errorf("generate yak template failed: %v", err)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()

			scriptNameFilter := make(map[string]struct{})
			feedback := func(t *YakTemplate) {
				if t == nil {
					return
				}

				if utils.StringArrayContains(c.ExcludeTemplates, t.Name) {
					return
				}

				_, ok := scriptNameFilter[t.Name]
				if ok {
					return
				}
				scriptNameFilter[t.Name] = struct{}{}
				ch <- t
			}

			if c.QueryAll {
				for y := range yakit.YieldYakScripts(
					consts.GetGormProfileDatabase().Where("type = 'nuclei'"),
					context.Background()) {
					tpl, err := CreateYakTemplateFromYakScript(y)
					if err != nil {
						log.Errorf("create yak template failed (fuzz query mode): %s", err)
						continue
					}
					feedback(tpl)
				}
				return
			}

			if c.SingleTemplateRaw != "" {
				tpl, err := CreateYakTemplateFromNucleiTemplateRaw(c.SingleTemplateRaw)
				if err != nil {
					log.Errorf("create yak template failed (raw): %s", err)
				}
				feedback(tpl)
			}

			for _, y := range c.ExactTemplateInstances {
				tpl, err := CreateYakTemplateFromYakScript(y)
				if err != nil {
					log.Errorf("create yak template failed (template names): %s", err)
					continue
				}
				feedback(tpl)
			}

			for _, template := range c.TemplateName {
				y, err := yakit.GetNucleiYakScriptByName(consts.GetGormProfileDatabase(), template)
				if err != nil {
					log.Errorf("get nuclei yak script by name failed: %s", err)
					continue
				}
				tpl, err := CreateYakTemplateFromYakScript(y)
				if err != nil {
					log.Errorf("create yak template failed (template names): %s", err)
					continue
				}
				feedback(tpl)
			}

			for _, queries := range funk.ChunkStrings(c.FuzzQueryTemplate, 3) {
				if len(queries) <= 0 {
					continue
				}
				db := bizhelper.FuzzSearchWithStringArrayOrEx(
					consts.GetGormProfileDatabase().Where("type = 'nuclei'"), []string{
						"script_name", "content",
					}, queries, false,
				)
				for y := range yakit.YieldYakScripts(db, context.Background()) {
					tpl, err := CreateYakTemplateFromYakScript(y)
					if err != nil {
						log.Errorf("create yak template failed (fuzz query mode): %s", err)
						continue
					}
					feedback(tpl)
				}
			}

			for _, tags := range funk.ChunkStrings(c.Tags, 3) {
				if len(tags) <= 0 {
					continue
				}
				db := bizhelper.FuzzSearchWithStringArrayOrEx(
					consts.GetGormProfileDatabase().Where("type = 'nuclei'"), []string{
						"tags",
					}, tags, false,
				)
				for y := range yakit.YieldYakScripts(db, context.Background()) {
					tpl, err := CreateYakTemplateFromYakScript(y)
					if err != nil {
						if !strings.Contains(err.Error(), "(*)") {
							// debug io
							fmt.Println(y.Content)
						}
						log.Errorf("create yak template failed(tags): %s", err)
						continue
					}
					feedback(tpl)
				}
			}
		}()
		return ch, nil
	}
	return nil, utils.Error("empty yak templates")
}
