package aiforge

import (
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
)

type RefineConfig struct {
	RefinePrompt         string
	KnowledgeBaseName    string
	KnowledgeBaseDesc    string
	KnowledgeBaseType    string
	KnowledgeEntryLength int
	Strict               bool
	EnableERMEnhance     bool

	Database *gorm.DB

	*AnalysisConfig
}

func NewRefineConfig(opts ...any) *RefineConfig {
	cfg := &RefineConfig{
		RefinePrompt:         "",
		KnowledgeBaseName:    uuid.New().String(),
		KnowledgeEntryLength: 1000,
		Strict:               false,
		EnableERMEnhance:     true,
		Database:             consts.GetGormProfileDatabase(),
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

func RefineWithCustomizeDatabase(db *gorm.DB) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.Database = db
	}
}

func RefineWithKnowledgeBaseDesc(desc string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseDesc = desc
	}
}

func RefineWithKnowledgeBaseType(typ string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseType = typ
	}
}

func _refine_WithRefinePrompt(prompt string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.RefinePrompt = prompt
	}
}

func RefineWithKnowledgeBaseName(name string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseName = name
	}
}

func RefineWithKnowledgeEntryLength(length int) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeEntryLength = length
	}
}

func _refine_WithStrict(strict bool) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.Strict = strict
	}
}
