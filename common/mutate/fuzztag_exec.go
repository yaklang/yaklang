package mutate

import (
	"github.com/pkg/errors"
	"yaklang/common/fuzztag"
	utils2 "yaklang/common/utils"
)

type FuzzTagConfig struct {
	tagHandlersEx map[string]func(string) []*fuzztag.FuzzExecResult
	tagHandlers   map[string]func(string) []string
	resultHandler func(string, []string) bool
}

func NewFuzzTagConfig() *FuzzTagConfig {
	return &FuzzTagConfig{
		tagHandlers:   make(map[string]func(string) []string),
		tagHandlersEx: make(map[string]func(string) []*fuzztag.FuzzExecResult),
	}
}

func (f *FuzzTagConfig) AddFuzzTagHandler(tag string, handler func(string) []string) {
	f.tagHandlers[tag] = handler
}

func (f *FuzzTagConfig) GetFuzzTagHandler(tag string) func(string) []string {
	return f.tagHandlers[tag]
}

func (f *FuzzTagConfig) GetFuzzTagHandlerKeys() []string {
	keys := make([]string, 0, len(f.tagHandlers))
	for k := range f.tagHandlers {
		keys = append(keys, k)
	}
	return keys
}

func (f *FuzzTagConfig) GetFuzzTagHandlerValues() []func(string) []string {
	values := make([]func(string) []string, 0, len(f.tagHandlers))
	for _, v := range f.tagHandlers {
		values = append(values, v)
	}
	return values
}

func (f *FuzzTagConfig) GetFuzzTagHandlerLen() int {
	return len(f.tagHandlers)
}

func (f *FuzzTagConfig) GetFuzzTagHandlerMap() map[string]func(string) []string {
	return f.tagHandlers
}
func (f *FuzzTagConfig) GetFuzzTagHandlerExMap() map[string]func(string) []*fuzztag.FuzzExecResult {
	return f.tagHandlersEx
}

func (f *FuzzTagConfig) SetFuzzTagHandlerMap(m map[string]func(string) []string) {
	f.tagHandlers = m
}

func (f *FuzzTagConfig) MergeFuzzTagHandlerExMap(m map[string]func(string) []*fuzztag.FuzzExecResult) {
	for k, v := range m {
		f.tagHandlersEx[k] = v
	}
}
func (f *FuzzTagConfig) MergeFuzzTagHandlerMap(m map[string]func(string) []string) {
	for k, v := range m {
		f.tagHandlers[k] = v
	}
}

func (f *FuzzTagConfig) MergeFuzzTagHandlerConfig(c *FuzzTagConfig) {
	for k, v := range c.tagHandlers {
		f.tagHandlers[k] = v
	}
}

func (f *FuzzTagConfig) ClearFuzzTagHandler() {
	f.tagHandlers = make(map[string]func(string) []string)
}

type FuzzConfigOpt func(config *FuzzTagConfig)

func Fuzz_WithExtraFuzzTagHandler(tag string, handler func(string) []string) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTagHandler(tag, handler)
	}
}

func Fuzz_WithExtraFuzzTagHandlerEx(tag string, handler func(string) interface{}) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTagHandler(tag, func(s string) []string {
			return utils2.InterfaceToStringSlice(s)
		})
	}
}

func Fuzz_WithResultHandler(handler func(string, []string) bool) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.resultHandler = handler
	}
}

func Fuzz_WithParams(i interface{}) FuzzConfigOpt {
	m := utils2.InterfaceToGeneralMap(i)
	return Fuzz_WithExtraFuzzTagHandler("params", func(s string) []string {
		if m == nil {
			return []string{""}
		}
		if i, ok := m[s]; ok {
			return utils2.InterfaceToStringSlice(i)
		}
		return []string{""}
	})
}

func FuzzTagExec(input interface{}, opts ...FuzzConfigOpt) (_ []string, err error) {

	config := NewFuzzTagConfig()
	config.MergeFuzzTagHandlerMap(defaultFuzzTag)
	config.MergeFuzzTagHandlerExMap(defaultFuzzTagEx)
	for _, opt := range opts {
		opt(config)
	}
	defer func() {
		if recoveredErr := recover(); recoveredErr != nil {
			err = errors.Errorf("reocvered for rendering fuzztag: %s", recoveredErr)
			if config.resultHandler != nil {
				config.resultHandler(utils2.InterfaceToString(input), []string{}) // 做一个兜底，panic了也不会影响发包
			}
		}
	}()
	return fuzztag.ExecuteWithStringHandlerWithCallbackEx(
		input, config.GetFuzzTagHandlerMap(), config.GetFuzzTagHandlerExMap(),
		config.resultHandler,
	)
}
