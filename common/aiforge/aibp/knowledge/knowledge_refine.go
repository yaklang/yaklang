package knowledge

import (
	_ "embed"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
)

//go:embed knowledge_refine_prompts/lite_prompt.txt
var refinePrompt string

//go:embed knowledge_refine_prompts/schema.json
var refineSchema string

var KnowledgeRefine = "knowledge-refine"

func init() {
	lfopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(refinePrompt),
		aiforge.WithLiteForge_OutputSchemaRaw("refine", refineSchema),
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

func NewKnowledgeRefineForge() (*aiforge.LiteForge, error) {
	lfopts := []aiforge.LiteForgeOption{
		aiforge.WithLiteForge_Prompt(refinePrompt),
		aiforge.WithLiteForge_OutputSchemaRaw("refine", refineSchema),
	}
	return aiforge.NewLiteForge(KnowledgeRefine, lfopts...)
}
