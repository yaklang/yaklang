package aiskillloader

import (
	"bytes"
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// SkillSelection represents a single skill selected by the AI with a reason.
type SkillSelection struct {
	SkillName string
	Reason    string
}

// SkillSearchAICallback is a function that performs AI-based skill search.
// It receives the constructed prompt and the JSON schema for expected output,
// executes LiteForge (or equivalent), and returns parsed skill selections.
//
// This callback type is defined in aiskillloader (rather than using aicommon types
// directly) to avoid circular imports, since aiskillloader is a subpackage of aicommon.
//
// The caller (who has access to aicommon/aiforge) is responsible for:
//   - Invoking LiteForge with the prompt and schema
//   - Configuring FieldStream for the "reason" field with nodeId "thought"
//   - Parsing the ForgeResult into []SkillSelection
type SkillSearchAICallback func(prompt string, schema string) ([]SkillSelection, error)

// SkillSelectSchemaJSON returns the JSON schema string for AI skill selection output.
// Callers should pass this to LiteForge as the output schema.
func SkillSelectSchemaJSON() string {
	return skillSelectSchemaJSON
}

// skillSelectSchemaJSON is the pre-built JSON schema for skill selection.
// Built using aitool.NewObjectSchemaWithAction at init time would cause import cycle,
// so we define it as a raw JSON string that matches the expected format.
var skillSelectSchemaJSON = `{
  "type": "object",
  "action": "object",
  "properties": {
    "skill_list": {
      "type": "array",
      "description": "Select up to 5 most relevant skills for the user's task. Return skill names and brief reasoning.",
      "items": {
        "type": "object",
        "properties": {
          "skill_name": {
            "type": "string",
            "description": "The exact name of the selected skill."
          },
          "reason": {
            "type": "string",
            "description": "Brief reason why this skill is relevant to the user's task."
          }
        },
        "required": ["skill_name", "reason"]
      }
    }
  },
  "required": ["skill_list"]
}`

// skillSelectPromptTemplate is the prompt template for AI skill selection.
const skillSelectPromptTemplate = `You are an AI skill selector. Your task is to analyze the user's need and select the most relevant skills from the available list.

## User's Task
%s

## Available Skills
%s

## Instructions
1. Analyze the user's task carefully.
2. Select up to 5 skills that are most relevant to completing the task.
3. For each selected skill, provide a brief reason explaining why it is relevant.
4. Only select skills that are genuinely useful. If fewer than 5 are relevant, select fewer.
5. Return the skill names exactly as listed above.
`

// BuildSearchByAIPrompt builds the prompt for AI skill search.
// Exported for use by callers who need to construct the prompt externally.
func BuildSearchByAIPrompt(skills []*SkillMeta, userNeed string) string {
	var candidatesBuf bytes.Buffer
	for i, s := range skills {
		candidatesBuf.WriteString(fmt.Sprintf("%d. %s: %s\n", i+1, s.Name, s.Description))
	}
	return fmt.Sprintf(skillSelectPromptTemplate, userNeed, candidatesBuf.String())
}

// SearchByAI uses the provided callback to select the most relevant skills
// for a given user need.
//
// Parameters:
//   - skills: all available skill metadata to choose from
//   - userNeed: the user's task description
//   - callback: function that invokes LiteForge and returns parsed selections
//
// Returns up to 5 SkillMeta entries that the AI considers most relevant.
func SearchByAI(skills []*SkillMeta, userNeed string, callback SkillSearchAICallback) ([]*SkillMeta, error) {
	if len(skills) == 0 {
		return nil, nil
	}
	if userNeed == "" {
		return nil, utils.Error("user need is required for AI search")
	}
	if callback == nil {
		return nil, utils.Error("search AI callback is nil")
	}

	// Build the prompt
	prompt := BuildSearchByAIPrompt(skills, userNeed)

	// Build skill map for matching
	skillMap := make(map[string]*SkillMeta, len(skills))
	for _, s := range skills {
		skillMap[s.Name] = s
	}

	// Execute the callback
	selections, err := callback(prompt, skillSelectSchemaJSON)
	if err != nil {
		return nil, utils.Wrapf(err, "AI skill search failed")
	}

	// Match back to SkillMeta, limit to 5
	var selected []*SkillMeta
	for _, sel := range selections {
		if len(selected) >= 5 {
			break
		}
		if sel.SkillName == "" {
			continue
		}
		if meta, ok := skillMap[sel.SkillName]; ok {
			selected = append(selected, meta)
			log.Infof("AI selected skill %q: %s", sel.SkillName, sel.Reason)
		} else {
			log.Warnf("AI selected unknown skill name %q, skipping", sel.SkillName)
		}
	}

	return selected, nil
}
