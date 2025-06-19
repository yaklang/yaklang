package ssa

type LanguageConfigOpt func(config *LanguageConfig)

func (b *FunctionBuilder) SetLanguageConfig(opts ...LanguageConfigOpt) {
	newConfig := NewLanguageConfig()
	b.GetProgram().Application.config = newConfig
	for _, opt := range opts {
		opt(newConfig)
	}
}
func WithLanguageConfigBind(bind bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.isBindLanguage = bind
	}
}
func WithLanguageConfigSupportConstMethod(support bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.isSupportConstMethod = support
	}
}
func WithLanguageConfigTryBuildValue(try bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.isTryBuildValue = try
	}
}
func WithLanguageConfigSupportClass(support bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.isSupportClass = support
	}
}
func WithLanguageConfigVirtualImport(virtual bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.VirtualImport = virtual
	}
}
func WithLanguageConfigShouldBuild(f func(filename string) bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.ShouldBuild = f
	}
}
func WithLanguageBuilder(builder Builder) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.LanguageBuilder = builder
	}
}

func WithLanguageConfigIsSupportClassStaticModifier(support bool) LanguageConfigOpt {
	return func(config *LanguageConfig) {
		config.isSupportClassStaticModifier = support
	}
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

	VirtualImport   bool
	VirtualGetter   bool
	ShouldBuild     func(fileName string) bool
	LanguageBuilder Builder
}

func NewLanguageConfig() *LanguageConfig {
	return &LanguageConfig{
		isBindLanguage:               false,
		isTryBuildValue:              false,
		isSupportClass:               false,
		isSupportClassStaticModifier: false,
		isSupportConstMethod:         false,
		ShouldBuild: func(fileName string) bool {
			return false
		},
	}
}
