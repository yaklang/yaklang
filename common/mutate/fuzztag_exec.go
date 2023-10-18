package mutate

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/fuzztagx"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

type FuzzTagConfig struct {
	resultHandler func(string, []string) bool
	tagMethodMap  map[string]*parser.TagMethod
}

func NewFuzzTagConfig() *FuzzTagConfig {
	return &FuzzTagConfig{
		tagMethodMap: map[string]*parser.TagMethod{},
	}
}

func (f *FuzzTagConfig) AddFuzzTagHandler(name string, handler func(string) []string) {
	AddFuzzTagDescriptionToMap(f.tagMethodMap, &FuzzTagDescription{
		TagName: name,
		Handler: handler,
	})
}
func (f *FuzzTagConfig) AddFuzzTagHandlerEx(name string, handler func(string) []*fuzztag.FuzzExecResult) {
	AddFuzzTagDescriptionToMap(f.tagMethodMap, &FuzzTagDescription{
		TagName:   name,
		HandlerEx: handler,
	})
}

type FuzzConfigOpt func(config *FuzzTagConfig)

func Fuzz_WithExtraFuzzTagHandler(tag string, handler func(string) []string) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTagHandler(tag, handler)
	}
}

func Fuzz_WithExtraFuzzTagHandlerEx(tag string, handler func(string) []*fuzztag.FuzzExecResult) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTagHandlerEx(tag, handler)
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
	for k, method := range tagMethodMap {
		config.tagMethodMap[k] = method
	}
	for _, opt := range opts {
		opt(config)
	}
	if v, ok := config.tagMethodMap["params"]; ok {
		config.tagMethodMap["param"] = v
		config.tagMethodMap["p"] = v
	}
	defer func() {
		if recoveredErr := recover(); recoveredErr != nil {
			err = errors.Errorf("reocvered for rendering fuzztag: %s", recoveredErr)
			if config.resultHandler != nil {
				config.resultHandler(utils2.InterfaceToString(input), []string{}) // 做一个兜底，panic了也不会影响发包
			}
		}
	}()
	generator, err := fuzztagx.NewGenerator(utils2.InterfaceToString(input), config.tagMethodMap)
	if err != nil {
		return nil, err
	}
	res := []string{}
	for generator.Next() {
		if generator.Error != nil {
			return nil, generator.Error
		}
		result := generator.Result()
		data := result.GetData()
		res = append(res, string(data))
		if config.resultHandler != nil {
			config.resultHandler(string(data), result.GetVerbose())
		}
	}
	return res, nil
}
func MutateQuick(i interface{}) (finalResult []string) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("fuzztag execute failed: %s", err)
			finalResult = []string{utils2.InterfaceToString(i)}
		}
	}()
	results, err := FuzzTagExec(i)
	if err != nil {
		return []string{utils2.InterfaceToString(i)}
	}
	return results
}
