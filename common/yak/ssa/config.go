package ssa

import "github.com/yaklang/yaklang/common/log"

type languageConfigOpt func(*LanguageConfig)

type Config struct {
	*LanguageConfig
	pointerAssign bool
}

func NewConfig() *Config {
	return &Config{
		LanguageConfig: NewLanguageConfig(),
		pointerAssign:  false,
	}
}

func (b *FunctionBuilder) SetLanguageConfig(opt ...languageConfigOpt) {
	newConfig := NewConfig()
	b.GetProgram().Application.config = newConfig
	for _, o := range opt {
		o(newConfig.LanguageConfig)
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

func (b *FunctionBuilder) isSupportClassStaticModifier() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportClassStaticModifier config is not init")
		return false
	}
	return config.isSupportClassStaticModifier
}

func (b *FunctionBuilder) SetPointerAssignConfig(bo bool) {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportClassStaticModifier config is not init")
		return
	}
	config.pointerAssign = bo
}

func (b *FunctionBuilder) GetPointerAssignConfig() bool {
	config := b.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportClassStaticModifier config is not init")
		return false
	}
	return config.pointerAssign
}

type LanguageConfig struct {
	isBindLanguage  bool
	isTryBuildValue bool
	// Support obtaining static members and static method, even if the class is not instantiated.
	isSupportClass               bool
	isSupportClassStaticModifier bool

	//script Language need to handle call method
	isSupportConstMethod bool
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

func (c *LanguageConfig) SetTryBuildValue(b bool) {
	c.isTryBuildValue = b
}
