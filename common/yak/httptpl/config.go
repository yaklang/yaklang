package httptpl

import (
	"context"
	"fmt"
	"strings"
	"yaklang/common/consts"
	"yaklang/common/filter"
	"yaklang/common/go-funk"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/bizhelper"
	"yaklang/common/utils/lowhttp"
	"yaklang/common/yakgrpc/yakit"
)

type ResultCallback func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{})

type ConfigOption func(*Config)

type Config struct {
	// Templates 内部 HTTP 网络并发
	ConcurrentInTemplates int
	// Templates 外部 HTTP 网络并发
	ConcurrentTemplates int
	// ConcurrentTarget 批量扫描的并发
	ConcurrentTarget int

	Callback ResultCallback

	// nuclei / xray
	Mode string

	EnableReverseConnectionFeature bool

	// 搜索 yakit.YakScript
	SingleTemplateRaw string
	TemplateName      []string
	FuzzQueryTemplate []string
	ExcludeTemplates  []string
	Tags              []string

	// DebugMode
	Debug         bool
	DebugRequest  bool
	DebugResponse bool

	Verbose bool
}

func WithDebug(b bool) ConfigOption {
	return func(config *Config) {
		config.Debug = b
		config.DebugResponse = b
		config.DebugRequest = b
	}
}

func WithVerbose(b bool) ConfigOption {
	return func(config *Config) {
		config.Verbose = b
	}
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

func WithFuzzQueryTemplate(s ...string) ConfigOption {
	return func(config *Config) {
		config.FuzzQueryTemplate = s
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

func WithResultCallback(f ResultCallback) ConfigOption {
	return func(config *Config) {
		config.Callback = f
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

func NewConfig(opts ...ConfigOption) *Config {
	var c = &Config{
		ConcurrentInTemplates: 20,
		ConcurrentTemplates:   20,
		ConcurrentTarget:      10,
		Mode:                  "nuclei",
	}
	for _, opt := range opts {
		opt(c)
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
	c.Callback = func(y *YakTemplate, reqBulk *YakRequestBulkConfig, rsp []*lowhttp.LowhttpResponse, result bool, extractor map[string]interface{}) {
		origin(y, reqBulk, rsp, result, extractor)
		handler(y, reqBulk, rsp, result, extractor)
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

			scriptFilter := filter.NewFilter()
			feedback := func(t *YakTemplate) {
				if t == nil {
					return
				}

				if utils.StringArrayContains(c.ExcludeTemplates, t.Name) {
					return
				}

				if scriptFilter.Exist(t.Name) {
					return
				}
				scriptFilter.Insert(t.Name)
				ch <- t
			}

			if c.SingleTemplateRaw != "" {
				tpl, err := CreateYakTemplateFromNucleiTemplateRaw(c.SingleTemplateRaw)
				if err != nil {
					log.Errorf("create yak template failed (raw): %s", err)
				}
				feedback(tpl)
			}

			for _, template := range c.TemplateName {
				y, err := yakit.GetNucleiYakScriptByName(consts.GetGormProfileDatabase(), template)
				if err != nil {
					log.Errorf("get nuclei yak script by name failed: %s", err)
					continue
				}
				tpl, err := CreateYakTemplateFromNucleiTemplateRaw(y.Content)
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
					tpl, err := CreateYakTemplateFromNucleiTemplateRaw(y.Content)
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
					tpl, err := CreateYakTemplateFromNucleiTemplateRaw(y.Content)
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
