package ssa

import "github.com/yaklang/yaklang/common/log"

type languageConfigOpt func(*LanguageConfig)

func (b *FunctionBuilder) SetLanguageConfig(opt ...languageConfigOpt) {
	newConfig := NewLanguageConfig()
	b.GetProgram().Application.config = newConfig
	for _, o := range opt {
		o(newConfig)
	}
}

func LanguageConfigIsBinding(config *LanguageConfig) {
	config.isBindLanguage = true
}
func LanguageConfigSupportConstMethod(config *LanguageConfig) {
	config.isSupportConstMethod = true
}

func LanguageConfigTryBuildValue(config *LanguageConfig) {
	config.isTryBuildValue = true
}

func LanguageConfigIsSupportClass(config *LanguageConfig) {
	config.isSupportClass = true
}
func LanguageConfigVirtualImport(config *LanguageConfig) {
	config.VirtualImport = true
}
func LanguageConfigVirtualGet(config *LanguageConfig) {
	config.VirtualGetter = true
}

func LanguageConfigIsSupportClassStaticModifier(config *LanguageConfig) {
	config.isSupportClassStaticModifier = true
}

func (b *FunctionBuilder) isBindLanguage() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]BindLanguage config is not init")
		return false
	}
	return config.isBindLanguage
}

func (b *FunctionBuilder) isTryBuildValue() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]TryBuildValue config is not init")
		return false
	}
	return config.isTryBuildValue
}

func (b *FunctionBuilder) isSupportClass() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportClass config is not init")
		return false
	}
	return config.isSupportClass
}
func (b *FunctionBuilder) IsVirtualImport() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportClass config is not init")
		return false
	}
	return config.VirtualImport
}
func (b *FunctionBuilder) isSupportClassStaticModifier() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportClassStaticModifier config is not init")
		return false
	}
	return config.isSupportClassStaticModifier
}

type LanguageConfig struct {
	isBindLanguage  bool
	isTryBuildValue bool
	// Support obtaining static members and static method, even if the class is not instantiated.
	isSupportClass               bool
	isSupportClassStaticModifier bool

	//script Language need to handle call method
	isSupportConstMethod bool

	VirtualImport bool
	VirtualGetter bool
}

func NewLanguageConfig() *LanguageConfig {
	return &LanguageConfig{
		isBindLanguage:               false,
		isTryBuildValue:              false,
		isSupportClass:               false,
		isSupportClassStaticModifier: false,
		isSupportConstMethod:         false,
	}
}

func (c *LanguageConfig) SetBindLanguage(b bool) {
	c.isBindLanguage = b
}
func (c *LanguageConfig) SetVirtualImport(b bool) {
	c.VirtualImport = b
}
func (c *LanguageConfig) SetVirtualGet(b bool) {
	c.VirtualGetter = b
}

func (c *LanguageConfig) SetTryBuildValue(b bool) {
	c.isTryBuildValue = b
}
