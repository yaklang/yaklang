package aicommon

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon/aiskillloader"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func init() {
	RegisterAttachedResourceDataFactory(
		CONTEXT_PROVIDER_TYPE_AIFORGE,
		func() AttachedResourceData { return &AttachedAIForgeResourceData{} },
	)
}

type AttachedAIForgeResourceData struct {
	Key         string
	Name        string
	forge       *schema.AIForge
	loadedSkill bool
	lookupErr   error
}

func (d *AttachedAIForgeResourceData) Type() string {
	return CONTEXT_PROVIDER_TYPE_AIFORGE
}

func (d *AttachedAIForgeResourceData) Unmarshal(resource *AttachedResource) error {
	d.Key = resource.Key
	d.Name = strings.TrimSpace(resource.Value)
	if d.Name == "" {
		return fmt.Errorf("attached AI Forge name is empty")
	}
	return nil
}

func (d *AttachedAIForgeResourceData) BindLoopData(loop ReActLoopIF) error {
	d.forge = nil
	d.loadedSkill = false
	d.lookupErr = nil
	if d.Key != "" && d.Key != CONTEXT_PROVIDER_KEY_NAME {
		d.lookupErr = fmt.Errorf("unsupported attached AI Forge key %q", d.Key)
		return nil
	}
	if loop == nil || loop.GetConfig() == nil {
		d.lookupErr = fmt.Errorf("AI Forge runtime config is unavailable")
		return nil
	}
	config, ok := loop.GetConfig().(*Config)
	if !ok {
		d.lookupErr = fmt.Errorf("AI Forge lookup requires a concrete AI config")
		return nil
	}
	forge, forgeErr := lookupTaskForge(config, d.Name)
	if forgeErr == nil && forge != nil {
		d.forge = forge
		return nil
	}

	skillMeta, skillErr := lookupTaskSkill(config, d.Name)
	if skillErr == nil {
		skillErr = loadTaskSkill(config.GetSkillsContextManager(), skillMeta)
		if skillErr == nil {
			d.loadedSkill = true
			return nil
		}
	}
	if forgeErr == nil {
		forgeErr = utils.Errorf("AI Forge %q was not found", d.Name)
	}
	if skillErr == nil {
		skillErr = utils.Errorf("AI Skill %q was not found", d.Name)
	}
	d.lookupErr = utils.Errorf(
		"AI Forge lookup failed: %v; AI Skill lookup/load failed: %v",
		forgeErr,
		skillErr,
	)
	return nil
}

func loadTaskSkill(manager *aiskillloader.SkillsContextManager, meta *aiskillloader.SkillMeta) error {
	if manager == nil {
		return utils.Error("AI Skill context manager is not configured")
	}
	if meta == nil || strings.TrimSpace(meta.Name) == "" {
		return utils.Error("AI Skill metadata is empty")
	}
	if _, err := manager.LoadForcedSkill(meta.Name); err != nil {
		return utils.Wrapf(err, "load AI Skill %q failed", meta.Name)
	}
	return nil
}

func lookupTaskForge(config *Config, name string) (*schema.AIForge, error) {
	name = strings.TrimSpace(name)
	if config == nil {
		return nil, utils.Error("AI config is nil")
	}
	for _, forge := range config.ExtendedForge {
		if forge == nil || !schema.IsRunnableForgeType(forge.ForgeType) {
			continue
		}
		if forge.ForgeName == name {
			return forge, nil
		}
	}
	manager := config.GetAIForgeManager()
	if manager == nil {
		return nil, utils.Error("AI Forge manager is not configured")
	}
	if forge, err := manager.GetAIForge(name); err == nil && forge != nil && schema.IsRunnableForgeType(forge.ForgeType) {
		return forge, nil
	}
	return nil, utils.Errorf("AI Forge %q was not found", name)
}

func lookupTaskSkill(config *Config, name string) (*aiskillloader.SkillMeta, error) {
	name = strings.TrimSpace(name)
	if config == nil {
		return nil, utils.Error("AI config is nil")
	}
	loader := config.GetSkillLoader()
	if loader == nil {
		return nil, utils.Error("AI Skill loader is not configured")
	}
	if meta, err := aiskillloader.LookupSkillMeta(loader, name); err == nil && meta != nil {
		return meta, nil
	}
	return nil, utils.Errorf("AI Skill %q was not found", name)
}

func (d *AttachedAIForgeResourceData) ToAttachData(ReActLoopIF) string {
	if d.lookupErr != nil {
		return fmt.Sprintf("User pinned an AI forge/skill named `%s`.", d.Name)
	}
	if d.forge == nil && !d.loadedSkill {
		return fmt.Sprintf("User pinned an AI forge/skill named `%s`.", d.Name)
	}
	if d.loadedSkill {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Attached AI Forge\n\n")
	b.WriteString(fmt.Sprintf("- Name: `%s`\n", d.Name))
	if forgeType := strings.TrimSpace(d.forge.ForgeType); forgeType != "" {
		b.WriteString(fmt.Sprintf("- Type: %s\n", forgeType))
	}
	if description := strings.TrimSpace(d.forge.Description); description != "" {
		b.WriteString(fmt.Sprintf("- Description: %s\n", description))
	}
	params := strings.TrimSpace(d.forge.Params)
	if params == "" {
		params = "{}"
	}
	b.WriteString("\n### Parameters\n```text\n")
	b.WriteString(params)
	b.WriteString("\n```\n")
	b.WriteString(fmt.Sprintf(
		"\nThe user explicitly selected this AI Forge. Prefer invoking it for the task and configure its parameters according to the definition above.\n"+
			"- JSON action: `{\"@action\":\"require_ai_blueprint\",\"blueprint_payload\":\"%s\"}`\n"+
			"- Function call: `require_ai_blueprint(blueprint_payload=\"%s\")`\n",
		d.forge.ForgeName,
		d.forge.ForgeName,
	))
	return strings.TrimSpace(b.String())
}
