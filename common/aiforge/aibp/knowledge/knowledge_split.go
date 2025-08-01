package knowledge

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed knowledge_split_prompts/lite_prompt.txt
var splitPrompt string

//go:embed knowledge_split_prompts/schema.json
var splitSchema string

func init() {
	lfopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(splitPrompt),
		aiforge.WithLiteForge_OutputSchemaRaw("split", splitSchema),
	}

	err := aiforge.RegisterAIDBuildInForge(KnowledgeRefine, lfopts...)
	if err != nil {
		log.Errorf("register knowledge-refine forge failed: %s", err)
	}
	err = aiforge.RegisterLiteForge(KnowledgeRefine, lfopts...)
	if err != nil {
		log.Errorf("register knowledge-refine forge failed: %s", err)
	}
}

func NewKnowledgeSplitForge() (*aiforge.LiteForge, error) {
	lfopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(splitPrompt),
		aiforge.WithLiteForge_OutputSchemaRaw("split", splitSchema),
	}
	return aiforge.NewLiteForge(KnowledgeRefine, lfopts...)
}
