package aiforge

import (
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/entityrepos"
	"github.com/yaklang/yaklang/common/consts"
)

type RefineConfig struct {
	RefinePrompt         string
	KnowledgeBaseName    string
	KnowledgeBaseDesc    string
	KnowledgeBaseType    string
	KnowledgeEntryLength int
	Strict               bool
	FocusQuery           string
	DisableBuildIndex    bool
	DisableERMBuild      bool

	Database *gorm.DB

	*AnalysisConfig

	ragSystemOptions []rag.RAGSystemConfigOption
}

func NewRefineConfig(opts ...any) *RefineConfig {
	cfg := &RefineConfig{
		RefinePrompt:         "",
		KnowledgeBaseName:    uuid.New().String(),
		KnowledgeEntryLength: 1000,
		Strict:               false,
		Database:             consts.GetGormProfileDatabase(),
	}
	otherOption := make([]any, 0)
	for _, opt := range opts {
		switch opt.(type) {
		case rag.RAGSystemConfigOption:
			cfg.ragSystemOptions = append(cfg.ragSystemOptions, opt.(rag.RAGSystemConfigOption))
		case RefineOption:
			opt.(RefineOption)(cfg)
		default:
			otherOption = append(otherOption, opt)
		}
	}
	cfg.AnalysisConfig = NewAnalysisConfig(otherOption...)
	cfg.ragSystemOptions = append(cfg.ragSystemOptions, rag.WithRAGCtx(cfg.Ctx))
	return cfg
}

func (a *RefineConfig) KHopOption() []entityrepos.KHopQueryOption {
	config := rag.NewRAGSystemConfig(a.ragSystemOptions...)
	options := append(a.AnalysisConfig.KHopOption(), config.ConvertToKHopOptions()...)
	return options
}

type RefineOption func(*RefineConfig)

func RefineWithCustomizeDatabase(db *gorm.DB) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.Database = db
	}
}

func RefineWithDisableBuildIndex(disable bool) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.DisableBuildIndex = disable
	}
}

func RefineWithDisableERMBuild(disable bool) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.DisableERMBuild = disable
	}
}

// RefineWithKnowledgeBaseDesc 设置生成知识库的描述（导出名为 liteforge.knowledgeBaseDesc）
// 参数:
//   - desc: 知识库描述
//
// 返回值:
//   - 知识构建可选项
//
// Example:
// ```
// opt = liteforge.knowledgeBaseDesc("security related knowledge")
// println(opt)
// ```
func RefineWithKnowledgeBaseDesc(desc string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseDesc = desc
	}
}

// RefineWithKnowledgeBaseType 设置生成知识库的类型（导出名为 liteforge.knowledgeBaseType）
// 参数:
//   - typ: 知识库类型
//
// 返回值:
//   - 知识构建可选项
//
// Example:
// ```
// opt = liteforge.knowledgeBaseType("text")
// println(opt)
// ```
func RefineWithKnowledgeBaseType(typ string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseType = typ
	}
}

// _refine_WithRefinePrompt 设置知识提炼使用的提示词（导出名为 liteforge.refinePrompt）
// 参数:
//   - prompt: 提炼提示词
//
// 返回值:
//   - 知识构建可选项
//
// Example:
// ```
// opt = liteforge.refinePrompt("extract key concepts only")
// println(opt)
// ```
func _refine_WithRefinePrompt(prompt string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.RefinePrompt = prompt
	}
}

// RefineWithKnowledgeBaseName 设置生成知识库的名称（导出名为 liteforge.knowledgeBaseName）
// 参数:
//   - name: 知识库名称
//
// 返回值:
//   - 知识构建可选项
//
// Example:
// ```
// opt = liteforge.knowledgeBaseName("my-kb")
// println(opt)
// ```
func RefineWithKnowledgeBaseName(name string) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeBaseName = name
	}
}

// RefineWithKnowledgeEntryLength 设置每条知识条目的目标长度（导出名为 liteforge.knowledgeEntryLength）
// 参数:
//   - length: 知识条目长度
//
// 返回值:
//   - 知识构建可选项
//
// Example:
// ```
// opt = liteforge.knowledgeEntryLength(512)
// println(opt)
// ```
func RefineWithKnowledgeEntryLength(length int) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.KnowledgeEntryLength = length
	}
}

// _refine_WithStrict 设置知识提炼是否使用严格模式（导出名为 liteforge.strictRefine）
// 参数:
//   - strict: 是否严格模式
//
// 返回值:
//   - 知识构建可选项
//
// Example:
// ```
// opt = liteforge.strictRefine(true)
// println(opt)
// ```
func _refine_WithStrict(strict bool) RefineOption {
	return func(cfg *RefineConfig) {
		cfg.Strict = strict
	}
}
