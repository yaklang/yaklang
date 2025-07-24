package mutate

import (
	"context"

	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/fuzztag"
	"github.com/yaklang/yaklang/common/fuzztagx"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/log"
	utils2 "github.com/yaklang/yaklang/common/utils"
)

type FuzzTagConfig struct {
	resultHandler     func(string, []string) bool
	tagMethodMap      map[string]*parser.TagMethod
	isSimple          bool
	syncRootNodeIndex bool
	resultLimit       int
	context           context.Context
}

func NewFuzzTagConfig() *FuzzTagConfig {
	return &FuzzTagConfig{
		tagMethodMap: map[string]*parser.TagMethod{},
		resultLimit:  -1,
	}
}

func (f *FuzzTagConfig) AddFuzzTag(name string, t *FuzzTagDescription) {
	AddFuzzTagDescriptionToMap(f.tagMethodMap, t)
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

func Fuzz_WithSimple(b bool) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.isSimple = b
	}
}

func Fuzz_SyncTag(b bool) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.syncRootNodeIndex = b
	}
}

func Fuzz_WithExtraFuzzTag(tag string, t *FuzzTagDescription) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTag(tag, t)
	}
}

func Fuzz_WithExtraFuzzTagHandler(tag string, handler func(string) []string) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTagHandler(tag, handler)
	}
}

func Fuzz_WithExtraDynFuzzTagHandler(tag string, handler func(string) []string) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.tagMethodMap[tag] = &parser.TagMethod{
			Name:  tag,
			IsDyn: true,
			Fun: func(s string) ([]*parser.FuzzResult, error) {
				var results []*parser.FuzzResult
				for _, result := range handler(s) {
					results = append(results, parser.NewFuzzResultWithData(result))
				}
				return results, nil
			},
		}
	}
}

func Fuzz_WithExtraFuzzTagHandlerEx(tag string, handler func(string) []*fuzztag.FuzzExecResult) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.AddFuzzTagHandlerEx(tag, handler)
	}
}

func Fuzz_WithExtraFuzzErrorTagHandler(tag string, handler func(string) ([]*parser.FuzzResult, error)) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.tagMethodMap[tag] = &parser.TagMethod{
			Name: tag,
			Fun:  handler,
		}
	}
}

func Fuzz_WithResultHandler(handler func(string, []string) bool) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.resultHandler = handler
	}
}

func Fuzz_WithParams(i interface{}) FuzzConfigOpt {
	m := utils2.InterfaceToMapInterface(i)
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

func Fuzz_WithResultLimit(limit int) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.resultLimit = limit
	}
}

func Fuzz_WithContext(ctx context.Context) FuzzConfigOpt {
	return func(config *FuzzTagConfig) {
		config.context = ctx
	}
}

func FuzzTagExec(input interface{}, opts ...FuzzConfigOpt) (_ []string, err error) {
	config := NewFuzzTagConfig()
	for k, method := range tagMethodMap {
		config.tagMethodMap[k] = method
	}
	for _, opt := range opts {
		opt(config)
	}
	ctx := config.context
	if ctx == nil {
		ctx = context.Background()
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
	generator, err := fuzztagx.NewGeneratorEx(ctx, utils2.InterfaceToString(input), config.tagMethodMap, config.isSimple, config.syncRootNodeIndex)
	if err != nil {
		return nil, err
	}
	defer generator.Cancel()
	var res []string
	count := 0
	for count != config.resultLimit && generator.Next() {
		result := generator.Result()
		data := result.GetData()
		res = append(res, string(data))
		if config.resultHandler != nil {
			verbose := fuzztagx.GetResultVerbose(result)
			if !config.resultHandler(string(data), verbose) {
				return res, nil
			}
		}
		count++
	}
	if err := generator.Error; err != nil {
		return nil, err
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
