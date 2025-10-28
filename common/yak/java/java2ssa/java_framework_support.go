//go:build !no_language
// +build !no_language

package java2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const FRAMEWORK_DEFAULT_CLASSPATH = "src/main"

type (
	HookReturnFunc           = func(*singleFileBuilder, ssa.Value)
	HookMemberCallMethodFunc = func(*singleFileBuilder, ssa.Value, ssa.Value, ...ssa.Value)
)

type JavaFramework struct {
	name                     string
	hookReturnFunc           []HookReturnFunc
	hookMemberCallMethodFunc []HookMemberCallMethodFunc
}

const (
	FrameworkSupportJAVAEE     = "java_ee"
	FrameworkSupportSpringBoot = "spring_boot"
)

var frameworks = make(map[string]*JavaFramework)

func init() {
	registerFrameworkSupport(FrameworkSupportJAVAEE, hookMemberCallMethod(hookJavaEEMemberCallMethod))
	registerFrameworkSupport(FrameworkSupportSpringBoot, hookReturn(hookSpringBootReturn), hookMemberCallMethod(hookSpringBootMemberCallMethod))
}

func registerFrameworkSupport(name string, options ...func(*JavaFramework)) {
	if name == "" {
		return
	}
	f := &JavaFramework{
		name: name,
	}
	for _, option := range options {
		option(f)
	}
	frameworks[name] = f
}

func hookReturn(hook HookReturnFunc) func(*JavaFramework) {
	return func(f *JavaFramework) {
		f.hookReturnFunc = append(f.hookReturnFunc, hook)
	}
}

func hookMemberCallMethod(hook HookMemberCallMethodFunc) func(*JavaFramework) {
	return func(f *JavaFramework) {
		f.hookMemberCallMethodFunc = append(f.hookMemberCallMethodFunc, hook)
	}
}

func (y *singleFileBuilder) HookMemberCallMethod(obj ssa.Value, key ssa.Value, args ...ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	if obj == nil || key == nil {
		return
	}

	for _, f := range frameworks {
		for _, hook := range f.hookMemberCallMethodFunc {
			hook(y, obj, key, args...)
		}
	}
}

func (y *singleFileBuilder) HookReturn(val ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	for _, f := range frameworks {
		for _, hook := range f.hookReturnFunc {
			hook(y, val)
		}
	}
}

func (y *singleFileBuilder) SetUIModel(val ssa.Value) {
	if y == nil || y.IsStop() {
		return
	}
	y.currentUIModel = val
}

func (y *singleFileBuilder) GetUIModel() ssa.Value {
	if y == nil || y.IsStop() {
		return nil
	}
	return y.currentUIModel
}

func (y *singleFileBuilder) ResetUIModel() {
	if y == nil || y.IsStop() {
		return
	}
	y.currentUIModel = nil
}

func isFreemarkerFile(prog *ssa.Program, path string) bool {
	if prog == nil || path == "" {
		return false
	}
	ext := filepath.Ext(path)
	if ext == "" {
		return false
	}
	if ext == ".ftl" {
		return true
	}
	configExt := prog.GetProjectConfigValue("spring.freemarker.suffix")
	if configExt == "" {
		return false
	}
	return ext == configExt
}
