package ssa

type languageConfigOpt func(*LanguageConfig)

func (p *Program) getLanguageConfig() *LanguageConfig {
	// builder := p.GetAndCreateFunctionBuilder()
	config := p.Application.config
	if config == nil {
		config = NewLanguageConfig()
		p.Application.config = config
	}
	return config
}

func (b *FunctionBuilder) SetLanguageConfig(opt ...languageConfigOpt) *LanguageConfig {
	newConfig := NewLanguageConfig()
	b.GetProgram().Application.config = newConfig
	for _, o := range opt {
		o(newConfig)
	}
	return newConfig
}

func (b *FunctionBuilder) getLanguageConfig() *LanguageConfig {
	app := b.GetProgram().Application
	config := app.config
	if config == nil {
		config = b.SetLanguageConfig()
	}
	return config
}

func (b *FunctionBuilder) isBindLanguage() bool {
	if config := b.getLanguageConfig(); config != nil {
		return config.isBindLanguage
	}
	return false
}

func (b *FunctionBuilder) isTryBuildValue() bool {
	if config := b.getLanguageConfig(); config != nil {
		return config.isTryBuildValue
	}
	return false
}

func (b *FunctionBuilder) isSupportClass() bool {
	if config := b.getLanguageConfig(); config != nil {
		return config.isSupportClass
	}
	return false
}

func (b *FunctionBuilder) isSupportClassStaticModifier() bool {
	if config := b.getLanguageConfig(); config != nil {
		return config.isSupportClassStaticModifier
	}
	return false
}

func (b *FunctionBuilder) isSupportVirtualImport() bool {
	if config := b.getLanguageConfig(); config != nil {
		return config.isSupportVirtualImport
	}
	return false
}

type LanguageConfig struct {
	isBindLanguage  bool
	isTryBuildValue bool
	// Support obtaining static members and static method, even if the class is not instantiated.
	isSupportClass               bool
	isSupportClassStaticModifier bool

	//script Language need to handle call method
	isSupportConstMethod bool

	// Support virtual import, will create empty package for import when it is not exist.
	isSupportVirtualImport bool
}

func NewLanguageConfig() *LanguageConfig {
	return &LanguageConfig{
		isBindLanguage:               false,
		isTryBuildValue:              false,
		isSupportClass:               false,
		isSupportClassStaticModifier: false,
		isSupportConstMethod:         false,
		isSupportVirtualImport:       false,
	}
}

func (c *LanguageConfig) SetBindLanguage(b bool) {
	c.isBindLanguage = b
}

func (c *LanguageConfig) SetTryBuildValue(b bool) {
	c.isTryBuildValue = b
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

func LanguageSupportVirtualImport(config *LanguageConfig) {
	config.isSupportVirtualImport = true
}
