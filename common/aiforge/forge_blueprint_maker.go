package aiforge

import (
	"encoding/json"
	"errors"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/log"
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
	Name             string
	InitPrompt       string
	PersistentPrompt string
	PlanPrompt       string
	ResultPrompt     string

	// cli code
	CLIParameterRuleYaklangCode string

	// tools
	ToolKeywords []string
	Tools        []string

	// aid options
	YakForgeBlueprintAIDOptionsConfig *YakForgeBlueprintAIDOptionsConfig

	// result handle
	ForgeResult *ForgeResult
	ActionName  string
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
		ToolKeywords:     []string{},
		Tools:            []string{},
	}
}

func (c *YakForgeBlueprintConfig) WithActionName(name string) *YakForgeBlueprintConfig {
	c.ActionName = name
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

func (c *YakForgeBlueprintConfig) WithResultPrompt(resultPrompt string) *YakForgeBlueprintConfig {
	c.ResultPrompt = resultPrompt
	return c
}

func (c *YakForgeBlueprintConfig) WithCLIParameterRuleYaklangCode(cliParameterRuleYaklangCode string) *YakForgeBlueprintConfig {
	c.CLIParameterRuleYaklangCode = cliParameterRuleYaklangCode
	return c
}

func (c *YakForgeBlueprintConfig) WithToolKeywords(toolKeywords ...string) *YakForgeBlueprintConfig {
	c.ToolKeywords = toolKeywords
	return c
}

func (c *YakForgeBlueprintConfig) WithTools(tools ...string) *YakForgeBlueprintConfig {
	c.Tools = tools
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
		WithToolKeywords(config.ToolKeywords),
		WithInitializePrompt(config.InitPrompt),
		WithPersistentPrompt(config.PersistentPrompt),
		WithPlanMocker(planMocker),
		WithResultPrompt(config.ResultPrompt),
		WithAIDOptions(aidOpts...),
		WithResultHandler(func(s string, err error) {
			action, err := aid.ExtractAction(s, config.ActionName)
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
