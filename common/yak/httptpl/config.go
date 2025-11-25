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
}

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

func WithDebug(b bool) ConfigOption {
	return func(config *Config) {
		config.Debug = b
		config.DebugResponse = b
		config.DebugRequest = b
	}
}

func WithOOBTimeout(f float64) ConfigOption {
	return func(config *Config) {
		config.OOBTimeout = f
	}
}

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

func WithDebugRequest(b bool) ConfigOption {
	return func(config *Config) {
		config.DebugRequest = b
		config.Debug = b
	}
}

func WithDebugResponse(b bool) ConfigOption {
	return func(config *Config) {
		config.Debug = b
		config.DebugResponse = b
	}
}

func WithTags(f ...string) ConfigOption {
	return func(config *Config) {
		config.Tags = f
	}
}

func WithConcurrentTarget(i int) ConfigOption {
	return func(config *Config) {
		config.ConcurrentTarget = i
	}
}

func WithEnableReverseConnectionFeature(b bool) ConfigOption {
	return func(config *Config) {
		config.EnableReverseConnectionFeature = b
	}
}

func WithTemplateRaw(b string) ConfigOption {
	return func(config *Config) {
		config.SingleTemplateRaw = b
	}
}

func WithTemplateName(s ...string) ConfigOption {
	return func(config *Config) {
		config.TemplateName = s
	}
}

func WithExactTemplateInstance(script *schema.YakScript) ConfigOption {
	return func(config *Config) {
		config.ExactTemplateInstances = append(config.ExactTemplateInstances, script)
	}
}

func WithFuzzQueryTemplate(s ...string) ConfigOption {
	return func(config *Config) {
		config.FuzzQueryTemplate = s
	}
}

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

func WithExcludeTemplates(s ...string) ConfigOption {
	return func(config *Config) {
		config.ExcludeTemplates = s
	}
}

func WithConcurrentInTemplates(i int) ConfigOption {
	return func(config *Config) {
		config.ConcurrentInTemplates = i
	}
}

func WithConcurrentTemplates(i int) ConfigOption {
	return func(config *Config) {
		config.ConcurrentTemplates = i
	}
}

func WithMode(s string) ConfigOption {
	return func(config *Config) {
		config.Mode = s
	}
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
