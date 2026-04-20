package schema

import (
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type AIForge struct {
	gorm.Model

	ForgeVerboseName   string
	ForgeName          string `gorm:"unique_index"`
	ForgeContent       string
	ForgeType          string // "yak" or "json"
	ParamsUIConfig     string
	Params             string // cli params
	UserPersistentData string // for user preferences
	Description        string // forge description
	Tools              string // tools
	ToolKeywords       string // tool keywords
	Actions            string
	Tags               string

	Author string

	InitPrompt       string
	PersistentPrompt string
	PlanPrompt       string
	ResultPrompt     string
	SkillPath        string
	FSBytes          []byte `gorm:"type:blob"`

	IsTemporary bool // for temporary use, will be cleaned up later
}

func (a *AIForge) ToUpdateMap() map[string]interface{} {
	if a == nil {
		return nil
	}

	return map[string]interface{}{
		"forge_verbose_name":   a.ForgeVerboseName,
		"forge_name":           a.ForgeName,
		"forge_content":        a.ForgeContent,
		"forge_type":           a.ForgeType,
		"params_ui_config":     a.ParamsUIConfig,
		"params":               a.Params,
		"user_persistent_data": a.UserPersistentData,
		"description":          a.Description,
		"tools":                a.Tools,
		"tool_keywords":        a.ToolKeywords,
		"actions":              a.Actions,
		"tags":                 a.Tags,
		"init_prompt":          a.InitPrompt,
		"persistent_prompt":    a.PersistentPrompt,
		"plan_prompt":          a.PlanPrompt,
		"result_prompt":        a.ResultPrompt,
		"skill_path":           a.SkillPath,
		"fs_bytes":             a.FSBytes,
		"is_temporary":         a.IsTemporary,
	}
}

func (a *AIForge) GetName() string {
	return a.ForgeName
}

func (a *AIForge) GetDescription() string {
	return a.Description
}

func (a *AIForge) GetVerboseName() string {
	return a.ForgeVerboseName
}

func (a *AIForge) GetKeywords() []string {
	return strings.Split(a.Tags, ",")
}

var FORGE_TYPE_YAK = "yak"
var FORGE_TYPE_Config = "config"
var FORGE_TYPE_SkillMD = "skillmd"

func IsRunnableForgeType(forgeType string) bool {
	return forgeType == FORGE_TYPE_YAK || forgeType == FORGE_TYPE_Config || forgeType == ""
}

func RunnableForgeTypes() []string {
	return []string{FORGE_TYPE_YAK, FORGE_TYPE_Config}
}

func (a *AIForge) AfterCreate(tx *gorm.DB) (err error) {
	broadcastData.Call("aiforge", "create")
	return nil
}

func (a *AIForge) AfterUpdate(tx *gorm.DB) (err error) {
	broadcastData.Call("aiforge", "update")
	return nil
}

func (a *AIForge) AfterDelete(tx *gorm.DB) (err error) {
	broadcastData.Call("aiforge", "delete")
	return nil
}

// todo  schema2grpc model
func (a *AIForge) ToGRPC() *ypb.AIForge {
	return &ypb.AIForge{
		Id:                 int64(a.ID),
		CreatedAt:          a.CreatedAt.Unix(),
		ForgeName:          a.ForgeName,
		ForgeVerboseName:   a.ForgeVerboseName,
		ForgeContent:       a.ForgeContent,
		ForgeType:          a.ForgeType,
		ParamsUIConfig:     a.ParamsUIConfig,
		Params:             a.Params,
		UserPersistentData: a.UserPersistentData,
		Description:        a.Description,
		ToolNames:          utils.StringSplitAndStrip(a.Tools, ","),
		ToolKeywords:       utils.StringSplitAndStrip(a.ToolKeywords, ","),
		Action:             a.Actions,
		Tag:                utils.StringSplitAndStrip(a.Tags, ","),
		InitPrompt:         a.InitPrompt,
		PersistentPrompt:   a.PersistentPrompt,
		PlanPrompt:         a.PlanPrompt,
		ResultPrompt:       a.ResultPrompt,
		UpdatedAt:          a.UpdatedAt.Unix(),
		Author:             a.Author,
		SkillPath:          a.SkillPath,
	}
}

func GRPC2AIForge(forge *ypb.AIForge) *AIForge {
	forgeIns := &AIForge{
		ForgeName:          forge.GetForgeName(),
		ForgeContent:       forge.GetForgeContent(),
		ForgeType:          forge.GetForgeType(),
		ParamsUIConfig:     forge.GetParamsUIConfig(),
		Params:             forge.GetParams(),
		UserPersistentData: forge.GetUserPersistentData(),
		Description:        forge.GetDescription(),
		Tools:              strings.Join(forge.GetToolNames(), ","),
		ToolKeywords:       strings.Join(forge.GetToolKeywords(), ","),
		Actions:            forge.GetAction(),
		Tags:               strings.Join(forge.GetTag(), ","),
		InitPrompt:         forge.GetInitPrompt(),
		PersistentPrompt:   forge.GetPersistentPrompt(),
		PlanPrompt:         forge.GetPlanPrompt(),
		ResultPrompt:       forge.GetResultPrompt(),
		Author:             forge.GetAuthor(),
		SkillPath:          forge.GetSkillPath(),
	}
	forgeIns.ID = uint(forge.Id)
	return forgeIns
}
