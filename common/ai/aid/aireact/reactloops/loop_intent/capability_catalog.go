package loop_intent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const catalogChunkSize = 30 * 1024

// BuildCapabilityCatalog collects all available tools, forges, skills, and focus modes,
// formats each as a single-line entry for AI consumption.
// Format: [type:identifier]: verbose_name - description. keywords: kw1,kw2
func BuildCapabilityCatalog(r aicommon.AIInvokeRuntime) string {
	var sb strings.Builder

	db := consts.GetGormProfileDatabase()
	if db != nil {
		tools, err := yakit.SearchAIYakTool(db, "")
		if err != nil {
			log.Warnf("capability catalog: failed to load tools: %v", err)
		} else {
			for _, t := range tools {
				name := t.VerboseName
				if name == "" {
					name = t.Name
				}
				desc := utils.ShrinkString(t.Description, 120)
				line := fmt.Sprintf("[tool:%s]: %s - %s", t.Name, name, desc)
				if t.Keywords != "" {
					line += fmt.Sprintf(". keywords: %s", utils.ShrinkString(t.Keywords, 80))
				}
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}

		forges, err := yakit.GetAllAIForge(db)
		if err != nil {
			log.Warnf("capability catalog: failed to load forges: %v", err)
		} else {
			for _, f := range forges {
				name := f.ForgeVerboseName
				if name == "" {
					name = f.ForgeName
				}
				desc := utils.ShrinkString(f.Description, 120)
				line := fmt.Sprintf("[forge:%s]: %s - %s", f.ForgeName, name, desc)
				if f.ToolKeywords != "" {
					line += fmt.Sprintf(". keywords: %s", utils.ShrinkString(f.ToolKeywords, 80))
				}
				sb.WriteString(line)
				sb.WriteString("\n")
			}
		}
	}

	type skillLoaderProvider interface {
		GetSkillLoader() aiskillloader.SkillLoader
	}
	cfg := r.GetConfig()
	if provider, ok := cfg.(skillLoaderProvider); ok {
		skillLoader := provider.GetSkillLoader()
		if skillLoader != nil && skillLoader.HasSkills() {
			for _, meta := range skillLoader.AllSkillMetas() {
				desc := utils.ShrinkString(meta.Description, 120)
				sb.WriteString(fmt.Sprintf("[skill:%s]: %s - %s\n", meta.Name, meta.Name, desc))
			}
		}
	}

	for _, meta := range reactloops.GetAllLoopMetadata() {
		if meta.IsHidden {
			continue
		}
		desc := utils.ShrinkString(meta.Description, 120)
		sb.WriteString(fmt.Sprintf("[focus_mode:%s]: %s - %s\n", meta.Name, meta.Name, desc))
	}

	return sb.String()
}

// MatchIdentifiersFromCatalog uses aireducer-style chunking + concurrent LiteForge
// to find matching identifiers from the capability catalog for a user query.
// If the catalog fits in one chunk (<=catalogChunkSize), a single AI call is made.
// Otherwise, the catalog is split and processed concurrently.
func MatchIdentifiersFromCatalog(
	r aicommon.AIInvokeRuntime,
	catalog string,
	userQuery string,
) []string {
	if catalog == "" || userQuery == "" {
		return nil
	}

	chunks := splitCatalogIntoChunks(catalog, catalogChunkSize)
	if len(chunks) == 0 {
		return nil
	}

	ctx := r.GetConfig().GetContext()

	var mu sync.Mutex
	var allIdentifiers []string
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		go func(idx int, chunkData string) {
			defer wg.Done()
			ids := matchChunk(ctx, r, chunkData, userQuery, idx)
			if len(ids) > 0 {
				mu.Lock()
				allIdentifiers = append(allIdentifiers, ids...)
				mu.Unlock()
			}
		}(i, chunk)
	}

	wg.Wait()

	seen := make(map[string]bool, len(allIdentifiers))
	var deduped []string
	for _, id := range allIdentifiers {
		id = strings.TrimSpace(id)
		if id != "" && !seen[id] {
			seen[id] = true
			deduped = append(deduped, id)
		}
	}
	return deduped
}

func splitCatalogIntoChunks(catalog string, maxChunkBytes int) []string {
	if len(catalog) <= maxChunkBytes {
		return []string{catalog}
	}

	lines := strings.Split(catalog, "\n")
	var chunks []string
	var current strings.Builder

	for _, line := range lines {
		if current.Len()+len(line)+1 > maxChunkBytes && current.Len() > 0 {
			chunks = append(chunks, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteString("\n")
	}
	if current.Len() > 0 {
		chunks = append(chunks, current.String())
	}
	return chunks
}

func matchChunk(
	ctx context.Context,
	r aicommon.AIInvokeRuntime,
	chunkData string,
	userQuery string,
	chunkIdx int,
) []string {
	nonce := utils.RandStringBytes(6)
	prompt := fmt.Sprintf(`<|INSTRUCTION_%s|>
You are a capability matcher. Given a user query and a catalog of available capabilities,
select ALL capabilities that are relevant to the user's intent.

CRITICAL RULES:
- You MUST ONLY select identifiers that appear in the catalog below. Do NOT invent or fabricate any identifier.
- If the user's input directly contains a capability identifier (e.g., user says "run hostscan"), that identifier MUST be included.
- Consider both Chinese and English meanings when matching.
- Select capabilities that could help accomplish the user's goal, even indirectly.
- Return ONLY the identifier part (the text after the type prefix, e.g., "web_search" from "[tool:web_search]").
<|INSTRUCTION_END_%s|>

<|USER_QUERY_%s|>
%s
<|USER_QUERY_END_%s|>

<|CAPABILITY_CATALOG_%s|>
%s
<|CAPABILITY_CATALOG_END_%s|>`, nonce, nonce, nonce, userQuery, nonce, nonce, chunkData, nonce)

	schema := []aitool.ToolOption{
		aitool.WithStringArrayParamEx("matched_identifiers", []aitool.PropertyOption{
			aitool.WithParam_Description("List of matched capability identifiers from the catalog. Only include identifiers that actually exist in the catalog."),
			aitool.WithParam_Required(true),
		}),
	}

	forgeResult, err := r.InvokeSpeedPriorityLiteForge(ctx, "capability-catalog-match", prompt, schema)
	if err != nil {
		log.Warnf("capability catalog match chunk %d failed: %v", chunkIdx, err)
		return nil
	}
	if forgeResult == nil {
		return nil
	}

	return forgeResult.GetStringSlice("matched_identifiers")
}

// VerifyIdentifiers filters a list of identifier names through ResolveIdentifier,
// removing any that don't correspond to real tools/forges/skills/focus modes.
func VerifyIdentifiers(loop *reactloops.ReActLoop, identifiers []string) []string {
	var verified []string
	for _, id := range identifiers {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		resolved := loop.ResolveIdentifier(id)
		if resolved.IsUnknown() {
			log.Infof("capability catalog: identifier %q not resolved, skipping", id)
			continue
		}
		verified = append(verified, id)
	}
	return verified
}
