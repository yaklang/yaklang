package format

// Options controls how static analyze results are formatted for AI feedback or copy.
type Options struct {
	// PluginType is passed to static analysis (yak, mitm, port-scan, codec, context-menu, syntaxflow).
	PluginType string
	// LineBase shifts displayed line numbers (0-based editor offset).
	LineBase int
	// MaxIssues limits how many issues are included; 0 means all.
	MaxIssues int
	// IncludeHints adds AI assistant hints when true.
	IncludeHints bool
	// IncludeCodeContext adds surrounding source lines when true.
	IncludeCodeContext bool
	// HintLabel prefixes intelligent hints; empty disables the label line break only.
	HintLabel string
	// TruncateMoreMessage is appended when MaxIssues truncates the list.
	TruncateMoreMessage string
}

func defaultOptions() Options {
	return Options{
		PluginType:          "yak",
		IncludeHints:        true,
		IncludeCodeContext:  true,
		HintLabel:           "AI助手提示: ",
		TruncateMoreMessage: "There are other errors, it's better to fix the critical issues above first before fixing others",
	}
}

// Option configures formatting behavior.
type Option func(*Options)

func WithPluginType(pluginType string) Option {
	return func(o *Options) {
		if pluginType != "" {
			o.PluginType = pluginType
		}
	}
}

func WithLineBase(lineBase int) Option {
	return func(o *Options) {
		if lineBase > 0 {
			o.LineBase = lineBase
		}
	}
}

func WithMaxIssues(maxIssues int) Option {
	return func(o *Options) {
		if maxIssues > 0 {
			o.MaxIssues = maxIssues
		}
	}
}

func WithIncludeHints(include bool) Option {
	return func(o *Options) {
		o.IncludeHints = include
	}
}

func WithHintLabel(label string) Option {
	return func(o *Options) {
		o.HintLabel = label
	}
}

func WithIncludeCodeContext(include bool) Option {
	return func(o *Options) {
		o.IncludeCodeContext = include
	}
}

func applyOptions(opts []Option) Options {
	cfg := defaultOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

// YakRunnerDefaults returns options matching write_yaklang_code loop behavior.
func YakRunnerDefaults(lineBase int) []Option {
	return []Option{
		WithPluginType("yak"),
		WithLineBase(lineBase),
		WithMaxIssues(2),
	}
}

// CopyAllDefaults returns options for plugin editor one-click copy (all issues).
func CopyAllDefaults(pluginType string) []Option {
	return []Option{
		WithPluginType(pluginType),
		WithMaxIssues(0),
		WithHintLabel("修改建议: "),
	}
}
