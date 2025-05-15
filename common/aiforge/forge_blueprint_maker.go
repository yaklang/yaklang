package aiforge

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

type YakForgeBlueprintAIDOptionsConfig struct {
	DisableToolUse *bool `json:"disable_tool_use"`
	YOLO           *bool `json:"yolo"`
}

func NewYakForgeBlueprintAIDOptionsConfig() *YakForgeBlueprintAIDOptionsConfig {
	return &YakForgeBlueprintAIDOptionsConfig{
		DisableToolUse: nil,
		YOLO:           nil,
	}
}

func (c *YakForgeBlueprintAIDOptionsConfig) WithDisableToolUse(disableToolUse bool) *YakForgeBlueprintAIDOptionsConfig {
	c.DisableToolUse = &disableToolUse
	return c
}

func (c *YakForgeBlueprintAIDOptionsConfig) WithYOLO(yolo bool) *YakForgeBlueprintAIDOptionsConfig {
	c.YOLO = &yolo
	return c
}

func (c *YakForgeBlueprintAIDOptionsConfig) ToOptions() []aid.Option {
	res := []aid.Option{}
	if c.DisableToolUse != nil {
		res = append(res, aid.WithDisableToolUse(*c.DisableToolUse))
	}
	if c.YOLO != nil {
		res = append(res, aid.WithAgreeYOLO(*c.YOLO))
	}
	return res
}

type YakForgeBlueprintConfig struct {
	// prompt
	Name             string `json:"name"`
	InitPrompt       string `json:"init_prompt"`
	PersistentPrompt string `json:"persistent_prompt"`
	PlanPrompt       string `json:"plan_prompt"`
	ResultPrompt     string `json:"result_prompt"`

	// cli code
	CLIParameterRuleYaklangCode string `json:"cli_parameter_rule_yaklang_code"`

	// tools
	ToolKeywords string `json:"tool_keywords"`
	Tools        string `json:"tools"`
	Description  string `json:"description"`

	// aid options
	YakForgeBlueprintAIDOptionsConfig *YakForgeBlueprintAIDOptionsConfig `json:"aid_options_config"`

	// result handle
	ForgeResult *ForgeResult `json:"forge_result"`
	Actions     string       `json:"actions"`
}

// NewYakForgeBlueprintConfigFromJson 从Json数据创建Forge
func NewYakForgeBlueprintConfigFromJson(data any) (*ForgeBlueprint, error) {
	jsonData := utils.InterfaceToBytes(data)
	var config YakForgeBlueprintConfig
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return nil, err
	}
	return config.Build()
}

// NewYakForgeBlueprintConfig 创建Forge Builder
func NewYakForgeBlueprintConfig(name string, initPrompt string, persistentPrompt string) *YakForgeBlueprintConfig {
	return &YakForgeBlueprintConfig{
		Name:             name,
		InitPrompt:       initPrompt,
		PersistentPrompt: persistentPrompt,
	}
}
func NewYakForgeBlueprintConfigFromSchemaForge(forge *schema.AIForge) *YakForgeBlueprintConfig {
	return NewYakForgeBlueprintConfig(forge.ForgeName, forge.InitPrompt, forge.PersistentPrompt).
		WithSchemaForge(forge)
}
func (c *YakForgeBlueprintConfig) WithSchemaForge(forge *schema.AIForge) *YakForgeBlueprintConfig {
	c.Name = forge.ForgeName
	c.InitPrompt = forge.InitPrompt
	c.PersistentPrompt = forge.PersistentPrompt
	c.PlanPrompt = forge.PlanPrompt
	c.ResultPrompt = forge.ResultPrompt
	c.CLIParameterRuleYaklangCode = forge.ForgeContent
	c.ToolKeywords = forge.ToolKeywords
	c.Tools = forge.Tools
	c.Description = forge.Description
	c.Actions = forge.Actions
	return c
}
func (c *YakForgeBlueprintConfig) WithActionName(name string) *YakForgeBlueprintConfig {
	c.Actions = name
	return c
}

func (c *YakForgeBlueprintConfig) WithAIDOptions(opts *YakForgeBlueprintAIDOptionsConfig) *YakForgeBlueprintConfig {
	c.YakForgeBlueprintAIDOptionsConfig = opts
	return c
}
func (c *YakForgeBlueprintConfig) WithPlanPrompt(planPrompt string) *YakForgeBlueprintConfig {
	c.PlanPrompt = planPrompt
	return c
}

func (c *YakForgeBlueprintConfig) WithInitPrompt(initPrompt string) *YakForgeBlueprintConfig {
	c.InitPrompt = initPrompt
	return c
}

func (c *YakForgeBlueprintConfig) WithPersistentPrompt(persistentPrompt string) *YakForgeBlueprintConfig {
	c.PersistentPrompt = persistentPrompt
	return c
}

func (c *YakForgeBlueprintConfig) WithResultPrompt(resultPrompt string) *YakForgeBlueprintConfig {
	c.ResultPrompt = resultPrompt
	return c
}

func (c *YakForgeBlueprintConfig) WithCLIParameterRuleYaklangCode(cliParameterRuleYaklangCode string) *YakForgeBlueprintConfig {
	c.CLIParameterRuleYaklangCode = cliParameterRuleYaklangCode
	return c
}

func (c *YakForgeBlueprintConfig) WithToolKeywords(toolKeywords ...string) *YakForgeBlueprintConfig {
	c.ToolKeywords = strings.Join(toolKeywords, ",")
	return c
}

func (c *YakForgeBlueprintConfig) WithTools(tools ...string) *YakForgeBlueprintConfig {
	c.Tools = strings.Join(tools, ",")
	return c
}

func (c *YakForgeBlueprintConfig) Validate() error {
	if c.Name == "" {
		return errors.New("name is required")
	}

	return nil
}

func (c *YakForgeBlueprintConfig) Build() (*ForgeBlueprint, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	config := c

	var aidOpts []aid.Option
	if c.YakForgeBlueprintAIDOptionsConfig != nil {
		aidOpts = c.YakForgeBlueprintAIDOptionsConfig.ToOptions()
	}

	var planMocker func(cfg *aid.Config) *aid.PlanResponse
	if c.PlanPrompt != "" {
		planMocker = func(cfg *aid.Config) *aid.PlanResponse {
			plan, err := aid.ExtractPlan(cfg, config.PlanPrompt)
			if err != nil {
				cfg.EmitError("mock SMART Plan failed: %v", err)
				return nil
			}
			return plan
		}
	}

	name := config.Name
	blueprint := NewForgeBlueprint(name,
		WithOriginYaklangCliCode(config.CLIParameterRuleYaklangCode),
		WithToolKeywords(strings.Split(config.ToolKeywords, ",")),
		WithInitializePrompt(config.InitPrompt),
		WithPersistentPrompt(config.PersistentPrompt),
		WithPlanMocker(planMocker),
		WithResultPrompt(config.ResultPrompt),
		WithAIDOptions(aidOpts...),
		WithResultHandler(func(s string, err error) {
			actions := strings.Split(config.Actions, ",")
			var actionName string
			var alias []string
			if len(actions) > 0 {
				actionName = actions[0]
				alias = actions[1:]
			}
			action, err := aid.ExtractAction(s, actionName, alias...)
			if err != nil {
				log.Errorf("Failed to extract action from smart: %s", err)
				return
			}
			config.ForgeResult.Action = action
		}),
	)
	forgeResult := &ForgeResult{
		Forge: blueprint,
	}
	config.ForgeResult = forgeResult
	return blueprint, nil
}
