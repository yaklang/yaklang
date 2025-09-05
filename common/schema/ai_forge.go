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

	InitPrompt       string
	PersistentPrompt string
	PlanPrompt       string
	ResultPrompt     string

	IsTemporary bool // for temporary use, will be cleaned up later
}

func (a *AIForge) GetName() string {
	return a.ForgeName
}

func (a *AIForge) GetDescription() string {
	return a.Description
}

func (a *AIForge) GetKeywords() []string {
	return strings.Split(a.Tags, ",")
}

var FORGE_TYPE_YAK = "yak"
var FORGE_TYPE_Config = "config"

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
		ForgeName:          a.ForgeName,
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
	}
	forgeIns.ID = uint(forge.Id)
	return forgeIns
}
