package aiforge

import "github.com/google/uuid"

type RefineConfig struct {
	RefinePrompt         string
	KnowledgeBaseName    string
	KnowledgeBaseDesc    string
	KnowledgeBaseType    string
	KnowledgeEntryLength int
	Strict               bool

	*AnalysisConfig
}

func NewRefineConfig(opts ...any) *RefineConfig {
	cfg := &RefineConfig{
		RefinePrompt:         "",
		KnowledgeBaseName:    uuid.New().String(),
		KnowledgeEntryLength: 1000,
		Strict:               false,
	}
	otherOption := make([]any, 0)
	for _, opt := range opts {
		if optFunc, ok := opt.(RefineOption); ok {
			optFunc(cfg)
		} else {
			otherOption = append(otherOption, opt)
		}
	}
	cfg.AnalysisConfig = NewAnalysisConfig(otherOption...)
	return cfg
}

type RefineOption func(*RefineConfig)

func _refine_WithKnowledgeBaseDesc(desc string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseDesc = desc
	}
}

func _refine_WithKnowledgeBaseType(typ string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseType = typ
	}
}

func _refine_WithRefinePrompt(prompt string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.RefinePrompt = prompt
	}
}

func _refine_WithKnowledgeBaseName(name string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseName = name
	}
}

func _refine_WithKnowledgeEntryLength(length int) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeEntryLength = length
	}
}

func _refine_WithStrict(strict bool) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.Strict = strict
	}
}
